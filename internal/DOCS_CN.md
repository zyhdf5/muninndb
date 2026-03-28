# MuninnDB 内部技术架构文档

本文档详细描述 MuninnDB 认知数据库的内部实现，包括认知引擎、核心算法、存储层、索引系统、认证模型、插件架构、传输协议和集群复制。所有技术细节均来源于实际代码。

---

## 1. 认知引擎 (internal/engine/)

认知引擎是 MuninnDB 的核心调度中心，通过 `EngineAPI` 接口向所有传输层暴露统一的语义操作。

### 1.1 EngineAPI 核心接口

引擎实现以下核心操作：
- **Write** — 写入 Engram（记忆痕迹）
- **Read** — 读取单个 Engram
- **Activate** — 上下文驱动的记忆激活/召回（6 阶段管线）
- **Link** — 建立或更新 Engram 间的关联
- **Forget** — 遗忘（软删除/降级）
- **Stat** — 获取 Vault 统计信息
- **Subscribe** — SSE 订阅实时推送事件

所有传输层（REST、gRPC、MBP、MCP）都是 EngineAPI 的薄适配器：验证输入 → 映射类型 → 调用引擎。

### 1.2 写入路径

写入路径遵循"先确认后增强"的设计：

```
客户端请求 → Engine.Write()
  1. 输入验证（格式、标签长度、权限检查）
  2. ERF 编码 → Pebble 批量写入（0x01 全量 + 0x02 元数据）
  3. MOL 日志追加（通过 GroupCommitter 批量 fsync）
  4. 返回 ACK（写入确认，延迟 <10ms）
  5. 异步后处理：
     a. FTS 索引更新（分词 → 倒排表 0x05）
     b. HNSW 向量插入（如有嵌入向量）
     c. 新颖性检测（NoveltyScan）
     d. 自动关联发现（AutoAssociator）
     e. 矛盾检测（ContradictionWorker）
     f. 语义触发器通知（TriggerSystem）
     g. 追溯处理器通知（RetroactiveProcessor.Notify）
```

### 1.3 六阶段激活管线

激活管线定义在 `internal/engine/activation/engine.go` 的 `ActivationEngine.Run()` 方法中。这是 MuninnDB 最核心的检索算法：

**Phase 1 — 嵌入生成与分词**
- 对输入上下文调用 `Embedder.Embed()` 生成向量
- 对查询文本进行 Porter2 词干提取 + 分词
- 输出：查询向量 `queryVec` + 词项列表 `terms`

**Phase 2 — 并行候选检索**
- 同时启动四路检索（`errgroup` 并行执行）：
  - **FTS 检索**：BM25 全文匹配，返回按得分排序的候选集
  - **HNSW 检索**：向量近似搜索，返回 Top-K 相似候选
  - **时间池（Temporal Pool）**：按最近访问时间扫描，返回时间相关候选
  - **PAS 候选**：从 TransitionCacheStore 查询预测性激活信号候选

**Phase 3 — 倒数排名融合 (RRF)**
- 将四路结果通过 RRF 公式合并：
  ```
  Score_RRF = Σ 1/(k + rank(r))
  ```
- 各检索通道的 k 值：
  - FTS: `k = 60`
  - HNSW: `k = 40`
  - Temporal: `k = 120`

**Phase 4 — 赫布增强**
- 查询 ActivationLog（环形缓冲区），获取最近激活的 Engram
- 通过 `GetAssociations()` 获取与最近激活 Engram 的关联权重
- 关联权重作为赫布增强因子叠加到候选得分上

**Phase 4.5 — PAS 转移增强**
- 从 TransitionStore 查询顺序转移模式（source → destination）
- 将转移频次归一化后注入候选得分
- PAS 注入排名常数 `K = 50`，每个 Vault 最大注入数 `PASMaxInjections`（默认 5，范围 1-20）

**Phase 4.75 — 归档边延迟恢复**
- 检查是否有被归档的关联边（存储在 0x25 命名空间）需要恢复
- 使用 Bloom 过滤器快速判断是否存在归档边
- 传递恢复（Transitive Recovery）

