# MuninnDB 查询系统与 MQL 技术文档

## 概述

MuninnDB 的查询系统是一个专为认知记忆检索设计的双层架构。它结合了高性能的后检索过滤（Post-retrieval Filtering）和表达力丰富的领域特定语言（MQL - Muninn Query Language）。

1.  **Filter 层 (后检索过滤)**：位于 `codedb/query` 包，提供谓词（Predicate）组合功能。在引擎完成初步的语义和全文检索后，Filter 用于对结果集进行精确过滤（如时间、标签、状态等）。
2.  **MQL 层 (领域特定语言)**：位于 `codedb/query/mql` 包，提供类 SQL 的语法，使用户能够通过字符串构建复杂的查询逻辑。MQL 经过词法分析、语法解析、抽象语法树（AST）构建，最终由执行器转化为引擎可识别的指令。

---

## 架构图

```text
                     +---------------------------+
                     | 用户查询 (MQL String)      |
                     +-------------+-------------+
                                   |
                                   v
                     +-------------+-------------+
                     | MQL Lexer (词法分析)       | -> Tokens
                     +-------------+-------------+
                                   |
                                   v
                     +-------------+-------------+
                     | MQL Parser (语法解析)      | -> AST (Abstract Syntax Tree)
                     +-------------+-------------+
                                   |
                                   v
                     +-------------+-------------+
                     | MQL Executor (执行器)      |
                     +-------------+-------------+
                                   |
              +--------------------+--------------------+
              |                    |                    |
              v                    v                    v
      +-------+-------+    +-------+-------+    +-------+-------+
      |  Activate     |    |  Recall       |    |  Traverse     |
      | (语义激活)    |    | (片段检索)    |    | (图谱遍历)    |
      +-------+-------+    +---------------+    +---------------+
              |
              v
      +-------+-------+
      |  Filter 过滤  | (基于时间、元数据、分数)
      +-------+-------+
              |
              v
      +-------+-------+
      |   最终结果    |
      +---------------+
```

---

## 核心原理

### 1. Filter 过滤器 (filter.go)

Filter 是后检索谓词，用于在初步激活结果产生后进行精细化筛选。

*   **多维度约束**：支持时间（CreatedAfter/Before）、元数据（States, Tags, Creator）、分数阈值（Relevance, Confidence, Stability）以及 Vault 范围。
*   **语义逻辑**：
    *   `Tags`：使用 **AND** 语义（记忆条目必须包含所有列出的标签）。
    *   `States`：使用 **OR** 语义（匹配任何列出的状态）。
    *   其他字段：精确匹配或范围匹配。
*   **分页支持**：内置 `Limit` 和 `Offset` 处理。

### 2. MQL 词法分析器 (mql/lexer.go)

Lexer 负责将 MQL 字符串分解为 Token 流。

*   **关键字识别**：识别 `ACTIVATE`, `FROM`, `CONTEXT`, `WHERE`, `AND`, `OR` 等。
*   **字面量处理**：处理双引号字符串、数字和标识符。
*   **特殊符号**：处理运算符（`=`, `>`, `>=`）、定界符（`[ ]`, `( )`, `,`）和注释（`--` 开始）。

### 3. MQL 解析器 (mql/parser.go)

Parser 采用递归下降算法将 Token 流转换为 AST 节点。

*   **语法规则**：
    *   `ACTIVATE` 查询：必须包含 `FROM` 和 `CONTEXT` 子句，可选 `WHERE`, `MAX_RESULTS`, `HOPS`, `MIN_RELEVANCE`。
    *   `WHERE` 谓词：支持嵌套的 `AND`/`OR` 表达式，支持括号改变优先级。
    *   其他查询：支持 `RECALL`, `TRAVERSE`, `CONSOLIDATE`, `WORKING_MEMORY`。
*   **AST 定义 (mql/ast.go)**：定义了各种查询结构体（如 `ActivateQuery`）和谓词接口（`Predicate`）。

### 4. MQL 执行器 (mql/executor.go)

Executor 负责遍历 AST 并调用引擎 API。

*   **翻译转换**：将 `ActivateQuery` 中的 `WHERE` 谓词树转换为 `query.Filter` 对象。
*   **分发执行**：根据查询类型分发到不同的引擎接口（如 `Activate`, `ListFrames` 等）。
*   **后过滤衔接**：生成的 `query.Filter` 会被传递给引擎，用于执行高效的候选者过滤。

---

## MQL 语言参考

### 1. 语法规范

#### ACTIVATE 查询
```sql
ACTIVATE FROM "vault_name" 
CONTEXT ["term1", "term2"] 
[WHERE <predicate>] 
[MAX_RESULTS 20] 
[HOPS 2] 
[MIN_RELEVANCE 0.5]
```

#### 其他查询
*   `RECALL EPISODE "uuid" [FRAMES 5]`：回溯特定认知片段。
*   `TRAVERSE FROM "engram_id" HOPS 3 [MIN_WEIGHT 0.8]`：遍历关联图谱。
*   `CONSOLIDATE VAULT "vault_name" [DRY_RUN]`：触发 vault 固化。
*   `WORKING_MEMORY SESSION "session_id"`：获取工作记忆。

