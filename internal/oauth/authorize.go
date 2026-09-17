package oauth

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo-server/internal/store"
)

// authorizeQuery 授权请求查询参数。
type authorizeQuery struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Resource            string
	Locale              string
}

func parseAuthorizeQuery(r *http.Request) authorizeQuery {
	q := r.URL.Query()
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if v := strings.TrimSpace(q.Get("lang")); v != "" {
		locale = types.NormalizeLocale(v)
	}
	return authorizeQuery{
		ResponseType:        q.Get("response_type"),
		ClientID:            q.Get("client_id"),
		RedirectURI:         q.Get("redirect_uri"),
		Scope:               q.Get("scope"),
		State:               q.Get("state"),
		CodeChallenge:       q.Get("code_challenge"),
		CodeChallengeMethod: q.Get("code_challenge_method"),
		Resource:            q.Get("resource"),
		Locale:              locale,
	}
}

func (s *Server) handleAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	aq := parseAuthorizeQuery(r)
	if errMsg := s.validateAuthorizeParams(r, aq); errMsg != "" {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(renderLoginHTML(aq)))
}

func (s *Server) handleAuthorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	aq := authorizeQuery{
		ResponseType:        r.FormValue("response_type"),
		ClientID:            r.FormValue("client_id"),
		RedirectURI:         r.FormValue("redirect_uri"),
		Scope:               r.FormValue("scope"),
		State:               r.FormValue("state"),
		CodeChallenge:       r.FormValue("code_challenge"),
		CodeChallengeMethod: r.FormValue("code_challenge_method"),
		Resource:            r.FormValue("resource"),
		Locale:              types.NormalizeLocale(r.FormValue("lang")),
	}
	if aq.Locale == "" {
		aq.Locale = types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	}
	if errMsg := s.validateAuthorizeParams(r, aq); errMsg != "" {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	user, err := s.store.FindUserByUsername(username)
	if err != nil || user == nil || !store.CheckPassword(user.PasswordHash, password) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(renderLoginHTMLWithError(aq, loginCopy(aq.Locale).badCreds)))
		return
	}

	enabled, err := s.store.IsMcpEnabled(user.ID)
	if err != nil || !enabled {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(renderLoginHTMLWithError(aq, loginCopy(aq.Locale).mcpDisabled)))
		return
	}

	code, err := randomToken(24)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	scope := strings.TrimSpace(aq.Scope)
	if scope == "" {
		scope = DefaultScope
	}
	resource := strings.TrimSpace(aq.Resource)
	if resource == "" {
		resource = s.ResourceURL(r)
	}
	s.mem.saveCode(&authCode{
		Code:                code,
		ClientID:            aq.ClientID,
		UserID:              user.ID,
		RedirectURI:         aq.RedirectURI,
		Scope:               scope,
		Resource:            resource,
		CodeChallenge:       aq.CodeChallenge,
		CodeChallengeMethod: normalizeChallengeMethod(aq.CodeChallengeMethod),
		ExpiresAt:           time.Now().Add(codeTTL),
	})

	redir, err := url.Parse(aq.RedirectURI)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	q := redir.Query()
	q.Set("code", code)
	if aq.State != "" {
		q.Set("state", aq.State)
	}
	redir.RawQuery = q.Encode()
	http.Redirect(w, r, redir.String(), http.StatusFound)
}

func (s *Server) validateAuthorizeParams(r *http.Request, aq authorizeQuery) string {
	if aq.ResponseType != "code" {
		return "response_type must be code"
	}
	if aq.ClientID == "" {
		return "client_id required"
	}
	if _, ok := s.mem.getClient(aq.ClientID); !ok {
		return "unknown client_id"
	}
	if aq.RedirectURI == "" {
		return "redirect_uri required"
	}
	if !s.mem.clientAllowsRedirect(aq.ClientID, aq.RedirectURI) {
		return "redirect_uri not registered for client"
	}
	if aq.CodeChallenge == "" {
		return "code_challenge required (PKCE)"
	}
	method := normalizeChallengeMethod(aq.CodeChallengeMethod)
	if method != "S256" && method != "plain" {
		return "unsupported code_challenge_method"
	}
	if aq.Resource != "" && !sameResource(aq.Resource, s.ResourceURL(r)) {
		return "resource does not match this MCP server"
	}
	return ""
}
