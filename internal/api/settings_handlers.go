package api

import (
	"net/http"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo-server/internal/store"
)

// HandleGetMcpSettings GET /api/settings/mcp
// 返回当前用户权限开关，以及后端维护的 capabilities 目录（按 Accept-Language 本地化）。
func (s *Server) HandleGetMcpSettings(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	cfg, err := s.Store.GetMcpSettings(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"userId":       cfg.UserID,
		"enabled":      cfg.Enabled,
		"permissions":  cfg.Permissions,
		"capabilities": store.McpCapabilityCatalogLocale(locale),
	})
}

// HandleSaveMcpSettings PUT /api/settings/mcp
func (s *Server) HandleSaveMcpSettings(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	var body struct {
		Enabled     *bool                `json:"enabled"`
		Permissions store.McpPermissions `json:"permissions"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	} else {
		// 未传 enabled 时保留原值
		if prev, err := s.Store.GetMcpSettings(user.ID); err == nil {
			enabled = prev.Enabled
		}
	}
	cfg, err := s.Store.SaveMcpSettings(user.ID, enabled, body.Permissions)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 关闭 MCP：立即断开该用户编辑器 WebSocket
	if !cfg.Enabled && s.Hub != nil {
		s.Hub.DisconnectUser(user.ID)
	}
	writeJSON(w, http.StatusOK, cfg)
}
