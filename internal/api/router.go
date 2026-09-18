package api

import (
	"net/http"

	"github.com/flowgo/flowgo-server/internal/ws"
)

// NewMux 注册全部 HTTP 路由。
func (s *Server) NewMux() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		n := 0
		if s.Hub != nil {
			n = s.Hub.ClientCount()
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"wsClients": n,
		})
	})
	if s.Hub != nil {
		mux.Handle("GET /api/ws", ws.Handler(s.Hub, s.Auth, s.Store))
	}

	mux.HandleFunc("POST /api/auth/login", s.HandleLogin)

	auth := s.Middleware
	mux.Handle("GET /api/auth/me", auth(http.HandlerFunc(s.HandleMe)))
	mux.Handle("POST /api/auth/password", auth(http.HandlerFunc(s.HandleChangePassword)))

	mux.Handle("GET /api/components", auth(http.HandlerFunc(s.HandleListComponents)))
	mux.Handle("GET /api/components/docs", auth(http.HandlerFunc(s.HandleListComponentDocs)))
	mux.Handle("POST /api/components/docs/reload", auth(http.HandlerFunc(s.HandleReloadComponentDocs)))
	mux.Handle("GET /api/components/{type}/doc", auth(http.HandlerFunc(s.HandleGetComponentDoc)))
	mux.Handle("GET /api/components/marketplace", auth(http.HandlerFunc(s.HandleListMarketplaceComponents)))
	mux.Handle("POST /api/components/marketplace/install", auth(http.HandlerFunc(s.HandleInstallMarketplaceComponent)))
	mux.Handle("POST /api/components/plugins/load", auth(http.HandlerFunc(s.HandleLoadComponentPlugin)))
	mux.Handle("PUT /api/components/plugins/{id}/enabled", auth(http.HandlerFunc(s.HandleSetPluginEnabled)))
	mux.Handle("DELETE /api/components/plugins/{id}", auth(http.HandlerFunc(s.HandleUninstallPlugin)))

	mux.Handle("GET /api/flows", auth(http.HandlerFunc(s.HandleListFlows)))
	mux.Handle("PUT /api/flows", auth(http.HandlerFunc(s.HandleSaveFlow)))
	mux.Handle("GET /api/flows/{id}", auth(http.HandlerFunc(s.HandleGetFlow)))
	mux.Handle("DELETE /api/flows/{id}", auth(http.HandlerFunc(s.HandleDeleteFlow)))
	mux.Handle("POST /api/flows/{id}/restore", auth(http.HandlerFunc(s.HandleRestoreFlow)))
	mux.Handle("PUT /api/flows/{id}/group", auth(http.HandlerFunc(s.HandleSetFlowGroup)))
	mux.Handle("PUT /api/flows/{id}/lock", auth(http.HandlerFunc(s.HandleSetFlowLocked)))
	mux.Handle("POST /api/flows/{id}/publish", auth(http.HandlerFunc(s.HandlePublishFlow)))
	mux.Handle("POST /api/flows/{id}/offline", auth(http.HandlerFunc(s.HandleUnpublishFlow)))
	mux.Handle("POST /api/flows/{id}/online", auth(http.HandlerFunc(s.HandleGoOnlineFlow)))
	mux.Handle("POST /api/flows/{id}/discard-draft", auth(http.HandlerFunc(s.HandleDiscardDraft)))
	mux.Handle("GET /api/flows/{id}/publish-history", auth(http.HandlerFunc(s.HandleListPublishHistory)))
	mux.Handle("DELETE /api/flows/{id}/publish-history/{version}", auth(http.HandlerFunc(s.HandleDeletePublishHistory)))
	mux.Handle("POST /api/flows/{id}/rollback", auth(http.HandlerFunc(s.HandleRollbackPublish)))
	mux.Handle("POST /api/flows/{id}/execute", auth(http.HandlerFunc(s.HandleExecuteFlow)))
	mux.Handle("POST /api/flows/{id}/execute-from", auth(http.HandlerFunc(s.HandleExecuteFromNode)))
	mux.Handle("POST /api/flows/{id}/debug/http-route", auth(http.HandlerFunc(s.HandleDebugHttpRoute)))
	mux.Handle("POST /api/flows/{id}/debug/inject", auth(http.HandlerFunc(s.HandleDebugInject)))
	mux.Handle("POST /api/flows/{id}/debug/http-client", auth(http.HandlerFunc(s.HandleDebugHttpClient)))
	mux.Handle("POST /api/flows/{id}/debug/js-transform", auth(http.HandlerFunc(s.HandleDebugJsTransform)))
	mux.Handle("POST /api/flows/{id}/debug/mqtt-probe", auth(http.HandlerFunc(s.HandleDebugMqttProbe)))

	mux.Handle("GET /api/flow-groups", auth(http.HandlerFunc(s.HandleListGroups)))
	mux.Handle("POST /api/flow-groups", auth(http.HandlerFunc(s.HandleCreateGroup)))
	mux.Handle("PUT /api/flow-groups/{id}", auth(http.HandlerFunc(s.HandleRenameGroup)))
	mux.Handle("DELETE /api/flow-groups/{id}", auth(http.HandlerFunc(s.HandleDeleteGroup)))

	mux.Handle("GET /api/users", auth(http.HandlerFunc(s.HandleListUsers)))
	mux.Handle("POST /api/users", auth(http.HandlerFunc(s.HandleCreateUser)))
	mux.Handle("PUT /api/users/{id}/flows", auth(http.HandlerFunc(s.HandleUpdateUserFlows)))

	mux.Handle("GET /api/apikeys", auth(http.HandlerFunc(s.HandleListAPIKeys)))
	mux.Handle("POST /api/apikeys", auth(http.HandlerFunc(s.HandleCreateAPIKey)))
	mux.Handle("DELETE /api/apikeys/{id}", auth(http.HandlerFunc(s.HandleDeleteAPIKey)))

	mux.Handle("GET /api/settings/mcp", auth(http.HandlerFunc(s.HandleGetMcpSettings)))
	mux.Handle("PUT /api/settings/mcp", auth(http.HandlerFunc(s.HandleSaveMcpSettings)))
	mux.Handle("GET /api/settings/components", auth(http.HandlerFunc(s.HandleGetComponentManage)))
	mux.Handle("PUT /api/settings/components", auth(http.HandlerFunc(s.HandleSaveComponentManage)))
	mux.Handle("GET /api/settings/http-response-templates", auth(http.HandlerFunc(s.HandleGetHttpResponseTemplates)))
	mux.Handle("PUT /api/settings/http-response-templates", auth(http.HandlerFunc(s.HandleSaveHttpResponseTemplates)))
	mux.Handle("POST /api/settings/http-response-templates", auth(http.HandlerFunc(s.HandleAddHttpResponseTemplate)))
	mux.Handle("PUT /api/settings/http-response-templates/{id}", auth(http.HandlerFunc(s.HandleUpdateHttpResponseTemplate)))
	mux.Handle("DELETE /api/settings/http-response-templates/{id}", auth(http.HandlerFunc(s.HandleDeleteHttpResponseTemplate)))

	return mux
}
