package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo-server/internal/api"
	"github.com/flowgo/flowgo-server/internal/auth"
	"github.com/flowgo/flowgo-server/internal/oauth"
	"github.com/flowgo/flowgo-server/internal/store"
	"github.com/flowgo/flowgo-server/internal/ws"
)

// Gateway 将 MCP 挂在 flowgo-server 同进程，所有工具调用需鉴权。
type Gateway struct {
	Auth      *auth.Service
	Store     *store.Store
	Exec      api.FlowExecutor
	Hub       *ws.Hub
	Endpoints api.EndpointSync
	OAuth     *oauth.Server // 可选：MCP OAuth；非空时 401 带 WWW-Authenticate
	server    *mcpserver.MCPServer
}

// New 创建 MCP 网关并注册工具。
func New(authSvc *auth.Service, st *store.Store, exec api.FlowExecutor, hub *ws.Hub, endpoints api.EndpointSync, oauthSrv *oauth.Server) *Gateway {
	g := &Gateway{
		Auth:      authSvc,
		Store:     st,
		Exec:      exec,
		Hub:       hub,
		Endpoints: endpoints,
		OAuth:     oauthSrv,
		server: mcpserver.NewMCPServer(
			"flowgo",
			"0.1.0",
			mcpserver.WithToolCapabilities(true),
		),
	}
	g.registerTools()
	return g
}

