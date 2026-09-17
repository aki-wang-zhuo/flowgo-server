package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/components/transform"
	"github.com/flowgo/flowgo/engine"
)

// SimulateJsTransformReq JS 转换节点的调试运行请求。
type SimulateJsTransformReq struct {
	DSL    *types.FlowDSL `json:"dsl,omitempty"`
	NodeID string         `json:"nodeId"`
	// Body 可选覆盖测试值；空则用节点 configuration.debugValue。
	Body string `json:"body,omitempty"`
	// RunOnly 为 true 时只执行本节点，不沿出边继续。
	RunOnly bool `json:"runOnly,omitempty"`
}

// SimulateJsTransformResult JS 转换调试运行结果。
type SimulateJsTransformResult struct {
	Data string           `json:"data"`
	Logs []types.DebugLog `json:"logs"`
	Meta map[string]string `json:"meta,omitempty"`
}

// SimulateJsTransform 用 debugValue 作为脚本 msg 入参，从该节点执行。
// RunOnly=false 时进入后续链路；RunOnly=true 时只跑本节点。
func (e *Executor) SimulateJsTransform(ctx context.Context, flowID string, req SimulateJsTransformReq) (*SimulateJsTransformResult, error) {
	dsl, err := e.resolveDraftDSL(flowID, req.DSL)
	if err != nil {
		return nil, err
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
	if node.Type != transform.Type {
		return nil, fmt.Errorf("node is not jsTransform")
	}

	body := strings.TrimSpace(req.Body)
	if body == "" {
		body = transform.ParseDebugValue(node.Configuration)
	}

	msg := types.NewMsg("DEBUG", types.JSON, body, types.Metadata{
		"debug":       "true",
		"jsTransform": "true",
	})
	out, logs, err := e.Engine.ExecuteFromWithLogsOpts(ctx, dsl, req.NodeID, msg, engine.ExecuteOptions{
		OnlyStart:  req.RunOnly,
		CacheTrack: engine.CacheTrackDraft,
	})
	result := &SimulateJsTransformResult{
		Logs: logs,
		Meta: map[string]string{
			"body":    body,
			"nodeId":  req.NodeID,
			"runOnly": fmt.Sprintf("%v", req.RunOnly),
		},
	}
	if err != nil {
		result.Data = ""
		return result, err
	}
	result.Data = out.Data
	return result, nil
}
