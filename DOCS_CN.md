# MuninnDB 核心文档 (DOCS_CN.md)

MuninnDB 是一款专为 AI 时代设计的认知记忆数据库。它不仅是一个存储系统，更是一个模拟人类记忆机制的智能引擎。MuninnDB 实现了时间的自然衰减、赫布联想学习和贝叶斯置信度评估，能够根据使用频率和上下文自动优化记忆的权重与关联。

---

## 项目概述

### MuninnDB 是什么
MuninnDB 是一款**认知记忆数据库**，旨在为 AI 智能体和分布式系统提供具有生命力的记忆能力。它通过以下机制模拟生物大脑的记忆行为：
- **时间学习（Temporal Learning）**：集成艾宾浩斯遗忘曲线，记忆随时间自然衰退。
- **赫布联想学习（Hebbian Association）**：实现"共同激发的神经元会连在一起"（Cells that fire together, wire together），自动建立概念间的关联。
- **贝叶斯置信度（Bayesian Confidence）**：通过后验概率评估记忆的可靠性，强化一致信息，弱化矛盾信息。
- **多模态检索**：原生支持向量检索（HNSW）、全文检索（BM25）与关联图谱（Adjacency List）的深度融合。

### 核心理念
- **使用即强化**：记忆的强度由访问频率决定。经常使用的记忆会保持活跃，长期不用的记忆会逐渐模糊（进入背景）。
- **相关即推送**：数据库不再是被动的查询接口，而是根据当前的上下文（Context）主动推送相关的记忆片段。
- **零成本关联**：无需预定义模式（Schema），系统在存储和激活过程中自动发现并加固概念间的联系。

### 命名来源
**Muninn**（穆宁）是北欧神话中奥丁（Odin）的两只渡鸦之一，其名字在古诺斯语中意为**"记忆"**。它每天环绕九界飞行，并将见闻带回给奥丁。

### 法律与许可
- **许可证**：采用 **BSL 1.1**（Business Source License 1.1）。对个人、爱好者、研究人员和小型组织（少于 50 名员工且年收入低于 500 万美元）完全免费。**2030 年 2 月 26 日**起将自动转为 **Apache 2.0** 许可证。
- **专利**：核心认知原语已提交美国临时专利申请（U.S. Provisional Patent Application No. 63/991,402），旨在保护开源项目的创新性不被恶意闭源利用。

---

## 系统架构

MuninnDB 采用单二进制文件设计，追求极简的部署体验与极高的执行效率。

### 架构图示
```text
+-------------------------------------------------------------+
|              用户接口 (REST / gRPC / SDK / Web UI)          |
+-------------------------------------------------------------+
|                传输层 (MBP / HTTP / gRPC / MCP)             |
+-------------------------------------------------------------+
|                        认知引擎 (Engine)                     |
|  +-------------------------------------------------------+  |
|  |  激活管线 (6阶段) | 评分系统 (ACT-R) | 赫布学习系统     |  |
|  +-------------------------------------------------------+  |
|  |  预测激活信号 (PAS) | 贝叶斯置信度评估 | 语义触发器     |  |
|  +-------------------------------------------------------+  |
+-------------------------------------------------------------+
|                        索引与检索层                          |
|  +-----------------+  +-----------------+  +--------------+ |
|  | HNSW 向量索引   |  | FTS 全文检索    |  | 关联图谱引擎 | |
|  +-----------------+  +-----------------+  +--------------+ |
+-------------------------------------------------------------+
|                        持久化存储层                          |
|  +-----------------+  +-----------------+  +--------------+ |
|  | Pebble (LSM)    |  | 自定义 ERF 格式 |  | MOL 预写日志 | |
|  +-----------------+  +-----------------+  +--------------+ |
+-------------------------------------------------------------+
|                        插件与集群层                          |
|  +-----------------+  +-----------------+  +--------------+ |
|  | 嵌入提供者 (8)  |  | LLM 富化提供者  |  | Cortex 集群  | |
|  +-----------------+  +-----------------+  +--------------+ |
+-------------------------------------------------------------+
```

