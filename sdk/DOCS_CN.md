# MuninnDB SDK 参考文档 (简体中文)

MuninnDB 是一套实现时间学习、海边关联 (Hebbian association) 和多模态检索的认知记忆数据库。为了方便开发者在不同技术栈中集成认知记忆能力，我们提供了 6 个官方 SDK。

## SDK 总览

MuninnDB 目前支持以下 6 个官方 SDK：
- **Go**: 异步优先，集成 `context.Context`，适合高性能后端。
- **Python**: 异步优先 (`httpx`)，提供 LangChain 深度集成，适合 AI/ML 任务。
- **Node.js/TypeScript**: 原生 `fetch` 实现，全类型支持，适合 Web 开发。
- **Kotlin**: 基于协程和 OkHttp，提供 Flow 支持，适合 Android 或 JVM 后端。
- **PHP**: 框架无关，同步 API 设计，适合传统 Web 应用 (Laravel/WordPress 等)。
- **Swift**: 基于 `async/await` 和 `URLSession`，适合 iOS/macOS 原生应用。

### 核心操作
所有 SDK 共享相似的 API 接口，直接映射到 REST 终端：
- **write**: 写入记忆痕迹 (engram)。
- **activate**: 激活/召回相关记忆（语义搜索 + 图遍历）。
- **read**: 按 ID 读取特定记忆。
- **link**: 在两个记忆之间建立关联。
- **forget**: 遗忘（软删除或硬删除）记忆。
- **subscribe**: 订阅 SSE 推送，实时接收记忆更新。

## SDK 对比表

| 特性 | Go | Python | Node.js | Kotlin | PHP | Swift |
|------|-----|--------|---------|--------|-----|-------|
| 安装 | go get | pip install | npm install | Gradle | Composer | SPM |
| 异步模型 | context.Context | async/await | async/await | 协程 (suspend) | 同步 (curl) | async/await |
| SSE 订阅 | chan Push | SSEStream | AsyncIterable | Flow<SseEvent> | SseStream | AsyncStream |
| 重试策略 | 指数退避 | 指数退避 | 指数退避 | 指数退避 | 指数退避 | 指数退避 |
| 认证方式 | Bearer token | Bearer token | Bearer token | Bearer token | Bearer token | Bearer token |

---

## Go SDK

Go SDK 提供了一个简洁、类型安全的接口，充分利用了 Go 的并发特性。

### 安装
```bash
go get github.com/scrypster/muninndb/sdk/go/muninn
```

### 客户端创建
```go
import "github.com/scrypster/muninndb/sdk/go/muninn"

// 使用默认配置创建
client := muninn.NewClient("http://127.0.0.1:8475", "your-api-token")

// 或者使用自定义选项
client := muninn.NewClientWithOptions(
    "http://127.0.0.1:8475", 
    "token", 
    5 * time.Second, // 超时
    3,               // 最大重试
    500 * time.Millisecond, // 初始退避
)
```

### 核心方法签名
```go
func (c *Client) Write(ctx context.Context, vault, concept, content string, tags []string) (string, error)
func (c *Client) WriteBatch(ctx context.Context, vault string, engrams []WriteRequest) (*BatchWriteResponse, error)
func (c *Client) Read(ctx context.Context, id, vault string) (*Engram, error)
func (c *Client) Activate(ctx context.Context, vault string, context []string, maxResults int) (*ActivateResponse, error)
func (c *Client) Link(ctx context.Context, vault, sourceID, targetID string, relType int, weight float64) error
func (c *Client) Forget(ctx context.Context, id, vault string) error
func (c *Client) Evolve(ctx context.Context, vault, engramID, newContent, reason string) (*EvolveResponse, error)
func (c *Client) Consolidate(ctx context.Context, vault string, ids []string, mergedContent string) (*ConsolidateResponse, error)
func (c *Client) Decide(ctx context.Context, vault, decision, rationale string, alternatives, evidenceIDs []string) (*DecideResponse, error)
func (c *Client) Restore(ctx context.Context, id, vault string) (*RestoreResponse, error)
func (c *Client) Traverse(ctx context.Context, vault, startID string, maxHops, maxNodes int, relTypes []string, followEntities bool) (*TraverseResponse, error)
func (c *Client) Explain(ctx context.Context, vault, engramID string, query []string) (*ExplainResponse, error)
func (c *Client) SetState(ctx context.Context, vault, engramID, state, reason string) (*SetStateResponse, error)
func (c *Client) Subscribe(ctx context.Context, vault string) (<-chan Push, error)
func (c *Client) Health(ctx context.Context) (bool, error)
func (c *Client) Contradictions(ctx context.Context, vault string) (*ContradictionsResponse, error)
func (c *Client) Guide(ctx context.Context, vault string) (string, error)
```

