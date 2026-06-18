package replication

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrTokenExpired = errors.New("join token: expired")
var ErrTokenInvalid = errors.New("join token: invalid")

type JoinTokenManager struct {
	secret []byte
	ttl    time.Duration
}

func NewJoinTokenManager(clusterSecret string, ttl time.Duration) *JoinTokenManager {
	return &JoinTokenManager{secret: []byte(clusterSecret), ttl: ttl}
}

func (m *JoinTokenManager) Generate() (string, error) {
	if len(m.secret) == 0 {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("jointoken: rand: %w", err)
		}
		return hex.EncodeToString(b), nil
	}
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("jointoken: rand nonce: %w", err)
	}
	nonceHex := hex.EncodeToString(nonce)
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(now + "." + nonceHex))
	sig := hex.EncodeToString(mac.Sum(nil))
	return now + "." + nonceHex + "." + sig, nil
}

func (m *JoinTokenManager) Validate(tok string) error {
	if len(m.secret) == 0 {
		if tok == "" {
			return ErrTokenInvalid
		}
		return nil
	}
	parts := strings.SplitN(tok, ".", 3)
	if len(parts) != 3 {
		return ErrTokenInvalid
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ErrTokenInvalid
	}
	issued := time.Unix(0, nanos)
	now := time.Now()
	if issued.After(now.Add(5 * time.Second)) {
		return ErrTokenInvalid
	}
	if now.Sub(issued) > m.ttl {
		return ErrTokenExpired
	}
	if len(parts[1]) != 16 {
		return ErrTokenInvalid
	}
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return ErrTokenInvalid
	}
	return nil
}

func (m *JoinTokenManager) TTL() time.Duration { return m.ttl }
