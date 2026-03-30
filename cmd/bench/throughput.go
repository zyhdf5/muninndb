package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scrypster/muninndb/internal/bench"
	"github.com/scrypster/muninndb/internal/engine"
	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// benchmarkThroughput measures write and activation throughput under concurrent load.
// Writers and activators run in dedicated goroutine pools so neither blocks the other.
func benchmarkThroughput(ctx context.Context, eng *engine.Engine, vaultName string, concurrency int, duration time.Duration) (*bench.ThroughputResult, error) {
	// Seed vault with initial data for activation queries.
	// A larger corpus makes FTS IDF scores meaningful and stresses the
	// in-memory posting lists realistically.
	const seedCount = 500
	concepts := []string{
		"machine learning", "neural network", "cognitive system", "memory storage",
		"knowledge graph", "semantic search", "vector embedding", "natural language",
		"reinforcement learning", "transformer model", "attention mechanism", "deep learning",
		"data pipeline", "stream processing", "distributed system", "consensus protocol",
		"memory consolidation", "episodic recall", "associative memory", "pattern recognition",
	}
	contents := []string{
		"explores the relationship between data structures and retrieval efficiency",
		"implements a novel approach to semantic similarity using sparse vectors",
		"demonstrates how cognitive architectures can scale to large knowledge bases",
		"analyzes trade-offs between memory capacity and access latency",
		"provides a framework for dynamic knowledge graph construction",
	}
	for i := 0; i < seedCount; i++ {
		req := &mbp.WriteRequest{
			Concept:    fmt.Sprintf("%s %d", concepts[i%len(concepts)], i),
			Content:    fmt.Sprintf("Seed entry %d: %s with detail about indexing and retrieval performance.", i, contents[i%len(contents)]),
			Tags:       []string{"benchmark", "seed", concepts[i%len(concepts)]},
			Confidence: 0.9,
			Stability:  1.0,
			Vault:      vaultName,
		}
		if _, err := eng.Write(ctx, req); err != nil {
			return nil, fmt.Errorf("seed write: %w", err)
		}
	}

	var (
		writeID     int64 // monotonic counter for unique concept names — never reported
		writeCount  int64 // successful writes — reported as throughput
		activations int64
		wg          sync.WaitGroup
	)

	tctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	startTime := time.Now()

	// Allocate 30% of concurrency to writers (minimum 3).
	// With NoSync, Pebble's memtable mutex is held for <1µs per batch commit.
	// Each writer spends most time in ERF encoding (CPU, unlocked), so 3-4
	// concurrent writers overlap CPU work with memtable insertion and achieve
	// near-linear write scaling without the coordination overhead of a batcher.
	// 15% (1 writer) benchmarked single-goroutine Pebble throughput — this
	// correctly measures multi-writer throughput.
	writerCount := concurrency * 30 / 100
	if writerCount < 3 {
		writerCount = 3
	}
	activatorCount := concurrency - writerCount
	if activatorCount < 1 {
		activatorCount = 1
	}

	// Writer goroutines: write as fast as possible, no activations.
	for i := 0; i < writerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-tctx.Done():
					return
				default:
				}
				id := atomic.AddInt64(&writeID, 1)
				req := &mbp.WriteRequest{
					Concept:    fmt.Sprintf("worker_%d_concept_%d", workerID, id),
					Content:    fmt.Sprintf("Throughput test engram from worker %d iteration %d", workerID, id),
					Tags:       []string{"throughput"},
					Confidence: 0.85,
					Stability:  0.8,
					Vault:      vaultName,
				}
				if _, err := eng.Write(tctx, req); err == nil {
					atomic.AddInt64(&writeCount, 1)
				}
			}
		}(i)
	}

	// Activator goroutines: activate as fast as possible, no writes.
	activationQueries := []string{
		"machine learning neural network",
		"cognitive memory retrieval",
		"semantic search vector embedding",
		"deep learning transformer",
		"knowledge graph associative",
		"pattern recognition episodic",
		"distributed system consensus",
		"stream processing pipeline",
		"memory consolidation recall",
		"natural language understanding",
	}
	for i := 0; i < activatorCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			queryIdx := workerID
			for {
				select {
				case <-tctx.Done():
					return
				default:
				}
				req := &mbp.ActivateRequest{
					Context:    []string{activationQueries[queryIdx%len(activationQueries)]},
					MaxResults: 10,
					Vault:      vaultName,
					BriefMode:  "off", // skip extractive brief generation in throughput benchmark
				}
				if _, err := eng.Activate(tctx, req); err == nil {
					atomic.AddInt64(&activations, 1)
				}
				queryIdx++
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	return &bench.ThroughputResult{
		WritesPerSec:      float64(writeCount) / elapsed.Seconds(),
		ActivationsPerSec: float64(activations) / elapsed.Seconds(),
		TotalWrites:       writeCount,
		TotalActivations:  activations,
		Concurrency:       concurrency,
		Duration:          elapsed,
	}, nil
}

