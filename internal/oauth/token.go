package oauth

import (
	"net/http"
	"strings"
	"time"

	"github.com/flowgo/flowgo-server/internal/store"
)

// handleToken OAuth token 端点：authorization_code / refresh_token。
func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "cannot parse form")
		return
	}
	grant := r.FormValue("grant_type")
	switch grant {
	case "authorization_code":
		s.tokenFromCode(w, r)
	case "refresh_token":
		s.tokenFromRefresh(w, r)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code and refresh_token")
	}
}

func (s *Server) tokenFromCode(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	verifier := r.FormValue("code_verifier")
	if code == "" || redirectURI == "" || clientID == "" || verifier == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code, redirect_uri, client_id, code_verifier required")
		return
	}
	client, ok := s.mem.getClient(clientID)
	if !ok {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "unknown client_id")
		return
	}
	if !s.authenticateClient(r, client) {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "client authentication failed")
		return
	}
	ac, ok := s.mem.takeCode(code)
	if !ok {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid or expired code")
		return
	}
	if ac.ClientID != clientID || ac.RedirectURI != redirectURI {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "code mismatch")
		return
	}
	if !verifyPKCE(verifier, ac.CodeChallenge, ac.CodeChallengeMethod) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}
	user, err := s.store.FindUserByID(ac.UserID)
	if err != nil || user == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "user not found")
		return
	}
	s.issueTokenResponse(w, r, user, clientID, ac.Scope, ac.Resource)
}

func (s *Server) tokenFromRefresh(w http.ResponseWriter, r *http.Request) {
	refresh := r.FormValue("refresh_token")
	clientID := r.FormValue("client_id")
	if refresh == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "refresh_token required")
		return
	}
	rt, ok := s.mem.takeRefresh(refresh)
	if !ok {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid or expired refresh_token")
		return
	}
	if clientID == "" {
		clientID = rt.ClientID
	}
	if rt.ClientID != clientID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "client mismatch")
		return
	}
	client, ok := s.mem.getClient(clientID)
	if !ok || !s.authenticateClient(r, client) {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "client authentication failed")
		return
	}
	user, err := s.store.FindUserByID(rt.UserID)
	if err != nil || user == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "user not found")
		return
	}
	scope := rt.Scope
	if v := strings.TrimSpace(r.FormValue("scope")); v != "" {
		scope = v
	}
	s.issueTokenResponse(w, r, user, clientID, scope, rt.Resource)
}

func (s *Server) authenticateClient(r *http.Request, client *Client) bool {
	method := client.TokenEndpointAuthMethod
	if method == "" || method == "none" {
		return true
	}
	secret := r.FormValue("client_secret")
	return secret != "" && secret == client.Secret
}

func (s *Server) issueTokenResponse(
	w http.ResponseWriter,
	r *http.Request,
	user *store.User,
	clientID, scope, resource string,
) {
	if resource == "" {
		resource = s.ResourceURL(r)
	}
	if scope == "" {
		scope = DefaultScope
	}
	access, err := s.auth.IssueAccessToken(user, resource, s.cfg.AccessTTL)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "cannot issue access_token")
		return
	}
	refresh, err := randomToken(32)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "cannot issue refresh_token")
		return
	}
	s.mem.saveRefresh(&refreshToken{
		Token:     refresh,
		ClientID:  clientID,
		UserID:    user.ID,
		Scope:     scope,
		Resource:  resource,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTTL),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(s.cfg.AccessTTL.Seconds()),
		"refresh_token": refresh,
		"scope":         scope,
	})
}
