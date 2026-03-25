# MuninnDB 部署与运维指南 (Deployment & Operations Guide)

本指南旨在为管理员和开发者提供 MuninnDB 的安装、配置、集群管理及日常运维的全面参考。MuninnDB 是一个集成了时间学习、赫布关联（Hebbian association）以及多模态检索（向量 + 全文 + 图）的认知内存数据库。

---

## 目录

1. [核心特性](#核心特性)
2. [快速开始](#快速开始)
3. [安装指南](#安装指南)
4. [命令行参考 (CLI Reference)](#命令行参考)
5. [配置说明 (Configuration)](#配置说明)
6. [Docker 部署](#docker-部署)
7. [启动与关闭流程](#启动与关闭流程)
8. [AI 工具集成 (MCP)](#ai-工具集成)
9. [集群架构与运维](#集群架构与运维)
10. [备份与恢复](#备份与恢复)
11. [监控与故障排除](#监控与故障排除)

---

## 1. 核心特性

- **多协议支持**: 原生支持 MBP (Binary), REST, gRPC 和 MCP (Model Context Protocol)。
- **认知引擎**: 实现艾宾浩斯遗忘曲线、赫布学习律和置信度评分。
- **混合索引**: 集成 HNSW 向量索引、FTS 全文索引和邻接图索引。
- **本地化 AI**: 内置 ONNX 运行时，支持本地向量嵌入（Embedding）。

---

## 2. 快速开始

在开始之前，请确保已安装 Go 1.25+ 和相关的编译工具。

```bash
# 克隆仓库
git clone https://github.com/your-repo/muninndb.git
cd muninndb

# 获取必要资产（模型与库）
make fetch-assets

# 编译并运行
go build -tags localassets -o muninn ./cmd/muninn
./muninn server start
```

---

## 3. 安装指南

### 3.1 编译要求
- **操作系统**: Linux, macOS, Windows (WSL2 推荐)。
- **构建标签**: 必须使用 `-tags localassets` 以启用内置的 ONNX 嵌入器。
- **外部依赖**: 
  - `Pebble`: 底层 KV 存储（内置）。
  - `ONNX Runtime`: 用于本地推理（通过 `make fetch-assets` 获取）。

### 3.2 目录结构
建议的部署目录结构：
```text
/opt/muninndb/
├── bin/            # 可执行文件
├── data/           # 数据库文件 (Pebble/ERF)
├── logs/           # 日志文件
├── config/         # 配置文件 (muninn.env)
└── assets/         # ONNX 模型与库文件
```

---

## 4. 命令行参考 (CLI Reference)

MuninnDB 采用多级命令结构：`muninn [category] [action] [flags]`。

### 4.1 服务器管理 (`server`)
- `muninn server start`: 启动数据库服务。
  - `--config`: 指定配置文件路径。
  - `--port`: 覆盖默认 MBP 端口 (8474)。
  - `--dev`: 开启开发模式。
- `muninn server stop`: 优雅关闭服务。
- `muninn server status`: 查看当前运行状态。

### 4.2 节点管理 (`node`)
- `muninn node join <seed-addr>`: 加入现有集群。
- `muninn node leave`: 退出集群。
- `muninn node info`: 显示当前节点角色（Cortex, Lobe, Sentinel）。

### 4.3 数据操作 (`data`)
- `muninn data backup <path>`: 执行热备份。
- `muninn data restore <path>`: 从备份恢复。
- `muninn data compact`: 强制执行磁盘压缩。

---

## 5. 配置说明 (Configuration)

MuninnDB 的配置遵循以下优先级：
1. 命令行参数 (CLI Flags)
2. 环境变量 (Environment Variables)
3. `muninn.env` 文件
4. 插件 JSON 配置
5. 硬编码默认值

### 5.1 常用环境变量

| 变量名 | 描述 | 默认值 |
|--------|------|--------|
| `MUNINN_DATA_DIR` | 数据存储路径 | `./data` |
| `MUNINN_MBP_PORT` | MBP 二进制协议端口 | `8474` |
| `MUNINN_REST_PORT` | REST API 端口 | `8475` |
| `MUNINN_GRPC_PORT` | gRPC 端口 | `8477` |
| `MUNINN_MCP_PORT` | MCP (SSE) 端口 | `8750` |
| `MUNINN_LOCAL_EMBED` | 是否启用本地嵌入引擎 | `true` |
| `MUNINN_LOG_LEVEL` | 日志级别 (debug, info, warn, error) | `info` |
| `MUNINN_CLUSTER_SEED` | 集群种子节点地址 | `""` |

---

## 6. Docker 部署

使用 Docker 是生产环境下最简单的部署方式。

### 6.1 Dockerfile 说明
官方 Dockerfile 会在构建阶段自动下载模型资产并将其嵌入镜像中。

### 6.2 Docker Compose 配置示例
```yaml
services:
  muninn:
    image: muninndb:latest
    ports:
      - "8474:8474" # MBP
      - "8475:8475" # REST
      - "8750:8750" # MCP
    volumes:
      - muninn_data:/var/lib/muninn
    environment:
      - MUNINN_DATA_DIR=/var/lib/muninn
      - MUNINN_LOG_LEVEL=info
    restart: always

volumes:
  muninn_data:
```

---

## 7. 启动与关闭流程

### 7.1 启动序列 (18 个步骤)
1. **加载环境变量**: 读取 `.env` 和系统环境。
2. **初始化日志**: 设置输出格式与级别。
3. **验证系统限制**: 检查文件描述符 (ulimit)。
4. **加载插件系统**: 加载 Embed/Enrich 驱动。
5. **打开存储引擎**: 初始化 Pebble 和 ERF。
6. **回放 WAL**: 确保崩溃后数据一致性。
7. **初始化索引**: 加载 HNSW 和 FTS 内存映射。
8. **启动认知引擎**: 激活衰减与强化调度器。
9. **建立集群身份**: 生成或读取节点 ID。
10. **启动协调器**: 开启集群 Leader 选举。
11. **注册本地工具**: 初始化内置认知工具。
12. **启动 MBP Server**: 开启二进制传输层。
13. **启动 gRPC Server**: 开启远程过程调用。
14. **启动 REST Server**: 开启 HTTP 接口。
15. **启动 Web UI**: 开启可视化管理后台。
16. **启动 MCP Server**: 开启模型上下文协议。
17. **开启备份调度器**: 执行定时备份任务。
18. **发布 Ready 状态**: 集群探测器标记节点在线。

### 7.2 关闭序列 (优雅关机)
当接收到 `SIGTERM` 或 `SIGINT` 信号时：
1. **拒绝新请求**: 停止所有 Transport 监听。
2. **处理积压事务**: 等待 Retroactive 处理器完成工作 (10s)。
3. **关闭网络服务**: 依次断开 MBP, gRPC, REST 连接。
4. **停止集群协调**: 退出选举，通知 Sentinel。
5. **停止引擎工作线程**: 关闭认知演算与清理任务。
6. **刷新并关闭存储**: 安全写入 WAL 并关闭存储引擎。
7. **退出进程**: 释放所有系统资源。

---

## 8. AI 工具集成 (MCP)

MuninnDB 支持 Model Context Protocol (MCP)，使其可以作为 AI 代理（如 Claude 或 Cursor）的外部知识库。

### 8.1 客户端配置

MuninnDB 能够自动检测并配置 Claude Desktop, Cursor, OpenClaw, Windsurf, OpenCode, VS Code 等工具：

```bash
muninn init
```

#### Claude Desktop
在 `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) 或 `%APPDATA%\Claude\claude_desktop_config.json` (Windows) 中添加：

```json
{
  "mcpServers": {
    "muninn": {
      "url": "http://127.0.0.1:8750/mcp"
    }
  }
}
```
> **注意**: 故意省略了 `"type"` 字段。Claude Desktop v1.1.4010+ 如果存在 `"type": "http"` 会在启动时崩溃。

#### Cursor
在 `~/.cursor/mcp.json` 中添加：

```json
{
  "mcpServers": {
    "muninn": {
      "type": "http",
      "url": "http://127.0.0.1:8750/mcp"
    }
  }
}
```

#### OpenCode
在 `~/.config/opencode/opencode.json` (macOS/Linux) 或 `%APPDATA%\opencode\opencode.json` (Windows) 中添加：

```json
{
  "mcp": {
    "muninn": {
      "type": "remote",
      "url": "http://127.0.0.1:8750/mcp",
      "oauth": false,
      "headers": {
        "Authorization": "Bearer {file:~/.muninn/mcp.token}"
      }
    }
  }
}
```

### 8.2 MCP 工具说明
MuninnDB 暴露了 **35 个 MCP 工具**，包括：
- `muninn_remember`: 存储新的记忆。
- `muninn_recall`: 根据上下文检索记忆。
- `muninn_search`: 文本搜索。
- `muninn_guide`: 获取针对当前库的 AI 使用指南。
- `muninn_batch_write`: 批量写入。

---

## 9. 集群架构与运维

### 9.1 核心/叶片模型 (Cortex/Lobe)
MuninnDB 使用 **Cortex/Lobe** 模型（内部称为 Leader/Replica）：

- **Cortex (核心/主节点)**: 单一写入者。接收写入请求，运行认知工作线程（时间衰减、赫布学习、矛盾检测、置信度计算），向 Lobe 推送 WAL 流，处理加入请求。
- **Lobe (叶片/从节点)**: 只读副本。从 Cortex 接收 WAL 流，应用到本地 Pebble 存储。在故障转移期间可提升为 Cortex。

### 9.2 节点角色

| 角色 | 数据存储 | 是否投票 | 用途 |
|------|----------|----------|------|
| **Primary** | 是 | 是 | Leader (Cortex)。唯一的写入者。 |
| **Replica** | 是 | 是 | Follower (Lobe)。接收同步数据。 |
| **Sentinel** | 否 | 是 | 仅投票成员。提高故障检测精度，不存储数据。 |
| **Observer** | 是 | 否 | 接收同步但不参与投票。用于只读扩展。 |

### 9.3 集群启动流程

#### 启动主节点
1. 在数据目录下创建 `cluster.yaml`：
```yaml
enabled: true
node_id: "primary-1"
role: primary
bind_addr: "0.0.0.0:8474"
cluster_secret: "你的安全密钥"
```
2. 启动服务：`muninn server start`。

#### 添加从节点
1. 在主节点获取加入令牌：`curl http://127.0.0.1:8475/api/admin/cluster/token`。
2. 在新节点配置 `cluster.yaml`：
```yaml
enabled: true
node_id: "replica-1"
role: replica
bind_addr: "0.0.0.0:8474"
seeds:
  - "主节点IP:8474"
cluster_secret: "你的安全密钥"
```
3. 启动新节点。

### 9.4 故障转移 (Failover)

#### 自动故障转移 (MSP 协议)
1. **心跳检测**: MSP (Muninn Sentinel Protocol) 每秒发送一次心跳。
2. **SDOWN (主观下线)**: 连续 3 次心跳丢失后标记为 SDOWN。
3. **ODOWN (客观下线)**: 当法定人数（Quorum）同意节点下线时，标记为 ODOWN。
4. **选举**: 剩余节点中 epoch 最高且 WAL 最新的 Lobe 将发起竞选。

#### 优雅故障转移 (Manual Handoff)
通过 API 将主节点权限移交给指定节点：
```bash
curl -X POST http://127.0.0.1:8475/api/admin/cluster/failover \
  -H "Content-Type: application/json" \
  -d '{"target_node_id": "replica-1"}'
```
流程包括：进入排水模式 (DRAINING) -> 刷新认知工作线程 -> 等待数据同步对齐 -> 发送 HANDOFF。

---

## 10. 备份与恢复

### 10.1 备份选项

**选项 A: 命令行备份 (推荐)**
```bash
# 在线备份 (服务器运行时)
curl -X POST http://127.0.0.1:8475/api/admin/backup \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"output_dir": "/backups/muninn-online"}'
```
该操作会创建一个 Pebble Checkpoint（硬链接，空间效率高），并拷贝 `wal/` 目录和 `auth_secret`。

**选项 B: 库导出 (Vault Export)**
```bash
curl -H "Cookie: <session>" \
  "http://127.0.0.1:8475/api/admin/vaults/default/export" \
  -o default.muninn
```

### 10.2 恢复流程
1. 停止 MuninnDB。
2. 将备份内容替换到 `{dataDir}`。
3. 如果是集群环境，先作为单机节点启动并验证。

---

## 11. 运维与监控

### 11.1 滚动升级 (Rolling Upgrade)
建议顺序：
1. **Sentinels (哨兵)**
2. **Replicas (从节点)**：逐一升级，等待 `replication_lag` 归零。
3. **Failover (故障转移)**：将 Cortex 切换到已升级的节点。
4. **Old Primary (旧主节点)**：降级为从节点后升级。

### 11.2 故障排除表

| 症状 | 检查项 |
|------|--------|
| 节点无法加入 | 检查 `cluster_secret` 是否一致；检查 8474 端口是否开放。 |
| 同步落后 | 检查网络带宽；如果落后太多，重启节点触发 Snapshot 同步。 |
| 无法选举 | 检查存活节点是否满足法定人数 (Voters/2 + 1)。 |
| ODT 错误 | Windows 环境下可能缺少 Visual C++ 2019+ 运行时。 |

### 11.3 关键端口需求

| 端口 | 协议 | 用途 |
|------|------|------|
| 8474 | TCP | MBP + 集群内部通信 (必须互通) |
| 8475 | HTTP | REST API |
| 8476 | HTTP | Web UI |
| 8477 | gRPC | gRPC 服务 |
| 8750 | HTTP | MCP 服务 |

---

## 12. 开发者集成指南 (SDK)

MuninnDB 提供了多语言 SDK，帮助开发者快速集成认知能力。

### 12.1 Go SDK
```go
import "github.com/scrypster/muninndb/sdk/go/muninn"

// 初始化客户端
client := muninn.NewClient("http://127.0.0.1:8475", "your-api-key")

// 写入记忆
id, _ := client.Write(ctx, "default", "架构设计", "基于多协议的分布式认知数据库", []string{"design"})

// 激活上下文
resp, _ := client.Activate(ctx, "default", []string{"如何设计系统？"}, 5)
```

### 12.2 Python SDK
```python
from muninn import MuninnClient

async with MuninnClient("http://127.0.0.1:8475") as m:
    # 写入
    await m.write(vault="default", concept="支付逻辑", content="使用幂等键防止重复扣款")
    
    # 激活相关联的记忆
    result = await m.activate(vault="default", context=["调试支付流程"], max_results=5)
```

### 12.3 Node.js / TypeScript SDK
```typescript
import { MuninnClient } from '@muninndb/client';

const client = new MuninnClient({ token: 'your-api-key' });
const { id } = await client.write({ concept: 'auth', content: '使用 HttpOnly Cookie 存储 Token' });
```

---

## 13. 配置详细参数参考

### 13.1 引擎配置 (Engine)

| 参数 | 环境变量 | 默认值 | 描述 |
|------|----------|--------|------|
| `ActivationThreshold` | `MUNINN_ACT_THRESHOLD` | `0.15` | 激活记忆的最低评分阈值。评分越低，召回的模糊性越高。 |
| `DecayRate` | `MUNINN_DECAY_RATE` | `0.005` | 记忆随时间的衰减率。 |
| `HebbianBoost` | `MUNINN_HEBBIAN_BOOST` | `1.2` | 共同激活记忆的关联增强因子。 |

### 13.2 存储配置 (Storage)

| 参数 | 环境变量 | 默认值 | 描述 |
|------|----------|--------|------|
| `WAL_Prune_Interval` | `MUNINN_WAL_PRUNE` | `60s` | 已提交的日志清理间隔。 |
| `Pebble_Cache_Size` | `MUNINN_PEBBLE_CACHE` | `256MB` | Pebble 底层存储的块缓存大小。 |

---

## 14. 常用运维脚本

### 14.1 检查所有节点同步状态 (Bash)
```bash
#!/bin/bash
NODES=("10.0.1.5" "10.0.1.6" "10.0.1.7")
SECRET="your-secret"

for node in "${NODES[@]}"; do
  echo "Node: $node"
  curl -s -H "Authorization: Bearer $SECRET" "http://$node:8475/v1/cluster/health" | jq .
done
```

### 14.2 自动化备份 Cron 任务
```bash
# 每天凌晨 2 点执行备份
0 2 * * * /usr/local/bin/muninn data backup /mnt/backups/muninn-$(date +\%Y\%m\%d)
```

---

*MuninnDB — 记忆是智慧的基石。*
