package mcp

import (
	"context"
	"fmt"

	"github.com/flowgo/flowgo-server/internal/store"
)

// mcpPerm 内部权限键。
type mcpPerm string

const (
	permFlowCreate   mcpPerm = "flowCreate"
	permFlowRead     mcpPerm = "flowRead"
	permFlowUpdate   mcpPerm = "flowUpdate"
	permFlowDelete   mcpPerm = "flowDelete"
	permFlowExecute  mcpPerm = "flowExecute"
	permFlowUnlock   mcpPerm = "flowUnlock"
	permNotifyEditor mcpPerm = "notifyEditor"
)

// requireMCPPerm 校验当前用户是否具备指定 MCP 权限。
func (g *Gateway) requireMCPPerm(ctx context.Context, p mcpPerm) (*store.User, error) {
	user, err := userFromMCP(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := g.Store.GetMcpSettings(user.ID)
	if err != nil {
		return nil, err
	}
	ok := false
	if !cfg.Enabled {
		return nil, fmt.Errorf("mcp disabled")
	}
	switch p {
	case permFlowCreate:
		ok = cfg.Permissions.FlowCreate
	case permFlowRead:
		ok = cfg.Permissions.FlowRead
	case permFlowUpdate:
		ok = cfg.Permissions.FlowUpdate
	case permFlowDelete:
		ok = cfg.Permissions.FlowDelete
	case permFlowExecute:
		ok = cfg.Permissions.FlowExecute
	case permFlowUnlock:
		ok = cfg.Permissions.FlowUnlock
	case permNotifyEditor:
		ok = cfg.Permissions.NotifyEditor
	}
	if !ok {
		return nil, fmt.Errorf("mcp permission denied: %s", p)
	}
	return user, nil
}
