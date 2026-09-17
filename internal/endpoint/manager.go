package endpoint

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flowgo/flowgo/api/types"
	epcomp "github.com/flowgo/flowgo/components/endpoint"
	"github.com/flowgo/flowgo/engine"
	"github.com/flowgo/flowgo-server/internal/store"
)

// routeTarget 一条路由绑定到的流程、入口节点与出边关系。
type routeTarget struct {
	flowID   string
	nodeID   string
	relation string // METHOD /path，与 DSL edge.relation 对应
	dsl      *types.FlowDSL
}

// sharedServer 同一监听地址上的共享 HTTP(S) 服务。
type sharedServer struct {
	addr     string
	mux      *http.ServeMux
	server   *http.Server
	ln       net.Listener
	routes   map[string]routeTarget // pattern → target
	refCount int
	// https / tlsFP：同地址后续路由必须 TLS 模式与证书指纹一致。
	https bool
	tlsFP string
}

// Manager 管理各流程中 httpEndpoint 节点拉起的 HTTP 服务。
type Manager struct {
	mu     sync.Mutex
	store  *store.Store
	engine *engine.Engine
	byAddr map[string]*sharedServer
	// flowID → 该流程占用的 (addr, pattern) 列表，便于卸载
	byFlow map[string][]flowRoute
}

type flowRoute struct {
	addr    string
	pattern string
}

// NewManager 创建入口管理器。
func NewManager(st *store.Store, eng *engine.Engine) *Manager {
	return &Manager{
		store:  st,
		engine: eng,
		byAddr: make(map[string]*sharedServer),
		byFlow: make(map[string][]flowRoute),
	}
}

// StartAll 启动库中全部流程的 HTTP 入口（服务启动时调用）。
func (m *Manager) StartAll() {
	list, err := m.store.ListFlows(&store.User{Role: store.RoleAdmin})
	if err != nil {
		log.Printf("endpoint: list flows: %v", err)
		return
	}
	for _, rec := range list {
		if rec == nil || rec.PublishedDSL == nil {
			continue
		}
		if err := m.SyncFlow(rec); err != nil {
			log.Printf("endpoint: sync flow %s: %v", rec.ID, err)
		}
	}
}

// SyncFlow 按已发布 DSL 重建该流程的 HTTP 路由（先卸后装）；未发布则只卸载。
func (m *Manager) SyncFlow(rec *store.FlowRecord) error {
	if rec == nil {
		return nil
	}
	dsl := rec.PublishedDSL
	if dsl == nil {
		m.RemoveFlow(rec.ID)
		return nil
	}
	m.RemoveFlow(rec.ID)

	for _, node := range dsl.Nodes {
		if node.Type != epcomp.Type {
			continue
		}
		cfg, err := epcomp.ParseHttpConfig(node.Configuration)
		if err != nil {
			return err
		}
		addr := epcomp.NormalizeServer(cfg.Server)
		for _, r := range cfg.Routers {
			pattern := epcomp.ToPattern(r.Method, r.Path)
			if err := m.addRoute(addr, pattern, cfg, routeTarget{
				flowID:   rec.ID,
				nodeID:   node.ID,
				relation: epcomp.RouterRelation(r),
				dsl:      cloneDSL(dsl),
			}, rec.ID); err != nil {
				m.RemoveFlow(rec.ID)
				return err
			}
		}
	}
	return nil
}

// RemoveFlow 卸载某流程占用的全部路由；若地址无路由则关闭监听。
func (m *Manager) RemoveFlow(flowID string) {
	if flowID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	routes := m.byFlow[flowID]
	delete(m.byFlow, flowID)
	for _, fr := range routes {
		ss := m.byAddr[fr.addr]
		if ss == nil {
			continue
		}
		if rt, ok := ss.routes[fr.pattern]; ok && rt.flowID == flowID {
			delete(ss.routes, fr.pattern)
			ss.refCount--
		}
		if ss.refCount <= 0 || len(ss.routes) == 0 {
			m.shutdownLocked(fr.addr, ss)
		} else {
			m.rebuildMuxLocked(ss)
		}
	}
}