**Phase 5 — BFS 图遍历**
- 从高分候选出发，沿关联图进行广度优先搜索
- 配置文件驱动的遍历策略：`default`、`causal`、`confirmatory`、`adversarial`、`structural`
- 关键参数：
  - `hopPenalty = 0.7`（每增加一跳，得分衰减 30%）
  - `maxBFSNodes = 500`（单次遍历最大节点数）
  - `maxEdgesPerNode = 20`（每个节点最大扩展边数）

**Phase 6 — 最终评分与过滤**
- 应用 ACT-R 评分模型（见 1.4）
- 按最终得分排序
- 应用过滤条件（状态、标签、时间范围）
- 生成流式响应

### 1.4 ACT-R 评分模型

MuninnDB 的主评分模型基于 ACT-R 认知架构（Adaptive Control of Thought-Rational）：

```
// 基线激活
B(M) = ln(n + 1) - d × ln(max(ageDays, ageFloor) / (n + 1))

// 总激活量（加入赫布和转移增强）
totalActivation = baseLevel + ACTRHebScale × hebbianBoost + ACTRHebScale × transitionBoost

// 上下文先验（通过 softplus 平滑为非负值）
contextualPrior = softplus(totalActivation) = ln(1 + e^totalActivation)

// 内容匹配得分
contentMatch = w_semantic × vectorScore + w_fts × normalizedFTS

// 原始得分
raw = contentMatch × contextualPrior / actrDenominator

// 最终得分（乘以置信度）
finalScore = clamp(raw, 0, 1) × Confidence
```

关键常量：
- `ACTRDecay (d)` = 0.5
- `ACTRHebScale` = 4.0
- `ageFloor` — 防止新记忆因时间过短导致对数溢出

### 1.5 替代评分模式

- **CGDN 评分**：认知图衰减网络，用于特定场景的替代评分
- **遗留加权求和**：传统的线性加权评分，权重配置：
  - semantic: 0.35
  - FTS: 0.25
  - temporal: 0.20
  - Hebbian: 0.10
  - access: 0.05
  - recency: 0.05

### 1.6 认知工作者

引擎管理多个异步后台工作者（均为非阻塞设计，通道满时丢弃输入）：
- **HebbianWorker** — 处理共激活事件，更新关联权重
- **ContradictionWorker** — 检测记忆间的矛盾
- **ConfidenceWorker** — 基于新证据更新置信度
- **TransitionWorker** — 记录顺序访问模式，维护 PAS 缓存

---

## 2. 认知算法 (internal/cognitive/)

### 2.1 赫布学习 (hebbian.go)

实现"共同激活的神经元会连接在一起"（Hebb's Rule）。

**核心接口：**
```go
type HebbianStore interface {
    UpdateAssocWeight(ctx, ws, src, dst, newWeight) error
    GetAssocWeight(ctx, ws, src, dst) (float32, error)
    DecayAssocWeights(ctx, ws, decayFactor, minWeight, archiveThreshold) (int, error)
    UpdateAssocWeightBatch(ctx, updates []AssocWeightUpdate) error
}
```

**对数空间乘性更新：**
```
// 1. 计算共激活信号强度
signal = Σ(scoreA × scoreB)    // 对结果集中所有配对

// 2. 对数空间更新（避免浮点精度问题）
logNew = ln(current) + effectiveSignal × ln(1 + HebbianLearningRate)
newWeight = min(1.0, exp(logNew))
```

关键常量：
- `HebbianLearningRate` = **0.01**
- `HebbianPassInterval` = 1 分钟（批处理间隔）
- 规范化排序：`canonicalPair = (min(idA, idB), max(idA, idB))`，基于 ULID 字典序

**批处理流程：**
1. 从事件通道收集 `CoActivationEvent`
2. 对每个事件生成所有 engram 配对
3. 按规范化键聚合信号强度
4. 通过 `UpdateAssocWeightBatch` 原子批量更新

### 2.2 艾宾浩斯衰减 (decay.go)

模拟人类记忆的自然遗忘过程。

**衰减公式：**
```
EbbinghausWithFloor(daysSinceAccess, stability, floor):
    decay = exp(-daysSinceAccess / stability)
    return max(decay, floor)
```

关键常量：
- `DefaultFloor` = 0.05（最低相关性底限，防止完全遗忘）
- `DefaultStability` = 14.0 天（默认记忆稳定性）
- 稳定性自适应：`ComputeStability(accessCount, avgDaysBetweenAccesses)` 根据访问模式动态调整

