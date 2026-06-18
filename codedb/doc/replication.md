# MuninnDB 复制系统 (replication) 技术文档

## 概述

MuninnDB 的 `replication` 包实现了一个完整的分布式集群复制系统。它专为认知数据库设计，不仅支持传统的数据行复制（WAL），还支持认知一致性同步（Hebbian 权重同步和采样）。

该系统借鉴了 Raft 协议的精髓，实现了自动领导者选举、基于序列号的日志追加、流式同步、快照传输以及成员关系管理。通过抽象存储层和协议层，它能够轻松集成到不同的物理存储引擎中。

### 核心能力
- **自动故障转移**：通过 MSP (Member Service Protocol) 监测节点状态，自动触发领导者选举。
- **高性能复制**：采用 MBP 二进制流协议，支持异步追加和批量同步。
- **认知一致性**：特有的 CCS (Cognitive Consistency Sampling) 机制，确保分布式节点间的 Hebbian 关联权重保持一致。
- **安全保障**：内置自动化 TLS 证书管理和基于 HMAC 的加入令牌机制。

---

## 架构总图

```text
                               +-----------------------------------+
                               |         ClusterCoordinator        |
                               |  (核心调度、状态管理、角色切换控制)  |
                               +-----------------------------------+
                                 /      |        |        |      \
         +-----------------------+      |        |        |       +-----------------------+
         |                              |        |        |                               |
+-------------------+          +--------v--------+  +-----v--------+            +-------------------+
|     Election      |          |       MSP       |  | ConnManager  |            |        TLS        |
| (领导者选举/任期) |          | (故障检测/心跳) |  | (连接/帧分发) |            | (证书/加密通信)   |
+---------+---------+          +--------+--------+  +-----+--------+            +-------------------+
          |                             |                 |
          |                             |        +--------v--------+
          |                             |        |    PeerConn     |
          |                             |        | (MBP 协议处理)  |
          |                             |        +-----------------+
          |                             |
+---------v---------+          +--------v--------+          +-------------------+
|  ReplicationLog   |          |    Streamer     |          |      Applier      |
| (WAL 追加与存储)  |----------> (流式推送数据)  |----------> (应用数据到本地) |
+-------------------+          +--------+--------+          +-------------------+
                                        |
                               +--------v--------+
                               |    Snapshot     |
                               | (全量快照传输)  |
                               +-----------------+
                                        |
         +------------------------------+------------------------------+
         |                              |                              |
+--------v-------+             +--------v-------+             +--------v-------+
|   Reconciler   |             |      CCS       |             |  JoinHandler   |
| (权重调和同步) |             | (一致性采样)   |             | (成员加入处理) |
+----------------+             +----------------+             +----------------+
```

---

## 核心原理

### 1. MBP 协议 (protocol.go)

MBP (Muninn Binary Protocol) 是专门为集群内节点通信设计的轻量级二进制协议。

- **帧结构**：
  - `Version` (1 byte): 协议版本。
  - `Type` (1 byte): 帧类型（如 TypePing, TypeWrite, TypeVoteRequest 等）。
  - `Flags` (2 bytes): 标志位（压缩、流、紧急等）。
  - `PayloadLength` (4 bytes): 负载长度。
  - `CorrelationID` (8 bytes): 关联 ID，用于请求-响应匹配。
  - `Payload` (variable): 实际数据负载，最大支持 16MB。

- **ReadFrame/WriteFrame**：提供了高性能的流式读写能力，确保帧的完整性和有效性。

### 2. KVStore 抽象 (storage.go)

为了解耦底层存储（如 Pebble 或自定义 ERF），该包定义了 `KVStore` 接口：

- **设计定位**：复制系统不直接操作磁盘，而是通过 `KVStore` 接口进行数据持久化。这使得系统可以轻松切换存储后端或在内存中运行测试。
- **主要接口**：
  - `Get`, `Set`, `Delete`：基础操作。
  - `NewIter`：支持范围扫描的迭代器。
  - `NewBatch`：支持原子更新的批量写入。

### 3. 集群配置 (config.go)

`ClusterConfig` 定义了集群运行的所有关键参数：