### 2. 操作符与数据类型
*   **比较符**：`=`, `>`, `>=`
*   **逻辑符**：`AND`, `OR` (WHERE 子句中)
*   **数据类型**：
    *   `STRING`：双引号包裹，支持转义。
    *   `NUMBER`：整数或浮点数。
    *   `IDENT`：未加引号的标识符。
    *   `BOOLEAN`：`DRY_RUN` 关键字。

### 3. 内置谓词字段 (WHERE 子句)
*   `state = active`：生命周期状态过滤。
*   `relevance > 0.7`：相关度分数过滤。
*   `confidence >= 0.8`：置信度分数过滤。
*   `tag = "urgent"`：标签匹配。
*   `creator = "user_1"`：创建者过滤。
*   `created_after "2026-03-31T10:00:00Z"`：时间戳过滤（RFC3339 格式）。
*   `provenance.source = human`：来源过滤。
*   `provenance.agent = "agent_id"`：代理 ID 过滤。

---

## Filter API 参考

### 类型定义

```go
type Filter struct {
    CreatedAfter  *time.Time
    CreatedBefore *time.Time
    UpdatedAfter  *time.Time
    States        []engine.LifecycleState
    Tags          []string // AND 语义
    Creator       string
    MinRelevance  float32
    MinConfidence float32
    MinStability  float32
    Vaults        []string
    CrossVault    bool
    Limit         int
    Offset        int
}
```

### 核心方法

*   `func (f *Filter) Validate() error`：验证参数合法性（如分数需在 0-1 之间）。
*   `func (f *Filter) Match(e *engine.Engram) bool`：判断单个记忆条目是否符合条件。
*   `func (f *Filter) Apply(engrams []*engine.Engram) []*engine.Engram`：对切片进行过滤和分页处理。

---

## MQL API 参考

### 核心组件

*   `mql.NewLexer(input)`：创建词法分析器。
*   `mql.Tokenize(input)`：快速获取 Token 列表。
*   `mql.NewParser(tokens)`：创建解析器。
*   `mql.Parse(input)`：一键解析字符串为 AST 节点。
*   `mql.Execute(ctx, engine, query)`：执行 ACTIVATE 查询。
*   `mql.ExecuteQuery(ctx, engine, query, ...)`：执行任意类型的 MQL 查询。

---

## 最佳实践

1.  **优先使用上下文**：MQL 的核心是 `CONTEXT`。提供更丰富的上下文术语比复杂的 `WHERE` 子句能获得更准确的初始激活。
2.  **善用分数阈值**：在 `WHERE` 子句中使用 `relevance > 0.5` 可以过滤掉低质量的关联，显著提高 AI 响应的准确性。
3.  **时间窗口限制**：对于有时效性的任务，务必使用 `created_after` 过滤掉过时的记忆。
4.  **标签分类**：通过 `tag = "knowledge"` 或 `tag = "task"` 对记忆进行分类存储和检索，可以模拟多维度索引。
5.  **避免过深嵌套**：虽然解析器支持最高 50 层的括号嵌套，但为了性能和可读性，建议保持查询扁平化。

---

## MQL 查询示例

### 场景 1：获取高置信度的安全相关记忆
```sql
ACTIVATE FROM "default" 
CONTEXT ["firewall", "security policy"] 
WHERE confidence > 0.8 AND tag = "security"
MAX_RESULTS 5
```

### 场景 2：检索最近更新的活动记忆
```sql
ACTIVATE FROM "knowledge_base"
CONTEXT ["authentication"]
WHERE state = active AND created_after "2026-01-01T00:00:00Z"
MIN_RELEVANCE 0.6
```

### 场景 3：图谱遍历查找关联
```sql
TRAVERSE FROM "01JKH8V2E3M4P5S6T7Y8Z9" 
HOPS 3 
MIN_WEIGHT 0.7
```

### 场景 4：复杂逻辑组合
```sql
ACTIVATE FROM "default"
CONTEXT ["performance tuning"]
WHERE (tag = "database" OR tag = "cache") AND relevance > 0.7
MAX_RESULTS 10
```

---

## 常见错误与规避

1.  **RFC3339 时间格式错误**：`created_after` 必须使用标准 ISO 格式字符串（如 `2026-03-31T10:00:00Z`）。
    *   *规避*：在代码中使用 `time.Now().Format(time.RFC3339)` 生成字符串。
2.  **WHERE 子句中的 OR 转换限制**：目前执行器在将 `OR` 谓词转换为 `mbp.Filter` 时可能存在限制（取决于引擎版本）。
    *   *规避*：尽量将 OR 逻辑拆分为多个标签，或使用更宽泛的上下文。
3.  **Vault 拼写错误**：如果指定的 Vault 不存在，查询将返回空结果。
    *   *规避*：在执行前先验证 Vault 是否在可用列表中。
4.  **遗漏 CONTEXT**：`ACTIVATE` 语句必须包含 `CONTEXT` 列表，即使是空列表 `[]`（虽然不建议空检索）。
5.  **分数越界**：`relevance` 和 `confidence` 的阈值必须在 `0.0` 到 `1.0` 之间。
    *   *规避*：解析器会自动验证，但应用层在动态构建语句时应预检。