**DecayWorker 调度：**
- 定期扫描检查过期记忆
- 通过 `UpdateRelevance` 更新相关性得分
- 低于阈值的记忆标记为 `Lapsed` 状态

### 2.3 贝叶斯置信度 (confidence.go)

基于新证据动态更新记忆可靠性评估。

**贝叶斯更新公式：**
```
posterior = (prior × evidence) / (prior × evidence + (1 - prior) × (1 - evidence))
```

**Laplace 平滑（防止极值）：**
```
confidence = 0.95 × posterior + 0.025
// 有效范围：[0.025, 0.975]
```

**证据强度常量：**
| 常量 | 值 | 含义 |
|------|-----|------|
| `EvidenceContradiction` | 0.1 | 发现矛盾信息 |
| `EvidenceCoActivation` | 0.65 | 共激活（间接支持） |
| `EvidenceUserConfirmed` | 0.95 | 用户明确确认 |
| `EvidenceUserRejected` | 0.05 | 用户明确拒绝 |

### 2.4 矛盾检测 (contradict.go)

两种矛盾检测模式：
- **结构矛盾**：使用 64×64 布尔矩阵（O(1) 查询），基于 concept 哈希
- **语义矛盾**：需要 EnrichPlugin 进行 LLM 语义分析

### 2.5 预测激活信号 PAS (transition.go)

记录 Engram 的顺序访问模式，用于预测下一个可能被需要的记忆。

**TransitionWorker** 接收 `TransitionEvent`（包含 source → destination 对），通过 `TransitionCacheStore.IncrBy` 累加转移频次。数据存储在热缓存（内存）和冷存储（Pebble 0x1C 前缀）中。

在激活管线的 Phase 4.5 中，系统查询当前上下文的转移目标，将转移频次归一化后注入候选得分。

### 2.6 通用 Worker 框架 (worker.go)

所有认知工作者使用统一的 `Worker[T]` 泛型框架：
- 事件通道缓冲 + 批处理
- 自适应批量大小
- 优雅关闭（drain channel → 处理残余 → 退出）
- 上下文感知取消

---

## 3. 存储层 (internal/storage/)

MuninnDB 使用混合持久化架构：Pebble (LSM-tree) 作为主 KV 存储 + 自定义 ERF 格式 + MOL 预写日志。

### 3.1 ERF 格式 (Engram Record Format)

ERF 是 MuninnDB 的自定义二进制记录格式（`internal/storage/erf/format.go`）。

**整体布局：**
```
┌──────────────────────────────────────┐
│ Header (8 字节)                       │
│   Magic: 0x4D554E4E ("MUNN") [4B]   │
│   Version: 0x01 或 0x02      [1B]   │
│   Flags:                      [1B]   │
│   CRC16 (CCITT-FALSE):       [2B]   │
├──────────────────────────────────────┤
│ 元数据固定区 (100 字节)               │
│   Offset  8: ID             [16B]   │
│   Offset 24: CreatedAt       [8B]   │
│   Offset 32: UpdatedAt       [8B]   │
│   Offset 40: LastAccess      [8B]   │
│   Offset 48: Confidence   [float32] │
│   Offset 52: Relevance    [float32] │
│   Offset 56: Stability    [float32] │
│   Offset 60: AccessCount   [uint32] │
│   Offset 64: State          [uint8] │
│   Offset 65: AssocCount    [uint16] │
│   Offset 67: EmbedDim      [uint8] │
│   Offset 68: MemoryType    [uint8] │
│   Offset 69: Classification [uint16]│
│   Offset 71: Reserved       [29B]  │
├──────────────────────────────────────┤
│ 偏移表 (位置 108, 共 40 字节)         │
│   ConceptOff/Len  [4B + 2B]        │
│   CreatedByOff/Len [4B + 2B]       │
│   ContentOff/Len  [4B + 4B]        │
│   TagsOff/Len     [4B + 4B]        │
│   AssocOff/Len    [4B + 4B]        │
│   EmbedOff/Len    [4B + 4B]        │
├──────────────────────────────────────┤
│ 变量数据区 (从位置 152 开始)           │
│   Concept → CreatedBy → Content →   │
│   Tags → Associations → Embeddings  │
├──────────────────────────────────────┤
│ 尾部 CRC32 Castagnoli (4 字节)       │
└──────────────────────────────────────┘
```

