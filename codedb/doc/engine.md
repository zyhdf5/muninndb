# MuninnDB Engine 技术文档 (codedb/engine)

## 1. 概述

`codedb/engine` 是 MuninnDB 的核心图数据库引擎。它实现了认知图模型，不仅支持传统的图存储，还引入了时间衰减（Ebbinghaus Forgetting Curve）、启发式关联（Hebbian Learning）和实体增强等认知原语。该包是对底层存储（Pebble）的高级封装，专注于记忆（Engram）、关联（Association）和实体（Entity）的生命周期管理与图检索。

### 模块定位与职责
- **记忆存储**：管理 `Engram` 的持久化，支持变长内容的 ERF 格式编码。
- **关联分析**：维护节点间的有向加权边，支持基于权重的 BFS 遍历。
- **实体系统**：全局命名实体提取、链接以及跨 Vault 的共现聚类分析。
- **认知计算**：实现相关性评分传播、激活扩散和实体激活增强。
- **可靠性**：提供原子批次写入、幂等性收据和周期性检查点备份。

---

## 2. 架构

Engine 采用了典型的层次化架构，确保核心逻辑与底层存储引擎解耦。

```text
+-----------------------------------------------------------+
|                        Engine API                         |
|   (ExportGraph, BFSTraverse, ApplyEntityBoost, etc.)      |
+----------------------------+------------------------------+
                             |
              +--------------v--------------+
              |       Cognitive Logic       |
              | (Decay, Hebbian, BFS, L1)   |
              +--------------+--------------+
                             |
      +----------------------v-----------------------+
      |                Store Interface               |
      |   (PebbleStore, ERF Encoder, Keys Generator)  |
      +----------+--------------------------+--------+
                 |                          |
        +--------v--------+        +--------v--------+
        |  Pebble (LSM)   |        |  Domain Cache   |
        | (KV Persistence)|        | (LRU/Relevance) |
        +-----------------+        +-----------------+
```

---

## 3. 核心原理

### 3.1 认知图模型 (Cognitive Graph Model)
Engine 维护三层数据结构：
1. **Engram (记忆迹)**：原子记忆单元，包含内容、元数据和向量。
2. **Association (关联)**：节点间的显式有向边。权重随共激活（Co-activation）增强，随时间或不使用而衰减。
3. **Entity (实体)**：从内容中提取的具名概念。实体作为隐式索引，连接不同的 Engram。

### 3.2 ULID 时间排序 ID
Engine 使用 16 字节的 `ULID` 作为唯一标识符。
- **可排序性**：前 6 字节为毫秒级时间戳，确保记录按创建顺序天然有序。
- **高性能**：相比 UUID，ULID 在 Pebble 的 LSM-Tree 结构中能显著减少随机写入带来的 compaction 压力。

### 3.3 记忆生命周期状态机
`LifecycleState` 定义了记忆从创建到消亡的过程：
- `planning` -> `active` (默认) -> `paused` -> `blocked` -> `completed` -> `cancelled` -> `archived`
- **软删除**：`StateSoftDeleted` 允许记忆在彻底物理清理前保留 7 天恢复窗口。

### 3.4 关联类型与记忆类型
- **RelType**：定义了 `depends_on` (依赖), `supports` (支持), `contradicts` (矛盾), `is_part_of` (从属) 等 16 种标准语义关系。
- **MemoryType**：基于规则的分类，如 `fact` (事实), `decision` (决策), `task` (待办) 等，用于上层过滤和呈现。

### 3.5 BFS 图遍历原理
`adjacency.go` 中的 `BFSTraverse` 实现了带衰减的分数传播：
- **分数传播**：从种子节点出发，分数沿边传递：`score = parentScore * edgeWeight * hopPenalty`。
- **提前终止**：索引按 `Weight` 降序排列，当当前分支分数低于 `bfsMinHopScore` (0.05) 时，停止搜索该分支。

