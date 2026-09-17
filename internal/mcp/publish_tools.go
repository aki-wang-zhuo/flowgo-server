package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/flowgo/flowgo/engine"
	"github.com/flowgo/flowgo-server/internal/store"
)

func (g *Gateway) toolPublishFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowUpdate)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	note, _ := req.GetArguments()["note"].(string)
	rec, err := g.Store.GetFlow(id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if rec.Locked {
		return mcp.NewToolResultError("flow is locked"), nil
	}
	if rec.DSL == nil {
		return mcp.NewToolResultError("draft dsl is empty"), nil
	}
	if g.Endpoints != nil {
		if err := g.Endpoints.CheckHTTPRoutes(id, rec.DSL); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}
	rec, err = g.Store.PublishFlow(id, note)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if g.Endpoints != nil {
		if err := g.Endpoints.SyncFlow(rec); err != nil {
			return mcp.NewToolResultError("published but http endpoint failed: " + err.Error()), nil
		}
	}
	if g.Exec != nil {
		g.Exec.InvalidateFlowTrack(rec.ID, engine.CacheTrackPublished)
	}
	if g.Hub != nil {
		g.Hub.NotifyFlowChanged("published", rec.ID, rec.Name, "mcp")
	}
	b, _ := json.Marshal(rec)
	return mcp.NewToolResultText(string(b)), nil
}

func (g *Gateway) toolDiscardDraft(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowUpdate)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	rec, err := g.Store.DiscardDraft(id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if g.Exec != nil {
		g.Exec.InvalidateFlowTrack(id, engine.CacheTrackDraft)
	}
	if g.Hub != nil {
		g.Hub.NotifyFlowChanged("discard-draft", rec.ID, rec.Name, "mcp")
	}
	b, _ := json.Marshal(rec)
	return mcp.NewToolResultText(string(b)), nil
}

func (g *Gateway) toolListPublishHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowRead)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	include := false
	if v, ok := req.GetArguments()["includeDsl"].(bool); ok {
		include = v
	}
	list, rec, err := g.Store.ListPublishHistory(id, include)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.Marshal(map[string]any{
		"flowId":             rec.ID,
		"published":          rec.Published,
		"publishedVersion":   rec.PublishedVersion,
		"unpublishedChanges": rec.UnpublishedChanges,
		"items":              list,
	})
	return mcp.NewToolResultText(string(b)), nil
}

func (g *Gateway) toolRollbackPublish(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowUpdate)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	ver, err := mcpVersionArg(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	rec, err := g.Store.GetFlow(id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if rec.Locked {
		return mcp.NewToolResultError("flow is locked"), nil
	}
	hist, _, err := g.Store.ListPublishHistory(id, true)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	var target *store.PublishHistoryItem
	for i := range hist {
		if hist[i].Version == ver {
			target = &hist[i]
			break
		}
	}
	if target == nil || target.DSL == nil {
		return mcp.NewToolResultError(store.ErrHistoryNotFound.Error()), nil
	}
	if g.Endpoints != nil {
		if err := g.Endpoints.CheckHTTPRoutes(id, target.DSL); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}
	rec, err = g.Store.RollbackPublish(id, ver)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if g.Endpoints != nil {
		if err := g.Endpoints.SyncFlow(rec); err != nil {
			return mcp.NewToolResultError("rolled back but http endpoint failed: " + err.Error()), nil
		}
	}
	if g.Exec != nil {
		g.Exec.InvalidateFlowTrack(rec.ID, engine.CacheTrackPublished)
	}
	if g.Hub != nil {
		g.Hub.NotifyFlowChanged("published", rec.ID, rec.Name, "mcp")
	}
	b, _ := json.Marshal(rec)
	return mcp.NewToolResultText(string(b)), nil
}

func (g *Gateway) toolDeletePublishHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, err := g.requireMCPPerm(ctx, permFlowUpdate)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	id, _ := req.RequireString("id")
	if !user.CanAccessFlow(id) {
		return mcp.NewToolResultError("forbidden"), nil
	}
	ver, err := mcpVersionArg(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	rec, err := g.Store.DeletePublishHistory(id, ver)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if g.Hub != nil {
		g.Hub.NotifyFlowChanged("history-deleted", rec.ID, rec.Name, "mcp")
	}
	b, _ := json.Marshal(rec)
	return mcp.NewToolResultText(string(b)), nil
}

func mcpVersionArg(req mcp.CallToolRequest) (int, error) {
	v, ok := req.GetArguments()["version"]
	if !ok {
		return 0, fmt.Errorf("version is required")
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	default:
		return 0, fmt.Errorf("invalid version")
	}
}