- **关键字段**：
  - `NodeID`: 每个节点的唯一标识。
  - `Seeds`: 初始连接节点地址列表。
  - `Role`: 节点角色（primary, replica, sentinel, observer）。
  - `HeartbeatMS`: 心跳间隔。
  - `SDOWNBeats`: 判定为故障所需连续丢失的心跳数。

### 4. 协调器 (coordinator.go)

`ClusterCoordinator` 是系统的中枢大脑，负责协调所有子组件。

- **角色管理**：
  - **Primary (Cortex)**: 领导者，负责处理写入并产生 WAL 流。
  - **Replica (Lobe)**: 副本节点，订阅 WAL 流并应用数据。
  - **Sentinel**: 哨兵节点，仅参与选举和故障检测，不存储数据。
  - **Observer**: 观察者节点，接收数据流但不参与选举投票。
- **状态机控制**：管理节点从“追赶状态”到“同步状态”的迁移，以及在丢失法定人数（Quorum）时的降级逻辑。

### 5. 领导者选举 (election.go)

实现了一个简化版 Raft 选举协议。

- **Epoch (任期)**：集群状态的版本号，每次选举都会增加。
- **Fencing Token**：与 Epoch 绑定，用于防止旧 Primary 在网络分区恢复后继续执行写入（脑裂保护）。
- **投票机制**：候选人需获得超过半数投票节点的 `Granted` 响应才能晋升。

### 6. 成员服务协议 MSP (msp.go)

负责集群内的故障检测。

- **心跳机制**：节点间定期发送 `TypePing`，响应 `TypePong`。
- **故障分级**：
  - **SDown (Subjective Down)**: 主观下线，单个节点发现目标不响应。
  - **ODown (Objective Down)**: 客观下线，当足够多的节点报告目标 SDown 时，由 Primary 或 Sentinel 标记并触发重新选举。

### 7. WAL 复制流 (log.go, streamer.go, applier.go)

这是数据同步的核心链路。

- **ReplicationLog**: 将所有变更记录为带有唯一序列号（Seq）的 WAL 记录，存储在持久化的 `KVStore` 中。
- **Streamer**: 负责将 WAL 记录推送到副本节点。支持“追赶模式”（从旧 Seq 开始读）和“监听模式”（实时推送新产生记录）。
- **Applier**: 副本节点接收到记录后，通过 `Applier` 原子地应用到本地数据库，并记录 `last_applied_seq`。

### 8. 快照 (snapshot.go)

当新节点加入或副本落后太多（WAL 已被裁剪）时，触发快照同步。

- **SnapshotSender**: 遍历本地所有 KV 数据，分块（Chunk）发送。
- **SnapshotReceiver**: 接收数据前先清空本地旧数据，接收过程中进行原子批量写入，完成后标记 `snap_complete`。

### 9. 节点加入 (join.go)

处理新节点的准入流程。

- **JoinRequest**: 包含节点 ID、地址、已知最后 Seq 和安全哈希。
- **令牌验证**: 如果配置了 `ClusterSecret`，加入请求必须携带正确的 HMAC 签名或 `JoinToken`。
- **快照触发**: JoinHandler 会根据请求者的 Seq 决定是直接开始流复制还是先传输快照。

### 10. 协调一致性 (reconcile.go)

针对认知数据库的特殊需求，同步 Hebbian 权重。

- **原理**：由于认知权重的更新可能是异步或带有概率性的，Leader 会定期发起 Reconcile。
- **流程**：探测（Probe）样本权重 -> 计算差异 -> 发送同步（Sync）指令 -> 副本应用权重更新。

### 11. CCS 认知一致性采样 (ccs.go)

用于实时评估集群的一致性健康度。

- **抽样哈希**：随机抽取一部分 Engram 及其权重，计算 SHA256 哈希值。
- **比对评分**：比对 Leader 与各副本的哈希值，计算集群一致性得分（0.0 - 1.0）。

### 12. TLS 证书管理 (tls.go)

支持全自动化的安全通信。

- **自动 CA**: 系统可自动生成根证书（CA）。
- **节点证书**: 基于 CA 为每个节点自动签发带有正确 NodeID 的证书。
- **证书轮转**: 支持在线更新证书而不中断服务。

