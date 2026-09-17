package endpoint

import (
	"fmt"

	"github.com/flowgo/flowgo/api/types"
	epcomp "github.com/flowgo/flowgo/components/endpoint"
)

// RouteConflictError 同监听地址下 METHOD+path 冲突（跨流程或本流程内重复）。
type RouteConflictError struct {
	Addr          string
	Pattern       string
	OwnerFlowID   string
	OwnerFlowName string
	// Local 为 true 表示冲突发生在当前 DSL 内部（多节点或重复路由）。
	Local bool
}

// Error 实现 error。
func (e *RouteConflictError) Error() string {
	if e == nil {
		return "http route conflict"
	}
	if e.Local {
		hint := e.OwnerFlowID
		if hint != "" {
			return fmt.Sprintf(
				"HTTP 路由冲突：本流程内重复注册 %s → %s（%s；同端口同方法路径仅允许一处）",
				e.Addr, e.Pattern, hint,
			)
		}
		return fmt.Sprintf(
			"HTTP 路由冲突：本流程内重复注册 %s → %s（同端口同方法路径仅允许一处）",
			e.Addr, e.Pattern,
		)
	}
	owner := e.OwnerFlowName
	if owner == "" {
		owner = e.OwnerFlowID
	}
	if e.OwnerFlowID != "" && e.OwnerFlowName != "" {
		owner = fmt.Sprintf("%s（%s）", e.OwnerFlowName, e.OwnerFlowID)
	}
	return fmt.Sprintf(
		"HTTP 路由冲突：%s 上的 %s 已被流程「%s」占用，请更换端口或路径后再保存",
		e.Addr, e.Pattern, owner,
	)
}

type routeKey struct {
	addr    string
	pattern string
}

// CheckHTTPRoutes 保存前校验：同端口可共享；同端口+同 METHOD/path 则拒绝。
// 同端口的 HTTPS / 证书配置必须一致；若地址已被其他流程占用且 TLS 模式不同也拒绝。
// flowID 为当前流程 id（更新时忽略该流程已占用的路由）。
func (m *Manager) CheckHTTPRoutes(flowID string, dsl *types.FlowDSL) error {
	if dsl == nil {
		return nil
	}

	type addrTLS struct {
		https bool
		fp    string
	}
	seen := map[routeKey]string{} // key → 本 DSL 中首次出现的 nodeID
	tlsByAddr := map[string]addrTLS{}

	for _, node := range dsl.Nodes {
		if node.Type != epcomp.Type {
			continue
		}
		cfg, err := epcomp.ParseHttpConfig(node.Configuration)
		if err != nil {
			return fmt.Errorf("节点 %s HTTP 配置无效: %w", node.ID, err)
		}
		addr := epcomp.NormalizeServer(cfg.Server)
		fp := cfg.TLSFingerprint()
		if prev, ok := tlsByAddr[addr]; ok {
			if prev.https != cfg.Https || prev.fp != fp {
				return fmt.Errorf(
					"本流程内监听地址 %s 的 HTTPS/证书配置不一致（节点 %s）",
					addr, node.ID,
				)
			}
		} else {
			tlsByAddr[addr] = addrTLS{https: cfg.Https, fp: fp}
		}
		for _, r := range cfg.Routers {
			pattern := epcomp.ToPattern(r.Method, r.Path)
			k := routeKey{addr: addr, pattern: pattern}
			if prev, ok := seen[k]; ok {
				return &RouteConflictError{
					Addr:    addr,
					Pattern: pattern,
					Local:   true,
					// 本地冲突时 OwnerFlowID 存放冲突节点 id 提示
					OwnerFlowID: "节点 " + prev + " 与 " + node.ID,
				}
			}
			seen[k] = node.ID
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for addr, want := range tlsByAddr {
		ss := m.byAddr[addr]
		if ss == nil {
			continue
		}
		if ss.https == want.https && ss.tlsFP == want.fp {
			continue
		}
		// 仅当前流程占用时可在 Sync 时重建 TLS；若有其他流程则拒绝
		for _, rt := range ss.routes {
			if rt.flowID != "" && rt.flowID != flowID {
				modeHave := "HTTP"
				if ss.https {
					modeHave = "HTTPS"
				}
				modeWant := "HTTP"
				if want.https {
					modeWant = "HTTPS"
				}
				return fmt.Errorf(
					"监听地址 %s 已由其他流程以 %s 占用，无法改为 %s（或更换证书），请换端口",
					addr, modeHave, modeWant,
				)
			}
		}
	}

	for k := range seen {
		ss := m.byAddr[k.addr]
		if ss == nil {
			continue
		}
		rt, ok := ss.routes[k.pattern]
		if !ok || rt.flowID == "" || rt.flowID == flowID {
			continue
		}
		name := ""
		if m.store != nil {
			if rec, err := m.store.GetFlow(rt.flowID); err == nil && rec != nil {
				name = rec.Name
			}
		}
		return &RouteConflictError{
			Addr:          k.addr,
			Pattern:       k.pattern,
			OwnerFlowID:   rt.flowID,
			OwnerFlowName: name,
		}
	}
	return nil
}
