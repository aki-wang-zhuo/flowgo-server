package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/components/action"
	epcomp "github.com/flowgo/flowgo/components/endpoint"
	"github.com/flowgo/flowgo/engine"
	"github.com/flowgo/flowgo-server/internal/store"
)

// Executor 将存储中的流程交给 flowgo 引擎执行。
type Executor struct {
	Store  *store.Store
	Engine *engine.Engine
}

// ExecuteFlow 按 flowID 加载 DSL 并执行消息。
func (e *Executor) ExecuteFlow(ctx context.Context, flowID, msgType, data string) (string, error) {
	rec, err := e.Store.GetFlow(flowID)
	if err != nil {
		return "", err
	}
	if rec.DSL == nil {
		return "", fmt.Errorf("flow dsl empty")
	}
	msg := types.NewMsg(msgType, types.JSON, data, nil)
	out, err := e.Engine.Execute(ctx, rec.DSL, msg)
	if err != nil {
		return "", err
	}
	return out.Data, nil
}

// ExecuteFromNode 按 flowID 加载 DSL，从指定节点开始执行消息（不经过 entryNode）。
func (e *Executor) ExecuteFromNode(ctx context.Context, flowID, nodeID, msgType, data string) (string, error) {
	rec, err := e.Store.GetFlow(flowID)
	if err != nil {
		return "", err
	}
	if rec.DSL == nil {
		return "", fmt.Errorf("flow dsl empty")
	}
	if strings.TrimSpace(nodeID) == "" {
		return "", fmt.Errorf("nodeId is required")
	}
	found := false
	for i := range rec.DSL.Nodes {
		if rec.DSL.Nodes[i].ID == nodeID {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("node not found: %s", nodeID)
	}
	if msgType == "" {
		msgType = "DEFAULT"
	}
	msg := types.NewMsg(msgType, types.JSON, data, nil)
	out, err := e.Engine.ExecuteFrom(ctx, rec.DSL, nodeID, msg)
	if err != nil {
		return "", err
	}
	return out.Data, nil
}

// SimulateHttpRouteReq 模拟 HTTP 入口某条路径的调试请求。
type SimulateHttpRouteReq struct {
	// DSL 可选：传画布当前图，便于未保存也能调试；空则读库。
	DSL *types.FlowDSL `json:"dsl,omitempty"`
	// NodeID HTTP 入口节点 id。
	NodeID string `json:"nodeId"`
	// RouterIndex 路径下标。
	RouterIndex int `json:"routerIndex"`
	// Body 可选覆盖调试体；空则用路径上的 debugValue。
	Body string `json:"body,omitempty"`
}

// SimulateHttpRouteResult 模拟运行结果。
type SimulateHttpRouteResult struct {
	Data string           `json:"data"`
	Logs []types.DebugLog `json:"logs"`
	// Meta 补充说明（方法/路径等），便于控制台展示请求摘要。
	Meta map[string]string `json:"meta,omitempty"`
}

// SimulateHttpRoute 用路径调试值模拟一次 HTTP 请求并执行后续节点。
func (e *Executor) SimulateHttpRoute(ctx context.Context, flowID string, req SimulateHttpRouteReq) (*SimulateHttpRouteResult, error) {
	dsl := req.DSL
	if dsl == nil {
		rec, err := e.Store.GetFlow(flowID)
		if err != nil {
			return nil, err
		}
		if rec.DSL == nil {
			return nil, fmt.Errorf("flow dsl empty")
		}
		dsl = rec.DSL
	}
	if req.NodeID == "" {
		return nil, fmt.Errorf("nodeId is required")
	}

	var node *types.FlowNode
	for i := range dsl.Nodes {
		if dsl.Nodes[i].ID == req.NodeID {
			node = &dsl.Nodes[i]
			break
		}
	}
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", req.NodeID)
	}
	if node.Type != epcomp.Type {
		return nil, fmt.Errorf("node is not httpEndpoint")
	}
	cfg, err := epcomp.ParseHttpConfig(node.Configuration)
	if err != nil {
		return nil, err
	}
	if req.RouterIndex < 0 || req.RouterIndex >= len(cfg.Routers) {
		return nil, fmt.Errorf("routerIndex out of range")
	}
	router := cfg.Routers[req.RouterIndex]
	body := strings.TrimSpace(req.Body)
	if body == "" {
		body = strings.TrimSpace(router.DebugValue)
	}
	if body == "" {
		body = "{}"
	}

	relation := epcomp.RouterRelation(router)
	startID := nextNodeByRelation(dsl, req.NodeID, relation)
	if startID == "" {
		return nil, fmt.Errorf("no outgoing edge for route: %s", relation)
	}

	method := strings.ToUpper(strings.TrimSpace(router.Method))
	if method == "" {
		method = "POST"
	}
	path := strings.TrimSpace(router.Path)
	if path == "" {
		path = "/"
	}
	meta := types.Metadata{
		"httpMethod": method,
		"httpPath":   path,
		"debug":      "true",
	}
	msg := types.NewMsg("HTTP", types.JSON, body, meta)
	out, logs, err := e.Engine.ExecuteFromWithLogs(ctx, dsl, startID, msg)
	result := &SimulateHttpRouteResult{
		Logs: logs,
		Meta: map[string]string{
			"method":   method,
			"path":     path,
			"relation": relation,
			"body":     body,
			"nodeId":   req.NodeID,
		},
	}
	if err != nil {
		result.Data = ""
		return result, err
	}
	result.Data = out.Data
	return result, nil
}

