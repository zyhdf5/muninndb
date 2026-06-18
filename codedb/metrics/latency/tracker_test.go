package latency

import (
	"sync"
	"testing"
	"time"
)

func TestTracker_RecordAndPercentiles(t *testing.T) {
	tr := New()
	ws := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	for i := 1; i <= 100; i++ {
		tr.Record(ws, "write", time.Duration(i)*time.Millisecond)
	}
	stats := tr.For(ws, "write")
	if stats.Count != 100 {
		t.Errorf("Count = %d，期望 100", stats.Count)
	}
	if stats.P50Ms < 45 || stats.P50Ms > 55 {
		t.Errorf("P50 = %.1f，期望约 50", stats.P50Ms)
	}
	if stats.P95Ms < 90 || stats.P95Ms > 100 {
		t.Errorf("P95 = %.1f，期望约 95", stats.P95Ms)
	}
	if stats.P99Ms < 95 || stats.P99Ms > 100 {
		t.Errorf("P99 = %.1f，期望约 99", stats.P99Ms)
	}
}

func TestTracker_EmptyStats(t *testing.T) {
	tr := New()
	ws := [8]byte{1}
	stats := tr.For(ws, "read")
	if stats.Count != 0 {
		t.Errorf("Count = %d，期望 0", stats.Count)
	}
}

func TestTracker_Snapshot(t *testing.T) {
	tr := New()
	ws1 := [8]byte{1}
	ws2 := [8]byte{2}
	tr.Record(ws1, "write", 5*time.Millisecond)
	tr.Record(ws1, "activate", 10*time.Millisecond)
	tr.Record(ws2, "write", 20*time.Millisecond)
	snap := tr.Snapshot()
	if len(snap) != 2 {
		t.Errorf("Snapshot 包含 %d 个 vault，期望 2", len(snap))
	}
	if len(snap[ws1]) != 2 {
		t.Errorf("vault 1 有 %d 个操作，期望 2", len(snap[ws1]))
	}
}

func TestTracker_RingWrap(t *testing.T) {
	tr := New()
	ws := [8]byte{1}
	for i := 0; i < ringSize+500; i++ {
		tr.Record(ws, "write", time.Millisecond)
	}
	stats := tr.For(ws, "write")
	if stats.Count != int64(ringSize+500) {
		t.Errorf("Count = %d，期望 %d", stats.Count, ringSize+500)
	}
	if stats.P50Ms < 0.5 || stats.P50Ms > 1.5 {
		t.Errorf("P50 = %.1f，期望约 1.0", stats.P50Ms)
	}
}

func TestTracker_ConcurrentAccess(t *testing.T) {
	tr := New()
	ws := [8]byte{1}

	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				tr.Record(ws, "write", time.Millisecond)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = tr.For(ws, "write")
			_ = tr.Snapshot()
			time.Sleep(time.Microsecond)
		}
	}()
	wg.Wait()

	stats := tr.For(ws, "write")
	if stats.Count != 1000 {
		t.Errorf("Count = %d，期望 1000", stats.Count)
	}
}

func TestTracker_SingleSample(t *testing.T) {
	tr := New()
	ws := [8]byte{1}
	tr.Record(ws, "write", 5*time.Millisecond)

	stats := tr.For(ws, "write")
	if stats.Count != 1 {
		t.Errorf("Count = %d，期望 1", stats.Count)
	}
	if stats.P50Ms < 4.5 || stats.P50Ms > 5.5 {
		t.Errorf("P50 = %.1f，期望约 5.0", stats.P50Ms)
	}
	if stats.P99Ms < 4.5 || stats.P99Ms > 5.5 {
		t.Errorf("P99 = %.1f，期望约 5.0", stats.P99Ms)
	}
}

func TestPercentile_EmptySlice(t *testing.T) {
	result := percentile(nil, 0.50)
	if result != 0 {
		t.Errorf("空切片的百分位应为 0，实际 %f", result)
	}
}

func TestPercentile_SingleElement(t *testing.T) {
	result := percentile([]float64{42.0}, 0.99)
	if result != 42.0 {
		t.Errorf("单元素的 P99 应为 42.0，实际 %f", result)
	}
}

func TestComputeStats_EmptyBuffer(t *testing.T) {
	rb := &ringBuffer{}
	stats := computeStats(rb)
	if stats.Count != 0 {
		t.Errorf("空缓冲区的 Count 应为 0，实际 %d", stats.Count)
	}
	if stats.AvgMs != 0 {
		t.Errorf("空缓冲区的 AvgMs 应为 0，实际 %f", stats.AvgMs)
	}
}