### 3.6 实体增强 (Entity Boost)
通过 `ApplyEntityBoost` 传播激活：
- 当一个 Engram 被激活时，它提到的实体也会被激活。
- 这些激活的实体会将能量回馈给其他提到同一实体的 Engram，实现跨节点的相关性提升。

### 3.7 实体聚类 (Entity Clusters)
利用共现分析（Co-occurrence Analysis）：
- 统计两个实体在同一个 Engram 中共同出现的频率。
- `GetEntityClusters` 返回高频共现对，用于发现潜在的概念关联。

### 3.8 缓存系统
- **L1 缓存**：Vault 隔离的 LRU 缓存，存储最近访问的 Engram 结构体。
- **Domain Cache**：跨 Vault 的二级缓存，基于访问模式和相关性/置信度组合评分（Hebbian-inspired）保留高价值数据。

### 3.9 合并保护 (mergeGuard)
使用分片互斥锁（Striped Mutex）：
- 防止在并发执行实体合并（Entity Merge）时出现竞争。
- 只有涉及相同实体的合并操作会被序列化，不同实体的操作保持并行。

### 3.10 ERF 格式原理
`ERF` (Engram Record Format) 是定制的追加式二进制格式：
- **头部**：Magic (`0x4D554E4E`) 和版本号。
- **定长区**：100 字节元数据（ID、状态、权重等），支持直接 `O(1)` 定位。
- **变长区**：内容、标签、关联和嵌入向量。
- **优化**：支持量化向量存储（量化到 `int8`）和 ZSTD/LZ4 内容压缩。

### 3.11 Keys 子包与键构造
所有 Pebble 键都遵循：`Prefix (1B) | VaultPrefix (8B) | Identifier (NB)`。
- `0x01` / `0x02`：Engram 记录与元数据。
- `0x03` / `0x04`：关联前向/反向索引（权重使用补数存储以实现降序扫描）。
- `0x10`：相关性桶索引，按 `(9 - floor(relevance*10))` 分桶，实现高相关性优先扫描。

### 3.12 备份系统
`BackupScheduler` 提供可靠的数据保护：
- **Checkpoint**：利用 Pebble 的检查点机制实现零停机备份。
- **Pruning**：根据 `Retain` 配置自动清理过期备份。
- **Atomic**：备份目录包含完整的 `pebble` 数据、`wal` 日志和 `auth_secret` 密钥。

---

## 4. 数据类型参考

### Engram
| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `ID` | `ULID` | 全局唯一 ID |
| `Concept` | `string` | 概念标签（最大 512B） |
| `Content` | `string` | 核心记忆内容（最大 16KB） |
| `State` | `LifecycleState` | 生命周期状态 |
| `Confidence` | `float32` | 置信度 (0.0-1.0) |
| `Relevance` | `float32` | 动态相关性评分 |
| `Associations` | `[]Association` | 显式边列表 |
| `Embedding` | `[]float32` | 向量嵌入（可选） |

### Association
| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `TargetID` | `ULID` | 目标节点 ID |
| `RelType` | `RelType` | 关系类型码 |
| `Weight` | `float32` | 边权重 (0.0-1.0) |
| `CoActivationCount` | `uint32` | 共激活计数，用于 Hebbian 学习 |

---

## 5. 公共 API 参考

### Engine 核心方法
- `New(store Store) *Engine`：创建引擎实例。
- `ExportGraph(ctx, vault, includeEngrams) (*ExportGraph, error)`：导出 JSON-LD 兼容的图结构。
- `BFSTraverse(ctx, store, ws, seeds, threshold, maxDepth) ([]TraversalResult, error)`：执行广度优先搜索。
- `ApplyEntityBoost(ctx, store, ws, initialResults, topN) ([]ScoredEngram, error)`：通过实体链接增强搜索结果。