// Handler 返回带鉴权的 MCP HTTP 处理器（Streamable HTTP）。
func (g *Gateway) Handler() http.Handler {
	inner := mcpserver.NewStreamableHTTPServer(g.server)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resource := ""
		if g.OAuth != nil {
			resource = g.OAuth.ResourceURL(r)
		}
		user, err := g.Auth.AuthenticateRequestForAudience(r, resource)
		if err != nil {
			if g.OAuth != nil {
				g.OAuth.WriteUnauthorized(w, r)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		enabled, err := g.Store.IsMcpEnabled(user.ID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !enabled {
			http.Error(w, "mcp disabled", http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), api.UserCtxKey(), user)
		// 将 Accept-Language 写入上下文，供 list_components / get_component_doc 本地化
		locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
		ctx = context.WithValue(ctx, localeCtxKey{}, locale)
		inner.ServeHTTP(w, r.WithContext(ctx))
	})
}

// registerTools 注册 MCP 工具。
// 工具清单与权限分组的展示文案以 store.McpCapabilityCatalog 为准；此处变更后请同步更新该目录。
func (g *Gateway) registerTools() {
	g.server.AddTool(mcp.NewTool("list_components",
		mcp.WithDescription("列出当前用户可用的流程节点（已启用）。返回 type/label/description；详细用法请再调 get_component_doc"),
		mcp.WithString("locale", mcp.Description("可选语言：zh-CN 或 en-US；默认跟随 Accept-Language / zh-CN")),
	), g.toolListComponents)

	g.server.AddTool(mcp.NewTool("get_component_doc",
		mcp.WithDescription("获取指定节点的完整文档：description、usage、configFields、relationTypes，供 AI 正确编写 FlowDSL 节点 configuration"),
		mcp.WithString("type", mcp.Required(), mcp.Description("节点类型，如 jsTransform、httpEndpoint")),
		mcp.WithString("locale", mcp.Description("可选语言：zh-CN 或 en-US；默认跟随 Accept-Language / zh-CN")),
	), g.toolGetComponentDoc)

	g.server.AddTool(mcp.NewTool("list_flows",
		mcp.WithDescription("列出当前用户可访问的流程图"),
	), g.toolListFlows)

	g.server.AddTool(mcp.NewTool("get_flow",
		mcp.WithDescription("获取指定流程图 DSL"),
		mcp.WithString("id", mcp.Required(), mcp.Description("流程 ID")),
	), g.toolGetFlow)

	g.server.AddTool(mcp.NewTool("get_active_flow",
		mcp.WithDescription("获取当前用户已打开编辑器中正在编辑的流程（含未保存画布 DSL 与 revision）；需编辑器在线且已建立 WebSocket"),
		mcp.WithBoolean("includeDsl", mcp.Description("是否返回当前画布 DSL（含未保存修改），默认 true")),
	), g.toolGetActiveFlow)

	g.server.AddTool(mcp.NewTool("patch_active_flow",
		mcp.WithDescription("增量修改编辑器中正在编辑的流程（只传变更的节点/边/名称等，勿传整份 DSL）。修改画布后返回 applied 变更摘要与 revision；需编辑器在线。"),
		mcp.WithString("patch", mcp.Required(), mcp.Description(`增量补丁 JSON。仅传变更字段。节点 op：update|add|remove（默认 update）；边 op：add|remove|update。移动节点请用 x/y（编辑器会整体移动外壳与文案，勿只改其中一个）。示例：{"nodes":[{"id":"n1","y":200}]}`)),
		mcp.WithString("flowId", mcp.Description("可选；若提供须与当前激活流程 ID 一致")),
		mcp.WithBoolean("includeDsl", mcp.Description("是否附带补丁后的完整 DSL，默认 true")),
	), g.toolPatchActiveFlow)

	g.server.AddTool(mcp.NewTool("save_flow",
		mcp.WithDescription("保存流程图（JSON DSL 字符串）；新建需 flowCreate，更新需 flowUpdate"),
		mcp.WithString("dsl", mcp.Required(), mcp.Description("FlowDSL JSON")),
	), g.toolSaveFlow)

	g.server.AddTool(mcp.NewTool("delete_flow",
		mcp.WithDescription("删除流程图"),
		mcp.WithString("id", mcp.Required(), mcp.Description("流程 ID")),
	), g.toolDeleteFlow)

	g.server.AddTool(mcp.NewTool("execute_flow",
		mcp.WithDescription("从流程入口（entryNode）执行流程图"),
		mcp.WithString("id", mcp.Required(), mcp.Description("流程 ID")),
		mcp.WithString("data", mcp.Description("输入 JSON 字符串")),
		mcp.WithString("type", mcp.Description("消息类型，默认 DEFAULT")),
	), g.toolExecuteFlow)

	g.server.AddTool(mcp.NewTool("execute_from_node",
		mcp.WithDescription("从指定节点开始执行流程图（跳过 entryNode；nodeId 为 DSL 中节点 id）"),
		mcp.WithString("id", mcp.Required(), mcp.Description("流程 ID")),
		mcp.WithString("nodeId", mcp.Required(), mcp.Description("起始节点 ID（DSL nodes[].id）")),
		mcp.WithString("data", mcp.Description("输入 JSON 字符串，作为进入该节点的消息体")),
		mcp.WithString("type", mcp.Description("消息类型，默认 DEFAULT")),
	), g.toolExecuteFromNode)

	g.server.AddTool(mcp.NewTool("unlock_flow",
		mcp.WithDescription("解锁已锁定的流程图；需 flowUnlock 权限（默认关闭）。password 可空。若返回 unlock failed: wrong password，必须向用户索取解锁密码后重试，且不得记忆、存储或复述该密码。"),
		mcp.WithString("id", mcp.Required(), mcp.Description("流程 ID")),
		mcp.WithString("password", mcp.Description("解锁密码；未设密码时可空。仅用于本次调用，勿持久化")),
	), g.toolUnlockFlow)

	g.server.AddTool(mcp.NewTool("notify_editor",
		mcp.WithDescription("通过 WebSocket 通知已打开的编辑器执行动作：refresh_canvas / reload_flows / open_flow"),
		mcp.WithString("action", mcp.Required(), mcp.Description("refresh_canvas | reload_flows | open_flow")),
		mcp.WithString("flowId", mcp.Description("流程 ID（refresh_canvas / open_flow 时建议提供）")),
	), g.toolNotifyEditor)
}

func userFromMCP(ctx context.Context) (*store.User, error) {
	u := api.UserFromContext(ctx)
	if u == nil {
		return nil, fmt.Errorf("unauthorized")
	}
	return u, nil
}

// localeCtxKey 在 MCP HTTP 入口写入的 Accept-Language 解析结果。
type localeCtxKey struct{}

// resolveToolLocale 优先工具参数 locale，其次请求上下文，最后默认中文。
func resolveToolLocale(ctx context.Context, req mcp.CallToolRequest) string {
	if v, _ := req.GetArguments()["locale"].(string); strings.TrimSpace(v) != "" {
		return types.NormalizeLocale(v)
	}
	if v, ok := ctx.Value(localeCtxKey{}).(string); ok && v != "" {
		return types.NormalizeLocale(v)
	}
	return types.DefaultLocale
}

func (g *Gateway) toolListFlows(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowRead)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	list, err := g.Store.ListFlows(user)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.Marshal(list)
	return mcp.NewToolResultText(string(b)), nil
}

