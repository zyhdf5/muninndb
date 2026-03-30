package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/scrypster/muninndb/internal/bench"
	"github.com/scrypster/muninndb/internal/engine"
	"github.com/scrypster/muninndb/internal/transport/mbp"
)

// benchmarkLatency measures activation query latency percentiles.
func benchmarkLatency(ctx context.Context, eng *engine.Engine, vaultName string) (*bench.LatencyResult, error) {
	// Seed vault with diverse content
	const seedCount = 100
	for i := 0; i < seedCount; i++ {
		req := &mbp.WriteRequest{
			Concept:    fmt.Sprintf("latency_concept_%d", i),
			Content:    fmt.Sprintf("Content for latency test concept %d with detailed information", i),
			Tags:       []string{"latency", "test"},
			Confidence: 0.9,
			Stability:  1.0,
			Vault:      vaultName,
		}
		_, err := eng.Write(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("seed write: %w", err)
		}
	}

	// Run 1000 sequential activation queries
	const queryCount = 1000
	durations := make([]time.Duration, 0, queryCount)

	for i := 0; i < queryCount; i++ {
		// Vary the query context
		conceptIdx := i % seedCount
		req := &mbp.ActivateRequest{
			Context:    []string{fmt.Sprintf("latency_concept_%d", conceptIdx)},
			MaxResults: 20,
			Vault:      vaultName,
		}

		start := time.Now()
		_, err := eng.Activate(ctx, req)
		elapsed := time.Since(start)

		if err != nil {
			return nil, fmt.Errorf("activate query %d: %w", i, err)
		}

		durations = append(durations, elapsed)
	}

	// Sort for percentile calculation
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Compute percentiles
	result := &bench.LatencyResult{
		P50:   computePercentile(durations, 50),
		P95:   computePercentile(durations, 95),
		P99:   computePercentile(durations, 99),
		P999:  computePercentile(durations, 99.9),
		Max:   durations[len(durations)-1],
		Count: len(durations),
	}

	return result, nil
}
