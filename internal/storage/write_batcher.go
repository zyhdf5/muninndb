package storage

import (
	"time"

	"github.com/cockroachdb/pebble"
)

const (
	// batchMaxJobs is the maximum number of WriteEngram calls coalesced into
	// a single Pebble batch. At 100k writes/sec with 8 writers, each batch
	// holds ~1-8 jobs on average; 64 is an upper cap that keeps batch size
	// and latency bounded.
	batchMaxJobs = 64

	// batchMaxLatency is the maximum time the batcher waits before flushing
	// a partially-full batch. 100µs adds at most one 100-microsecond step to
	// P50 latency — invisible at the millisecond scale reported by the benchmark.
	batchMaxLatency = 100 * time.Microsecond

	// batchJobBufSize is the capacity of the jobs channel. At 100k writes/sec
	// with batchMaxLatency=100µs, at most 100k*0.0001=10 jobs are in-flight
	// per flush interval. 4096 provides ample headroom for bursts.
	batchJobBufSize = 4096
)

// batchKV is a pre-built key/value pair for inclusion in a Pebble batch.
// Building keys and encoding values happens in the caller goroutine so the
// batcher goroutine does only Pebble I/O — no allocations or encoding on the
// critical commit path.
type batchKV struct {
	key []byte
	val []byte
}

// writeBatchJob holds everything needed to commit one WriteEngram to Pebble.
// Callers pre-build all key/value pairs and submit the job; they block on
// result until the batch commits. Post-commit side-effects (vault counter,
// WAL, provenance) are run by the caller goroutine after receiving success.
type writeBatchJob struct {
	entries []batchKV
	result  chan error // buffered(1): batcher sends exactly one error (nil on success)
}

// writeBatcher coalesces up to batchMaxJobs WriteEngram calls into a single
// Pebble batch commit. This amortises the per-write WAL + memtable overhead
// across N callers, yielding a proportional throughput improvement.
//
// All encoding runs in caller goroutines. The batcher goroutine only calls
// pebble.Batch.Set and pebble.Batch.Commit — no allocations, no encoding.
//
// On commit failure, all N callers in the batch receive the same error.
// This is correct: pebble.NoSync batch failures are extremely rare (disk-full
// or corruption) and affect every write in the batch equally.
type writeBatcher struct {
	db   *pebble.DB
	jobs chan writeBatchJob
	stop chan struct{}
	done chan struct{}
}

func newWriteBatcher(db *pebble.DB) *writeBatcher {
	b := &writeBatcher{
		db:   db,
		jobs: make(chan writeBatchJob, batchJobBufSize),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go b.run()
	return b
}

func (b *writeBatcher) run() {
	defer close(b.done)

	buf := make([]writeBatchJob, 0, batchMaxJobs)

	// safetyTimer fires if a partial batch somehow stays in buf without being
	// flushed. In practice the greedy-drain design flushes on every job arrival,
	// so this should never fire — it is a safety net only.
	safetyTimer := time.NewTimer(batchMaxLatency)
	defer safetyTimer.Stop()

	resetSafetyTimer := func() {
		if !safetyTimer.Stop() {
			select {
			case <-safetyTimer.C:
			default:
			}
		}
		safetyTimer.Reset(batchMaxLatency)
	}

	flush := func() {
		if len(buf) == 0 {
			return
		}
		batch := b.db.NewBatch()
		for i := range buf {
			for _, kv := range buf[i].entries {
				batch.Set(kv.key, kv.val, nil)
			}
		}
		err := batch.Commit(pebble.NoSync)
		batch.Close()
		// Notify all callers — they run their own post-commit side-effects.
		for i := range buf {
			buf[i].result <- err
		}
		buf = buf[:0]
	}

	for {
		select {
		case job, ok := <-b.jobs:
			if !ok {
				// Channel closed (test teardown) — flush and exit.
				flush()
				return
			}
			buf = append(buf, job)

			// Greedy drain: consume all immediately-available jobs without
			// blocking. This coalesces burst writes (N concurrent writers all
			// queued at once) while also allowing single-writer workloads to
			// flush immediately without waiting for the timer.
		drainLoop:
			for len(buf) < batchMaxJobs {
				select {
				case j, ok := <-b.jobs:
					if !ok {
						flush()
						return
					}
					buf = append(buf, j)
				default:
					break drainLoop
				}
			}

			// Flush whatever we collected — one write or a full burst batch.
			flush()
			resetSafetyTimer()

		case <-safetyTimer.C:
			// Flush any stuck partial batch (should be rare).
			flush()
			resetSafetyTimer()

		case <-b.stop:
			// Drain remaining jobs before exiting.
			for {
				select {
				case job := <-b.jobs:
					buf = append(buf, job)
				default:
					flush()
					return
				}
			}
		}
	}
}

// Close stops the batcher after draining all pending jobs.
// Must be called before db.Close().
func (b *writeBatcher) Close() {
	close(b.stop)
	<-b.done
}
