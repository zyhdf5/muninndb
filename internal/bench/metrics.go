package bench

import "time"

// RetrievalResult contains precision, recall, and ranking metrics
// for measuring retrieval quality of semantic search.
type RetrievalResult struct {
	PrecisionAt1  float64
	PrecisionAt5  float64
	PrecisionAt10 float64
	NDCGAt10      float64
	MRR           float64
	RecallAt10    float64
	QueryCount    int
	Duration      time.Duration
}

// ThroughputResult measures write and activation throughput.
type ThroughputResult struct {
	WritesPerSec      float64
	ActivationsPerSec float64
	TotalWrites       int64
	TotalActivations  int64
	Concurrency       int
	Duration          time.Duration
}

// LatencyResult measures latency percentiles for activation queries.
type LatencyResult struct {
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	P999  time.Duration
	Max   time.Duration
	Count int
}

// MemoryResult measures memory usage under sustained write load.
type MemoryResult struct {
	BaselineHeapMB float64
	PeakHeapMB     float64
	GrowthMB       float64
	GCCount        uint32
	Duration       time.Duration
}

// ScaleResult measures retrieval quality and latency at large corpus sizes (10k–20k engrams).
type ScaleResult struct {
	CorpusSize   int
	Topics       int
	IngestDur    time.Duration // time to write all engrams
	FlushDur     time.Duration // time waiting for FTS flush
	PrecisionAt1 float64
	PrecisionAt5 float64
	NDCGAt10     float64
	MRR          float64
	RecallAt10   float64
	QueryCount   int
	QueryDur     time.Duration
	P50Latency   time.Duration
	P95Latency   time.Duration
	P99Latency   time.Duration
	MaxLatency   time.Duration
}
