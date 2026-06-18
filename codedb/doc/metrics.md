# MuninnDB Metrics 技术文档

本文档详细介绍了 `codedb/metrics` 包的设计架构、核心原理、指标参考以及最佳实践。该包负责 MuninnDB 的 Prometheus 指标收集、动态 Vault 统计和内存环形缓冲区延迟追踪。

## 概述

`metrics` 包提供了一套完整的观测性工具，用于监控 MuninnDB 的运行状态。它主要包含两个核心组件：
1. **Prometheus 集成**：标准 Prometheus 指标收集和自定义 `VaultEngramCollector`，支持按 Vault 动态统计记忆（engram）数量。
2. **延迟追踪器 (Latency Tracker)**：基于高性能环形缓冲区的实时延迟统计，支持计算 P50/P95/P99 百分位。

## 架构图

```text
+---------------------+      +------------------------+      +-------------------+
|    MuninnDB Core    |      |    metrics 包           |      |  External System  |
+---------+-----------+      +-----------+------------+      +---------+---------+
          |                              |                             |
          | 1. Record Metrics/Latency    |                             |
          +----------------------------> |                             |
          |                              |                             |
          |                              |  2. /metrics endpoint       |
          |                              | <---------------------------+ (Prometheus)
          |                              |                             |
          | 3. Fetch Vault Stats         |                             |
          | <--------------------------+ |                             |
          |                              |                             |
          |      4. Serve Metrics Server |                             |
          +----------------------------> |                             |
                                         +-----------------------------+
```

## 核心原理

### 1. Prometheus 指标类型
本包使用了多种 Prometheus 指标类型来适配不同场景：
- **Counter**：用于记录单调递增的次数，如总写入次数。
- **Histogram**：用于记录分布情况，特别是延迟分布，支持自定义 Buckets。
- **Gauge**：用于记录当前状态的瞬时值，如正在等待嵌入的任务数。
- **Vec (Vector)**：带标签（Labels）的指标，支持按维度（如 Vault 名称、HTTP 方法）进行拆分统计。

### 2. VaultEngramCollector
这是一个自定义的 `prometheus.Collector` 接口实现。它解决了 Prometheus 静态指标无法感知动态创建的 Vault 的问题。
- **动态发现**：在每次 Prometheus 抓取（Scrape）时，通过 `VaultStore` 接口列出所有 Vault。
- **实时采集**：对每个 Vault 分别查询其 engram 计数。
- **按需暴露**：生成带有 `vault` 标签的瞬时指标 `muninndb_vault_engrams`。

### 3. VaultStore 接口
为了解耦存储层与监控层，我们定义了 `VaultStore` 接口。它允许 `metrics` 包在不依赖具体存储实现（如 Pebble）的情况下获取 Vault 信息。这种抽象也极大地方便了单元测试。

### 4. Serve 函数
`Serve` 函数提供了一个独立的 HTTP 服务器，专门用于暴露 `/metrics` 端点。
- **非阻塞**：在独立的 goroutine 中运行。
- **优雅关闭**：接收 `context.Context`，当上下文取消时，自动调用 `srv.Shutdown` 停止服务。

### 5. 延迟追踪器原理
`latency.Tracker` 使用环形缓冲区（Ring Buffer）来记录最近的延迟采样：
- **固定大小**：默认大小为 1024，防止内存无限增长。
- **高性能**：读写均采用读写锁保护，写操作仅需 O(1) 时间复杂度。
- **维度聚合**：按 `(vault, operation)` 组合进行独立追踪。

### 6. 百分位计算
百分位数（P50/P95/P99）采用排序插值算法计算：
- **采样快照**：从环形缓冲区提取当前有效的采样值。
- **线性插值**：对采样值进行排序，并使用线性插值法计算精确的百分位点。

## 指标完整列表

