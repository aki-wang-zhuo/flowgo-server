package api

import (
	"net/http"

	"github.com/flowgo/flowgo-server/internal/store"
)

type createKeyReq struct {
	Name string `json:"name"`
}

// HandleListAPIKeys GET /api/apikeys
func (s *Server) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	list, err := s.Store.ListAPIKeys(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// HandleCreateAPIKey POST /api/apikeys
func (s *Server) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	var req createKeyReq
	_ = decodeJSON(r, &req)
	if req.Name == "" {
		req.Name = "default"
	}
	rec, plain, err := s.Store.CreateAPIKey(user.ID, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"key":    plain,
		"record": rec,
	})
}

// HandleDeleteAPIKey DELETE /api/apikeys/{id}
func (s *Server) HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	err := s.Store.DeleteAPIKey(id, user.ID, user.Role == store.RoleAdmin)
	if err == store.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
