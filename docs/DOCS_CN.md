# MuninnDB 中文文档 (DOCS_CN.md)

MuninnDB 是一种认知记忆数据库，实现了时间学习、赫布关联（Hebbian association）和多模态检索（向量 + 全文 + 图谱）。它支持 REST、gRPC、MCP（模型上下文协议）和自定义 MBP 二进制协议。

---

## 文档导航（阅读指南）

本指南按使用意图组织，帮助您快速找到所需信息。

### 如果您想了解 MuninnDB 是什么
1. **[记忆的工作原理](#记忆的工作原理)** — 解释什么是记忆痕迹（engram）以及记忆如何存储、评分和检索。
2. **[与其他数据库的比较](#与其他数据库的比较)** — 将 MuninnDB 与向量数据库、图数据库和键值存储进行对比。

### 如果您想了解记忆的结构
1. **[记忆痕迹 (Engram)](#记忆痕迹engram)** — 核心数据结构：字段、生命周期状态和键空间布局。
2. **[认知原语](#认知原语数学详解)** — 赫布学习、时间衰减、激活扩散和贝叶斯置信度。

### 如果您想了解检索的工作原理
1. **[检索设计](#检索设计6-阶段激活管线)** — 6 阶段 ACTIVATE 管线：如何处理回想查询。
2. **[系统架构](#系统架构)** — 系统组件、数据流和 Pebble 存储层。

### 如果您想使用 MuninnDB
1. **[快速入门](#功能参考)** — 让 MuninnDB 运行起来并进行首次记忆写入。
2. **[功能参考](#功能参考)** — 35 个 MCP 工具及其参数的完整参考。

---

## 记忆的工作原理

MuninnDB 的核心理念是**模拟人类大脑的记忆机制**，而不仅仅是持久化存储数据。

### 艾宾浩斯遗忘曲线与间隔效应
记忆不是永恒的。MuninnDB 实现了**时间衰减（Temporal Decay）**。如果不被访问，记忆的“激活度”会随时间下降。相反，频繁访问或在关键时刻访问会加强记忆。

### 赫布学习："一起激活的神经元会连接在一起"
MuninnDB 遵循**赫布定律 (Hebbian Theory)**。当两个概念（记忆痕迹）在短时间内被同时激活时，它们之间的关联度会增加。这意味着数据库会自动学习事物之间的隐性联系，而无需人工定义模式（Schema）。

### 贝叶斯推理与置信度更新
每条记忆都有一个**置信度（Confidence）**得分。新的证据可以强化或削弱现有的记忆。MuninnDB 使用贝叶斯公式根据证据强度（Evidence Strength）持续更新这一得分。

### ACT-R 认知架构与基线激活
我们采用 **ACT-R (Adaptive Control of Thought—Rational)** 模型来计算记忆的基线激活度（Base-level Activation）。这结合了访问频率和自上次访问以来的时间。

### 为什么 MuninnDB 在查询时计算激活（total-recall 设计）
传统的 RAG 系统仅执行静态向量搜索。MuninnDB 在查询时运行一个动态引擎，实时计算哪些记忆在当前上下文中最具“相关性”，结合了语义、时间、关联和置信度。

### 语义触发器的推送模型
不同于被动等待查询的数据库，MuninnDB 支持**语义触发器（Semantic Triggers）**。当新写入的记忆与某个已订阅的上下文高度相关时，数据库会主动将该记忆推送给订阅者。

---

## 认知原语（数学详解）

MuninnDB 的核心引擎基于以下数学公式：

### 1. ACT-R 时间衰减 (Temporal Decay)
用于计算记忆痕迹 $M$ 的基线激活度 $B(M)$：

$$B(M) = \ln(n+1) - 0.5 \times \ln(ageDays/(n+1))$$

其中 $n$ 是访问次数，$ageDays$ 是记忆存在的天数。为了获得非负激活度，我们应用 **softplus** 函数：
$$softplus(x) = \ln(1 + e^x)$$

### 2. 赫布学习 (Hebbian Learning)
当两个记忆痕迹共同激活时，其权重 $w$ 的更新公式（学习率 $\eta=0.01$）：

$$w_{new} = \min(1.0, w_{old} \times (1+\eta)^n)$$

### 3. 贝叶斯置信度 (Bayesian Confidence)
根据证据强度 $s$ 更新后验概率（Posterior）：

$$posterior = \frac{p \times s}{p \times s + (1-p) \times (1-s)}$$

并应用 **Laplace 平滑**：
$$confidence = 0.95 \times posterior + 0.025$$
这将置信度限制在 $[0.025, 0.975]$ 区间内。

### 4. 预测激活信号 (PAS - Predictive Activation Signal)
通过记录记忆激活的顺序序列，MuninnDB 学习预测下一个可能需要的记忆。在检索阶段，PAS 会注入候选记忆，排名常数 $K=50$。

### 5. 矛盾检测 (Contradiction Detection)
系统支持三种模式：
- **结构化检测**：基于 64×64 的布尔矩阵，检查不兼容的关系类型（O(1) 复杂度）。
- **概念簇检测**：基于 FTS（全文检索）重叠。
- **语义检测**：通过 Enrich 插件调用大语言模型（LLM）进行分析。

---

## 系统架构

MuninnDB 旨在提供极高性能和确定性的持久化。

### 写入路径契约 (Write Path)
- **响应延迟**：ACK 延迟通常 <10ms。
- **持久化**：使用 **ERF (Engram Record Format)** 编码。
- **NoSync 模式**：可选 `NoSyncEngrams` 模式，配合 `walSyncer` 每 10ms 进行一次组提交（Group-commit）。

### 6 阶段 ACTIVATE 管线
1. **输入处理**：向量化和分词。
2. **并行检索**：同时运行 FTS、HNSW 向量检索、时间池检索和 PAS 注入。
3. **结果融合**：使用 **RRF (Reciprocal Rank Fusion)** 融合多路结果。
   - $k$ 常数：FTS $k=60$, HNSW $k=40$, 时间池 $k=120$。
4. **赫布增强**：根据关联强度提升得分。
5. **图谱遍历**：通过 BFS（广度优先搜索）探索实体和记忆关系。
6. **最终评分与过滤**：应用加权得分并返回。

### 综合得分权重 (Composite Score Weights)
- 语义 (Semantic): 0.35
- 全文检索 (FTS): 0.25
- 时间 (Temporal): 0.20
- 赫布 (Hebbian): 0.10
- 访问频率 (Access): 0.05
- 最近性 (Recency): 0.05
- **最终得分** = 综合得分 $\times$ 贝叶斯置信度。

### 认知工作者 (Cognitive Workers)
系统包含多个异步、非阻塞的工作者线程：
- `HebbianWorker`：负责更新关联权重。
- `ConfidenceWorker`：负责贝叶斯得分更新。
- `TransitionWorker`：负责 PAS 信号的学习。
- `ContradictionWorker`：负责检测新旧记忆间的冲突。

---

## 检索设计（6 阶段激活管线）

ACTIVATE API 是 MuninnDB 的核心入口。它不仅仅是“搜索”，而是“回想”。

- **阶段 1：向量化与分词**：调用嵌入插件（Embed Plugin）将输入转换为向量。
- **阶段 2：并行检索**：
  - 全文索引（Pebble + Trigram）。
  - 向量索引（HNSW）。
  - 时间池（根据 ACT-R 得分最高的 120 个项）。
  - PAS 预测注入（前 50 个预测项）。
- **阶段 3：RRF 融合**：消除不同指标间的量纲差异，生成初步排名。
- **阶段 4：关联增强**：如果查询激活了 A，而 A 与 B 有强赫布关联，则 B 的得分会得到增强。
- **阶段 5：图谱探索**：从当前激活的节点出发，进行 BFS 遍历，最大节点数 500。
- **阶段 6：流式输出**：支持通过 SSE（Server-Sent Events）或 gRPC 流式返回结果。

---

## 记忆痕迹（Engram）

**Engram** 是 MuninnDB 存储的基本单元。

### 数据结构
- **ID**: ULID (16 字节，按时间排序)。
- **Concept**: 核心概念（最大 512 字节）。
- **Content**: 详细内容（最大 16KB，超过 512 字节使用 zstd 压缩）。
- **Confidence**: 0.0 - 1.0。
- **Associations**: 与其他 Engram 的关联，每个 Engram 最多支持 256 个关联。
- **State**: 生命周期状态（PLANNING, ACTIVE, PAUSED, BLOCKED, COMPLETED, CANCELLED, ARCHIVED, SOFT_DELETED）。

### ERF 布局 (Engram Record Format)
ERF 是一种固定偏移量的二进制格式，优化了小规模元数据更新和大规模内容读取。
- **Header**: 8 字节 (Magic + Version)。
- **Metadata Fixed Block**: 100 字节，包含分数、计数和状态。
- **Offset Table**: 40 字节。
- **Variable Data**: 实际的内容、标签和关联。

---

## 实体图谱

MuninnDB 提取并管理一个两层实体图谱：
1. **全局注册表**：跨保管库（Vault）共享的实体信息。
2. **保管库作用域链接**：将实体连接到具体的记忆痕迹。

### 关系类型 (Relationship Types)
系统预定义了 15 种类型（如 `IS_A`, `PART_OF`, `WORKS_FOR`, `LOCATED_AT` 等），用户也可以定义自定义类型（0x8000+）。

### 存储前缀
- `0x1F`: 全局实体记录。
- `0x20`: 保管库前向索引 (Engram -> Entity)。
- `0x23`: 跨保管库反向索引 (Entity -> Engram)。
- `0x24`: 实体共现矩阵 (Co-occurrence)。

---

## 语义触发器

语义触发器是 MuninnDB 实现“主动回想”的关键。

### 触发事件类型
- `new_write`: 当新写入的记忆与订阅上下文匹配时触发。
- `threshold_crossed`: 当某个记忆的认知得分超过设定阈值时触发。
- `contradiction_detected`: 发现逻辑矛盾时的高优先级触发（绕过速率限制）。

### 架构
`TriggerWorker` 负责扫描订阅。为了性能，它使用非阻塞异步通道，并针对每个订阅者配备令牌桶（Token Bucket）进行速率限制。

---

## 认证与保管库

MuninnDB 采用双层认证模型。

### 两层模型
1. **管理员凭证**：用于系统配置和 Web UI 管理。
2. **保管库 API 密钥 (mk_...)**：用于数据操作。密钥以 SHA-256 哈希形式存储。

### 密钥模式
- **Full Mode**: 允许读写及认知状态变更（如更新访问计数）。
- **Observe Mode**: 仅允许读取，不产生任何认知副作用。

### 保管库塑性配置 (Vault Plasticity)
每个保管库可以配置其“认知特性”，例如：
- `hebbian_enabled`: 是否开启赫布学习。
- `temporal_halflife`: 时间衰减半衰期（天）。
- `relevance_floor`: 最低相关性阈值。

---

## 插件系统

系统分为三个层级：

1. **Tier 1 (Core)**: 基础认知引擎，无需配置。
2. **Tier 2 (Embed)**: 嵌入模型插件。支持本地捆绑模型 (`all-MiniLM-L6-v2`)、OpenAI、Ollama、Voyage 等。
3. **Tier 3 (Enrich)**: 强化插件。支持 LLM 摘要、实体提取、追溯性强化（Retroactive Enrichment）。

**追溯性强化**：当你添加新的嵌入模型或 LLM 插件时，系统会在后台自动升级已有的记忆。

---

## 层次化记忆

用于管理大纲、计划和任务层次结构。

### 实现方式
- 节点通过 `is_part_of` 关联连接。
- 兄弟节点顺序存储在 `0x1E` 前缀下的 Ordinal 索引中。
- **限制**：最大树深度为 20。

---

## 集群操作

MuninnDB 使用 **Cortex/Lobe** 集群模型。

- **Cortex**: 主节点（Primary），负责所有写操作。
- **Lobe**: 从节点（Replica），负责读扩展和故障切换。
- **MOL (Muninn Operation Log)**: 用于流式复制写操作日志。
- **纪元 (Epoch)**: 每次主节点切换时纪元递增，防止脑裂（Split-brain）。

---

## 持久性保证

MuninnDB 提供两层持久化：

### 1. 同步操作 (Sync)
- 写入 Engram、评分存储、认证信息、保管库元数据。
- 保证零数据丢失。

### 2. NoSync + WAL Syncer (10ms)
- 关联权重、元数据计数器、FTS 更新、实体图谱写入。
- 即使发生系统崩溃，最多仅丢失最近 10ms 的此类非关键更新。

---

## 键空间模式

所有数据存储在 Pebble 键值引擎中，使用 1 字节前缀区分：

| 前缀 | 名称 | 作用 |
|------|------|------|
| `0x01` | Engram | 记忆痕迹内容 |
| `0x02` | Metadata | ACT-R 分数和元数据 |
| `0x03` | Assoc Forward | 赫布关联（权重排序） |
| `0x05` | FTS Posting | 全文检索倒排列表 |
| `0x07` | HNSW Neighbors| 向量索引邻居列表 |
| `0x0A` | Contradiction | 矛盾索引 |
| `0x1E` | Ordinal | 层次化记忆顺序 |
| `0x1F` | Entity Registry| 全局实体注册表 |

---

## 能力声明

- **认知特性**：内置 Ebbinghaus 衰减、Hebbian 关联、PAS 预测。
- **高性能写入**：基于 Pebble LSM-tree 优化，单机支持高并发写入。
- **AI 友好**：原生支持 MCP 协议，可直接连接 Claude, Cursor, Windsurf 等工具。
- **数据溯源**：每条记忆均包含来源（Provenance）追踪。
- **并不是什么**：MuninnDB 不是通用关系数据库，不适合进行复杂的 SQL 联表查询；它也不是横向切分的数据库（单保管库为单脑模型）。

---

## 与其他数据库的比较

| 数据库类型 | MuninnDB 优势 | 为什么不能只用传统 DB？ |
|------------|---------------|---------------------------|
| **关系型 (SQL)** | 自动计算相关性和衰减 | SQL 缺乏时间的连续评分模型。 |
| **文档型 (NoSQL)** | 动态关联与激活 | 文档数据库是静态存储，不会自动“连接”概念。 |
| **键值对 (KV)** | 复杂的检索管线 | KV 仅支持精确匹配，不支持认知回想。 |
| **图数据库** | 动态权重与自动演化 | 图数据库的边通常是静态定义的。 |
| **向量数据库** | 结合时间与逻辑置信度 | 向量相似度不等于“相关性”（Relevance）。 |

---

## 功能参考

### 核心 MCP 工具 (部分展示)
- `muninn_remember`: 存储新记忆。
- `muninn_recall`: 根据关键词或向量查找。
- `muninn_activate`: 认知回想（推荐）。
- `muninn_guide`: 针对 AI 代理的自动引导工具。
- `muninn_batch_insert`: 批量导入（最高 50 条）。
- `muninn_subscribe`: 订阅语义触发器。
- `muninn_find_by_entity`: 通过实体查找关联记忆。

### 默认配置
- **数据目录**: `~/.muninn/data`
- **默认端口**: 8474 (MBP), 8475 (REST), 8476 (Web UI), 8750 (MCP)
- **内存限制**: `MUNINN_MEM_LIMIT_GB=4`

---

*MuninnDB 以前瞻性的认知算法重塑了我们与信息的关系。*
*基于 BSL 1.1 协议授权。*
