package main

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/scrypster/muninndb/internal/bench"
	"github.com/scrypster/muninndb/internal/engine"
	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// benchmarkMemory measures memory pressure under sustained write load.
func benchmarkMemory(ctx context.Context, eng *engine.Engine, vaultName string, soakDuration time.Duration) (*bench.MemoryResult, error) {
	// Force GC to establish baseline
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	var baselineStats runtime.MemStats
	runtime.ReadMemStats(&baselineStats)
	baselineHeap := float64(baselineStats.Alloc) / 1024.0 / 1024.0

	// Track peak and sample GC
	var (
		peakHeap float64
		gcCount  = baselineStats.NumGC
		ticker   = time.NewTicker(5 * time.Second)
		stopChan = make(chan struct{})
		writeIdx int64
	)

	defer ticker.Stop()

	// Sample memory every 5 seconds
	go func() {
		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				currentHeap := float64(m.Alloc) / 1024.0 / 1024.0
				if currentHeap > peakHeap {
					peakHeap = currentHeap
				}
				gcCount = m.NumGC
			}
		}
	}()

	// Write engrams for the duration
	startTime := time.Now()
	for {
		select {
		case <-ctx.Done():
			close(stopChan)
			return nil, ctx.Err()
		default:
		}

		if time.Since(startTime) >= soakDuration {
			close(stopChan)
			break
		}

		idx := atomic.AddInt64(&writeIdx, 1)
		req := &mbp.WriteRequest{
			Concept:    fmt.Sprintf("memory_test_concept_%d", idx),
			Content:    fmt.Sprintf("Memory soak test engram %d with substantial content to measure heap growth and GC behavior under sustained write load", idx),
			Tags:       []string{"memory", "soak"},
			Confidence: 0.85,
			Stability:  0.8,
			Vault:      vaultName,
		}

		_, err := eng.Write(ctx, req)
		if err != nil {
			// Continue even if write fails
		}

		// Throttle writes to avoid overwhelming the system
		time.Sleep(1 * time.Millisecond)
	}

	elapsed := time.Since(startTime)

	// Final GC and heap sample
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var finalStats runtime.MemStats
	runtime.ReadMemStats(&finalStats)

	return &bench.MemoryResult{
		BaselineHeapMB: baselineHeap,
		PeakHeapMB:     peakHeap,
		GrowthMB:       peakHeap - baselineHeap,
		GCCount:        finalStats.NumGC - gcCount,
		Duration:       elapsed,
	}, nil
}