func (g *Gateway) toolGetFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowRead)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	rec, err := g.Store.GetFlow(id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.Marshal(rec)
	return mcp.NewToolResultText(string(b)), nil
}

// toolGetActiveFlow 通过 WebSocket 询问编辑器当前激活 Tab（可含未保存 DSL）。
func (g *Gateway) toolGetActiveFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowRead)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if g.Hub == nil {
		return mcp.NewToolResultError("websocket hub not ready"), nil
	}
	includeDSL := true
	if v, ok := req.GetArguments()["includeDsl"].(bool); ok {
		includeDSL = v
	}
	reply, err := g.Hub.QueryActiveFlow(user.ID, includeDSL, 0)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.Marshal(reply)
	return mcp.NewToolResultText(string(b)), nil
}

// toolPatchActiveFlow 通过 WebSocket 对编辑器当前激活流程做增量修改。
func (g *Gateway) toolPatchActiveFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowUpdate)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if g.Hub == nil {
		return mcp.NewToolResultError("websocket hub not ready"), nil
	}
	raw, err := req.RequireString("patch")
	if err != nil || strings.TrimSpace(raw) == "" {
		return mcp.NewToolResultError("patch is required"), nil
	}
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return mcp.NewToolResultError("invalid patch json"), nil
	}
	flowID, _ := req.GetArguments()["flowId"].(string)
	includeDSL := true
	if v, ok := req.GetArguments()["includeDsl"].(bool); ok {
		includeDSL = v
	}
	reply, err := g.Hub.PatchActiveFlow(user.ID, strings.TrimSpace(flowID), json.RawMessage(raw), includeDSL, 0)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.Marshal(reply)
	return mcp.NewToolResultText(string(b)), nil
}