| 指标名称 | 类型 | 标签 (Labels) | 含义 |
| :--- | :--- | :--- | :--- |
| `muninndb_engine_writes_total` | Counter | - | Engram 写入总次数 |
| `muninndb_engine_activations_total` | Counter | - | 激活（Recall）调用总次数 |
| `muninndb_fts_search_duration_seconds` | Histogram | - | 全文搜索延迟分布 |
| `muninndb_novelty_drops_total` | Counter | - | 因通道满而静默丢弃的新颖性任务总数 |
| `muninn_rest_request_duration_seconds` | HistogramVec | `method`, `path`, `status_class` | HTTP 请求延迟分布 |
| `muninn_rate_limit_rejections_total` | CounterVec | `limiter_type` | 因限流拒绝的请求总数 |
| `muninn_import_jobs_total` | CounterVec | `status` | Vault 导入任务总数 |
| `muninn_fts_index_failures_total` | CounterVec | `vault` | FTS 索引写入失败总数 |
| `muninndb_write_duration_seconds` | HistogramVec | `vault` | 引擎写入延迟分布 |
| `muninndb_activate_duration_seconds` | HistogramVec | `vault` | 引擎激活延迟分布 |
| `muninndb_read_duration_seconds` | HistogramVec | `vault` | 引擎读取延迟分布 |
| `muninndb_embed_pending` | Gauge | - | 当前等待嵌入处理的 engram 数量 |

## 公共 API 参考

### `metrics` 包
- `NewVaultEngramCollector(store VaultStore) *VaultEngramCollector`: 创建 Vault 采集器。
- `Serve(ctx context.Context, addr string)`: 启动指标 HTTP 服务器。

### `latency` 包
- `New() *Tracker`: 创建新的延迟追踪器。
- `Tracker.Record(ws [8]byte, operation string, d time.Duration)`: 记录延迟。
- `Tracker.For(ws [8]byte, operation string) Stats`: 获取特定操作的统计信息。
- `Tracker.Snapshot() map[[8]byte]map[string]Stats`: 获取所有统计的快照。

## 最佳实践

1. **Grafana 看板建议**：
   - 使用 `rate(muninndb_engine_writes_total[5m])` 观察写入速率。
   - 关注 `muninndb_embed_pending` 指标，如果该值持续升高，说明嵌入计算资源不足。
2. **告警阈值建议**：
   - 当 `muninn_rate_limit_rejections_total` 出现非零增长时，考虑调整客户端访问策略。
   - 监控 `muninndb_fts_search_duration_seconds` 的 P99 值，若超过 100ms 应排查磁盘性能。
3. **延迟追踪使用建议**：
   - 生产环境中建议只在核心路径调用 `Tracker.Record`，虽然它是并发安全的，但在极端高频场景下仍有轻微锁竞争。
4. **指标聚合**：
   - 利用 `vault` 标签进行细粒度监控，识别是否有特定租户/Vault 负载异常。
5. **优雅关闭**：
   - 务必透传 `context.Context` 给 `Serve` 函数，确保在应用重启或退出时，监控服务器能正确释放端口。

## 代码示例

### 启动指标服务器
```go
import "github.com/scrypster/muninndb/codedb/metrics"

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 启动 metrics 服务器，监听 8475 端口
    metrics.Serve(ctx, ":8475")
}
```

### 使用延迟追踪器
```go
import "github.com/scrypster/muninndb/codedb/metrics/latency"

tracker := latency.New()
start := time.Now()

// 执行某个操作...
operation := "activate"
vaultPrefix := [8]byte{0x01, 0x02}

tracker.Record(vaultPrefix, operation, time.Since(start))

// 获取统计
stats := tracker.For(vaultPrefix, operation)
fmt.Printf("P99 Latency: %.2f ms\n", stats.P99Ms)
```

## 常见错误与规避

- **重复注册指标**：`metrics` 包内部使用了 `promauto`，在多次初始化或包被循环引用时可能触发 Prometheus 的重复注册 panic。确保 `metrics` 相关的初始化逻辑在全局只运行一次。
- **VaultStore 为空**：在初始化 `VaultEngramCollector` 时，如果传入的 `store` 为 nil，采集过程会失效。建议在初始化前进行显式检查。
- **地址冲突**：调用 `Serve` 时如果指定的 `addr` 已经被占用，`ListenAndServe` 会静默失败（由于是在 goroutine 中运行且忽略了错误）。建议在启动前检查端口占用情况。
