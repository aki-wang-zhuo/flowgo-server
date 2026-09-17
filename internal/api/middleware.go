package api

import (
	"context"
	"io"
	"net/http"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo-server/internal/app"
	"github.com/flowgo/flowgo-server/internal/auth"
	"github.com/flowgo/flowgo-server/internal/store"
	"github.com/flowgo/flowgo-server/internal/ws"
)

type ctxKey int

const userCtxKey ctxKey = 1

// UserCtxKey 返回 context key，供 MCP 等包写入同一用户上下文。
func UserCtxKey() interface{} { return userCtxKey }

// Server HTTP API 聚合。
type Server struct {
	Auth      *auth.Service
	Store     *store.Store
	Exec      FlowExecutor
	Hub       *ws.Hub
	Endpoints EndpointSync // 可选：HTTP 入口随流程启停
	Plugins   PluginInstaller
}

// PluginInstaller 本地节点插件安装与生命周期（方案 B）。
type PluginInstaller interface {
	InstallFile(fileName string, r io.Reader, locale string) (id string, types []string, err error)
	SetEnabled(pluginID string, enabled bool, locale string) error
	Uninstall(pluginID string, locale string) error
	// ForEachInstalled 遍历已安装插件（含停用）；defs 来自安装时缓存。
	ForEachInstalled(fn func(id string, disabled bool, defs []types.ComponentDef))
}

// EndpointSync 流程保存/删除时同步 HTTP 入口监听。
type EndpointSync interface {
	// CheckHTTPRoutes 保存前校验同端口同路径冲突；冲突返回错误且不应落库。
	CheckHTTPRoutes(flowID string, dsl *types.FlowDSL) error
	SyncFlow(rec *store.FlowRecord) error
	RemoveFlow(flowID string)
}

// FlowExecutor 流程执行抽象，避免 api 包直接依赖引擎细节。
type FlowExecutor interface {
	ExecuteFlow(ctx context.Context, flowID string, msgType, data string) (resultData string, err error)
	// ExecuteFromNode 从指定节点开始执行（跳过 entryNode）。
	ExecuteFromNode(ctx context.Context, flowID, nodeID, msgType, data string) (resultData string, err error)
	SimulateHttpRoute(ctx context.Context, flowID string, req app.SimulateHttpRouteReq) (*app.SimulateHttpRouteResult, error)
	SimulateInject(ctx context.Context, flowID string, req app.SimulateInjectReq) (*app.SimulateInjectResult, error)
	SimulateHttpClient(ctx context.Context, flowID string, req app.SimulateHttpClientReq) (*app.SimulateHttpClientResult, error)
	SimulateJsTransform(ctx context.Context, flowID string, req app.SimulateJsTransformReq) (*app.SimulateJsTransformResult, error)
	// InvalidateFlow 丢弃指定流程的已编译节点缓存（保存/删除后调用）。
	InvalidateFlow(flowID string)
}

// Middleware 鉴权中间件：注入 *store.User 到 context。
func (s *Server) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.Auth.AuthenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserFromContext 取出当前用户。
func UserFromContext(ctx context.Context) *store.User {
	u, _ := ctx.Value(userCtxKey).(*store.User)
	return u
}

// CORS 简易跨域中间件。
func CORS(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, MCP-Protocol-Version, Accept, Accept-Language")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