func (m *Manager) addRoute(addr, pattern string, cfg epcomp.HttpConfig, target routeTarget, flowID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fp := cfg.TLSFingerprint()
	ss, ok := m.byAddr[addr]
	if !ok {
		ss = &sharedServer{
			addr:   addr,
			mux:    http.NewServeMux(),
			routes: make(map[string]routeTarget),
			https:  cfg.Https,
			tlsFP:  fp,
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		ss.ln = ln
		ss.server = &http.Server{
			Handler:           ss.mux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		serveLn := net.Listener(ln)
		proto := "HTTP"
		if cfg.Https {
			cert, err := tls.X509KeyPair([]byte(cfg.CertPem), []byte(cfg.KeyPem))
			if err != nil {
				_ = ln.Close()
				return fmt.Errorf("HTTPS 证书无效: %w", err)
			}
			serveLn = tls.NewListener(ln, &tls.Config{
				Certificates: []tls.Certificate{cert},
			})
			proto = "HTTPS"
		}
		m.byAddr[addr] = ss
		go func(s *sharedServer, l net.Listener, label string) {
			log.Printf("endpoint: %s 入口监听 %s", label, s.addr)
			if err := s.server.Serve(l); err != nil && err != http.ErrServerClosed {
				log.Printf("endpoint: serve %s: %v", s.addr, err)
			}
		}(ss, serveLn, proto)
	} else if ss.https != cfg.Https || ss.tlsFP != fp {
		modeWant := "HTTP"
		if cfg.Https {
			modeWant = "HTTPS"
		}
		modeHave := "HTTP"
		if ss.https {
			modeHave = "HTTPS"
		}
		return fmt.Errorf(
			"监听地址 %s 已以 %s 运行，无法再以 %s（或不同证书）注册路由，请更换端口或统一 TLS 配置",
			addr, modeHave, modeWant,
		)
	}

	if existing, exists := ss.routes[pattern]; exists {
		// 同流程重绑允许更新；跨流程占用则拒绝（保存前应已 Check）
		if existing.flowID != "" && existing.flowID != flowID {
			name := ""
			if m.store != nil {
				if rec, err := m.store.GetFlow(existing.flowID); err == nil && rec != nil {
					name = rec.Name
				}
			}
			return &RouteConflictError{
				Addr:          addr,
				Pattern:       pattern,
				OwnerFlowID:   existing.flowID,
				OwnerFlowName: name,
			}
		}
		ss.routes[pattern] = target
		m.byFlow[flowID] = append(m.byFlow[flowID], flowRoute{addr: addr, pattern: pattern})
		return nil
	}
	ss.routes[pattern] = target
	ss.refCount++
	m.byFlow[flowID] = append(m.byFlow[flowID], flowRoute{addr: addr, pattern: pattern})
	m.registerPatternLocked(ss, pattern, cfg.AllowCors)
	return nil
}

func (m *Manager) registerPatternLocked(ss *sharedServer, pattern string, allowCors bool) {
	ss.mux.HandleFunc(pattern, m.makeHandler(ss, pattern, allowCors))
}

func (m *Manager) rebuildMuxLocked(ss *sharedServer) {
	ss.mux = http.NewServeMux()
	ss.server.Handler = ss.mux
	for pattern := range ss.routes {
		m.registerPatternLocked(ss, pattern, true)
	}
}

func (m *Manager) shutdownLocked(addr string, ss *sharedServer) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = ss.server.Shutdown(ctx)
	delete(m.byAddr, addr)
	log.Printf("endpoint: 已关闭监听 %s", addr)
}

func (m *Manager) makeHandler(ss *sharedServer, pattern string, allowCors bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if allowCors {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,PATCH,OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		m.mu.Lock()
		target, ok := ss.routes[pattern]
		var dsl *types.FlowDSL
		if ok && target.dsl != nil {
			dsl = cloneDSL(target.dsl)
		}
		nodeID := target.nodeID
		relation := target.relation
		m.mu.Unlock()
		if !ok || dsl == nil {
			http.Error(w, "route not found", http.StatusNotFound)
			return
		}

		body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		_ = r.Body.Close()
		dataType := types.TEXT
		ct := r.Header.Get("Content-Type")
		if strings.Contains(ct, "json") || (len(body) > 0 && (body[0] == '{' || body[0] == '[')) {
			dataType = types.JSON
		}
		if r.Method == http.MethodGet && len(body) == 0 {
			// GET：把 query 收成 JSON
			q := map[string]string{}
			for k, vs := range r.URL.Query() {
				if len(vs) > 0 {
					q[k] = vs[0]
				}
			}
			if b, err := json.Marshal(q); err == nil {
				body = b
				dataType = types.JSON
			}
		}

		meta := types.Metadata{
			"httpMethod": r.Method,
			"httpPath":   r.URL.Path,
			"httpQuery":  r.URL.RawQuery,
		}
		for k, vs := range r.Header {
			if len(vs) > 0 {
				meta["header_"+k] = vs[0]
			}
		}
		// 路径参数
		if r.Pattern != "" {
			for _, name := range pathParamNames(pattern) {
				if v := r.PathValue(name); v != "" {
					meta[name] = v
				}
			}
		}

		msg := types.NewMsg("HTTP", dataType, string(body), meta)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// 从匹配路由对应的出边目标开始执行（跳过入口节点本身）
		startID := nextNodeByRelation(dsl, nodeID, relation)
		if startID == "" {
			http.Error(w, "no outgoing edge for route: "+relation, http.StatusBadGateway)
			return
		}
		// 已发布 HTTP 入口：忽略节点 Debug，不采集调试日志
		out, _, err := m.engine.ExecuteFromWithLogsOpts(ctx, dsl, startID, msg, engine.ExecuteOptions{
			CacheTrack: engine.CacheTrackPublished,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		status := http.StatusOK
		if out.Meta != nil {
			if s := strings.TrimSpace(out.Meta["httpStatus"]); s != "" {
				if code, err := strconv.Atoi(s); err == nil && code >= 100 && code <= 599 {
					status = code
				}
			}
		}
		if out.DataType == types.JSON {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(out.Data))
	}
}

// nextNodeByRelation 查找 from 节点指定 relation 的出边目标。
func nextNodeByRelation(dsl *types.FlowDSL, fromID, relation string) string {
	if dsl == nil || fromID == "" || relation == "" {
		return ""
	}
	for _, e := range dsl.Edges {
		if e.From != fromID {
			continue
		}
		rel := e.Relation
		if rel == "" {
			rel = types.RelationSuccess
		}
		if rel == relation {
			return e.To
		}
	}
	return ""
}

func pathParamNames(pattern string) []string {
	// pattern like "POST /api/{id}"
	parts := strings.Fields(pattern)
	path := pattern
	if len(parts) >= 2 {
		path = parts[1]
	}
	var names []string
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") && len(seg) > 2 {
			names = append(names, seg[1:len(seg)-1])
		}
	}
	return names
}

func cloneDSL(in *types.FlowDSL) *types.FlowDSL {
	if in == nil {
		return nil
	}
	out := *in
	out.Nodes = append([]types.FlowNode(nil), in.Nodes...)
	out.Edges = append([]types.FlowEdge(nil), in.Edges...)
	return &out
}
