package endpoint_test

import (
	"testing"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo-server/internal/endpoint"
	"github.com/flowgo/flowgo-server/internal/store"
)

func TestCheckHTTPRoutes_LocalDuplicate(t *testing.T) {
	m := endpoint.NewManager(nil, nil)
	dsl := &types.FlowDSL{
		ID: "flow-a",
		Nodes: []types.FlowNode{
			{
				ID:   "n1",
				Type: "httpEndpoint",
				Configuration: map[string]interface{}{
					"server":  ":18099",
					"routers": []interface{}{map[string]interface{}{"method": "POST", "path": "/x"}},
				},
			},
			{
				ID:   "n2",
				Type: "httpEndpoint",
				Configuration: map[string]interface{}{
					"server":  ":18099",
					"routers": []interface{}{map[string]interface{}{"method": "POST", "path": "/x"}},
				},
			},
		},
	}
	err := m.CheckHTTPRoutes("flow-a", dsl)
	if err == nil {
		t.Fatal("expected local conflict")
	}
	if _, ok := err.(*endpoint.RouteConflictError); !ok {
		t.Fatalf("want RouteConflictError, got %T %v", err, err)
	}
}

func TestCheckHTTPRoutes_CrossFlow(t *testing.T) {
	m := endpoint.NewManager(nil, nil)
	dsl1 := &types.FlowDSL{
		ID: "flow-1",
		Nodes: []types.FlowNode{
			{
				ID:   "n1",
				Type: "httpEndpoint",
				Configuration: map[string]interface{}{
					"server":  ":18100",
					"routers": []interface{}{map[string]interface{}{"method": "GET", "path": "/shared"}},
				},
			},
		},
	}
	if err := m.SyncFlow(&store.FlowRecord{ID: "flow-1", Name: "一号", DSL: dsl1}); err != nil {
		t.Fatal(err)
	}
	defer m.RemoveFlow("flow-1")

	dsl2 := &types.FlowDSL{
		ID: "flow-2",
		Nodes: []types.FlowNode{
			{
				ID:   "n1",
				Type: "httpEndpoint",
				Configuration: map[string]interface{}{
					"server":  ":18100",
					"routers": []interface{}{map[string]interface{}{"method": "GET", "path": "/shared"}},
				},
			},
		},
	}
	err := m.CheckHTTPRoutes("flow-2", dsl2)
	if err == nil {
		t.Fatal("expected cross-flow conflict")
	}

	// 同流程更新自己占用的路由应通过
	if err := m.CheckHTTPRoutes("flow-1", dsl1); err != nil {
		t.Fatalf("same flow re-save should pass: %v", err)
	}

	// 同端口不同路径应通过
	dsl3 := &types.FlowDSL{
		ID: "flow-3",
		Nodes: []types.FlowNode{
			{
				ID:   "n1",
				Type: "httpEndpoint",
				Configuration: map[string]interface{}{
					"server":  ":18100",
					"routers": []interface{}{map[string]interface{}{"method": "GET", "path": "/other"}},
				},
			},
		},
	}
	if err := m.CheckHTTPRoutes("flow-3", dsl3); err != nil {
		t.Fatalf("same port different path should pass: %v", err)
	}
}
