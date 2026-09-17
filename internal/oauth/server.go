// Package oauth 实现 MCP 所需的 OAuth 2.1 Authorization Code + PKCE（本机 HTTP 可用）。
// 上线环境应在反向代理后启用 HTTPS；本包不强制 TLS。
package oauth

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/flowgo/flowgo-server/internal/auth"
	"github.com/flowgo/flowgo-server/internal/store"
)

const (
	// DefaultScope MCP 默认 scope。
	DefaultScope = "mcp"
	// codeTTL 授权码有效期。
	codeTTL = 10 * time.Minute
)

// Config OAuth 服务配置。
type Config struct {
	// PublicBaseURL 对外基址，如 http://127.0.0.1:8090；空则按请求 Host 推断。
	PublicBaseURL string
	// AccessTTL access_token 有效期。
	AccessTTL time.Duration
	// RefreshTTL refresh_token 有效期。
	RefreshTTL time.Duration
}

// Server OAuth 授权服务器（可与 MCP 资源同进程）。
type Server struct {
	cfg   Config
	auth  *auth.Service
	store *store.Store
	mem   *memoryStore
}

// New 创建 OAuth 服务器，并预置 Cursor 常用回调的静态客户端。
func New(cfg Config, authSvc *auth.Service, st *store.Store) *Server {
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = time.Hour
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = 30 * 24 * time.Hour
	}
	s := &Server{
		cfg:   cfg,
		auth:  authSvc,
		store: st,
		mem:   newMemoryStore(),
	}
	s.seedCursorClient()
	return s
}

// Mount 将 OAuth 相关路由挂到 mux（无需先鉴权）。
func (s *Server) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handleProtectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/{path...}", s.handleProtectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleAuthorizationServerMetadata)
	mux.HandleFunc("POST /oauth/register", s.handleRegister)
	mux.HandleFunc("GET /oauth/authorize", s.handleAuthorizeGet)
	mux.HandleFunc("POST /oauth/authorize", s.handleAuthorizePost)
	mux.HandleFunc("POST /oauth/token", s.handleToken)
}

// ResourceURL 返回 MCP 资源规范 URI（无尾斜杠）。
func (s *Server) ResourceURL(r *http.Request) string {
	return strings.TrimRight(s.baseURL(r), "/") + "/mcp"
}

// MetadataURL 返回 Protected Resource Metadata 文档 URL。
func (s *Server) MetadataURL(r *http.Request) string {
	return strings.TrimRight(s.baseURL(r), "/") + "/.well-known/oauth-protected-resource"
}

// WriteUnauthorized 写出 MCP 401，并带上 RFC9728 WWW-Authenticate。
func (s *Server) WriteUnauthorized(w http.ResponseWriter, r *http.Request) {
	meta := s.MetadataURL(r)
	w.Header().Set("WWW-Authenticate", `Bearer FAKESECRET_g3h4i5j6k7l8m9n0o1p2="`+meta+`", scope="`+DefaultScope+`"`)
	w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate")
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func (s *Server) baseURL(r *http.Request) string {
	if u := strings.TrimSpace(s.cfg.PublicBaseURL); u != "" {
		return strings.TrimRight(u, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// 反代常见头
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.TrimSpace(strings.Split(proto, ",")[0])
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1:8090"
	}
	return scheme + "://" + host
}

func (s *Server) issuer(r *http.Request) string {
	return s.baseURL(r)
}

func (s *Server) seedCursorClient() {
	_ = s.mem.putClient(&Client{
		ID:                      "flowgo-cursor",
		Name:                    "Cursor",
		RedirectURIs:            cursorRedirectURIs(),
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		CreatedAt:               time.Now().UTC(),
	})
}

func cursorRedirectURIs() []string {
	return []string{
		"http://localhost:8787/callback",
		"http://127.0.0.1:8787/callback",
		"cursor://anysphere.cursor-mcp/oauth/callback",
		"https://www.cursor.com/agents/mcp/oauth/callback",
	}
}

func sameResource(a, b string) bool {
	return strings.TrimRight(strings.TrimSpace(a), "/") == strings.TrimRight(strings.TrimSpace(b), "/")
}

func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return false
	}
	// 允许 https、localhost/127.0.0.1 http、以及 cursor:// 私有回调
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme == "http" {
		host := strings.ToLower(u.Hostname())
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
	if u.Scheme == "cursor" {
		return true
	}
	return false
}