### 核心组件说明
- **存储层**：底层使用 **Pebble** (LSM-tree KV) 保证高性能写入，配合自定义的 **ERF** (Engram Record Format) 二进制格式存储大块内容，**MOL** 预写日志确保数据持久化与集群流式复制。
- **认知引擎**：核心调度中心，实现 6 阶段激活管线。它负责 ACT-R 评分计算、关联权重更新以及置信度推理。
- **索引层**：通过 HNSW 提供毫秒级向量近似搜索，结合 BM25 算法的全文检索，以及基于邻接表的关联图谱检索。
- **传输层**：支持高性能 **MBP** 二进制协议（延迟 <10ms）、标准 **REST/gRPC**，以及面向 AI Agent 的 **MCP** (Model Context Protocol)。
- **集群架构**：基于 WAL 流式复制的 **Cortex/Lobe** 主从架构，支持高可用与横向读取扩展。

---

## 快速开始

### 1. 安装
**macOS / Linux:**
```bash
curl -sSL https://muninndb.com/install.sh | sh
```

**Windows (PowerShell):**
```powershell
irm https://muninndb.com/install.ps1 | iex
```

### 2. 启动服务器
```bash
muninn start
```
首次运行会自动完成环境检测与默认配置生成。

### 3. 存储第一条记忆
使用 REST API 存入一条关于支付逻辑的记忆：
```bash
curl -sX POST http://127.0.0.1:8475/api/engrams \
  -H 'Content-Type: application/json' \
  -d '{
    "concept": "支付幂等性",
    "content": "在第三季度双重收费事故后，我们统一切换到了幂等键（Idempotency Keys）机制。"
  }'
```

### 4. 激活/召回记忆
即使没有直接提及"幂等键"，MuninnDB 也能通过上下文理解语义：
```bash
curl -sX POST http://127.0.0.1:8475/api/activate \
  -H 'Content-Type: application/json' \
  -d '{"context": ["正在调试支付重试逻辑的问题"]}'
```

### 5. 访问 Web UI
在浏览器中打开：`http://127.0.0.1:8476`
默认管理员：`root` / `password`（请在首次登录后修改）。

### 6. 连接 AI 工具 (MCP)
如果你使用 Claude Desktop、Cursor 或 VS Code，运行以下命令自动配置：
```bash
muninn init
```

---

## 端口参考

| 服务名称 | 默认端口 | 协议类型 | 说明 |
| :--- | :--- | :--- | :--- |
| **MBP** | 8474 | TCP (MsgPack) | 高性能二进制协议，延迟极低 |
| **REST** | 8475 | HTTP/JSON | 标准 Web 开发与调试接口 |
| **Web UI** | 8476 | HTTP/HTML | 控制台、数据可视化与管理界面 |
| **gRPC** | 8477 | HTTP/2 (Proto) | 强类型、支持流式的后端通信协议 |
| **MCP** | 8750 | HTTP/SSE | Model Context Protocol，对接 AI Agent |

---

## 核心概念

### Engram（记忆痕迹）
MuninnDB 的核心数据单元。不同于传统的行或文档，Engram 包含以下关键元数据：
- **Concept**：记忆的抽象标签或标题。
- **Content**：具体的记忆内容。
- **Confidence**：贝叶斯置信度，反映记忆的可靠程度。
- **Stability**：记忆的稳定性，决定衰减速度。
- **Relevance**：相对于当前上下文的实时相关性得分。

### Vault（保管库）
逻辑隔离的数据容器。类似于数据库的 Schema 或 Namespace。不同的 AI 角色或应用可以使用不同的 Vault 来隔离记忆空间。

### 激活 (Activation)
记忆召回的过程。MuninnDB 不执行简单的"搜索"，而是执行"激活"。激活管线包含 6 个阶段：预过滤、多模态检索、赫布加权、时间评分、关联扩展、结果融合。

### 关联 (Association)
记忆间的连接纽带。当两条记忆在短时间内被共同激活时，它们之间的关联权重会增加。这种关联是动态生成的，不需要手动维护。

### 时间衰减 (Temporal Decay)
遵循艾宾浩斯遗忘曲线模型。记忆如果不被再次激活，其重要性得分（Base Level Activation）会随时间对数下降。

### 置信度 (Confidence)
基于贝叶斯后验概率。如果新的信息证实了旧记忆，置信度上升；如果新信息与旧记忆矛盾，置信度下降。

---

## 认知算法概要

MuninnDB 的强大源于其底层数学模型：

### ACT-R 记忆基础激活得分
用于计算记忆的内在强度：
$$B(M) = \ln(n+1) - 0.5 \times \ln\left(\frac{ageDays}{n+1}\right)$$
通过 $softplus(B) = \ln(1+e^B)$ 进行平滑处理。
- $n$: 记忆被激活的次数。
- $ageDays$: 自记忆创建以来的天数。

