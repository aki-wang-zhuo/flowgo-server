package api

import (
	"net/http"

	"github.com/flowgo/flowgo-server/internal/store"
)

type createUserReq struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Role     string   `json:"role"`
	FlowIDs  []string `json:"flowIds"`
}

type updateFlowsReq struct {
	FlowIDs []string `json:"flowIds"`
}

// HandleListUsers GET /api/users （仅 admin）
func (s *Server) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user.Role != store.RoleAdmin {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	list, err := s.Store.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type item struct {
		ID        string   `json:"id"`
		Username  string   `json:"username"`
		Role      string   `json:"role"`
		FlowIDs   []string `json:"flowIds"`
		CreatedAt string   `json:"createdAt"`
	}
	out := make([]item, 0, len(list))
	for _, u := range list {
		out = append(out, item{
			ID: u.ID, Username: u.Username, Role: u.Role,
			FlowIDs: u.FlowIDs, CreatedAt: u.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// HandleCreateUser POST /api/users （仅 admin）
func (s *Server) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user.Role != store.RoleAdmin {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	var req createUserReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	created, err := s.Store.CreateUser(req.Username, req.Password, req.Role, req.FlowIDs)
	if err == store.ErrConflict {
		writeError(w, http.StatusConflict, "username exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id": created.ID, "username": created.Username, "role": created.Role, "flowIds": created.FlowIDs,
	})
}

// HandleUpdateUserFlows PUT /api/users/{id}/flows （仅 admin）
func (s *Server) HandleUpdateUserFlows(w http.ResponseWriter, r *http.Request) {
	actor := UserFromContext(r.Context())
	if actor.Role != store.RoleAdmin {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	var req updateFlowsReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.Store.UpdateUserFlows(r.PathValue("id"), req.FlowIDs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
