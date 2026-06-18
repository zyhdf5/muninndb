package metrics

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ── mock 实现 ──

// mockStore 是 VaultStore 的内存 mock，用于单元测试。
type mockStore struct {
	vaults      []string
	countResult int64
	listErr     error
}

func (m *mockStore) ListVaultNames() ([]string, error) {
	return m.vaults, m.listErr
}

func (m *mockStore) ResolveVaultPrefix(name string) [8]byte {
	var prefix [8]byte
	copy(prefix[:], name)
	return prefix
}

func (m *mockStore) GetVaultCount(_ context.Context, _ [8]byte) int64 {
	return m.countResult
}

// ── VaultEngramCollector 测试 ──

func TestVaultEngramCollector_Basic(t *testing.T) {
	store := &mockStore{
		vaults:      []string{"default", "work"},
		countResult: 42,
	}
	collector := NewVaultEngramCollector(store)

	// Describe 应发出恰好 1 个描述符
	descCh := make(chan *prometheus.Desc, 2)
	collector.Describe(descCh)
	close(descCh)

	var descs []*prometheus.Desc
	for d := range descCh {
		descs = append(descs, d)
	}
	if len(descs) != 1 {
		t.Fatalf("期望 1 个描述符，实际 %d", len(descs))
	}

	// Collect 应为每个 vault 发出 1 个指标
	metricCh := make(chan prometheus.Metric, 10)
	collector.Collect(metricCh)
	close(metricCh)

	var count int
	for range metricCh {
		count++
	}
	if count != 2 {
		t.Fatalf("期望 2 个指标（每个 vault 一个），实际 %d", count)
	}
}

func TestVaultEngramCollector_ErrorRecovery(t *testing.T) {
	store := &mockStore{listErr: errors.New("存储不可用")}
	collector := NewVaultEngramCollector(store)

	metricCh := make(chan prometheus.Metric, 10)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Collect 发生 panic: %v", r)
			}
		}()
		collector.Collect(metricCh)
	}()
	close(metricCh)

	var count int
	for range metricCh {
		count++
	}
	if count != 0 {
		t.Errorf("ListVaultNames 出错时期望 0 个指标，实际 %d", count)
	}
}

// ── Prometheus 指标注册测试 ──

func TestRESTRequestDuration_Registered(t *testing.T) {
	RESTRequestDuration.WithLabelValues("GET", "/api/health", "2xx").Observe(0.001)
}

func TestRateLimitRejections_Registered(t *testing.T) {
	if RateLimitRejections == nil {
		t.Fatal("RateLimitRejections 不应为 nil")
	}
	RateLimitRejections.WithLabelValues("global").Inc()
	RateLimitRejections.WithLabelValues("per_ip").Inc()
}

func TestImportJobsTotal_Registered(t *testing.T) {
	if ImportJobsTotal == nil {
		t.Fatal("ImportJobsTotal 不应为 nil")
	}
	ImportJobsTotal.WithLabelValues("completed").Inc()
	ImportJobsTotal.WithLabelValues("failed").Inc()
}

func TestFTSIndexFailures_Registered(t *testing.T) {
	if FTSIndexFailures == nil {
		t.Fatal("FTSIndexFailures 不应为 nil")
	}
	FTSIndexFailures.WithLabelValues("default").Inc()
}

func TestWriteDuration_Registered(t *testing.T) {
	if WriteDuration == nil {
		t.Fatal("WriteDuration 不应为 nil")
	}
	WriteDuration.WithLabelValues("default").Observe(0.01)
}

func TestActivateDuration_Registered(t *testing.T) {
	if ActivateDuration == nil {
		t.Fatal("ActivateDuration 不应为 nil")
	}
	ActivateDuration.WithLabelValues("default").Observe(0.05)
}

func TestReadDuration_Registered(t *testing.T) {
	if ReadDuration == nil {
		t.Fatal("ReadDuration 不应为 nil")
	}
	ReadDuration.WithLabelValues("default").Observe(0.002)
}

func TestEmbedPending_Registered(t *testing.T) {
	if EmbedPending == nil {
		t.Fatal("EmbedPending 不应为 nil")
	}
	EmbedPending.Set(10)
}

func TestEngineWritesTotal_Registered(t *testing.T) {
	if EngineWritesTotal == nil {
		t.Fatal("EngineWritesTotal 不应为 nil")
	}
	EngineWritesTotal.Inc()
}

func TestEngineActivationsTotal_Registered(t *testing.T) {
	if EngineActivationsTotal == nil {
		t.Fatal("EngineActivationsTotal 不应为 nil")
	}
	EngineActivationsTotal.Inc()
}

func TestFTSSearchDuration_Registered(t *testing.T) {
	if FTSSearchDuration == nil {
		t.Fatal("FTSSearchDuration 不应为 nil")
	}
	FTSSearchDuration.Observe(0.005)
}

func TestNoveltyDropsTotal_Registered(t *testing.T) {
	if NoveltyDropsTotal == nil {
		t.Fatal("NoveltyDropsTotal 不应为 nil")
	}
	NoveltyDropsTotal.Inc()
}

// ── Serve 测试 ──

func TestServe_EmptyAddr(t *testing.T) {
	Serve(context.Background(), "")
}

func TestServe_StartsAndStops(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("查找可用端口失败: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Serve(ctx, addr)

	deadline := time.Now().Add(2 * time.Second)
	var resp *http.Response
	for time.Now().Before(deadline) {
		resp, err = http.Get("http://" + addr + "/metrics")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /metrics 在截止时间内未成功: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("期望 200，实际 %d", resp.StatusCode)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)
}
