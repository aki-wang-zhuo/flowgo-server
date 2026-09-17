package oauth

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// registrationRequest RFC7591 动态客户端注册请求（字段按需解析）。
type registrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	ApplicationType         string   `json:"application_type"`
	Scope                   string   `json:"scope"`
}

// handleRegister OAuth 2.0 Dynamic Client Registration。
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "invalid JSON body")
		return
	}
	if len(req.RedirectURIs) == 0 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris required")
		return
	}
	for _, u := range req.RedirectURIs {
		if !validRedirectURI(u) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect URI not allowed: "+u)
			return
		}
	}
	authMethod := strings.TrimSpace(req.TokenEndpointAuthMethod)
	if authMethod == "" {
		authMethod = "none"
	}
	if authMethod != "none" && authMethod != "client_secret_post" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "unsupported token_endpoint_auth_method")
		return
	}
	grants := req.GrantTypes
	if len(grants) == 0 {
		grants = []string{"authorization_code", "refresh_token"}
	}
	responses := req.ResponseTypes
	if len(responses) == 0 {
		responses = []string{"code"}
	}
	id, err := randomClientID()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "cannot allocate client_id")
		return
	}
	secret := ""
	if authMethod != "none" {
		secret, err = randomToken(24)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "cannot allocate client_secret")
			return
		}
	}
	name := strings.TrimSpace(req.ClientName)
	if name == "" {
		name = "MCP Client"
	}
	c := &Client{
		ID:                      id,
		Secret:                  secret,
		Name:                    name,
		RedirectURIs:            append([]string(nil), req.RedirectURIs...),
		TokenEndpointAuthMethod: authMethod,
		GrantTypes:              grants,
		ResponseTypes:           responses,
		CreatedAt:               time.Now().UTC(),
	}
	if err := s.mem.putClient(c); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "persist client failed")
		return
	}
	resp := map[string]any{
		"client_id":                  c.ID,
		"client_id_issued_at":        c.CreatedAt.Unix(),
		"client_name":                c.Name,
		"redirect_uris":              c.RedirectURIs,
		"grant_types":                c.GrantTypes,
		"response_types":             c.ResponseTypes,
		"token_endpoint_auth_method": c.TokenEndpointAuthMethod,
	}
	if secret != "" {
		resp["client_secret"] = secret
	}
	if req.ApplicationType != "" {
		resp["application_type"] = req.ApplicationType
	}
	writeJSON(w, http.StatusCreated, resp)
}
