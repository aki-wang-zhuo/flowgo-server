package api

import (
	"net/http"

	"github.com/flowgo/flowgo-server/internal/store"
)

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordReq struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// HandleLogin POST /api/auth/login
func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	token, user, err := s.Auth.Login(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"flowIds":  user.FlowIDs,
		},
	})
}

// HandleMe GET /api/auth/me
func (s *Server) HandleMe(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":       user.ID,
		"username": user.Username,
		"role":     user.Role,
		"flowIds":  user.FlowIDs,
	})
}

// HandleChangePassword POST /api/auth/password
func (s *Server) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	var req changePasswordReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !store.CheckPassword(user.PasswordHash, req.OldPassword) {
		writeError(w, http.StatusBadRequest, "old password incorrect")
		return
	}
	if len(req.NewPassword) < 4 {
		writeError(w, http.StatusBadRequest, "new password too short")
		return
	}
	if err := s.Store.UpdatePassword(user.ID, req.NewPassword); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
