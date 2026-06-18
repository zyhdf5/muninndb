# MuninnDB Web UI 后端服务 (codedb/ui)

`codedb/ui` 包实现了 MuninnDB 的 Web 管理界面后端服务器。它是一个集成了 HTTP 服务、SSE 实时推送、Cookie 认证、CORS 处理及嵌入式静态资源的综合性组件。

## 概述

Web UI 后端服务作为 MuninnDB 的可视化管理入口，不仅提供传统的 REST API 代理和静态资源分发，还通过 Server-Sent Events (SSE) 实现了认知事件（如记忆添加、系统状态更新）的实时推送。它被设计为单进程运行，无需外部 Web 服务器即可提供完整的管理体验。

## 架构图

```text
                                  +-------------------+
                                  |   Muninn Engine   |
                                  +---------+---------+
                                            | (API Call / Stats)
                                            v
+-------------------+           +-----------+-----------+
|      Client       | <-------+ |      SSE Hub          | (Real-time Events)
| (Browser / SPA)   |   (SSE)   | (Connection Manager)  |
+---------+---------+           +-----------+-----------+
          |                                 ^
          | (HTTP/HTTPS)                    | (Broadcast)
          v                                 |
+---------+---------+           +-----------+-----------+
|   HTTP Server     | +-------> |    Server Logic       |
| (Mux / Middleware)|           | (Handlers / Auth)     |
+-------------------+           +-----------------------+
          |                                 |
          +---------------------------------+
                    (Static Assets / Templates)
```

## 核心原理

### 1. HTTP 服务器设计
服务器基于 Go 标准库 `http.ServeMux` 构建，采用了清晰的路由分发机制：
- **静态资源路由**：`/static/` 映射到嵌入的静态文件系统。
- **认证路由**：处理管理员登录 (`/api/auth/login`) 与注销 (`/api/auth/logout`)。
- **实时事件**：`/events` 提供 SSE 长期连接。
- **API 代理**：`/api/` 下的所有请求（除认证外）被转发给核心 API 处理器。
- **SPA 路由**：所有未匹配的路径默认返回 `index.html`，支持单页应用的客户端路由。

### 2. SSE 实时推送 (sseHub)
SSE 实现基于 `sseHub` 结构，负责管理所有活跃的流式连接：
- **连接管理**：客户端访问 `/events` 时，`sseHub` 为其分配一个缓冲通道并加入订阅列表。
- **实时广播**：服务器内部的 `broadcaster` 协程每 5 秒轮询一次引擎状态，通过 `hub.broadcast` 将 `stats_update`、`workers_update` 和 `memory_added` 事件推送到所有客户端。
- **心跳机制**：每 20 秒发送一次空注释作为心跳，防止连接被中间代理（如 Nginx）断开。

### 3. 认证机制
采用基于 Cookie 的轻量级认证：
- **存储**：管理员凭据存储在共享的 Pebble 数据库中，密码通过 bcrypt 加密。
- **会话**：登录成功后，服务器下发加密签名的 `muninn_session` Cookie。
- **中间件**：`AdminAPIMiddleware` 拦截受保护路由，验证 Cookie 的有效性与过期时间（默认 24 小时）。

### 4. CORS 跨域处理
服务器内置了细粒度的 CORS 处理逻辑：
- 根据配置的 `corsOrigins` 列表验证请求的 `Origin` 头。
- 自动处理 `OPTIONS` 预检请求。
- 正确设置 `Access-Control-Allow-Credentials` 为 `true`，以支持跨域携带 Cookie。

### 5. TLS 支持
通过 `tls.Config` 可选开启 TLS。若配置了证书，服务器将自动升级为 HTTPS，并为会话 Cookie 设置 `Secure` 标志。

### 6. 嵌入式静态资源
利用 `io/fs.FS` 接口集成前端资源：
- **webFS**：包含编译后的前端代码、图片和 CSS。
- **tmplFS**：提供 SPA 的入口 HTML 模版。
这种设计确保了 MuninnDB 能够以单个二进制文件的形式分发，无需部署额外的前端静态文件。

### 7. 模版渲染
虽然主要作为 SPA 后端，但服务器仍保留了基础的模版加载能力，用于向前端注入动态配置或直接渲染初始页面环境。

## 公共 API 参考

### `Server` 结构体
核心服务器结构，管理监听器、Hub、路由和状态。

### 导出方法
- `NewServer(webFS, engine, apiHandler, authStore, sessionSecret, ring, tlsConfig, corsOrigins) (*Server, error)`: 初始化服务器。
- `Start(ctx, addr) error`: 在指定地址开始监听并运行。
- `Stop(ctx) error`: 优雅关闭服务器。
- `Addr() string`: 返回服务器实际监听的地址（Start 后有效）。
- `Broadcast(data []byte)`: 手动向所有 SSE 客户端广播数据。
- `ServeHTTP(w, r)`: 实现 `http.Handler` 接口，便于集成和测试。

## 配置参考

| 配置项 | 说明 | 默认值 |
| :--- | :--- | :--- |
| `Addr` | 监听地址及端口 | `:8476` |
| `TLSConfig` | TLS 配置对象 | `nil` (禁用) |
| `SessionSecret` | 会话签名密钥 | 32 字节随机数 |
| `CORSOrigins` | 允许的跨域来源列表 | `[]` |
| `AuthStore` | 认证持久化存储 | 必需 |

## 最佳实践

1. **启用 TLS**：在生产环境中务必配置 TLS，以确保认证 Cookie 不被窃听。
2. **连接限制**：SSE 会消耗长连接，建议在反向代理（如 Nginx）层限制单个 IP 的最大连接数，防止资源耗尽。
3. **安全 Secret**：`SessionSecret` 应在首次运行阶段生成并持久化，不应在代码中硬编码。
4. **SSE 缓冲管理**：在高负载环境下，客户端消费过慢会导致消息被丢弃。可以通过监控日志中的丢包情况调整 `sseHub` 的通道缓冲区大小。
5. **CORS 收敛**：尽量指定具体的允许域名，而非使用通配符，以增强安全性。

## 代码示例

### 初始化并启动 UI 服务器

```go
// 准备嵌入式文件系统和引擎
webFS := os.DirFS("./web/dist")
engine := engine.New(...)
authStore := auth.NewStore(db)

// 创建服务器
srv, _ := ui.NewServer(
    webFS, 
    engine, 
    apiHandler, 
    authStore, 
    []byte("your-secure-secret"), 
    logRing, 
    nil, 
    []string{"http://localhost:3000"},
)

// 启动
ctx, cancel := context.WithCancel(context.Background())
go srv.Start(ctx, ":8476")
```

## 常见错误与规避

- **SSE 连接频繁断开**：
  - *原因*：负载均衡器或 Nginx 的超时设置过短。
  - *规避*：配置 `proxy_read_timeout` 为较长时间，并确保服务器心跳周期小于代理超时。
- **登录后 API 仍报 401**：
  - *原因*：Cookie 域名或路径配置不匹配，或者浏览器禁用了第三方 Cookie。
  - *规避*：检查 `Path` 设置及请求头的 `Cookie` 字段，确保前端与后端处于同一父域下。
- **静态资源加载 404**：
  - *原因*：`webFS` 根路径指向不正确。
  - *规避*：确保 `webFS` 的结构中包含 `static/` 目录。
