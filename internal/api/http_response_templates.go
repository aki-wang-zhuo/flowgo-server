package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo-server/internal/store"
)

func (s *Server) writeTemplateStoreErr(w http.ResponseWriter, r *http.Request, err error) {
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	switch {
	case errors.Is(err, store.ErrTemplateName):
		writeError(w, http.StatusBadRequest, types.PickI18n(map[string]string{
			types.LocaleEnUS: "Template name is required (max 64 characters)",
		}, locale, "模板名称必填（最多 64 字）"))
	case errors.Is(err, store.ErrTemplateStatus):
		writeError(w, http.StatusBadRequest, types.PickI18n(map[string]string{
			types.LocaleEnUS: "statusCode must be 100–599",
		}, locale, "状态码须为 100–599"))
	case errors.Is(err, store.ErrTemplateBody):
		writeError(w, http.StatusBadRequest, types.PickI18n(map[string]string{
			types.LocaleEnUS: "Template body is too large",
		}, locale, "响应体过大"))
	case errors.Is(err, store.ErrTemplateLimit):
		writeError(w, http.StatusBadRequest, types.PickI18n(map[string]string{
			types.LocaleEnUS: "Too many custom templates (max 50)",
		}, locale, "自定义模板过多（最多 50 条）"))
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, types.PickI18n(map[string]string{
			types.LocaleEnUS: "Template not found",
		}, locale, "模板不存在"))
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// HandleGetHttpResponseTemplates GET /api/settings/http-response-templates
func (s *Server) HandleGetHttpResponseTemplates(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	items, err := s.Store.GetHttpResponseTemplates(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleSaveHttpResponseTemplates PUT /api/settings/http-response-templates
func (s *Server) HandleSaveHttpResponseTemplates(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	var body struct {
		Items []store.HttpResponseTemplateItem `json:"items"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	items, err := s.Store.SaveHttpResponseTemplates(user.ID, body.Items)
	if err != nil {
		s.writeTemplateStoreErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// HandleAddHttpResponseTemplate POST /api/settings/http-response-templates
func (s *Server) HandleAddHttpResponseTemplate(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	var body struct {
		Name       string `json:"name"`
		StatusCode int    `json:"statusCode"`
		Body       string `json:"body"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.Store.AddHttpResponseTemplate(user.ID, body.Name, body.StatusCode, body.Body)
	if err != nil {
		s.writeTemplateStoreErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// HandleUpdateHttpResponseTemplate PUT /api/settings/http-response-templates/{id}
// 用当前 statusCode/body 覆盖已有自定义模板（名称不变）。
func (s *Server) HandleUpdateHttpResponseTemplate(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	var body struct {
		StatusCode int    `json:"statusCode"`
		Body       string `json:"body"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.Store.UpdateHttpResponseTemplate(user.ID, id, body.StatusCode, body.Body)
	if err != nil {
		s.writeTemplateStoreErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// HandleDeleteHttpResponseTemplate DELETE /api/settings/http-response-templates/{id}
func (s *Server) HandleDeleteHttpResponseTemplate(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	items, err := s.Store.DeleteHttpResponseTemplate(user.ID, id)
	if err != nil {
		s.writeTemplateStoreErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
