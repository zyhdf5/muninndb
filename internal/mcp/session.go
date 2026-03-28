package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	sessionTTL       = 24 * time.Hour
	defaultPushBufSz = 64 // absorbs consolidation bursts; drops acceptable beyond this
)

// ErrSessionCapReached is returned by Create when the session cap is hit.
var ErrSessionCapReached = errors.New("mcp: session cap reached, too many active sessions")

// mcpSession holds per-session state. Fields accessed concurrently use atomics.
// pushCh is closed ONLY by the session reaper — never by the SSE goroutine.
type mcpSession struct {
	vault         string
	tokenHash     [32]byte
	createdAt     time.Time
	lastUsed      time.Time // protected by store.mu
	initialized   atomic.Bool
	streaming     atomic.Bool
	droppedEvents atomic.Int64
	pushCh        chan MCPEvent
}

// sessionStore manages active MCP sessions.
// Unexported because it returns *mcpSession (unexported type) and is only
// used within the internal/mcp package.
type sessionStore interface {
	Create(vault string, tokenHash [32]byte) (sessionID string, err error)
	Get(sessionID string) (*mcpSession, bool)
	Touch(sessionID string)
	MarkInitialized(sessionID string) error
	ByVault(vault string) []*mcpSession // returns snapshot — safe to iterate after lock released
	DroppedCount(sessionID string) int64
	Close()
}

type concreteSessionStore struct {
	mu        sync.RWMutex
	sessions  map[string]*mcpSession
	cap       int
	now       func() time.Time
	pushBuf   int
	done      chan struct{}
	closeOnce sync.Once
}

func newSessionStore(cap int, now func() time.Time) sessionStore {
	s := &concreteSessionStore{
		sessions: make(map[string]*mcpSession),
		cap:      cap,
		now:      now,
		pushBuf:  defaultPushBufSz,
		done:     make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

func (s *concreteSessionStore) Create(vault string, tokenHash [32]byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) >= s.cap {
		return "", fmt.Errorf("%w (cap=%d)", ErrSessionCapReached, s.cap)
	}
	id, err := newSessionID()
	if err != nil {
		return "", err
	}
	sess := &mcpSession{
		vault:     vault,
		tokenHash: tokenHash,
		createdAt: s.now(),
		lastUsed:  s.now(),
		pushCh:    make(chan MCPEvent, s.pushBuf),
	}
	s.sessions[id] = sess
	return id, nil
}

func (s *concreteSessionStore) Get(sessionID string) (*mcpSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	return sess, ok
}

func (s *concreteSessionStore) Touch(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok {
		sess.lastUsed = s.now()
	}
}

func (s *concreteSessionStore) MarkInitialized(sessionID string) error {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	sess.initialized.Store(true)
	return nil
}

func (s *concreteSessionStore) ByVault(vault string) []*mcpSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*mcpSession
	for _, sess := range s.sessions {
		if sess.vault == vault {
			result = append(result, sess)
		}
	}
	return result // snapshot slice; elements are pointers, atomic fields are safe
}

func (s *concreteSessionStore) DroppedCount(sessionID string) int64 {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return 0
	}
	return sess.droppedEvents.Load()
}

// Close stops the cleanup goroutine and sweeps all sessions.
// Safe to call multiple times — subsequent calls are no-ops.
// IMPORTANT: shutdown ordering in MCPServer.Shutdown() must call Close()
// after EventBus.Close() — this sweeps and closes all pushCh channels,
// terminating any blocked SSE goroutines.
func (s *concreteSessionStore) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.sweepAll() // final sweep: close all pushCh channels regardless of TTL
	})
}

func (s *concreteSessionStore) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sweep()
		case <-s.done:
			return
		}
	}
}

// sweep removes expired sessions and closes their pushCh channels.
// This is the ONLY place that closes pushCh (for TTL-expired sessions).
func (s *concreteSessionStore) sweep() {
	s.mu.Lock()
	now := s.now()
	var expired []*mcpSession
	for id, sess := range s.sessions {
		if now.Sub(sess.lastUsed) > sessionTTL {
			expired = append(expired, sess)
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()

	// Close channels outside the lock — SSE goroutines will unblock via closed channel
	for _, sess := range expired {
		sess.streaming.Store(false)
		close(sess.pushCh)
	}
}

// sweepAll removes all remaining sessions on Close and closes their pushCh channels.
func (s *concreteSessionStore) sweepAll() {
	s.mu.Lock()
	var all []*mcpSession
	for id, sess := range s.sessions {
		all = append(all, sess)
		delete(s.sessions, id)
	}
	s.mu.Unlock()

	for _, sess := range all {
		sess.streaming.Store(false)
		close(sess.pushCh)
	}
}

// newSessionID generates a 256-bit cryptographically random session ID.
func newSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Compile-time assertion that concreteSessionStore implements sessionStore.
var _ sessionStore = (*concreteSessionStore)(nil)
