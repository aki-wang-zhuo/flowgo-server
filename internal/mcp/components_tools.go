package mcp

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/components"
	"github.com/flowgo/flowgo/engine"
)

// toolListComponents 列出当前用户可用（已启用）的节点及简要说明。
func (g *Gateway) toolListComponents(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowRead)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	prefs, err := g.Store.GetComponentPrefs(user.ID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	locale := resolveToolLocale(ctx, req)
	defs := types.LocalizeComponentDefs(
		components.FilterDefs(engine.DefaultRegistry.ListDefs(), prefs.IsComponentEnabled),
		locale,
	)
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Category != defs[j].Category {
			return defs[i].Category < defs[j].Category
		}
		return defs[i].Type < defs[j].Type
	})
	type item struct {
		Type        string   `json:"type"`
		Label       string   `json:"label"`
		Category    string   `json:"category"`
		Description string   `json:"description"`
		Source      string   `json:"source"`
		Relations   []string `json:"relationTypes,omitempty"`
	}
	out := make([]item, 0, len(defs))
	for _, d := range defs {
		src := d.Source
		if src == "" {
			src = types.ComponentSourceBuiltin
		}
		out = append(out, item{
			Type:        d.Type,
			Label:       d.Label,
			Category:    d.Category,
			Description: d.Description,
			Source:      src,
			Relations:   d.RelationTypes,
		})
	}
	b, _ := json.Marshal(map[string]interface{}{
		"components": out,
		"locale":     locale,
		"hint":       "使用 get_component_doc 并传入 type 获取 configuration 字段与用法说明",
	})
	return mcp.NewToolResultText(string(b)), nil
}

// toolGetComponentDoc 返回单个节点的完整文档（供 AI 正确填写 DSL）。
func (g *Gateway) toolGetComponentDoc(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowRead)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	typeName, _ := req.RequireString("type")
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return mcp.NewToolResultError("type is required"), nil
	}
	prefs, err := g.Store.GetComponentPrefs(user.ID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if !prefs.IsComponentEnabled(typeName) {
		return mcp.NewToolResultError("component disabled: " + typeName), nil
	}
	locale := resolveToolLocale(ctx, req)
	var found *types.ComponentDef
	for _, d := range engine.DefaultRegistry.ListDefs() {
		if d.Type == typeName {
			dd := types.LocalizeComponentDef(d, locale)
			found = &dd
			break
		}
	}
	if found == nil {
		return mcp.NewToolResultError("unknown component type: " + typeName), nil
	}
	if found.Source == "" {
		found.Source = types.ComponentSourceBuiltin
	}
	b, _ := json.Marshal(found)
	return mcp.NewToolResultText(string(b)), nil
}
