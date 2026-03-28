package storage

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/scrypster/muninndb/internal/provenance"
)

const provenanceChanDepth = 32768 // was 8192 — absorbs burst at 63k writes/sec

type provenanceJob struct {
	wsPrefix [8]byte
	id       ULID
	entry    provenance.ProvenanceEntry
}

// provenanceWorker fans out provenance appends across NumCPU goroutines.
// Each Append in provenance.Store is now a single O(1) Pebble Set (no
// read-modify-write), so concurrent goroutines are safe to run in parallel
// without per-ID coordination. The global atomic sequence in the key format
// ensures uniqueness even under sub-nanosecond timestamp collisions.
type provenanceWorker struct {
	store    *provenance.Store
	pending  chan provenanceJob
	dropped  atomic.Int64
	wg       sync.WaitGroup
	inFlight sync.WaitGroup // tracks jobs currently being processed
	done     chan struct{}
}

func newProvenanceWorker(store *provenance.Store) *provenanceWorker {
	n := runtime.NumCPU()
	w := &provenanceWorker{
		store:   store,
		pending: make(chan provenanceJob, provenanceChanDepth),
		done:    make(chan struct{}),
	}
	w.wg.Add(n)
	for range n {
		go func() {
			defer w.wg.Done()
			w.run()
		}()
	}
	return w
}

// Submit enqueues a provenance append. Non-blocking — drops and warns if full.
// Recovers from "send on closed channel" during graceful shutdown (e.g., when
// the decay worker processes a final batch after Close() has been called).
// inFlight.Add(1) is called before the channel send so that Drain() can never
// observe a zero count between enqueue and processing (prevents Add-after-Wait
// data race detected by the race detector).
func (w *provenanceWorker) Submit(wsPrefix [8]byte, id ULID, entry provenance.ProvenanceEntry) {
	w.inFlight.Add(1)
	sent := false
	func() {
		defer func() { recover() }() // safe against send-on-closed-channel during shutdown
		select {
		case w.pending <- provenanceJob{wsPrefix: wsPrefix, id: id, entry: entry}:
			sent = true
		default:
		}
	}()
	if !sent {
		w.inFlight.Done() // not enqueued — release immediately
		n := w.dropped.Add(1)
		if n&(n-1) == 0 { // log at powers of 2 to rate-limit slog I/O
			slog.Warn("storage: provenance worker full, entries dropped", "total_dropped", n)
		}
	}
}

// safeAppend calls store.Append but recovers from panics (e.g., "pebble: closed"
// during test teardown when db.Close() races with the worker).
func (w *provenanceWorker) safeAppend(ctx context.Context, job provenanceJob) {
	defer func() { recover() }()
	if err := w.store.Append(ctx, job.wsPrefix, [16]byte(job.id), job.entry); err != nil {
		slog.Warn("storage: provenance append failed", "engram_id", job.id, "err", err)
	}
}

// run drains the shared channel until it is closed (on Stop).
// inFlight.Add(1) is called in Submit before the job enters the channel,
// so we only need Done() here after processing completes.
func (w *provenanceWorker) run() {
	ctx := context.Background()
	for job := range w.pending {
		w.safeAppend(ctx, job)
		w.inFlight.Done()
	}
}

// Drain waits for all in-flight provenance appends to complete.
// Used by ClearVault to ensure no 0x16 keys land after range tombstones.
func (w *provenanceWorker) Drain() {
	w.inFlight.Wait()
}

// Close drains the queue and waits for all worker goroutines to exit.
// Must be called before db.Close().
func (w *provenanceWorker) Close() {
	close(w.pending)
	w.wg.Wait()
	close(w.done)
}
