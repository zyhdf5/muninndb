# MuninnDB Web 管理界面技术文档

## 概述
MuninnDB Web 管理界面是一个集成了记忆管理、知识图谱可视化、系统监控和配置管理的现代化控制面板。它作为 MuninnDB 生态系统中的重要组成部分，为用户提供了一个直观的图形化界面，用于探索认知引擎存储的记忆轨迹（Engrams）、观察实体间的语义联系以及监控系统的运行状态。

该 Web 界面被设计为低延迟、响应式且易于部署。所有的前端资源在构建阶段被优化后，通过 Go 的 `embed` 机制直接嵌入到主二进制文件中，实现了“单文件部署，零配置运行”的极致体验。

## 架构图
以下展示了前端资源从开发构建到最终集成进 Go 二进制文件的全流程：

```text
[ 源代码 ] (HTML/JS/CSS/Assets)
    ↓
[ 构建工具: Vite ] (打包、压缩、Tree-shaking)
    ↓
[ 静态资源: static/dist/ ] (app.css, 压缩后的 JS)
    ↓
[ Go 嵌入层: embed.go ] (使用 //go:embed 编译指令)
    ↓
[ Go 编译器 ] (将资源链接进机器码)
    ↓
[ MuninnDB 二进制 ] (含 Web 服务器与静态资源)
```

## 技术栈说明

### a. Vite
作为构建工具和开发服务器。在本项目中，Vite 主要负责处理 Tailwind CSS 的编译，并将多个 CSS 层级合并压缩。其配置文件 `vite.config.js` 指定了 `static/css/app.css` 作为入口，并将输出重定向到 `static/dist/`。

### b. Tailwind CSS
采用工具类优先（Utility-first）的 CSS 框架。通过 `tailwind.config.js` 配置，它会自动扫描 `templates/` 中的 HTML 文件和 `static/js/` 中的脚本，生成仅包含所需样式的最小化 CSS 文件。

### c. Alpine.js
轻量级 JavaScript 响应式框架。用于驱动 Web 界面的大部分交互逻辑（如侧边栏切换、模态框弹出、视图切换等）。它通过 `x-data` 和 `x-init` 属性直接在 HTML 中定义状态，避免了大型框架的复杂性。

### d. Cytoscape.js
高性能图可视化库。专门用于“图谱（Graph）”视图中展示记忆节点和实体之间的关联。它支持复杂的布局算法（如 `fcose`），能够清晰地呈现出知识图谱的拓扑结构。

### e. Chart.js
强大的图表库。用于仪表板（Dashboard）中的统计展示，例如活跃度趋势、存储增长情况等。

### f. PostCSS
CSS 后处理器。配合 `autoprefixer` 插件，确保生成的样式在不同浏览器中具有良好的兼容性。

## 核心原理

### a. Go embed 嵌入机制
在 `web/embed.go` 中，通过以下指令将资源嵌入：
```go
//go:embed static templates
var FS embed.FS
```
这使得 Go 语言可以在运行时直接从内存中读取 HTML 模板和静态资源，无需依赖外部文件系统路径。

### b. 静态资源组织
- `static/css/`: 存放按层级组织的原始样式文件。
- `static/js/`: 存放业务逻辑代码，如 `app.js`。
- `static/vendor/`: 存放第三方预编译库（如 `alpine.min.js`, `chart.min.js`），以减少构建负担。
- `templates/`: 存放 Go 模板文件（如 `index.html`）。

### c. CSS 架构
样式采用模块化层级组织：
- `theme.css`: 定义 CSS 变量（颜色、字体、间距）。
- `base.css`: 全局基础样式重置。
- `components.css`: 自定义组件样式（按钮、卡片、侧边栏）。
- `app.css`: 最终入口，使用 `@import` 整合上述层级并注入 `@tailwind` 指令。

### d. JavaScript 架构
- `app.js`: 核心逻辑。负责 Alpine.js 组件定义、API 请求封装、图表初始化等。
- `plugin-config-utils.js`: 插件配置工具函数。作为 ES Module 加载，处理动态配置注入。

### e. HTML 模板
使用标准的 `html/template` 包。模板中大量集成了 Alpine.js 的指令，并引用了 `/static/dist/app.css` 作为样式入口。

### f. 第三方库管理
采用 `vendor/` 目录策略，将核心依赖库直接内置。这种做法保证了在离线环境下的构建稳定性，同时也简化了 CDN 依赖管理。

## 构建流程参考
构建主要依赖 `npm` 脚本：
- **本地开发**: `npm run dev`（启动 Vite 热更新服务器）。
- **生产构建**: `npm run build`（执行 `vite build` 生成 `static/dist/` 下的压缩资源）。
- **Go 编译**: `make build` 或 `go build -tags localassets`（将构建好的资源打入二进制）。

## 目录结构参考
```text
web/
├── embed.go             # Go 嵌入指令定义
├── package.json         # Node.js 依赖与脚本
├── postcss.config.js    # PostCSS 配置
├── tailwind.config.js   # Tailwind 扫描路径与主题配置
├── vite.config.js       # Vite 构建参数
├── templates/           # Go HTML 模板
│   └── index.html       # 主入口模板
└── static/              # 静态资源
    ├── css/             # 原始 CSS
    ├── dist/            # 构建产物 (app.css)
    ├── js/              # 业务 JavaScript
    └── vendor/          # 第三方库
```

## 最佳实践
1. **资源优化**: 始终通过 `npm run build` 压缩 CSS。Tailwind 的 JIT 引擎会根据 HTML 实际使用的 class 来剔除无用代码，大幅减小体积。
2. **CSS 组织**: 遵循 `base` -> `components` -> `theme` 的导入顺序，确保样式覆盖逻辑清晰。
3. **版本管理**: 第三方库（vendor）应明确标注版本号，并在升级后进行全路径回归测试。
4. **构建缓存**: 在 CI/CD 中缓存 `node_modules` 以加速构建过程。
5. **主题适配**: 利用 Tailwind 的 `dark` 模式支持，结合 HTML 的 `class="dark"` 实现一键换肤，并确保在渲染前应用主题以防止闪烁（FOUC）。

## 开发工作流
1. **环境准备**: 确保已安装 Go 1.25+ 和 Node.js。
2. **启动后端**: 在项目根目录运行 `go run cmd/muninn/main.go start`。
3. **启动前端**: 进入 `web/` 目录，运行 `npm run dev`。此时前端会通过 Vite 代理将 API 请求转发到 8475 端口。
4. **代码修改**: 修改 CSS 或 JS 后，浏览器会自动热重载（HMR）。
5. **最终发布**: 运行 `npm run build`，然后使用 `go build -tags localassets` 编译最终程序。

## 常见错误与规避
- **样式未生效**: 检查 `tailwind.config.js` 中的 `content` 数组是否包含了新创建的 HTML 路径。
- **Go 嵌入失败**: 确保运行 `go build` 时身处正确的目录，且 `static/dist/` 下已有构建产物。
- **Alpine.js 冲突**: 避免在同一个元素上混合使用多个复杂的 `x-data`。尽量将逻辑拆分为小的组件函数并定义在 `app.js` 中。
