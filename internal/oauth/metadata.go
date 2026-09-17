package oauth

import (
	"encoding/json"
	"net/http"
)

// handleProtectedResourceMetadata RFC9728 Protected Resource Metadata。
func (s *Server) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	issuer := s.issuer(r)
	resource := s.ResourceURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 resource,
		"authorization_servers":    []string{issuer},
		"scopes_supported":         []string{DefaultScope},
		"bearer_methods_supported": []string{"header"},
	})
}

// handleAuthorizationServerMetadata RFC8414 Authorization Server Metadata。
func (s *Server) handleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	issuer := s.issuer(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"registration_endpoint":                 issuer + "/oauth/register",
		"scopes_supported":                      []string{DefaultScope},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post"},
		"revocation_endpoint_auth_methods_supported": []string{"none"},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{
		"error":             code,
		"error_description": desc,
	})
}