### 使用示例
```go
// 写入记忆
id, err := client.Write(ctx, "default", "架构设计", "采用微服务架构，使用 gRPC 通信", []string{"arch", "internal"})

// 激活记忆
resp, err := client.Activate(ctx, "default", []string{"如何通信？"}, 5)
for _, act := range resp.Activations {
    fmt.Printf("召回概念: %s, 评分: %f\n", act.Concept, act.Score)
}

// 订阅更新
pushes, _ := client.Subscribe(ctx, "default")
go func() {
    for push := range pushes {
        fmt.Println("接收到新记忆更新:", push.EngramID)
    }
}()
```

---

## Python SDK

Python SDK 是为 AI 智能体和数据科学家设计的，提供了便捷的异步支持。

### 安装
```bash
pip install muninn-python
```

### 异步上下文管理器
```python
from muninn import MuninnClient

async with MuninnClient("http://127.0.0.1:8475", token="token") as client:
    # 自动处理连接池开启与关闭
    pass
```

### LangChain 集成
Python SDK 包含一个 `MuninnDBMemory` 类，可以直接作为 LangChain 的记忆组件：
```python
from muninn.langchain import MuninnDBMemory
from langchain.chains import ConversationChain

memory = MuninnDBMemory(vault="my-agent")
chain = ConversationChain(llm=my_llm, memory=memory)
# 每一轮对话都会自动存入 MuninnDB，并在下一轮自动检索相关上下文
```

### 核心方法签名
```python
async def write(self, vault="default", concept="", content="", tags=None, confidence=0.9, stability=0.5, ...) -> WriteResponse
async def activate(self, vault="default", context=None, max_results=10, threshold=0.1, max_hops=0, ...) -> ActivateResponse
async def read(self, id: str, vault: str = "default") -> ReadResponse
async def forget(self, id: str, vault: str = "default", hard: bool = False) -> bool
async def link(self, source_id, target_id, vault="default", rel_type=5, weight=1.0) -> bool
async def evolve(self, id, new_content, reason, vault="default") -> EvolveResponse
async def consolidate(self, ids, merged_content, vault="default") -> ConsolidateResponse
async def decide(self, decision, rationale, alternatives=None, evidence_ids=None, vault="default") -> DecideResponse
async def restore(self, id, vault="default") -> RestoreResponse
async def traverse(self, start_id, max_hops=2, max_nodes=20, rel_types=None, follow_entities=False, vault="default") -> TraverseResponse
async def explain(self, engram_id, query, vault="default") -> ExplainResponse
async def set_state(self, id, state, reason="", vault="default") -> SetStateResponse
def subscribe(self, vault="default", push_on_write=True, threshold=None) -> SSEStream
async def guide(self, vault="default") -> str
async def contradictions(self, vault="default") -> ContradictionsResponse
async def health(self) -> bool
```

### 使用示例
```python
# 语义召回
result = await client.activate(
    vault="default",
    context=["最近的技术讨论"],
    max_results=5,
    brief_mode="extractive"
)

# SSE 订阅
async for push in client.subscribe(vault="default"):
    print(f"实时推送记忆: {push.engram_id}")
```

