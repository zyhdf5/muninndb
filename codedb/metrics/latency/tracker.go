// Package latency 提供基于环形缓冲区的延迟追踪器。
//
// Tracker 为每个 (vault, operation) 组合维护一个固定大小的环形缓冲区，
// 记录最近 ringSize 条采样并按需计算 P50/P95/P99 百分位数和平均值。
// 所有操作均为并发安全。
package latency

import (
	"math"
	"sort"
	"sync"
	"time"
)

// ringSize 是每个环形缓冲区的最大采样数。
const ringSize = 1024

// Stats 包含一组延迟统计信息。
type Stats struct {
	P50Ms float64 `json:"p50_ms"` // 第 50 百分位延迟（毫秒）
	P95Ms float64 `json:"p95_ms"` // 第 95 百分位延迟（毫秒）
	P99Ms float64 `json:"p99_ms"` // 第 99 百分位延迟（毫秒）
	AvgMs float64 `json:"avg_ms"` // 平均延迟（毫秒）
	Count int64   `json:"count"`  // 总采样次数（包含已被环形缓冲区淘汰的历史采样）
}

// bufferKey 唯一标识一个 (vault, operation) 组合。
type bufferKey struct {
	vault     [8]byte
	operation string
}

// ringBuffer 是固定大小的环形采样缓冲区。
type ringBuffer struct {
	samples [ringSize]float64 // 采样值（毫秒）
	pos     int               // 下一个写入位置
	filled  bool              // 缓冲区是否已至少完整填充一次
	count   int64             // 历史总采样次数
	sum     float64           // 当前缓冲区内采样之和
}

// Tracker 按 (vault, operation) 维度追踪延迟统计。
type Tracker struct {
	mu      sync.RWMutex
	buffers map[bufferKey]*ringBuffer
}

// New 创建一个新的延迟追踪器。
func New() *Tracker {
	return &Tracker{
		buffers: make(map[bufferKey]*ringBuffer),
	}
}

// Record 记录一条延迟采样。ws 为 vault 前缀，operation 为操作名称，d 为延迟时长。
func (t *Tracker) Record(ws [8]byte, operation string, d time.Duration) {
	ms := float64(d.Nanoseconds()) / 1e6
	t.mu.Lock()
	key := bufferKey{vault: ws, operation: operation}
	rb, ok := t.buffers[key]
	if !ok {
		rb = &ringBuffer{}
		t.buffers[key] = rb
	}
	if rb.filled {
		rb.sum -= rb.samples[rb.pos] // 减去被淘汰的旧采样值
	}
	rb.samples[rb.pos] = ms
	rb.sum += ms
	rb.count++
	rb.pos++
	if rb.pos >= ringSize {
		rb.pos = 0
		rb.filled = true
	}
	t.mu.Unlock()
}

// For 返回指定 (vault, operation) 的延迟统计。
// 如果没有对应的采样记录，返回零值 Stats。
func (t *Tracker) For(ws [8]byte, operation string) Stats {
	t.mu.RLock()
	rb, ok := t.buffers[bufferKey{vault: ws, operation: operation}]
	if !ok {
		t.mu.RUnlock()
		return Stats{}
	}
	stats := computeStats(rb)
	t.mu.RUnlock()
	return stats
}

// Snapshot 返回所有 vault 和操作的延迟统计快照。
// 外层 key 为 vault 前缀，内层 key 为操作名称。
func (t *Tracker) Snapshot() map[[8]byte]map[string]Stats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[[8]byte]map[string]Stats)
	for key, rb := range t.buffers {
		if _, ok := result[key.vault]; !ok {
			result[key.vault] = make(map[string]Stats)
		}
		result[key.vault][key.operation] = computeStats(rb)
	}
	return result
}

// computeStats 从环形缓冲区计算延迟统计。
func computeStats(rb *ringBuffer) Stats {
	n := rb.pos
	if rb.filled {
		n = ringSize
	}
	if n == 0 {
		return Stats{Count: rb.count}
	}
	sorted := make([]float64, n)
	copy(sorted, rb.samples[:n])
	sort.Float64s(sorted)
	return Stats{
		P50Ms: percentile(sorted, 0.50),
		P95Ms: percentile(sorted, 0.95),
		P99Ms: percentile(sorted, 0.99),
		AvgMs: rb.sum / float64(n),
		Count: rb.count,
	}
}

// percentile 使用线性插值计算已排序切片的指定百分位值。
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