func (g *Gateway) toolSaveFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := userFromMCP(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	raw, _ := req.RequireString("dsl")
	var dsl types.FlowDSL
	if err := json.Unmarshal([]byte(raw), &dsl); err != nil {
		return mcp.NewToolResultError("invalid dsl json"), nil
	}
	if dsl.ID == "" {
		dsl.ID = types.NewID()
	}
	existing, getErr := g.Store.GetFlow(dsl.ID)
	isNew := getErr != nil || existing == nil
	if isNew {
		if _, err := g.requireMCPPerm(ctx, permFlowCreate); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	} else {
		if _, err := g.requireMCPPerm(ctx, permFlowUpdate); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !user.CanAccessFlow(dsl.ID) {
			return mcp.NewToolResultError("forbidden"), nil
		}
		if existing.Locked {
			return mcp.NewToolResultError("flow is locked"), nil
		}
	}

	// 保存前校验 HTTP 路由冲突（与 API 一致，冲突不落库）
	if g.Endpoints != nil {
		if err := g.Endpoints.CheckHTTPRoutes(dsl.ID, &dsl); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	rec, err := g.Store.SaveFlow(user.ID, &dsl)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	// HTTP 入口同步由 API 层统一处理；MCP 保存后同样需要
	if g.Endpoints != nil {
		if err := g.Endpoints.SyncFlow(rec); err != nil {
			return mcp.NewToolResultError("saved but http endpoint failed: " + err.Error()), nil
		}
	}
	if g.Exec != nil {
		g.Exec.InvalidateFlow(rec.ID)
	}
	if g.Hub != nil {
		g.Hub.NotifyFlowChanged("saved", rec.ID, rec.Name, "mcp")
	}
	b, _ := json.Marshal(rec)
	return mcp.NewToolResultText(string(b)), nil
}

func (g *Gateway) toolDeleteFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowDelete)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	if rec, err := g.Store.GetFlow(id); err == nil && rec != nil && rec.Locked {
		return mcp.NewToolResultError("flow is locked"), nil
	}
	if err := g.Store.DeleteFlow(id); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if g.Endpoints != nil {
		g.Endpoints.RemoveFlow(id)
	}
	if g.Exec != nil {
		g.Exec.InvalidateFlow(id)
	}
	if g.Hub != nil {
		g.Hub.NotifyFlowChanged("deleted", id, "", "mcp")
	}
	return mcp.NewToolResultText(`{"status":"ok"}`), nil
}

func (g *Gateway) toolExecuteFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowExecute)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	data, _ := req.GetArguments()["data"].(string)
	msgType, _ := req.GetArguments()["type"].(string)
	if msgType == "" {
		msgType = "DEFAULT"
	}
	out, err := g.Exec.ExecuteFlow(ctx, id, msgType, data)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// toolExecuteFromNode 从指定节点开始执行，受 flowExecute 权限约束。
func (g *Gateway) toolExecuteFromNode(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowExecute)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	nodeID, err := req.RequireString("nodeId")
	if err != nil || nodeID == "" {
		return mcp.NewToolResultError("nodeId is required"), nil
	}
	data, _ := req.GetArguments()["data"].(string)
	msgType, _ := req.GetArguments()["type"].(string)
	if msgType == "" {
		msgType = "DEFAULT"
	}
	out, err := g.Exec.ExecuteFromNode(ctx, id, nodeID, msgType, data)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

// toolUnlockFlow 解锁流程；需 flowUnlock 权限；密码错误则失败且不解锁。
func (g *Gateway) toolUnlockFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowUnlock)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	password, _ := req.GetArguments()["password"].(string)
	rec, err := g.Store.SetFlowLocked(id, false, password)
	if err != nil {
		if errors.Is(err, store.ErrUnlockFailed) {
			// 明确指导 AI：向用户索取密码并重试，且不得记录密码
			return mcp.NewToolResultError(
				"unlock failed: wrong password. Ask the user for the unlock password and call unlock_flow again; do not store, remember, or repeat the password.",
			), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}
	if g.Hub != nil {
		g.Hub.NotifyFlowChanged("lock", rec.ID, rec.Name, "mcp")
	}
	b, _ := json.Marshal(rec)
	return mcp.NewToolResultText(string(b)), nil
}

func (g *Gateway) toolNotifyEditor(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if _, err := g.requireMCPPerm(ctx, permNotifyEditor); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	action, _ := req.RequireString("action")
	switch action {
	case "refresh_canvas", "reload_flows", "open_flow":
	default:
		return mcp.NewToolResultError("action must be refresh_canvas | reload_flows | open_flow"), nil
	}
	flowID, _ := req.GetArguments()["flowId"].(string)
	if g.Hub == nil {
		return mcp.NewToolResultError("websocket hub not ready"), nil
	}
	g.Hub.NotifyEditorCommand(action, flowID)
	return mcp.NewToolResultText(`{"status":"ok"}`), nil
}