---

## Node.js/TypeScript SDK

Node SDK 使用原生 `fetch` API，无需外部依赖，非常适合现代 TypeScript 环境。

### 安装
```bash
npm install @muninndb/client
```

### 客户端配置
```typescript
import { MuninnClient } from "@muninndb/client";

const client = new MuninnClient({
  baseUrl: "http://127.0.0.1:8475",
  token: "your-token",
  timeout: 30000,
  maxRetries: 3
});
```

### 核心方法签名
```typescript
async write(options: WriteOptions): Promise<WriteResponse>
async writeBatch(vault: string, engrams: WriteOptions[]): Promise<BatchWriteResponse>
async read(id: string, vault?: string): Promise<Engram>
async forget(id: string, vault?: string): Promise<void>
async activate(options: ActivateOptions): Promise<ActivateResponse>
async link(options: LinkOptions): Promise<void>
async evolve(id: string, newContent: string, reason: string, vault?: string): Promise<EvolveResponse>
async consolidate(options: ConsolidateOptions): Promise<ConsolidateResponse>
async decide(options: DecideOptions): Promise<DecideResponse>
async restore(id: string, vault?: string): Promise<RestoreResponse>
async traverse(options: TraverseOptions): Promise<TraverseResponse>
async explain(options: ExplainOptions): Promise<ExplainResponse>
async setState(id: string, state: string, reason?: string, vault?: string): Promise<SetStateResponse>
async listDeleted(vault?: string, limit?: number): Promise<ListDeletedResponse>
async retryEnrich(id: string, vault?: string): Promise<RetryEnrichResponse>
async contradictions(vault?: string): Promise<ContradictionsResponse>
async guide(vault?: string): Promise<string>
async stats(vault?: string): Promise<StatsResponse>
async listEngrams(vault?: string, limit?: number, offset?: number): Promise<ListEngramsResponse>
async getLinks(id: string, vault?: string): Promise<AssociationItem[]>
async listVaults(): Promise<string[]>
async session(vault?: string, since?: string, limit?: number, offset?: number): Promise<SessionResponse>
subscribe(vault?: string, pushOnWrite?: boolean, threshold?: number): AsyncIterable<SseEvent>
async health(): Promise<HealthResponse>
close(): void
```

### SSE 订阅
Node SDK 将 SSE 映射为 `AsyncIterable`：
```typescript
const events = client.subscribe("default");
for await (const event of events) {
  console.log("事件:", event.event, "数据:", event.data);
}
```

### 使用示例
```typescript
const { id } = await client.write({
  concept: "部署指南",
  content: "使用 Docker Compose 运行：docker-compose up -d",
  tags: ["devops"]
});

const { activations } = await client.activate({
  context: ["如何运行？"],
  limit: 3
});
```

---

## Kotlin SDK

Kotlin SDK 充分利用了协程 (Coroutines) 和 Flow，为 JVM 生态提供了一流的支持。

### Gradle 配置
```kotlin
dependencies {
    implementation("com.muninndb:client:1.0.0")
}
```

