package api

import (
	"errors"
	"net/http"

	"github.com/flowgo/flowgo-server/internal/store"
)

// HandleListGroups GET /api/flow-groups
func (s *Server) HandleListGroups(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type groupNameReq struct {
	Name string `json:"name"`
}

// HandleCreateGroup POST /api/flow-groups
func (s *Server) HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req groupNameReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	rec, err := s.Store.CreateGroup(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// HandleRenameGroup PUT /api/flow-groups/{id}
func (s *Server) HandleRenameGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	var req groupNameReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	rec, err := s.Store.RenameGroup(id, req.Name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrSystemGroup) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// HandleDeleteGroup DELETE /api/flow-groups/{id}
// 分组内流程会移入「未分组」，不会删除流程本身。
func (s *Server) HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := s.Store.DeleteGroup(id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if errors.Is(err, store.ErrSystemGroup) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type setFlowGroupReq struct {
	GroupID string `json:"groupId"`
}

// HandleSetFlowGroup PUT /api/flows/{id}/group
func (s *Server) HandleSetFlowGroup(w http.ResponseWriter, r *http.Request) {
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
	var req setFlowGroupReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	rec, err := s.Store.SetFlowGroup(id, req.GroupID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrCannotMoveToTrash) || errors.Is(err, store.ErrMustRestoreFromTrash) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.Hub != nil {
		s.Hub.NotifyFlowChanged("group", rec.ID, rec.Name, "api")
	}
	writeJSON(w, http.StatusOK, rec)
}