// benchmarkWriteOnly measures peak write throughput with all goroutines writing.
// No activators — eliminates write-read Pebble contention so writes hit their true ceiling.
func benchmarkWriteOnly(ctx context.Context, eng *engine.Engine, vaultName string, concurrency int, duration time.Duration) (*bench.ThroughputResult, error) {
	var writeID, writeCount int64
	var wg sync.WaitGroup

	tctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-tctx.Done():
					return
				default:
				}
				id := atomic.AddInt64(&writeID, 1)
				req := &mbp.WriteRequest{
					Concept:    fmt.Sprintf("write_only_%d_%d", workerID, id),
					Content:    fmt.Sprintf("Write-only benchmark engram from worker %d iteration %d — testing peak write throughput", workerID, id),
					Tags:       []string{"write-only"},
					Confidence: 0.9,
					Stability:  1.0,
					Vault:      vaultName,
				}
				if _, err := eng.Write(tctx, req); err == nil {
					atomic.AddInt64(&writeCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)
	return &bench.ThroughputResult{
		WritesPerSec: float64(writeCount) / elapsed.Seconds(),
		Concurrency:  concurrency,
		Duration:     elapsed,
	}, nil
}

// benchmarkActivateOnly seeds a corpus then measures peak activation throughput.
// No concurrent writers — shows activation throughput without write-read contention.
func benchmarkActivateOnly(ctx context.Context, eng *engine.Engine, vaultName string, concurrency int, duration time.Duration) (*bench.ThroughputResult, error) {
	// Seed with a larger corpus so FTS scoring is meaningful.
	const seedCount = 2000
	concepts := []string{
		"machine learning", "neural network", "cognitive system", "memory storage",
		"knowledge graph", "semantic search", "vector embedding", "natural language",
		"reinforcement learning", "transformer model", "attention mechanism", "deep learning",
		"data pipeline", "stream processing", "distributed system", "consensus protocol",
		"memory consolidation", "episodic recall", "associative memory", "pattern recognition",
	}
	contents := []string{
		"explores the relationship between data structures and retrieval efficiency in high-performance systems",
		"implements a novel approach to semantic similarity using sparse and dense vector representations",
		"demonstrates how cognitive architectures can scale to support large knowledge bases efficiently",
		"analyzes the trade-offs between memory capacity, access latency, and throughput characteristics",
		"provides a framework for dynamic knowledge graph construction and real-time association discovery",
	}
	for i := 0; i < seedCount; i++ {
		if _, err := eng.Write(ctx, &mbp.WriteRequest{
			Concept: fmt.Sprintf("%s %d", concepts[i%len(concepts)], i),
			Content: fmt.Sprintf("Corpus entry %d: %s with detail about indexing and retrieval.", i, contents[i%len(contents)]),
			Tags:    []string{"corpus", concepts[i%len(concepts)]},
			Vault:   vaultName,
		}); err != nil {
			return nil, fmt.Errorf("seed: %w", err)
		}
	}

	queries := []string{
		"machine learning neural network", "cognitive memory retrieval",
		"semantic search vector embedding", "deep learning transformer",
		"knowledge graph associative", "pattern recognition episodic",
		"distributed system consensus", "stream processing pipeline",
		"memory consolidation recall", "natural language understanding",
	}

	var activations int64
	var wg sync.WaitGroup

	tctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			qi := workerID
			for {
				select {
				case <-tctx.Done():
					return
				default:
				}
				req := &mbp.ActivateRequest{
					Context:    []string{queries[qi%len(queries)]},
					MaxResults: 10,
					Vault:      vaultName,
					BriefMode:  "off",
				}
				if _, err := eng.Activate(tctx, req); err == nil {
					atomic.AddInt64(&activations, 1)
				}
				qi++
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)
	return &bench.ThroughputResult{
		ActivationsPerSec: float64(activations) / elapsed.Seconds(),
		Concurrency:       concurrency,
		Duration:          elapsed,
	}, nil
}