关键常量：
- `FixedOverhead` = 152 字节 (Header 8 + Metadata 100 + OffsetTable 40 + Trailer 4)
- `MaxConceptBytes` = 512
- `MaxContentBytes` = 16KB
- `ContentCompressThreshold` = 512 字节（超过此大小使用 zstd 压缩）
- `AssocRecordSize` = 40 字节（单条关联子记录）

**v2 改进：** 嵌入向量和关联数据支持外联存储（out-of-line），减少主记录体积。

### 3.2 键空间布局

所有 Pebble 键以 1 字节前缀开头，后跟 8 字节的 Vault 前缀（SipHash-2-4 计算）：

| 前缀 | 用途 | 键模式 | 值 |
|------|------|--------|-----|
| `0x01` | 完整 Engram | `ws(8)\|id(16)` | ERF 编码的完整记录 |
| `0x02` | 元数据 | `ws(8)\|id(16)` | ERF 前 664 字节（含 concept） |
| `0x03` | 前向关联 | `ws(8)\|src(16)\|weightComplement(4)\|dst(16)` | 关联类型 + 元数据 |
| `0x04` | 反向关联 | `ws(8)\|dst(16)\|weightComplement(4)\|src(16)` | 同上 |
| `0x05` | FTS 倒排表 | `ws(8)\|term\|0x00\|id(16)` | TF(float32) + Field(1) + DocLen(2) |
| `0x06` | 三元组索引 | `ws(8)\|trigram(3)\|id(16)` | 空值（仅键） |
| `0x07` | HNSW 节点 | `ws(8)\|id(16)\|layer(1)` | 邻居列表 |
| `0x08` | FTS 全局统计 | `ws(8)\|"stats"` | 文档数 + 平均长度 |
| `0x09` | 词项统计 | `ws(8)\|term` | 文档频率 |
| `0x0A` | 矛盾索引 | `ws(8)\|conceptHash(4)\|relType(2)\|id(16)` | 矛盾详情 |
| `0x0B` | 状态索引 | `ws(8)\|state(1)\|id(16)` | 空值 |
| `0x0C` | 标签索引 | `ws(8)\|tagHash(4)\|id(16)` | 空值 |
| `0x0D` | 创建者索引 | `ws(8)\|creatorHash(4)\|id(16)` | 空值 |
| `0x0E` | Vault 元数据 | `ws(8)` | Vault 名称字符串 |
| `0x0F` | Vault 名称索引 | `siphash(name)(8)` | 实际 ws 前缀 |
| `0x10` | 相关性桶 | `ws(8)\|bucket(1)\|id(16)` | 空值（按相关性降序） |
| `0x14` | 关联权重 | `ws(8)\|min(a,b)(16)\|max(a,b)(16)` | 权重 float32 |
| `0x18` | 嵌入向量 | `ws(8)\|id(16)` | float32 数组 |
| `0x19` | 复制日志 | `seq(8)` | ReplicationEntry (msgpack) |
| `0x1C` | 转移记录 | `ws(8)\|src(16)\|dst(16)` | 转移频次 |
| `0x1E` | 序数索引 | `ws(8)\|parent(16)\|ordinal(4)` | 子 Engram ID |
| `0x1F` | 实体记录 | `ws(8)\|entity` | 实体元数据 |
| `0x20` | 实体→Engram | `ws(8)\|entity\|id(16)` | 链接元数据 |
| `0x21` | 关系记录 | `ws(8)\|rel` | 关系元数据 |
| `0x22` | 最后访问 | `ws(8)\|id(16)` | Unix 时间戳 |
| `0x23` | Engram→实体 | `ws(8)\|id(16)\|entity` | 反向链接 |
| `0x24` | 共现计数 | `ws(8)\|e1\|e2` | 共现次数 |
| `0x25` | 归档关联 | `ws(8)\|src(16)\|dst(16)` | 归档的关联权重 |

**Vault 前缀计算：**
```go
VaultPrefix(name) = SipHash-2-4(name, key0=0x736f6d6570736575, key1=0x646f72616e646f6d)
// 产生 8 字节确定性前缀
```