### 核心 API
```kotlin
val client = MuninnClient(baseUrl = "...", token = "...")

suspend fun write(options: WriteOptions): WriteResponse
suspend fun writeBatch(vault: String, engrams: List<WriteOptions>): BatchWriteResponse
suspend fun read(id: String, vault: String? = null): Engram
suspend fun forget(id: String, vault: String? = null, hard: Boolean = false)
suspend fun activate(options: ActivateOptions): ActivateResponse
suspend fun link(options: LinkOptions)
suspend fun traverse(options: TraverseOptions): TraverseResponse
suspend fun evolve(id: String, newContent: String, reason: String, vault: String? = null): EvolveResponse
suspend fun consolidate(ids: List<String>, mergedContent: String, vault: String? = null): ConsolidateResponse
suspend fun decide(options: DecideOptions): DecideResponse
suspend fun restore(id: String, vault: String? = null): RestoreResponse
suspend fun explain(options: ExplainOptions): ExplainResponse
suspend fun setState(id: String, state: String, reason: String? = null, vault: String? = null): SetStateResponse
suspend fun listDeleted(vault: String? = null, limit: Int? = null): ListDeletedResponse
suspend fun retryEnrich(id: String, vault: String? = null): RetryEnrichResponse
suspend fun contradictions(vault: String? = null): ContradictionsResponse
suspend fun guide(vault: String? = null): String
suspend fun stats(vault: String? = null): StatsResponse
suspend fun listEngrams(vault: String? = null, limit: Int? = null, offset: Int? = null): ListEngramsResponse
suspend fun getLinks(id: String, vault: String? = null): List<AssociationItem>
suspend fun listVaults(): List<String>
suspend fun session(vault: String? = null, since: String? = null, limit: Int? = null, offset: Int? = null): SessionResponse
fun subscribe(vault: String? = null, pushOnWrite: Boolean = true, threshold: Double? = null): Flow<SseEvent>
suspend fun health(): HealthResponse
```

// suspend 函数调用
val response = client.activate(ActivateOptions(context = listOf("查询词")))

// Flow 推送
client.subscribe("default").collect { event ->
    println("收到推送: ${event.data}")
}
```

### 使用示例
```kotlin
val engramId = client.write(WriteOptions(
    concept = "API 设计",
    content = "遵循 RESTful 规范",
    vault = "default"
)).id
```

---

## PHP SDK

PHP SDK 是同步设计的，非常适合在传统的 Web 服务流程中使用。

### 安装
```bash
composer require muninndb/client
```

### 核心方法签名
```php
public function write(string $content, string $concept = '', string $vault = 'default', ...) : WriteResponse
public function writeBatch(array $engrams, string $vault = 'default') : BatchWriteResponse
public function read(string $id, string $vault = 'default') : Engram
public function forget(string $id, string $vault = 'default', bool $hard = false) : void
public function activate(array $context, string $vault = 'default', ...) : ActivateResponse
public function link(string $sourceId, string $targetId, int $relType = 1, ...) : void
public function evolve(string $id, string $newContent, string $reason, ...) : EvolveResponse
public function consolidate(array $ids, string $mergedContent, ...) : ConsolidateResponse
public function decide(string $decision, string $rationale, ...) : DecideResponse
public function restore(string $id, string $vault = 'default') : RestoreResponse
public function traverse(string $startId, int $maxHops = 2, ...) : TraverseResponse
public function explain(string $engramId, array $query, ...) : ExplainResponse
public function setState(string $id, string $state, string $reason = '', ...) : SetStateResponse
public function listDeleted(string $vault = 'default', int $limit = 20) : ListDeletedResponse
public function retryEnrich(string $id, string $vault = 'default') : RetryEnrichResponse
public function contradictions(string $vault = 'default') : ContradictionsResponse
public function guide(string $vault = 'default') : string
public function stats(string $vault = 'default') : StatsResponse
public function listEngrams(string $vault = 'default', int $limit = 20, ...) : ListEngramsResponse
public function getLinks(string $id, string $vault = 'default') : array
public function listVaults() : array
public function session(string $vault = 'default', ...) : SessionResponse
public function subscribe(string $vault = 'default', bool $pushOnWrite = true) : SseStream
public function health() : HealthResponse
```

### 同步 API
```php
use MuninnDB\MuninnClient;

$client = new MuninnClient(token: 'secret-token');

// 写入
$res = $client->write(
    content: 'PHP 8.1 支持 readonly 属性',
    concept: 'PHP 特性'
);