### 赫布学习规则 (Hebbian Learning)
用于更新记忆间的关联权重：
$$w_{new} = \min(1.0, w_{old} \times (1+\eta)^{signal})$$
- $\eta$ (学习率): 默认 0.01。
- $signal$: 激活信号强度。

### 贝叶斯置信度更新
$$posterior = \frac{p \times s}{p \times s + (1-p) \times (1-s)}$$
$$confidence = 0.95 \times posterior + 0.025$$
- $p$: 先验置信度。
- $s$: 证据信号强度。

### BM25 全文检索评分
$$score = \sum IDF(t) \times \frac{tf \times (k_1+1)}{tf + k_1 \times (1 - b + b \times \frac{dl}{avgdl})}$$
- 参数设置：$k_1=1.2$, $b=0.75$。

---

## 配置参考

可以通过环境变量或 `.env` 文件配置 MuninnDB。

| 变量名 | 说明 | 默认值/示例 |
| :--- | :--- | :--- |
| **MUNINN_LOCAL_EMBED** | 是否启用内置本地向量嵌入引擎 | `1` (启用) |
| **MUNINN_OLLAMA_URL** | Ollama 服务地址 | `http://localhost:11434` |
| **MUNINN_OPENAI_KEY** | OpenAI API Key | `sk-...` |
| **MUNINN_ANTHROPIC_KEY** | Anthropic API Key (用于 LLM 富化) | `sk-ant-...` |
| **MUNINN_ENRICH_URL** | 富化插件使用的模型 URL | `anthropic://claude-3-haiku` |
| **MUNINNDB_DATA** | 数据存储目录 | `~/.muninn/data` |
| **MUNINN_MEM_LIMIT_GB** | 最大内存限制 (GB) | `4` |
| **MUNINN_MCP_TOKEN** | MCP 访问令牌（用于安全验证） | 自动生成 |
| **MUNINN_HNSW_MAX_MB** | HNSW 索引最大内存占用 | `512` |

---

## 多协议 API 概览

### REST API
提供 70 多个端点，涵盖 Engram 管理、Vault 配置、系统监控等。返回标准 JSON 格式。
- `POST /api/engrams`：存入记忆。
- `POST /api/activate`：上下文驱动激活。

### gRPC 服务
包含 9 个核心 RPC 方法，提供强类型的 Protobuf 定义，适合后端微服务间的高性能通信。支持流式结果返回。

### MBP (Muninn Binary Protocol)
极致性能的二进制协议。采用 16 字节固定包头 + MsgPack 载荷，适用于毫秒级响应要求的嵌入式或高频交易场景。

### MCP (Model Context Protocol)
为 AI Agent 定制，暴露 35 个工具（Tools）。允许 Claude、Cursor 等工具直接调用 `muninn_remember`（记忆）和 `muninn_recall`（回想）。

---

## SDK 概览

| SDK 语言 | 安装命令 | 异步模型 | 核心特性 |
| :--- | :--- | :--- | :--- |
| **Go** | `go get .../sdk/go/muninn` | Context/Goroutines | 原生性能，支持插件开发 |
| **Python** | `pip install muninn-python` | asyncio | 集成 LangChain, LlamaIndex |
| **Node.js** | `npm install @muninndb/client` | Promises/TS | 类型安全，支持 Web 侧调用 |
| **Kotlin** | `implementation("...")` | Coroutines | 移动端优化，支持多平台 |
| **PHP** | `composer require ...` | Synchronous | 简洁 API，适合 Web 后端 |
| **Swift** | `Swift Package Manager` | Swift Concurrency | 原生 iOS/macOS 集成 |

---

## 文档导航

为了深入了解 MuninnDB，请参考以下中文文档：

- [内部实现细节](internal/DOCS_CN.md)：深入探讨索引实现与存储格式。
- [SDK 开发指南](sdk/DOCS_CN.md)：各语言 SDK 的详细使用示例。
- [系统架构与原理](docs/DOCS_CN.md)：认知算法与 6 阶段管线的数学证明。
- [部署与运维指南](DEPLOY_CN.md)：Docker 部署、集群配置与性能调优。

---
*Generated by MuninnDB Documentation Team. Last updated: 2026-03-24.*
