# MuninnDB MCP (Model Context Protocol) 开发指南

Model Context Protocol (MCP) 是 MuninnDB 实现 AI 代理（Agent）集成的核心协议。`codedb/mcp` 包提供了一套 Transport 无关的 Handler 函数，封装了 MuninnDB 认知引擎的所有能力，使 AI 代理能够通过标准的 JSON-RPC 2.0 接口实现记忆的存储、检索、图遍历、实体管理等复杂操作。

## 概述

MCP 是由 Anthropic 发起的开放标准，旨在为 AI 代理提供统一的上下文获取接口。MuninnDB 通过 MCP 实现了以下核心目标：
- **原子化记忆**：将复杂的上下文拆分为细粒度的 engrams。
- **语义召回**：基于上下文向量和 FTS 的混合检索。
- **关联学习**：通过 Hebbian 原理自动构建知识图谱。
- **时空权重**：根据艾宾浩斯遗忘曲线自动计算信息的重要性。

## 架构设计

MuninnDB 的 MCP 实现采用分层架构，确保逻辑与传输层解耦：

```text
+-----------------------+
|  AI Agents (Claude)   |
+-----------+-----------+
            | (JSON-RPC 2.0 via SSE/Stdio)
+-----------v-----------+
|  Transport (HTTP/gRPC)|
+-----------+-----------+
            | (calls)
+-----------v-----------+
| codedb/mcp Handlers   | <--- 36 个原子化处理函数
+-----------+-----------+
            | (implements EngineInterface)
+-----------v-----------+
| engine (Cognitive)    | <--- 认知引擎 (Hebbian, Decay, ACT-R)
+-----------+-----------+
            |
+-----------v-----------+
| storage (Pebble/ERF)  | <--- 持久化层
+-----------------------+
```

## 核心原理

### 1. Transport 无关设计
`HandleXXXX` 函数签名统一为：
```go
func HandleXXXX(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error)
```
这使得同一套逻辑可以无缝运行在 HTTP/SSE、Stdio 或自定义的 gRPC 传输层之上。

### 2. JSON-RPC 2.0 请求/响应模型
所有操作均遵循标准 JSON-RPC。
- **请求**：包含 `method` (如 `muninn_remember`) 和 `params`。
- **响应**：包含 `result` 对象或 `error`。

### 3. 工具分类
工具分为**只读操作**（查询、分析）和**可变操作**（写入、删除、状态变更）。

### 4. Vault 隔离
每个调用必须显式或隐式通过 `vault` 参数隔离数据空间。默认使用 `default` vault。

---

## 36 个 Handler 函数详解

### A. 记忆 CRUD (9 个)

#### 1. muninn_remember
- **功能**: 存储单条记忆。
- **参数**:
  - `content` (string, 必填): 记忆内容。
  - `concept` (string, 可选): 简短标题。
  - `tags` (string[], 可选): 标签列表。
  - `confidence` (float, 可选): 初始置信度 (0-1)。
  - `type` (string, 可选): 记忆类型 (fact, goal, event 等)。
  - `created_at` (ISO8601, 可选): 创建时间。
  - `op_id` (string, 可选): 幂等 ID。
- **返回值**: `WriteResult` 对象。

#### 2. muninn_remember_batch
- **功能**: 批量存储多条记忆（最多 50 条）。
- **参数**: `memories` (WriteRequest 数组)。
- **返回值**: 包含各条执行结果的列表。

#### 3. muninn_read
- **功能**: 精确读取特定记忆。
- **参数**: `id` (string, 必填)。
- **返回值**: `Memory` 对象。

#### 4. muninn_forget
- **功能**: 软删除记忆。
- **参数**: `id` (string, 必填)。
- **返回值**: `{"ok": true}`。

#### 5. muninn_evolve
- **功能**: 更新记忆内容并记录演化原因。
- **参数**: `id`, `new_content`, `reason`。
- **返回值**: `{"id": "...", "updated": true}`。

#### 6. muninn_consolidate
- **功能**: 合并多条相关记忆。
- **参数**: `ids` (string[]), `merged_content` (string)。
- **返回值**: 新记忆的 ID。

#### 7. muninn_restore
- **功能**: 恢复已删除的记忆。
- **参数**: `id` (string)。
- **返回值**: `{"id": "...", "restored": true}`。

#### 8. muninn_retry_enrich
- **功能**: 重新触发异步富化（如嵌入提取或总结）。
- **参数**: `id` (string)。
- **返回值**: `{"queued": true}`。

