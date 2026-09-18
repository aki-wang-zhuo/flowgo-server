package mqttendpoint

import (
	"strings"

	"github.com/flowgo/flowgo/api/types"
)

func nodeKey(track, flowID, nodeID string) string {
	return track + "\x00" + flowID + "\x00" + nodeID
}

func flowKey(track, flowID string) string {
	return track + "\x00" + flowID
}

func ensureDSLID(dsl *types.FlowDSL, flowID string) *types.FlowDSL {
	if dsl == nil {
		return nil
	}
	if dsl.ID == flowID {
		return dsl
	}
	out := cloneDSL(dsl)
	out.ID = flowID
	return out
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

func cloneDSL(in *types.FlowDSL) *types.FlowDSL {
	if in == nil {
		return nil
	}
	out := *in
	out.Nodes = append([]types.FlowNode(nil), in.Nodes...)
	out.Edges = append([]types.FlowEdge(nil), in.Edges...)
	return &out
}

func looksJSON(s string) bool {
	t := strings.TrimSpace(s)
	return len(t) > 0 && (t[0] == '{' || t[0] == '[')
}

func shortID(s string) string {
	s = strings.ReplaceAll(s, "-", "")
	if len(s) > 12 {
		return s[:12]
	}
	if s == "" {
		return "x"
	}
	return s
}