**权重补数（WeightComplement）：** 用于在前向/反向关联键中实现权重降序排列。`wc = float32ToBytes(1.0 - weight)`，使得 Pebble 的升序扫描自然返回高权重优先的结果。

### 3.3 WAL — MOL (Muninn Operation Log)

MOL 是 MuninnDB 的预写日志（与 Pebble 自身的 WAL 独立），用于复制流和审计。

**条目格式（`internal/wal/mol.go`）：**
```
┌─────────────────────────────────────┐
│ 条目头 (32 字节)                      │
│   Magic: 0x4D4F4C20 ("MOL ")  [4B] │
│   SeqNum:                      [8B] │
│   Timestamp: (Unix nanoseconds)[8B] │
│   OpType:                      [2B] │
│   VaultID:                     [4B] │
│   PayloadLen:                  [4B] │
│   Flags:                       [1B] │
│   Reserved:                    [1B] │
├─────────────────────────────────────┤
│ Payload (msgpack 编码)               │
├─────────────────────────────────────┤
│ CRC32 Castagnoli                [4B]│
└─────────────────────────────────────┘
```

**操作类型 (OpType)：**
| 操作码 | 名称 | 含义 |
|--------|------|------|
| 0x0001 | OpEngramWrite | 写入新 Engram |
| 0x0002 | OpEngramUpdate | 更新 Engram |
| 0x0003 | OpEngramForget | 遗忘 Engram |
| 0x0004 | OpEngramPurge | 永久清除 |
| 0x0005 | OpAssocLink | 建立关联 |
| 0x0006 | OpAssocUnlink | 删除关联 |
| 0x0007 | OpHebbianBatch | 赫布权重批量更新 |
| 0x0008 | OpDecayBatch | 衰减批量更新 |
| 0x0009 | OpVaultCreate | 创建 Vault |
| 0x000A | OpVaultUpdate | 更新 Vault 配置 |
| 0x00FF | OpCheckpoint | 检查点标记 |

**Flags：**
- `FlagCompressed` (bit 0) — Payload 使用 zstd 压缩
- `FlagLargeBatch` (bit 1) — 大批量写入
- `FlagCheckpoint` (bit 2) — 检查点条目

**GroupCommitter：**
- 批量追加多个条目 → 单次 `fsync` 提交
- `DefaultMaxWait` = 2ms（最大等待时间）
- `DefaultMaxGroupSize` = 1000（最大批量大小）
- `DefaultMaxSegmentSize` = 256MB（段文件轮转阈值）

### 3.4 持久性模型

MuninnDB 支持两种持久性模式：

**Sync 模式（默认）：** 每次关键写入使用 `pebble.Sync` 立即 fsync。

**NoSync 模式：** 使用 `pebble.NoSync` 写入 + `walSyncer` 组 fsync。
- `walSyncer` 每 10ms 执行一次 `fsync`
- 最多丢失 10ms 的写入数据
- 性能显著提升，适合对极端持久性要求不高的场景

**崩溃恢复：** 依赖 Pebble WAL 保证数据一致性；MOL 用于复制流恢复（密封段按序号读取 + CRC 验证）。

---

## 4. 索引层 (internal/index/)

### 4.1 HNSW 向量索引 (hnsw/)

基于 Hierarchical Navigable Small Worlds 算法的近似最近邻搜索。

**核心参数：**
| 参数 | 值 | 说明 |
|------|-----|------|
| M | 16 | 各层最大连接数 |
| M0 | 32 | 第 0 层最大连接数 |
| EfConstruction | 200 | 构建时探索深度 |
| EfSearch | 50 | 查询时探索深度 |

**距离度量：** 余弦相似度（Cosine Similarity）

**持久化键：** `0x07 | ws(8) | id(16) | layer(1)` — 向量槽使用 `layer = 0xFF`

**内存管理：**
- `MUNINN_HNSW_WARN_THRESHOLD_MB` — 内存警告阈值
- `MUNINN_HNSW_MAX_MB` — 内存硬限制
- `HNSWRegistry` 管理每个 Vault 的 HNSW 实例

### 4.2 FTS 全文检索 (fts/)

基于 BM25 算法的全文搜索引擎。