#### 9. muninn_state
- **功能**: 变更记忆生命周期状态。
- **参数**: `id`, `state` (planning, active, archived 等), `reason`。
- **返回值**: `{"updated": true}`。

### B. 搜索与召回 (4 个)

#### 10. muninn_recall
- **功能**: 基于语义上下文进行深度召回。
- **参数**:
  - `context` (string/string[]): 搜索上下文。
  - `threshold` (float, 0.5): 召回阈值。
  - `mode` (string): 预设模式 (semantic, recent, balanced, deep)。
  - `since/before`: 时间范围过滤。
- **返回值**: `Memory` 数组。

#### 11. muninn_traverse
- **功能**: 从一个起始点开始进行图遍历。
- **参数**: `start_id`, `max_hops` (0-5), `rel_types` (string[]), `follow_entities` (bool)。
- **返回值**: 包含 Nodes 和 Edges 的图数据。

#### 12. muninn_explain
- **功能**: 解释为何特定记忆会被召回及其评分组成。
- **参数**: `engram_id`, `query` (string[])。
- **返回值**: 评分分解 (ExplainComponents)。

#### 13. muninn_where_left_off
- **功能**: 寻找最近活跃的、未完成的任务或上下文。
- **参数**: `limit` (int)。
- **返回值**: 记忆简要列表。

### C. 图与关联操作 (3 个)

#### 14. muninn_link
- **功能**: 在两个记忆间手动建立关联。
- **参数**: `source_id`, `target_id`, `relation` (supports, contradicts, depends_on 等), `weight` (0-1)。
- **返回值**: `{"ok": true}`。

#### 15. muninn_contradictions
- **功能**: 查找 vault 中已知的冲突或矛盾记忆对。
- **参数**: 无。
- **返回值**: `ContradictionPair` 数组。

#### 16. muninn_export_graph
- **功能**: 导出整个知识图谱。
- **参数**: `format` (json-ld, graphml), `include_engrams` (bool)。
- **返回值**: 格式化后的图数据字符串。

### D. 管理与元数据 (6 个)

#### 17. muninn_status
- **功能**: 获取 vault 的健康状态和统计数据。
- **返回值**: 记忆计数、Vault 计数等。

#### 18. muninn_session
- **功能**: 获取指定时间点后的所有活动快照。
- **参数**: `since` (ISO8601)。
- **返回值**: `SessionSummary`。

#### 19. muninn_guide
- **功能**: 获取当前 vault 的使用建议和 AI 引导信息。
- **返回值**: 文本格式的说明文档。

#### 20. muninn_list_deleted
- **功能**: 列出近期删除的可恢复记忆。
- **参数**: `limit` (int)。
- **返回值**: `DeletedEngram` 数组。

#### 21. muninn_provenance
- **功能**: 获取记忆的审计轨迹（谁在何时做了什么）。
- **参数**: `id` (string)。
- **返回值**: `ProvenanceEntry` 数组。

#### 22. muninn_replay_enrichment
- **功能**: 对旧记忆重跑富化流程。
- **参数**: `stages` (entities, relationships, classification, summary), `limit`, `dry_run`。
- **返回值**: 进度统计。

### E. 实体管理 (9 个)

#### 23. muninn_find_by_entity
- **功能**: 查找提及特定实体的所有记忆。
- **参数**: `entity_name` (string)。
- **返回值**: 记忆简要列表。

#### 24. muninn_entity_state
- **功能**: 变更实体状态（如标记为 deprecated 或 merged）。
- **参数**: `entity_name`, `state`, `merged_into` (可选)。
- **返回值**: `{"ok": true}`。

#### 25. muninn_entity_state_batch
- **功能**: 批量更新实体状态。
- **参数**: `operations` 数组。
- **返回值**: 批量处理结果。

#### 26. muninn_entity_clusters
- **功能**: 发现经常共同出现的实体对。
- **参数**: `min_count`, `top_n`。
- **返回值**: `EntityClusterPair` 数组。

#### 27. muninn_similar_entities
- **功能**: 基于名称相似度查找可能的重复实体。
- **参数**: `threshold` (0-1)。
- **返回值**: 相似实体对数组。

#### 28. muninn_merge_entity
- **功能**: 合并两个实体并重链接相关记忆。
- **参数**: `entity_a` (被合并者), `entity_b` (目标), `dry_run`。
- **返回值**: `MergeEntityResult`。

#### 29. muninn_entity_timeline
- **功能**: 获取实体的演化时间线。
- **参数**: `entity_name` (string)。
- **返回值**: 时间线条目。

