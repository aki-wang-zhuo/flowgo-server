# FlowGo Server

[中文文档](./README_ZH.md)

**FlowGo Server** is the deployable host for the FlowGo low-code platform. It exposes REST and WebSocket APIs, JWT / API Key / OAuth authentication, an MCP (Model Context Protocol) gateway for AI editors such as Cursor, local persistence, HTTP endpoint listeners for published flows, and out-of-process plugin hosting.

It **imports** the [flowgo](https://github.com/aki-wang-zhuo/flowgo) engine library and the [flowgo-node](https://github.com/aki-wang-zhuo/flowgo-node) plugin SDK. It does **not** implement built-in node logic or the Vue canvas UI.

| Related project | Role |
| --- | --- |
| [flowgo](https://github.com/aki-wang-zhuo/flowgo) | Core engine & built-in nodes |
| [flowgo-editor](https://github.com/aki-wang-zhuo/flowgo-editor) | Visual editor (Vue 3) |
| [flowgo-node](https://github.com/aki-wang-zhuo/flowgo-node) | Plugin SDK & example nodes |

---

## Features

- **REST API** (`/api/*`) — health, auth, flows (draft / publish / history / lock), execute, components, plugins, users, API keys, settings
- **WebSocket** (`/api/ws`) — editor presence and live canvas sync used by MCP (`get_active_flow` / `patch_active_flow` / `notify_editor`)
- **Auth** — JWT for editor sessions; API Key for MCP/scripts; OAuth 2.1 + PKCE for browser-based MCP clients
- **MCP gateway** — Streamable HTTP at `/mcp` ([mcp-go](https://github.com/mark3labs/mcp-go)); bilingual tool descriptions; fine-grained permission flags
- **Persistence** — [CloverDB](https://github.com/ostafen/clover) under `data/flowgo.db`
- **Plugin host** — loads executables from `data/plugins/` via `flowgo-node/sdk` and registers them into the engine registry
- **HTTP endpoints** — restores/listens for `httpEndpoint` nodes from **published** flows
- **Optional editor static hosting** — serves `./editor/` at `/editor/` when present

---

## Requirements

- Go **1.23+**
- Sibling checkouts for local `replace` (default monorepo layout):

```text
RuleGo/
├── flowgo/
├── flowgo-server/   ← this repo
├── flowgo-node/
└── flowgo-editor/   (optional, for UI)
```

`go.mod` replace directives:

```text
github.com/flowgo/flowgo           => ../flowgo
github.com/flowgo/flowgo-node/sdk  => ../flowgo-node/sdk
```

---

## Quick start

### Build & run (Windows)

```powershell
cd flowgo-server
.\build.ps1              # tidy + build flowgo-server.exe and start (foreground)
# or
.\build.ps1 -NoStart     # build only
.\start.ps1              # start previously built binary in background
```

Cross-compile Linux amd64:

```powershell
.\build.ps1 -linux       # produces flowgo-server (no auto-start)
```

### Default access

| Item | Default |
| --- | --- |
| Listen address | `:8090` (`FLOWGO_ADDR`) |
| Health | `GET http://127.0.0.1:8090/api/health` |
| Editor (if built) | `http://127.0.0.1:8090/editor/` |
| Admin user | `admin` / `admin` (created on first boot — **change immediately**) |

Working directory should be the repository root so relative paths `./data` and `./editor` resolve correctly.

---

## Configuration (environment)

| Variable | Default | Description |
| --- | --- | --- |
| `FLOWGO_ADDR` | `:8090` | HTTP listen address |
| `FLOWGO_DATA_DIR` | `./data` | Data root |
| `FLOWGO_DB_PATH` | `{DataDir}/flowgo.db` | CloverDB path |
| `FLOWGO_JWT_SECRET` | `flowgo-dev-secret-change-me` | **Must change in production** |
| `FLOWGO_TOKEN_TTL_HOURS` | `24` | Editor JWT TTL |
| `FLOWGO_PUBLIC_BASE_URL` | `http://127.0.0.1:8090` | Public base for OAuth / MCP metadata |
| `FLOWGO_OAUTH_ACCESS_TTL_MIN` | `60` | OAuth access token TTL |
| `FLOWGO_OAUTH_REFRESH_TTL_DAYS` | `30` | OAuth refresh token TTL |
| `FLOWGO_MCP_ENABLED` | `true` | Process-level MCP switch |
| `FLOWGO_CORS_ORIGIN` | `*` | CORS origin |

Client-side (for Cursor / scripts, not loaded by the server binary itself):

- `FLOWGO_API_KEY` — API key value referenced from MCP config headers

### HTTPS / production notes

- The management server uses plain `http.ListenAndServe` by default. **Put a reverse proxy with HTTPS in front for production**, especially for MCP OAuth.
- Local development over HTTP is supported.
- Flow-level `httpEndpoint` nodes may configure their own TLS certificates; that is independent of the `:8090` admin port.
- OAuth dynamic clients / refresh tokens are currently **in-memory** — restart requires re-authorization.

---

## Repository layout

```text
flowgo-server/
├── cmd/server/main.go
├── internal/
│   ├── api/             # REST handlers
│   ├── app/             # Flow executor (calls flowgo engine)
│   ├── auth/            # JWT / API Key
│   ├── componentdocs/   # Component Markdown docs
│   ├── config/          # Env-based config
│   ├── endpoint/        # Published httpEndpoint listeners
│   ├── mcp/             # MCP tools & gateway
│   ├── oauth/           # MCP OAuth 2.1 + PKCE
│   ├── pluginhost/      # Plugin process manager
│   ├── store/           # CloverDB
│   └── ws/              # WebSocket hub
├── scripts/             # MCP helper scripts
├── build.ps1 / start.ps1
├── mcp.example.txt      # Cursor MCP setup notes
├── data/                # runtime (gitignored)
└── editor/              # editor static build (gitignored)
```

---

## Authentication

| Method | Typical use |
| --- | --- |
| `Authorization: Bearer <JWT>` | Editor login session |
| `X-API-Key: <key>` or `Authorization: ApiKey <key>` | MCP, automation scripts |
| OAuth 2.1 + PKCE | Browser-based MCP clients (Cursor Connect) |

Create an API key via the REST API after login (`POST /api/apikeys`). The secret is shown **once** — store it in your environment, never commit it.

---

## MCP (Cursor)

Streamable HTTP endpoint: `http://127.0.0.1:8090/mcp`

### Recommended: API Key

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

### Alternative: OAuth

Configure only the `url` (no headers). Cursor will open a browser for authorization. Prefer HTTPS behind a reverse proxy in production.

### Tools (examples)

`list_flows`, `get_flow`, `get_active_flow`, `patch_active_flow`, `save_flow`, `publish_flow`, `discard_draft`, `delete_flow`, `execute_flow`, `execute_from_node`, `list_components`, `get_component_doc`, `unlock_flow`, `notify_editor`, …

Permissions are configurable per user (flow read/create/update/delete/execute, unlock, notify editor, …). The editor must be connected over WebSocket for active-canvas tools.

More detail: see `mcp.example.txt` in this repository.

---

## API overview

| Area | Examples |
| --- | --- |
| Health | `GET /api/health` |
| Auth | login / me / change password |
| Flows | CRUD, groups, lock, publish, discard-draft, history, rollback |
| Execute | `/api/.../execute`, execute-from (published) |
| Components | list, docs, marketplace install, plugin load/enable/uninstall |
| Settings | MCP flags, component admin, HTTP response templates |
| WebSocket | `GET /api/ws?token=<JWT>` |
| OAuth metadata | `/.well-known/oauth-protected-resource`, `/.well-known/oauth-authorization-server` |

---

## Data & build artifacts

| Path | Purpose | In git? |
| --- | --- | --- |
| `data/flowgo.db/` | CloverDB | No |
| `data/plugins/` | Installed plugin binaries | No |
| `data/docs/` | Component / plugin Markdown | No |
| `editor/` | Copied from `flowgo-editor` build | No |
| `*.exe` / binaries | Build output | No |

To serve the UI from the server:

```powershell
cd ../flowgo-editor
.\build.ps1    # builds and copies dist → ../flowgo-server/editor
```

---

## Architecture

```text
Browser / Cursor
      │
      ▼
flowgo-server  ──import──►  flowgo (engine)
      │
      ├── REST / JWT / API Key / OAuth
      ├── WebSocket (editor ↔ MCP)
      ├── CloverDB persistence
      └── pluginhost ──RPC──► flowgo-node plugins
```

Module boundary: new **built-in** nodes belong in `flowgo`; new **host** features (auth, MCP, storage) belong here; UI belongs in `flowgo-editor`.

---

## License

License file is not yet published in this repository. Contact the maintainers if you need redistributable terms.

---

## Security checklist

- [ ] Change default `admin` password
- [ ] Set a strong `FLOWGO_JWT_SECRET`
- [ ] Do not commit `.env` or API keys
- [ ] Terminate TLS at a reverse proxy for public MCP OAuth
- [ ] Restrict `FLOWGO_CORS_ORIGIN` in production
