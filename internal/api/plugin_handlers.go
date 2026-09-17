package api

import (
	"net/http"
	"strings"

	"github.com/flowgo/flowgo/api/types"
)

// HandleLoadComponentPlugin POST /api/components/plugins/load
func (s *Server) HandleLoadComponentPlugin(w http.ResponseWriter, r *http.Request) {
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if s.Plugins == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"status": "not_implemented",
			"message": types.PickI18n(map[string]string{
				types.LocaleEnUS: "Plugin manager is not enabled",
			}, locale, "插件管理器未启用"),
		})
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart: "+err.Error())
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, types.PickI18n(map[string]string{
			types.LocaleEnUS: "missing file field",
		}, locale, "缺少 file 字段"))
		return
	}
	defer file.Close()
	name := hdr.Filename
	if name == "" {
		name = "plugin"
	}
	id, typeNames, err := s.Plugins.InstallFile(name, file, locale)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"message": types.PickI18n(map[string]string{types.LocaleEnUS: "Plugin loaded"}, locale, "插件已加载"),
		"id":      id,
		"types":   typeNames,
	})
}

// HandleSetPluginEnabled PUT /api/components/plugins/{id}/enabled
// body: { "enabled": true|false }
func (s *Server) HandleSetPluginEnabled(w http.ResponseWriter, r *http.Request) {
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if s.Plugins == nil {
		writeError(w, http.StatusNotImplemented, types.PickI18n(map[string]string{
			types.LocaleEnUS: "Plugin manager is not enabled",
		}, locale, "插件管理器未启用"))
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.Plugins.SetEnabled(id, body.Enabled, locale); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	msgZh := "插件已停用（文件保留，重启后不会加载）"
	msgEn := "Plugin disabled (files kept; will not load on restart)"
	if body.Enabled {
		msgZh = "插件已启用并加载"
		msgEn = "Plugin enabled and loaded"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"enabled": body.Enabled,
		"message": types.PickI18n(map[string]string{types.LocaleEnUS: msgEn}, locale, msgZh),
	})
}

// HandleUninstallPlugin DELETE /api/components/plugins/{id}
func (s *Server) HandleUninstallPlugin(w http.ResponseWriter, r *http.Request) {
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if s.Plugins == nil {
		writeError(w, http.StatusNotImplemented, types.PickI18n(map[string]string{
			types.LocaleEnUS: "Plugin manager is not enabled",
		}, locale, "插件管理器未启用"))
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := s.Plugins.Uninstall(id, locale); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"message": types.PickI18n(map[string]string{
			types.LocaleEnUS: "Plugin uninstalled and files removed",
		}, locale, "插件已卸载并删除文件"),
	})
}
