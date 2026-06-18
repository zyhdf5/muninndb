// Package metrics 提供 Prometheus 指标定义和 Vault 级别的 engram 计数采集器。
//
// 本包从 internal/metrics 迁移而来，保留全部 Prometheus 指标变量和
// VaultEngramCollector，可独立用于任何需要 MuninnDB 风格指标的项目。
package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ────────────────────────────────────────────────────
// Prometheus 指标变量
// ────────────────────────────────────────────────────

var (
	// EngineWritesTotal 记录 engram 写入总次数。
	EngineWritesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "muninndb_engine_writes_total",
		Help: "engram 写入总次数",
	})

	// EngineActivationsTotal 记录激活（recall）调用总次数。
	EngineActivationsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "muninndb_engine_activations_total",
		Help: "激活调用总次数",
	})

	// FTSSearchDuration 记录全文搜索延迟分布。
	FTSSearchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "muninndb_fts_search_duration_seconds",
		Help:    "全文搜索延迟（秒）",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5},
	})

	// NoveltyDropsTotal 记录因通道满而静默丢弃的新颖性任务总数。
	NoveltyDropsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "muninndb_novelty_drops_total",
		Help: "因通道满而丢弃的新颖性任务总数",
	})

	// RESTRequestDuration 记录 HTTP 请求延迟分布，按方法、路径和状态码分类。
	RESTRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muninn_rest_request_duration_seconds",
		Help:    "HTTP 请求延迟（秒）",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status_class"})

	// RateLimitRejections 记录因限流拒绝的请求总数，按限流器类型分类。
	RateLimitRejections = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "muninn_rate_limit_rejections_total",
		Help: "因限流拒绝的请求总数",
	}, []string{"limiter_type"})

	// ImportJobsTotal 记录 vault 导入任务总数，按完成状态分类。
	ImportJobsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "muninn_import_jobs_total",
		Help: "vault 导入任务总数",
	}, []string{"status"})

	// FTSIndexFailures 记录重建索引期间 FTS 索引写入失败总数，按 vault 分类。
	FTSIndexFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "muninn_fts_index_failures_total",
		Help: "FTS 索引写入失败总数",
	}, []string{"vault"})

	// WriteDuration 记录引擎写入延迟分布，按 vault 分类。
	WriteDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muninndb_write_duration_seconds",
		Help:    "引擎写入延迟（秒）",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"vault"})

	// ActivateDuration 记录引擎激活（recall）延迟分布，按 vault 分类。
	ActivateDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muninndb_activate_duration_seconds",
		Help:    "引擎激活延迟（秒）",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
	}, []string{"vault"})

	// ReadDuration 记录引擎读取延迟分布，按 vault 分类。
	ReadDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muninndb_read_duration_seconds",
		Help:    "引擎读取延迟（秒）",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
	}, []string{"vault"})

	// EmbedPending 记录当前等待嵌入处理的 engram 数量。
	EmbedPending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "muninndb_embed_pending",
		Help: "等待嵌入处理的 engram 数量",
	})
)

// ────────────────────────────────────────────────────
// VaultEngramCollector — 按 vault 采集 engram 计数
// ────────────────────────────────────────────────────

// VaultStore 是 VaultEngramCollector 所依赖的最小存储接口。
// 生产环境中由 PebbleStore 实现，测试中可使用内存 mock 替代。
type VaultStore interface {
	// ListVaultNames 返回所有已知 vault 名称。
	ListVaultNames() ([]string, error)
	// ResolveVaultPrefix 将 vault 名称映射为 8 字节前缀。
	ResolveVaultPrefix(name string) [8]byte
	// GetVaultCount 返回指定 vault 前缀下的 engram 总数。
	GetVaultCount(ctx context.Context, ws [8]byte) int64
}

// VaultEngramCollector 在 Prometheus 抓取时按 vault 采集 engram 计数。
type VaultEngramCollector struct {
	store VaultStore
	desc  *prometheus.Desc
}

// NewVaultEngramCollector 创建一个由 store 支撑的 VaultEngramCollector。
func NewVaultEngramCollector(store VaultStore) *VaultEngramCollector {
	return &VaultEngramCollector{
		store: store,
		desc: prometheus.NewDesc(
			"muninndb_vault_engrams",
			"每个 vault 的当前 engram 数量",
			[]string{"vault"},
			nil,
		),
	}
}

// Describe 实现 prometheus.Collector 接口。
func (c *VaultEngramCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect 实现 prometheus.Collector 接口。
// 遍历所有 vault 并为每个 vault 生成一个 GaugeValue 指标。
func (c *VaultEngramCollector) Collect(ch chan<- prometheus.Metric) {
	vaults, err := c.store.ListVaultNames()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, name := range vaults {
		ws := c.store.ResolveVaultPrefix(name)
		count := c.store.GetVaultCount(ctx, ws)
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(count), name)
	}
}
