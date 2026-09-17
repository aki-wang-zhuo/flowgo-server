package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/flowgo/flowgo/engine"
	"github.com/flowgo/flowgo/components/nodedocs"
	"github.com/flowgo/flowgo-server/internal/api"
	"github.com/flowgo/flowgo-server/internal/app"
	"github.com/flowgo/flowgo-server/internal/auth"
	"github.com/flowgo/flowgo-server/internal/componentdocs"
	"github.com/flowgo/flowgo-server/internal/config"
	"github.com/flowgo/flowgo-server/internal/endpoint"
	mcppkg "github.com/flowgo/flowgo-server/internal/mcp"
	"github.com/flowgo/flowgo-server/internal/oauth"
	"github.com/flowgo/flowgo-server/internal/pluginhost"
	"github.com/flowgo/flowgo-server/internal/store"
	wspkg "github.com/flowgo/flowgo-server/internal/ws"
)

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	authSvc := auth.NewService(cfg.JWTSecret, cfg.TokenTTL, st)
	eng := engine.New()
	exec := &app.Executor{Store: st, Engine: eng}
	hub := wspkg.NewHub()
	epMgr := endpoint.NewManager(st, eng)

	pluginDir := filepath.Join(cfg.DataDir, "plugins")
	docsDir := filepath.Join(cfg.DataDir, "docs")
	_ = os.MkdirAll(pluginDir, 0o755)
	_ = os.MkdirAll(docsDir, 0o755)

	docStore := componentdocs.New(docsDir, nodedocs.BuiltinMap())
	if err := docStore.Reload(); err != nil {
		log.Printf("componentdocs: initial reload: %v", err)
	}

	pluginMgr := pluginhost.NewManager(pluginDir, engine.DefaultRegistry, docStore)
	pluginMgr.LoadAll()

	apiSrv := &api.Server{Auth: authSvc, Store: st, Exec: exec, Hub: hub, Endpoints: epMgr, Plugins: pluginMgr, Docs: docStore}

	mux := http.NewServeMux()
	mux.Handle("/", apiSrv.NewMux())

	if cfg.MCPEnabled {
		oauthSrv := oauth.New(oauth.Config{
			PublicBaseURL: cfg.PublicBaseURL,
			AccessTTL:     cfg.OAuthAccessTTL,
			RefreshTTL:    cfg.OAuthRefreshTTL,
		}, authSvc, st)
		oauthSrv.Mount(mux)

		gw := mcppkg.New(authSvc, st, exec, hub, epMgr, oauthSrv)
		mux.Handle("/mcp", gw.Handler())
		mux.Handle("/mcp/", gw.Handler())
		base := strings.TrimRight(cfg.PublicBaseURL, "/")
		log.Printf("MCP endpoint: %s/mcp （OAuth 浏览器登录 或 X-API-Key / Bearer）", base)
		log.Printf("MCP OAuth authorize: %s/oauth/authorize", base)
	}

	// 可选：托管编辑器静态资源（构建产物放在 editor/）
	editorDir := filepath.Join(".", "editor")
	if fi, err := os.Stat(editorDir); err == nil && fi.IsDir() {
		mux.Handle("/editor/", http.StripPrefix("/editor/", http.FileServer(http.Dir(editorDir))))
	}

	// 恢复已保存流程的 HTTP 入口
	go epMgr.StartAll()

	log.Printf("FlowGo Server listening on %s", cfg.Addr)
	log.Printf("DB path: %s", cfg.DBPath)
	log.Printf("默认管理员: admin / admin （请登录后立即修改密码）")
	if err := http.ListenAndServe(cfg.Addr, api.CORS(cfg.CORSOrigin)(mux)); err != nil {
		log.Fatal(err)
	}
}