// SimulateInjectReq 注入执行节点的调试运行请求。
type SimulateInjectReq struct {
	// DSL 可选：传画布当前图，便于未保存也能调试；空则读库。
	DSL *types.FlowDSL `json:"dsl,omitempty"`
	// NodeID 注入节点 id。
	NodeID string `json:"nodeId"`
	// Body 可选覆盖注入体；空则用节点 configuration.payload。
	Body string `json:"body,omitempty"`
}

// SimulateInjectResult 注入运行结果。
type SimulateInjectResult struct {
	Data string           `json:"data"`
	Logs []types.DebugLog `json:"logs"`
	Meta map[string]string `json:"meta,omitempty"`
}

// SimulateInject 用注入节点的 JSON 作为消息体，从该节点执行并进入后续链路。
func (e *Executor) SimulateInject(ctx context.Context, flowID string, req SimulateInjectReq) (*SimulateInjectResult, error) {
	dsl := req.DSL
	if dsl == nil {
		rec, err := e.Store.GetFlow(flowID)
		if err != nil {
			return nil, err
		}
		if rec.DSL == nil {
			return nil, fmt.Errorf("flow dsl empty")
		}
		dsl = rec.DSL
	}
	if req.NodeID == "" {
		return nil, fmt.Errorf("nodeId is required")
	}

	var node *types.FlowNode
	for i := range dsl.Nodes {
		if dsl.Nodes[i].ID == req.NodeID {
			node = &dsl.Nodes[i]
			break
		}
	}
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", req.NodeID)
	}
	if node.Type != epcomp.TypeInject {
		return nil, fmt.Errorf("node is not inject")
	}

	body := strings.TrimSpace(req.Body)
	if body == "" {
		body = epcomp.ParseInjectPayload(node.Configuration)
	}

	// 覆盖节点 payload，使 OnMsg 使用本次注入内容（含调试覆盖）
	runDSL := *dsl
	runDSL.Nodes = append([]types.FlowNode(nil), dsl.Nodes...)
	for i := range runDSL.Nodes {
		if runDSL.Nodes[i].ID != req.NodeID {
			continue
		}
		conf := map[string]interface{}{}
		if runDSL.Nodes[i].Configuration != nil {
			for k, v := range runDSL.Nodes[i].Configuration {
				conf[k] = v
			}
		}
		conf["payload"] = body
		runDSL.Nodes[i].Configuration = conf
		break
	}

	msg := types.NewMsg("INJECT", types.JSON, body, types.Metadata{"debug": "true", "inject": "true"})
	out, logs, err := e.Engine.ExecuteFromWithLogs(ctx, &runDSL, req.NodeID, msg)
	result := &SimulateInjectResult{
		Logs: logs,
		Meta: map[string]string{
			"body":   body,
			"nodeId": req.NodeID,
		},
	}
	if err != nil {
		result.Data = ""
		return result, err
	}
	result.Data = out.Data
	return result, nil
}

// SimulateHttpClientReq HTTP 客户端节点的调试运行请求。
type SimulateHttpClientReq struct {
	DSL    *types.FlowDSL `json:"dsl,omitempty"`
	NodeID string         `json:"nodeId"`
	// Body 可选覆盖本次实际请求体；空则用节点 configuration.debugValue。不走 body 模板。
	Body string `json:"body,omitempty"`
}

// SimulateHttpClientResult HTTP 客户端调试运行结果。
type SimulateHttpClientResult struct {
	Data string            `json:"data"`
	Logs []types.DebugLog  `json:"logs"`
	Meta map[string]string `json:"meta,omitempty"`
}

// SimulateHttpClient 用 debugValue 作为本次实际 HTTP 请求体，从该节点执行并进入后续链路。
// 不渲染节点 body 模板；真实部署 / HTTP 入口触发仍走模板。
func (e *Executor) SimulateHttpClient(ctx context.Context, flowID string, req SimulateHttpClientReq) (*SimulateHttpClientResult, error) {
	dsl := req.DSL
	if dsl == nil {
		rec, err := e.Store.GetFlow(flowID)
		if err != nil {
			return nil, err
		}
		if rec.DSL == nil {
			return nil, fmt.Errorf("flow dsl empty")
		}
		dsl = rec.DSL
	}
	if req.NodeID == "" {
		return nil, fmt.Errorf("nodeId is required")
	}

	var node *types.FlowNode
	for i := range dsl.Nodes {
		if dsl.Nodes[i].ID == req.NodeID {
			node = &dsl.Nodes[i]
			break
		}
	}
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", req.NodeID)
	}
	if node.Type != action.Type {
		return nil, fmt.Errorf("node is not httpClient")
	}

	body := strings.TrimSpace(req.Body)
	if body == "" {
		body = action.ParseDebugValue(node.Configuration)
	}

	msg := types.NewMsg("DEBUG", types.JSON, body, types.Metadata{
		"debug":      "true",
		"httpClient": "true",
	})
	out, logs, err := e.Engine.ExecuteFromWithLogs(ctx, dsl, req.NodeID, msg)
	result := &SimulateHttpClientResult{
		Logs: logs,
		Meta: map[string]string{
			"body":   body,
			"nodeId": req.NodeID,
		},
	}
	if err != nil {
		result.Data = ""
		return result, err
	}
	result.Data = out.Data
	return result, nil
}

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