// 激活
$activated = $client->activate(context: ['PHP 更新了什么？']);
```

### SSE 迭代
```php
foreach ($client->subscribe('default') as $event) {
    echo "收到更新: " . $event->engramId . "\n";
}
```

---

## Swift SDK

Swift SDK 支持现代的 `async/await` 模式，并与 Apple 的 `Combine` 和 `AsyncStream` 深度集成。

### 安装 (SPM)
在 `Package.swift` 中添加：
```swift
.package(url: "https://github.com/scrypster/muninndb-swift.git", from: "1.0.0")
```

### 核心方法签名
```swift
func write(_ options: WriteOptions) async throws -> WriteResponse
func writeBatch(vault: String? = nil, engrams: [WriteOptions]) async throws -> BatchWriteResponse
func read(_ id: String, vault: String? = nil) async throws -> Engram
func forget(_ id: String, vault: String? = nil, hard: Bool = false) async throws
func activate(_ options: ActivateOptions) async throws -> ActivateResponse
func link(_ options: LinkOptions) async throws
func traverse(_ options: TraverseOptions) async throws -> TraverseResponse
func evolve(id: String, newContent: String, reason: String, vault: String? = nil) async throws -> EvolveResponse
func consolidate(ids: [String], mergedContent: String, vault: String? = nil) async throws -> ConsolidateResponse
func decide(_ options: DecideOptions) async throws -> DecideResponse
func restore(_ id: String, vault: String? = nil) async throws -> RestoreResponse
func explain(_ options: ExplainOptions) async throws -> ExplainResponse
func setState(id: String, state: String, reason: String? = nil, vault: String? = nil) async throws -> SetStateResponse
func listDeleted(vault: String? = nil, limit: Int? = null) async throws -> ListDeletedResponse
func retryEnrich(_ id: String, vault: String? = nil) async throws -> RetryEnrichResponse
func contradictions(vault: String? = nil) async throws -> ContradictionsResponse
func guide(vault: String? = nil) async throws -> String
func stats(vault: String? = nil) async throws -> StatsResponse
func listEngrams(vault: String? = nil, limit: Int? = null, offset: Int? = null) async throws -> ListEngramsResponse
func getLinks(_ id: String, vault: String? = nil) async throws -> [AssociationItem]
func listVaults() async throws -> [String]
func session(vault: String? = nil, since: String? = nil, limit: Int? = null, offset: Int? = null) async throws -> SessionResponse
func subscribe(vault: String? = nil, pushOnWrite: Bool = true, threshold: Double? = nil) -> AsyncThrowingStream<SseEvent, Error>
func health() async throws -> HealthResponse
```

### 使用示例
```swift
let client = MuninnClient(token: "my-token")

// 异步读取
let engram = try await client.read("engram-id")

// SSE 流
for try await event in client.subscribe(vault: "default") {
    print("收到推送: \(event.data)")
}
```

---

## 通用模式

### 认证
所有 SDK 统一使用 `Authorization: Bearer <token>` 头部进行身份验证。

### 默认 Vault
如果没有明确指定 `vault` 参数，所有 SDK 默认都会请求 `default` 存储库。建议在多租户场景下为每个租户创建独立的 vault。

### 重试与退避
所有 SDK 都内置了针对 5xx (服务器错误) 和网络超时错误的自动重试逻辑：
- **默认重试次数**: 3 次。
- **退避算法**: 指数退避 (Exponential Backoff)，初始延迟通常为 500ms，并带有随机抖动 (Jitter) 以避免惊群效应。

### 错误处理
SDK 抛出的异常通常包含以下信息：
- **MuninnAuthError**: 401 认证失败。
- **MuninnNotFoundError**: 404 资源不存在。
- **MuninnServerError**: 5xx 服务器执行失败。
- **MuninnConnectionError**: 网络连接中断。

### SSE 订阅
SSE 流用于接收实时认知触发。当记忆在后台被强化、弱化或有新写入时，服务器会主动推送事件。
- **push_on_write**: 默认开启，当有新记忆写入时立即推送。
- **threshold**: 可选评分阈值，仅推送高于此权重的记忆更新。