**BM25 公式：**
```
score(D, Q) = Σ IDF(t) × [tf(t,D) × (k1+1)] / [tf(t,D) + k1 × (1 - b + b × dl/avgdl)]
```

**参数配置：**
- `k1 = 1.2`（词频饱和参数）
- `b = 0.75`（文档长度归一化参数）

**字段权重：**
| 字段 | 权重 | 说明 |
|------|------|------|
| Concept | 3.0 | 概念/标题（最高权重） |
| Tags | 2.0 | 标签 |
| Content | 1.0 | 内容正文 |
| CreatedBy | 0.5 | 创建者 |

**索引结构：**
- 倒排表键：`0x05 | ws(8) | term | 0x00 | id(16)` — 值为 7 字节 (TF float32 + Field uint8 + DocLen uint16)
- 三元组回退：`0x06 | ws(8) | trigram(3) | id(16)` — 用于模糊匹配
- 词干提取：Porter2 Stemmer
- 全局统计：`0x08` 前缀存储文档总数和平均长度
- 词项统计：`0x09` 前缀存储每个词项的文档频率

### 4.3 关联图谱 (adjacency/)

基于邻接表的图结构，支持 BFS 遍历式检索。

**键设计（权重补数降序排列）：**
- 前向：`0x03 | ws | src | weightComplement | dst`
- 反向：`0x04 | ws | dst | weightComplement | src`

权重补数确保 Pebble 升序扫描时高权重边优先返回。

**BFS 遍历参数：**
- `hopPenalty` = 0.7 — 每跳得分衰减 30%
- `maxBFSNodes` = 500 — 最大遍历节点数
- `maxEdgesPerNode` = 20 — 每节点最大扩展边数

**15 种关联类型：**
supports, contradicts, depends_on, elaborates, generalizes, specializes, causes, caused_by, related_to, part_of, contains, precedes, follows, co_occurs, similar_to

---

## 5. 认证与保管库 (internal/auth/)

### 5.1 两层权限模型

**第一层 — Vault 隔离：**
- 每个 Engram 归属一个 Vault（命名空间）
- Vault 之间完全隔离
- 解析优先级：`?vault` 查询参数 → 请求体 `vault` 字段 → 默认 `"default"`
- 失败关闭：未配置的 Vault 默认 `Public = false`

**第二层 — API 密钥：**
- 格式：`mk_` 前缀 + 随机熵
- 存储：`sha256(raw)[:16]` 哈希存储
- 模式：
  - `ModeFull` — 完全读写权限
  - `ModeObserve` — 只读（仅激活和读取）
  - `ModeWrite` — 可写入但受限管理操作

### 5.2 管理会话

- HMAC 签名的 cookie (`muninn_session`)
- 会话 TTL = 24 小时
- HTTP-only、Secure（TLS 时）

### 5.3 Plasticity 预设

控制 Vault 中记忆的可塑性行为：
- **default** — 标准认知处理
- **reference** — 参考资料模式，降低衰减速率
- **scratchpad** — 临时记忆，加速衰减
- **knowledge-graph** — 知识图谱模式，强化关联

---

## 6. 插件系统 (internal/plugin/)

### 6.1 核心接口

```go
// EmbedPlugin 提供文本嵌入向量化能力
type EmbedPlugin interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimension() int
    MaxBatchSize() int
}

// EnrichPlugin 提供 LLM 增强能力
type EnrichPlugin interface {
    Enrich(ctx context.Context, engram *Engram) (*EnrichmentResult, error)
}
```

### 6.2 嵌入提供者 (8 个)

| 提供者 | 配置 | 维度 | 说明 |
|--------|------|------|------|
| local (ONNX) | 内置，需 `-tags localassets` | 384 | 零外部依赖 |
| Ollama | `MUNINN_OLLAMA_URL` | 可变 | 本地部署 |
| OpenAI | `MUNINN_OPENAI_KEY` | 1536 | text-embedding-3-small |
| Voyage | `MUNINN_VOYAGE_KEY` | 1024 | voyage-3 |
| Cohere | `MUNINN_COHERE_KEY` | 1024 | embed-multilingual |
| Google | `MUNINN_GOOGLE_KEY` | 768 | text-embedding-004 |
| Jina | `MUNINN_JINA_KEY` | 1024 | jina-embeddings-v3 |
| Mistral | `MUNINN_MISTRAL_KEY` | 1024 | mistral-embed |