### 实体管理
- `GetEntityRecord(ctx, name) (*EntityRecord, error)`：获取全局实体元数据。
- `UpsertEntityRecord(ctx, record, source) error`：创建或更新实体。
- `MergeEntities(ctx, vault, fromName, toName) error`：原子合并两个实体，重定向所有链接。

---

## 6. 接口参考

### Store (最小图操作接口)
图检索层只需实现该接口即可运行。
```go
type Store interface {
    ResolveVaultPrefix(name string) [8]byte
    GetEngram(ctx context.Context, ws [8]byte, id ULID) (*Engram, error)
    GetAssociations(ctx context.Context, ws [8]byte, ids []ULID, maxPerNode int) (map[ULID][]Association, error)
    ScanRelationships(ctx context.Context, ws [8]byte, fn func(record RelationshipRecord) error) error
    // ... 实体相关扫描方法
}
```

### EngineStore (完整持久化接口)
生产环境由 `PebbleStore` 实现，包含元数据更新、批量写入和相关性桶管理。

---

## 7. 最佳实践

1. **利用批次写入**：大规模导入时使用 `WriteEngramBatch`，可显著减少 fsync 次数。
2. **状态机驱动逻辑**：不要手动硬删除，优先通过 `SoftDelete` 进入生命周期流。
3. **实体标准化**：在写入前对实体名称进行 `Trim` 和 `ToLower`，引擎内部已做 NFKC 规范化。
4. **控制 BFS 深度**：一般推荐 `maxDepth` 为 2 或 3，深度过大会导致结果集噪声过多。
5. **权重微调**：在建立 `Association` 时，根据业务语义赋予初始 `Weight`（如依赖关系赋予 0.9，引用关系赋予 0.4）。
6. **定期触发 Decay**：通过调用 `DecayAssocWeights` 让不常用的关联自然消退。
7. **配置 Retain 策略**：备份目录不要无限增长，建议 `Retain` 设置为 7 或 14。
8. **监控磁盘大小**：定期检查 `DiskSize()`，防止 LSM-Tree 膨胀。

---

## 8. 代码示例

### 基本存储与关联
```go
package main

import (
	"context"
	"github.com/scrypster/muninndb/codedb/engine"
)

func main() {
	// 假设已初始化 pebbleStore
	eng := engine.New(pebbleStore)
	ctx := context.Background()
	ws := pebbleStore.ResolveVaultPrefix("my-vault")

	// 1. 存储记忆
	mem := &engine.Engram{
		Concept: "Go Concurrency",
		Content: "Use channels for communication, not shared memory.",
		State:   engine.StateActive,
	}
	id, _ := pebbleStore.WriteEngram(ctx, ws, mem)

	// 2. 建立关联
	assoc := &engine.Association{
		TargetID: targetID,
		RelType:  engine.RelDependsOn,
		Weight:   0.8,
	}
	_ = pebbleStore.WriteAssociation(ctx, ws, id, targetID, assoc)
}
```

### 图遍历检索
```go
results, _ := engine.BFSTraverse(ctx, pebbleStore, ws, []engine.ULID{seedID}, 0.1, 2)
for _, res := range results {
    fmt.Printf("发现相关记忆: %s, 传播分数: %.2f\n", res.ID, res.Score)
}
```

---

## 9. 常见错误与规避

- **并发合并冲突**：虽然有 `mergeGuard`，但应尽量避免在短时间内对同一实体进行高频合并操作。
- **ULID 碰撞概率**：虽极低，但在单毫秒生成超过 10 万个 ID 时需注意。引擎已使用 `Monotonic` 熵源规避。
- **Vault 名称冲突**：Vault 名称通过 SipHash 映射，尽管有 8 字节前缀，理论上存在碰撞可能，建议 Vault 名称具有足够的辨识度。
- **大图遍历超时**：BFS 默认上限为 500 节点，若图极度稠密，务必在 `ctx` 中设置 `Deadline`。
