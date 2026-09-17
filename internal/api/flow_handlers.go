package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo-server/internal/app"
	"github.com/flowgo/flowgo-server/internal/store"
)

// HandleListFlows GET /api/flows
func (s *Server) HandleListFlows(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	list, err := s.Store.ListFlows(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// HandleGetFlow GET /api/flows/{id}
func (s *Server) HandleGetFlow(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	rec, err := s.Store.GetFlow(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// HandleSaveFlow PUT /api/flows  body: FlowDSL
func (s *Server) HandleSaveFlow(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	var dsl types.FlowDSL
	if err := decodeJSON(r, &dsl); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if dsl.ID == "" {
		dsl.ID = types.NewID()
	}
	if existing, err := s.Store.GetFlow(dsl.ID); err == nil && existing != nil {
		if !user.CanAccessFlow(dsl.ID) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		if existing.Locked {
			writeError(w, http.StatusConflict, "flow is locked")
			return
		}
	}

	// 保存前校验 HTTP 路由：同端口可共享，同端口+同路径则拒绝且不落库
	if s.Endpoints != nil {
		if err := s.Endpoints.CheckHTTPRoutes(dsl.ID, &dsl); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}

	rec, err := s.Store.SaveFlow(user.ID, &dsl)
	if errors.Is(err, store.ErrFlowLocked) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if user.Role != store.RoleAdmin {
		ids := append([]string{}, user.FlowIDs...)
		found := false
		for _, id := range ids {
			if id == dsl.ID {
				found = true
				break
			}
		}
		if !found {
			ids = append(ids, dsl.ID)
			_ = s.Store.UpdateUserFlows(user.ID, ids)
			user.FlowIDs = ids
		}
	}
	if s.Endpoints != nil {
		if err := s.Endpoints.SyncFlow(rec); err != nil {
			writeError(w, http.StatusBadRequest, "flow saved but http endpoint failed: "+err.Error())
			return
		}
	}
	if s.Exec != nil {
		s.Exec.InvalidateFlow(rec.ID)
	}
	if s.Hub != nil {
		s.Hub.NotifyFlowChanged("saved", rec.ID, rec.Name, "api")
	}
	writeJSON(w, http.StatusOK, rec)
}

// HandleDeleteFlow DELETE /api/flows/{id}
func (s *Server) HandleDeleteFlow(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.Store.DeleteFlow(id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if errors.Is(err, store.ErrFlowLocked) {
		writeError(w, http.StatusConflict, err.Error())
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.Endpoints != nil {
		s.Endpoints.RemoveFlow(id)
	}
	if s.Exec != nil {
		s.Exec.InvalidateFlow(id)
	}
	if s.Hub != nil {
		s.Hub.NotifyFlowChanged("deleted", id, "", "api")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type setFlowLockedReq struct {
	Locked   bool   `json:"locked"`
	Password string `json:"password"` // 锁定时可设（可空）；解锁时若已设密码则必填正确
}

// HandleSetFlowLocked PUT /api/flows/{id}/lock
// 切换流程锁定；锁定后禁止保存 DSL 与删除（含 MCP）。解锁密码错误返回 403。
func (s *Server) HandleSetFlowLocked(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req setFlowLockedReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	rec, err := s.Store.SetFlowLocked(id, req.Locked, req.Password)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrUnlockFailed) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.Hub != nil {
		s.Hub.NotifyFlowChanged("lock", rec.ID, rec.Name, "api")
	}
	writeJSON(w, http.StatusOK, rec)
}

type executeReq struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type executeFromNodeReq struct {
	NodeID string `json:"nodeId"`
	Type   string `json:"type"`
	Data   string `json:"data"`
}

// HandleExecuteFlow POST /api/flows/{id}/execute
func (s *Server) HandleExecuteFlow(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req executeReq
	_ = decodeJSON(r, &req)
	if req.Type == "" {
		req.Type = "DEFAULT"
	}
	if s.Exec == nil {
		writeError(w, http.StatusServiceUnavailable, "executor not ready")
		return
	}
	out, err := s.Exec.ExecuteFlow(r.Context(), id, req.Type, req.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"data": out})
}

// HandleExecuteFromNode POST /api/flows/{id}/execute-from
// 从指定节点开始执行（不经过 entryNode），供调试与 MCP 使用。
func (s *Server) HandleExecuteFromNode(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req executeFromNodeReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.NodeID) == "" {
		writeError(w, http.StatusBadRequest, "nodeId is required")
		return
	}
	if req.Type == "" {
		req.Type = "DEFAULT"
	}
	if s.Exec == nil {
		writeError(w, http.StatusServiceUnavailable, "executor not ready")
		return
	}
	out, err := s.Exec.ExecuteFromNode(r.Context(), id, req.NodeID, req.Type, req.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"data": out})
}

// HandleDebugHttpRoute POST /api/flows/{id}/debug/http-route
// 使用路径调试值模拟 HTTP 请求并执行后续节点。
func (s *Server) HandleDebugHttpRoute(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.Exec == nil {
		writeError(w, http.StatusServiceUnavailable, "executor not ready")
		return
	}
	var req app.SimulateHttpRouteReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := s.Exec.SimulateHttpRoute(r.Context(), id, req)
	if out == nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 即使下游节点报错，也把已采集的 logs 返回给控制台
	status := http.StatusOK
	resp := map[string]any{
		"data": out.Data,
		"logs": out.Logs,
		"meta": out.Meta,
	}
	if err != nil {
		status = http.StatusOK // 业务错误仍 200，带 error 字段
		resp["error"] = err.Error()
	}
	writeJSON(w, status, resp)
}

// HandleDebugInject POST /api/flows/{id}/debug/inject
// 用注入节点 payload 作为消息体，从该节点执行后续链路。
func (s *Server) HandleDebugInject(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.Exec == nil {
		writeError(w, http.StatusServiceUnavailable, "executor not ready")
		return
	}
	var req app.SimulateInjectReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := s.Exec.SimulateInject(r.Context(), id, req)
	if out == nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := http.StatusOK
	resp := map[string]any{
		"data": out.Data,
		"logs": out.Logs,
		"meta": out.Meta,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, status, resp)
}

// HandleDebugHttpClient POST /api/flows/{id}/debug/http-client
// 用节点 debugValue 作为实际请求体（不走 body 模板），从该 HTTP 客户端节点执行后续链路。
func (s *Server) HandleDebugHttpClient(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.Exec == nil {
		writeError(w, http.StatusServiceUnavailable, "executor not ready")
		return
	}
	var req app.SimulateHttpClientReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := s.Exec.SimulateHttpClient(r.Context(), id, req)
	if out == nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := http.StatusOK
	resp := map[string]any{
		"data": out.Data,
		"logs": out.Logs,
		"meta": out.Meta,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, status, resp)
}

// HandleDebugJsTransform POST /api/flows/{id}/debug/js-transform
// 用节点 debugValue 作为脚本 msg 入参，从该 JS 转换节点执行后续链路。
func (s *Server) HandleDebugJsTransform(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if !user.CanAccessFlow(id) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.Exec == nil {
		writeError(w, http.StatusServiceUnavailable, "executor not ready")
		return
	}
	var req app.SimulateJsTransformReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := s.Exec.SimulateJsTransform(r.Context(), id, req)
	if out == nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := http.StatusOK
	resp := map[string]any{
		"data": out.Data,
		"logs": out.Logs,
		"meta": out.Meta,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, status, resp)
}
