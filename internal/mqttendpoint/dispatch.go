/**
 * MQTT 消息到达后的流程执行与调试日志推送。
 */
package mqttendpoint

import (
	"context"
	"fmt"
	"log"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/engine"
)

func (m *Manager) onMessage(dsl *types.FlowDSL, flowID, nodeID, cacheTrack string, pm paho.Message) {
	if dsl == nil || pm == nil {
		return
	}
	if dsl.ID == "" {
		dsl = cloneDSL(dsl)
		dsl.ID = flowID
	}
	payload := string(pm.Payload())
	dataType := types.TEXT
	if looksJSON(payload) {
		dataType = types.JSON
	}
	meta := types.Metadata{
		"mqttTopic": pm.Topic(),
		"mqttQos":   fmt.Sprintf("%d", pm.Qos()),
	}
	if pm.Retained() {
		meta["mqttRetained"] = "true"
	}
	msg := types.NewMsg("MQTT", dataType, payload, meta)
	startID := nextNodeByRelation(dsl, nodeID, types.RelationSuccess)
	if startID == "" {
		log.Printf("mqttendpoint: flow=%s node=%s 无 Success 出边，忽略消息", flowID, nodeID)
		// 无出边：仍仅在草稿且收节点 Debug 时提示
		if cacheTrack == engine.CacheTrackDraft && inDebug(dsl, nodeID) {
			m.pushDebug(flowID, []types.DebugLog{{
				Ts:       time.Now().UnixMilli(),
				FlowType: "ERROR",
				NodeID:   nodeID,
				NodeName: nodeName(dsl, nodeID),
				Err:      "no Success outgoing edge",
				Data:     payload,
			}}, "no Success outgoing edge")
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	opts := engine.ExecuteOptions{CacheTrack: cacheTrack}
	wantDebug := cacheTrack == engine.CacheTrackDraft && inDebug(dsl, nodeID)
	if !wantDebug {
		opts.SkipDebugLogs = true
	}
	_, logs, err := m.engine.ExecuteFromWithLogsOpts(ctx, dsl, startID, msg, opts)
	if wantDebug {
		name := nodeName(dsl, nodeID)
		now := time.Now().UnixMilli()
		head := []types.DebugLog{
			{Ts: now, FlowType: types.DebugFlowIN, NodeID: nodeID, NodeName: name, Data: payload},
			{
				Ts: now, FlowType: types.DebugFlowOUT, NodeID: nodeID, NodeName: name,
				RelationType: types.RelationSuccess, Data: payload,
			},
		}
		logs = append(head, logs...)
	}
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
		log.Printf("mqttendpoint: execute flow=%s from=%s: %v", flowID, startID, err)
	}
	if !wantDebug {
		return
	}
	m.pushDebug(flowID, logs, errMsg)
}

func (m *Manager) pushDebug(flowID string, logs []types.DebugLog, errMsg string) {
	if m == nil || m.hub == nil {
		return
	}
	m.hub.NotifyFlowDebug(flowID, logs, errMsg)
}

func inDebug(dsl *types.FlowDSL, nodeID string) bool {
	if dsl == nil {
		return false
	}
	for _, n := range dsl.Nodes {
		if n.ID == nodeID {
			return n.Debug
		}
	}
	return false
}

func nodeName(dsl *types.FlowDSL, nodeID string) string {
	if dsl == nil {
		return nodeID
	}
	for _, n := range dsl.Nodes {
		if n.ID == nodeID {
			if n.Name != "" {
				return n.Name
			}
			return n.Type
		}
	}
	return nodeID
}