#### 30. muninn_entity
- **功能**: 获取实体的全景视图（元数据、提及记忆、关联关系）。
- **参数**: `name`, `limit`。
- **返回值**: `EntityAggregate`。

#### 31. muninn_entities
- **功能**: 列出所有已知实体。
- **参数**: `state` (可选过滤), `limit`。
- **返回值**: `EntitySummary` 数组。

### F. 树形结构 (3 个)

#### 32. muninn_remember_tree
- **功能**: 递归写入一棵知识树（如项目计划或大纲）。
- **参数**: `root` (TreeNodeInput 嵌套对象)。
- **返回值**: `RootID` 和 `NodeMap`。

#### 33. muninn_recall_tree
- **功能**: 递归读取整棵知识树。
- **参数**: `root_id`, `max_depth`, `include_completed`。
- **返回值**: `TreeNodeOutput` 结构。

#### 34. muninn_add_child
- **功能**: 向现有树节点添加子节点。
- **参数**: `parent_id`, `concept`, `content`, `ordinal` (可选排序)。
- **返回值**: `{"child_id": "..."}`。

### G. 决策 (1 个)

#### 35. muninn_decide
- **功能**: 记录一个决策及其背后的权衡。
- **参数**: `decision`, `rationale`, `alternatives` (string[]), `evidence_ids` (string[])。
- **返回值**: 决策记忆 ID。

### H. 反馈 (1 个)

#### 36. muninn_feedback
- **功能**: 为召回结果提供正/负反馈以优化后续排序。
- **参数**: `engram_id`, `useful` (bool)。
- **返回值**: `{"ok": true}`。

---

## 数据类型参考

### Memory (记忆主体)
| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `id` | string | 唯一标识符 (ULID) |
| `concept` | string | 简短概念/标题 |
| `content` | string | 完整内容文本 |
| `summary` | string | 自动生成的摘要 |
| `score` | float64 | 总加权分 |
| `confidence` | float32 | Bayesian 置信度 (0-1) |
| `state` | string | 状态 (active, archived 等) |
| `tags` | string[] | 标签列表 |
| `created_at` | Time | 创建时间 |

### EntitySummary (实体摘要)
| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `name` | string | 实体名称（小写归一化） |
| `type` | string | 类型 (person, technology, project 等) |
| `state` | string | 状态 (active, merged 等) |
| `mention_count`| int32 | 被提及次数 |

---

## 最佳实践

1. **记忆原子化**: 建议每条记忆只包含一个核心事实。长文本建议拆分为 batch 写入。
2. **利用 Concept**: `concept` 是 FTS 索引的重要加权项，提供清晰的标题能显著提升召回准确率。
3. **主动反馈**: 每次通过 `muninn_recall` 获取信息后，通过 `muninn_feedback` 提供评价，系统会自动调整 Hebbian 权重。
4. **合理设置生命周期**: 对于已完成的任务，及时通过 `muninn_state` 将其设为 `completed` 或 `archived`，避免在 `where_left_off` 中干扰视线。
5. **实体名称归一化**: 写入时尽量使用标准的实体名。如果发现重复，使用 `muninn_merge_entity` 进行清理。
6. **使用 Mode 预设**: 
   - `mode=recent`: 优先获取近期信息。
   - `mode=deep`: 触发 4 跳图遍历以发现隐性关联。

---

## JSON-RPC 调用示例

### 存储记忆
```json
{
  "jsonrpc": "2.0",
  "method": "muninn_remember",
  "params": {
    "concept": "MCP Protocol",
    "content": "MCP uses JSON-RPC 2.0 over SSE or Stdio transports.",
    "tags": ["protocol", "ai"],
    "type": "fact"
  },
  "id": 1
}
```

### 语义召回
```json
{
  "jsonrpc": "2.0",
  "method": "muninn_recall",
  "params": {
    "context": ["How does MuninnDB integrate with AI?"],
    "mode": "balanced",
    "threshold": 0.4
  },
  "id": 2
}
```

---

## 常见错误与规避

- **Context 参数格式错误**: `context` 必须是 `string` 或 `string[]`。传递 `null` 或空数组会报错。
- **ID 不存在**: 所有引用 `id` 的操作如果找不到对象均会返回 `404` 错误，调用前建议先进行 `recall`。
- **Batch 限制**: `remember_batch` 和 `entity_state_batch` 的单次上限均为 50 个。
- **时间格式**: 所有的日期字符串必须遵循 **ISO 8601** (RFC3339) 格式。
- **Vault 隔离**: 如果在多用户环境运行，确保在 transport 层正确映射 `vault` 参数，防止数据越权访问。