### 6.3 富化提供者 (4 个)

通过 `MUNINN_ENRICH_URL` 配置：
- OpenAI (`openai://model`)
- Anthropic (`anthropic://model`)
- Google (`google://model`)
- Ollama (`ollama://model`)

**富化管线（EnrichmentPipeline）：**
1. 实体抽取（Entity Extraction）
2. 关系抽取（Relationship Extraction）
3. 分类（Classification）
4. 摘要生成（Summarization）

### 6.4 注册表与热加载

- `PluginRegistry` 管理活跃插件，每类最多一个
- 支持运行时 Register/Unregister
- `HardwareAwarePlugin` 接口用于检测硬件加速

### 6.5 追溯处理器 (RetroactiveProcessor)

当新的嵌入/富化插件安装后，后台处理器自动对历史 Engram 进行重处理：
- 微批量扫描（micro-batch）
- `DigestFlags` 标记处理状态（DigestEmbed、DigestEnrich、DigestEmbedFailed 等）
- Engine.SetOnWrite 回调链通知处理器
- Circuit Breaker 保护外部 LLM 调用

---

## 7. 传输协议 (internal/transport/)

### 7.1 REST (internal/transport/rest/)

70+ HTTP 端点，端口 8475。

**中间件链：**
```
Recovery → RequestID → Logging → BodySize → Auth → RateLimit → Handler
```

**请求体大小限制：**
- 公开端点：64KB
- 认证端点：4MB
- 大型操作（导入）：512MB

**核心端点：**
- `POST /api/engrams` — 写入记忆
- `POST /api/activate` — 激活召回
- `GET /api/engrams/:id` — 读取单条
- `DELETE /api/engrams/:id` — 遗忘
- `POST /api/engrams/:id/link` — 建立关联
- `GET /api/subscribe` — SSE 订阅
- `GET /api/health` — 健康检查
- `GET /api/ready` — 就绪检查

### 7.2 gRPC (internal/transport/grpc/)

9 个 RPC 方法，端口 8477：
- `Hello` — 握手
- `Write` — 写入
- `BatchWrite` — 批量写入
- `Read` — 读取
- `Forget` — 遗忘
- `Stat` — 统计
- `Link` — 关联
- `Activate` (服务端流) — 流式激活结果
- `Subscribe` (双向流) — 实时订阅

### 7.3 MBP (internal/transport/mbp/)

MuninnDB 二进制协议，端口 8474。

**帧格式（16 字节头）：**
```
┌────────────────────────────────────┐
│ Version        [1B]               │
│ Type           [1B]               │
│ Flags          [2B]               │
│ PayloadLen     [4B]               │
│ CorrelationID  [8B]               │
├────────────────────────────────────┤
│ Payload (msgpack 编码)             │
└────────────────────────────────────┘
```

支持管线化（pipelining）：客户端可在收到响应前发送多个请求，通过 CorrelationID 匹配。

### 7.4 MCP (internal/mcp/)

Model Context Protocol，端口 8750，基于 JSON-RPC 2.0 + SSE。

提供 35 个 AI 工具，用于智能体集成：
- `muninn_remember` — 存储记忆
- `muninn_recall` — 激活召回
- `muninn_read` — 读取记忆
- `muninn_forget` — 遗忘
- `muninn_link` — 建立关联
- `muninn_traverse` — 图遍历
- `muninn_explain` — 关联解释
- `muninn_evolve` — 演化记忆
- `muninn_consolidate` — 合并记忆
- `muninn_decide` — 基于记忆决策
- `muninn_guide` — 引导回忆
- 等等...

认证：Bearer Token（通过 `MUNINN_MCP_TOKEN` 或 `~/.muninn/mcp.token` 配置）

---

## 8. 集群与复制 (internal/replication/)

### 8.1 角色定义

| 角色 | 说明 | 接受写入 | 投票 | 认知工作者 |
|------|------|----------|------|-----------|
| **Cortex** (主) | 处理所有写入，管理 WAL 流 | ✓ | ✓ | ✓ |
| **Lobe** (从) | 接收复制流，提供读扩展 | ✗ | ✓ | ✗ |
| **Sentinel** | 仅参与投票，不存储数据 | ✗ | ✓ | ✗ |
| **Observer** | 接收复制流，不参与投票 | ✗ | ✗ | ✗ |

