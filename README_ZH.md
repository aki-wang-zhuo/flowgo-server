# FlowGo Server

[English](./README.md)

**FlowGo Server** 是 FlowGo 低代码平台的可部署服务端。提供 REST 与 WebSocket API、JWT / API Key / OAuth 鉴权、面向 Cursor 等 AI 编辑器的 MCP 网关、本地持久化、已发布流程的 HTTP 入口监听，以及进程外插件托管。

本仓库通过 **import** 使用 [flowgo](https://github.com/aki-wang-zhuo/flowgo) 引擎库与 [flowgo-node](https://github.com/aki-wang-zhuo/flowgo-node) 插件 SDK。**不**实现内置节点逻辑，也**不**包含 Vue 画布 UI。

| 相关项目 | 定位 |
| --- | --- |
| [flowgo](https://github.com/aki-wang-zhuo/flowgo) | 核心引擎与内置节点 |
| [flowgo-editor](https://github.com/aki-wang-zhuo/flowgo-editor) | 可视化编辑器（Vue 3） |
| [flowgo-node](https://github.com/aki-wang-zhuo/flowgo-node) | 插件 SDK 与示例节点 |

---

## 特性

- **REST API**（`/api/*`）— 健康检查、鉴权、流程（草稿/发布/历史/锁定）、执行、组件、插件、用户、API Key、设置
- **WebSocket**（`/api/ws`）— 编辑器在线与画布实时同步，供 MCP 的 `get_active_flow` / `patch_active_flow` / `notify_editor` 使用
- **鉴权** — 编辑器会话用 JWT；MCP/脚本用 API Key；浏览器 MCP 客户端可用 OAuth 2.1 + PKCE
- **MCP 网关** — `/mcp` Streamable HTTP（[mcp-go](https://github.com/mark3labs/mcp-go)）；工具说明双语；细粒度权限开关
- **持久化** — [CloverDB](https://github.com/ostafen/clover)，目录 `data/flowgo.db`
- **插件托管** — 从 `data/plugins/` 加载可执行文件（`flowgo-node/sdk`），注册到引擎 Registry
- **HTTP 入口** — 恢复/监听**已发布**流程中的 `httpEndpoint` 节点
- **可选编辑器静态托管** — 存在 `./editor/` 时挂载到 `/editor/`

---

## 环境要求

- Go **1.23+**
- 本地 `replace` 所需的同级目录（默认 monorepo 布局）：

```text
RuleGo/
├── flowgo/
├── flowgo-server/   ← 本仓库
├── flowgo-node/
└── flowgo-editor/   （可选，用于 UI）
```

`go.mod` 中的 replace：

```text
github.com/flowgo/flowgo           => ../flowgo
github.com/flowgo/flowgo-node/sdk  => ../flowgo-node/sdk
```

---

## 快速开始

### 构建与运行（Windows）

```powershell
cd flowgo-server
.\build.ps1              # tidy + 编译 flowgo-server.exe 并前台启动
# 或
.\build.ps1 -NoStart     # 仅编译
.\start.ps1              # 后台启动已编译二进制
```

交叉编译 Linux amd64：

```powershell
.\build.ps1 -linux       # 产出 flowgo-server（不自动启动）
```

### 默认访问

| 项 | 默认值 |
| --- | --- |
| 监听地址 | `:8090`（`FLOWGO_ADDR`） |
| 健康检查 | `GET http://127.0.0.1:8090/api/health` |
| 编辑器（若已构建） | `http://127.0.0.1:8090/editor/` |
| 管理员 | `admin` / `admin`（首次启动自动创建 — **请立刻修改**） |

工作目录应为仓库根目录，以便相对路径 `./data`、`./editor` 正确解析。

---

## 配置（环境变量）

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `FLOWGO_ADDR` | `:8090` | HTTP 监听地址 |
| `FLOWGO_DATA_DIR` | `./data` | 数据根目录 |
| `FLOWGO_DB_PATH` | `{DataDir}/flowgo.db` | CloverDB 路径 |
| `FLOWGO_JWT_SECRET` | `flowgo-dev-secret-change-me` | **生产环境必须修改** |
| `FLOWGO_TOKEN_TTL_HOURS` | `24` | 编辑器 JWT 有效期（小时） |
| `FLOWGO_PUBLIC_BASE_URL` | `http://127.0.0.1:8090` | OAuth / MCP 元数据对外基址 |
| `FLOWGO_OAUTH_ACCESS_TTL_MIN` | `60` | OAuth access token 有效期（分钟） |
| `FLOWGO_OAUTH_REFRESH_TTL_DAYS` | `30` | OAuth refresh token 有效期（天） |
| `FLOWGO_MCP_ENABLED` | `true` | 进程级 MCP 总开关 |
| `FLOWGO_CORS_ORIGIN` | `*` | CORS 来源 |

客户端侧（Cursor / 脚本使用，非服务端二进制直接读取）：

- `FLOWGO_API_KEY` — 写入本机环境，供 MCP 配置里的请求头引用

### HTTPS / 生产说明

- 管理端口默认使用 `http.ListenAndServe`。**生产环境请在反向代理后启用 HTTPS**，尤其是 MCP OAuth。
- 本机开发可使用 HTTP。
- 流程中的 `httpEndpoint` 可单独配置 TLS 证书，与 `:8090` 管理端口无关。
- OAuth 动态客户端 / refresh token 当前为**内存态** — 重启后需重新授权。

---

## 目录结构

```text
flowgo-server/
├── cmd/server/main.go
├── internal/
│   ├── api/             # REST
│   ├── app/             # 流程执行（调用 flowgo 引擎）
│   ├── auth/            # JWT / API Key
│   ├── componentdocs/   # 组件 Markdown 文档
│   ├── config/          # 环境变量配置
│   ├── endpoint/        # 已发布 httpEndpoint 监听
│   ├── mcp/             # MCP 工具与网关
│   ├── oauth/           # MCP OAuth 2.1 + PKCE
│   ├── pluginhost/      # 插件进程管理
│   ├── store/           # CloverDB
│   └── ws/              # WebSocket Hub
├── scripts/             # MCP 辅助脚本
├── build.ps1 / start.ps1
├── mcp.example.txt      # Cursor MCP 配置说明
├── data/                # 运行时（gitignore）
└── editor/              # 编辑器静态构建（gitignore）
```

---

## 鉴权

| 方式 | 典型用途 |
| --- | --- |
| `Authorization: Bearer <JWT>` | 编辑器登录会话 |
| `X-API-Key: <key>` 或 `Authorization: ApiKey <key>` | MCP、自动化脚本 |
| OAuth 2.1 + PKCE | 浏览器 MCP 客户端（Cursor Connect） |

登录后通过 REST 创建 API Key（`POST /api/apikeys`）。密钥**只显示一次** — 请存入环境变量，切勿提交到 Git。

---

## MCP（Cursor）

Streamable HTTP 地址：`http://127.0.0.1:8090/mcp`

### 推荐：API Key

```json
{
  "mcpServers": {
    "FlowGo": {
      "url": "http://127.0.0.1:8090/mcp",
      "headers": {
        "X-API-Key": "${env:FLOWGO_API_KEY}"
      }
    }
  }
}
```

### 备选：OAuth

仅配置 `url`（不写 headers）。Cursor 会打开浏览器完成授权。生产环境建议经反向代理使用 HTTPS。

### 工具示例

`list_flows`、`get_flow`、`get_active_flow`、`patch_active_flow`、`save_flow`、`publish_flow`、`discard_draft`、`delete_flow`、`execute_flow`、`execute_from_node`、`list_components`、`get_component_doc`、`unlock_flow`、`notify_editor` …

权限可按用户配置（流程读写删执行、解锁、通知编辑器等）。操作当前画布的工具要求编辑器已通过 WebSocket 连接。

更多说明见仓库内 `mcp.example.txt`。

---

## API 概览

| 领域 | 示例 |
| --- | --- |
| 健康检查 | `GET /api/health` |
| 鉴权 | 登录 / me / 修改密码 |
| 流程 | CRUD、分组、锁定、发布、放弃草稿、历史、回滚 |
| 执行 | execute、execute-from（已发布） |
| 组件 | 列表、文档、市场安装、插件加载/启用/卸载 |
| 设置 | MCP 开关、组件管理、HTTP 响应模板 |
| WebSocket | `GET /api/ws?token=<JWT>` |
| OAuth 元数据 | `/.well-known/oauth-protected-resource` 等 |

---

## 数据与构建产物

| 路径 | 用途 | 是否入库 |
| --- | --- | --- |
| `data/flowgo.db/` | CloverDB | 否 |
| `data/plugins/` | 已安装插件二进制 | 否 |
| `data/docs/` | 组件/插件 Markdown | 否 |
| `editor/` | 来自 `flowgo-editor` 的构建拷贝 | 否 |
| `*.exe` / 二进制 | 编译产物 | 否 |

由服务端托管 UI：

```powershell
cd ../flowgo-editor
.\build.ps1    # 构建并复制 dist → ../flowgo-server/editor
```

---

## 架构

```text
浏览器 / Cursor
      │
      ▼
flowgo-server  ──import──►  flowgo（引擎）
      │
      ├── REST / JWT / API Key / OAuth
      ├── WebSocket（编辑器 ↔ MCP）
      ├── CloverDB 持久化
      └── pluginhost ──RPC──► flowgo-node 插件
```

模块边界：**内置节点**放 `flowgo`；**宿主能力**（鉴权、MCP、存储）放本仓库；**UI** 放 `flowgo-editor`。

---

## 许可证

本仓库尚未发布 LICENSE 文件。如需再分发条款，请联系维护者。

---

## 安全检查清单

- [ ] 修改默认 `admin` 密码
- [ ] 设置高强度 `FLOWGO_JWT_SECRET`
- [ ] 勿将 `.env` 或 API Key 提交入库
- [ ] 公网 MCP OAuth 经反向代理终止 TLS
- [ ] 生产环境收紧 `FLOWGO_CORS_ORIGIN`