---

## 复制流程图 (Leader Write -> Replica Apply)

```text
  Leader Node                       Replica Node
+--------------+                  +----------------+
| Write Request|                  |                |
+------+-------+                  |                |
       |                          |                |
+------v-------+                  |                |
| WAL Append   |                  |                |
| (Seq=100)    |                  |                |
+------+-------+                  |                |
       |                          |                |
       | (Notify)                 |                |
+------v-------+   MBP Stream     +-------v--------+
|   Streamer   |------------------>|    Applier     |
| (Push 100)   |                  | (Atomic Apply) |
+--------------+                  +-------+--------+
       ^                                  |
       |          Replica Ack             |
       +----------------------------------+
                (Update Lag Info)
```

---

## 故障转移流程图 (Detect -> Failover)

```text
   Node A (Primary)          Node B (Replica)          Node C (Sentinel)
+------------------+      +------------------+      +------------------+
|      [CRASH]     |      |                  |      |                  |
+------------------+      +---------+--------+      +---------+--------+
                            | (Ping Timeout)          | (Ping Timeout)
                            |                         |
                            +-------> SDown ---------->
                                      |
                            +<-------ODown <----------+
                            | (Quorum Reached)
                            |
                  +---------v---------+
                  |  Start Election   |
                  |  (New Epoch=2)    |
                  +---------+---------+
                            |
                  +---------v---------+
                  |    Promoted to    |
                  |      Primary      |
                  +-------------------+
```

---

## 接口参考

### KVStore
底层存储的通用抽象接口。
```go
type KVStore interface {
    Get(key []byte) ([]byte, io.Closer, error)
    Set(key, value []byte) error
    Delete(key []byte) error
    NewIter(lowerBound, upperBound []byte) (KVIterator, error)
    NewBatch() KVBatch
}
```

### LeaseBackend
用于领导者选举的租约后端抽象（可基于内置选举器或外部系统如 etcd）。
```go
type LeaseBackend interface {
    TryAcquire(ctx context.Context, nodeID string, ttl time.Duration) (bool, error)
    Renew(ctx context.Context, nodeID string, ttl time.Duration) error
    Release(ctx context.Context, nodeID string) error
    CurrentHolder(ctx context.Context) (string, error)
    Token(ctx context.Context) (uint64, error)
}
```

---

## 最佳实践

1. **NodeID 唯一性**：确保集群内所有节点的 `NodeID` 永久唯一且固定，不要使用随机生成的临时 ID。
2. **时钟同步**：虽然选举不完全依赖绝对时间，但心跳、CCS 采样和权重同步都依赖相对准确的本地时钟。强烈建议运行 NTP。
3. **隔离心跳网络**：如果可能，将集群复制流量（MBP）与外部 API 流量物理或逻辑隔离，防止大查询导致的心跳抖动。
4. **合理设置 SDOWNBeats**：在不稳定的网络环境中，适当增加 `SDOWNBeats` 以减少虚假选举，但这会增加故障恢复延迟。
5. **定期裁剪 WAL**：使用 `WALPruner` 定期清理已成功同步到所有副本的旧记录，防止存储空间膨胀。
6. **配置集群密钥**：在生产环境中必须设置 `ClusterSecret`，以防止未授权节点非法加入集群并窃取数据。
7. **启用 TLS**：跨数据中心部署时，必须启用 TLS 加密，并使用自动证书轮转功能。
8. **备份快照**：虽然快照用于同步，但定期手动保留一份快照可以作为灾难恢复的最后手段。

---

## 常见错误与规避

- **脑裂 (Split Brain)**：当网络发生分区时，可能出现两个 Primary。
  - *规避*：始终配置奇数个投票节点（如 3, 5, 7），并确保 `Quorum` 逻辑生效。应用层必须检查 `Fencing Token`。
- **快照传输超时**：超大规模数据库在传输快照时可能耗时数小时。
  - *规避*：调整 `ackTimeout` 参数，或使用更高速的骨干网络。
- **存储空间耗尽**：如果不定期 Prune，WAL 会占满磁盘。
  - *规避*：监控 `CurrentSeq` 与 `MinReplicatedSeq` 的差值，配置自动清理策略。