### 8.2 选举机制（Epoch-based，非 Raft）

MuninnDB 使用基于 **Epoch** 的投票选举（不是 Raft 算法）：

1. 候选人通过 `EpochStore.CompareAndSet` 原子递增 Epoch
2. 候选人给自己投票，广播 `VoteRequest`
3. 节点对相同或更高 Epoch 的请求授予投票（每个 Epoch 最多投一票）
4. 获得法定人数（`len(voters)/2 + 1`）的候选人通过 `tryPromote` 提升
5. 新 Cortex 广播 `CortexClaim`，其他节点更新状态

**安全保证：**
- Epoch 持久化存储，防止并发主节点
- 围栏令牌（Fencing Token）= Epoch 值
- `ValidateFencingToken` 拒绝来自过期主节点的写入

### 8.3 WAL 流式复制

```
Cortex 写入 → ReplicationLog.Append (0x19 前缀)
    → Subscribe 通知
    → NetworkStreamer.Stream() → 发送 ReplEntry 帧
    → Lobe 接收 → Applier.Apply (幂等，跳过 seq ≤ lastApplied)
    → Lobe 发送 ReplAck
    → Cortex 更新 ReplicaSeq
```

**初始同步：**
新 Lobe 通过 `JoinClient` 加入集群：
1. 发送 JoinRequest（含 nodeID、地址、lastApplied、HMAC 签名）
2. Cortex 验证集群密钥和协议版本
3. 如需快照：Cortex 流式传输当前状态
4. Lobe 从 snapshotSeq 开始追赶

### 8.4 优雅切换 (GracefulFailover)

1. 进入 DRAINING（拒绝新写入）
2. 刷新认知工作者
3. 等待所有 Lobe 确认当前 cortexSeq（`waitForConvergence`）
4. 发送 HANDOFF 帧到目标节点（携带 Epoch 和 CortexSeq）
5. 等待 HANDOFF_ACK
6. 降级自身（`handleDemotion`）

### 8.5 一致性模式

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| **Eventual** | 异步复制，主节点立即返回 | 高吞吐、可接受短暂不一致 |
| **Strong** | 等待所有已知副本确认 | 强一致性要求 |
| **BoundedStaleness** | 等待副本追赶到配置的延迟范围内 | 平衡一致性与性能 |

### 8.6 分区后协调

`Reconciler` 运行 probe → reply → sync → ack 协议（`reconcile.go`），在网络分区恢复后同步认知权重（赫布状态）。

---

## 附录：常量速查表

| 常量 | 值 | 来源 |
|------|-----|------|
| ERF Magic | `0x4D554E4E` | erf/format.go |
| MOL Magic | `0x4D4F4C20` | wal/mol.go |
| HebbianLearningRate | 0.01 | cognitive/hebbian.go |
| ACTRDecay | 0.5 | engine/activation/ |
| ACTRHebScale | 4.0 | engine/activation/ |
| DefaultFloor | 0.05 | cognitive/decay.go |
| DefaultStability | 14.0 天 | cognitive/decay.go |
| RRF k_FTS | 60 | engine/activation/ |
| RRF k_HNSW | 40 | engine/activation/ |
| RRF k_Temporal | 120 | engine/activation/ |
| BFS hopPenalty | 0.7 | index/adjacency/ |
| BFS maxNodes | 500 | index/adjacency/ |
| HNSW M | 16 | index/hnsw/ |
| HNSW M0 | 32 | index/hnsw/ |
| HNSW EfConstruction | 200 | index/hnsw/ |
| HNSW EfSearch | 50 | index/hnsw/ |
| BM25 k1 | 1.2 | index/fts/ |
| BM25 b | 0.75 | index/fts/ |
| PAS K (注入排名) | 50 | engine/activation/ |
| GroupCommitter MaxWait | 2ms | wal/mol.go |
| GroupCommitter MaxGroup | 1000 | wal/mol.go |
| Segment MaxSize | 256MB | wal/mol.go |

---

*基于 MuninnDB 源代码生成。最后更新：2026-03-24*
