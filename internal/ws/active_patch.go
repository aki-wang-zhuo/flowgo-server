// 编辑器当前激活流程增量补丁（MCP patch_active_flow ↔ WebSocket 往返）。
package ws

import (
	"encoding/json"
	"fmt"
	"time"
)

// EditorPatchPayload 服务端请求编辑器对当前激活流程做增量修改。
type EditorPatchPayload struct {
	RequestID  string          `json:"requestId"`
	FlowID     string          `json:"flowId,omitempty"` // 可选；非空则须与激活流程一致
	IncludeDSL bool            `json:"includeDsl"`
	Patch      json.RawMessage `json:"patch"` // 仅含变更字段的 JSON
}

// PatchActiveFlowReply 编辑器应用补丁后的应答。
type PatchActiveFlowReply struct {
	RequestID string          `json:"requestId,omitempty"`
	OK        bool            `json:"ok"`
	FlowID    string          `json:"flowId,omitempty"`
	Name      string          `json:"name,omitempty"`
	Dirty     bool            `json:"dirty,omitempty"`
	Locked    bool            `json:"locked,omitempty"`
	Revision  int64           `json:"revision,omitempty"`
	Applied   json.RawMessage `json:"applied,omitempty"` // 实际生效的变更摘要
	Errors    []PatchItemError `json:"errors,omitempty"`
	DSL       json.RawMessage `json:"dsl,omitempty"`
	Message   string          `json:"message,omitempty"`
}

// PatchItemError 单条补丁失败说明。
type PatchItemError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

const defaultActivePatchTimeout = 5 * time.Second

// CompleteActivePatch 编辑器应答到达时调用。
func (h *Hub) CompleteActivePatch(reply PatchActiveFlowReply) {
	if h == nil || reply.RequestID == "" {
		return
	}
	h.ensureQuery()
	h.query.mu.Lock()
	ch, ok := h.query.pendingPatch[reply.RequestID]
	if ok {
		delete(h.query.pendingPatch, reply.RequestID)
	}
	h.query.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- reply:
	default:
	}
}

// PatchActiveFlow 向该用户已连接编辑器下发增量补丁并等待结果。
func (h *Hub) PatchActiveFlow(userID string, flowID string, patch json.RawMessage, includeDSL bool, timeout time.Duration) (*PatchActiveFlowReply, error) {
	if h == nil {
		return nil, fmt.Errorf("websocket hub not ready")
	}
	if userID == "" {
		return nil, fmt.Errorf("unauthorized")
	}
	if len(patch) == 0 || string(patch) == "null" {
		return nil, fmt.Errorf("patch is required")
	}
	if timeout <= 0 {
		timeout = defaultActivePatchTimeout
	}
	h.ensureQuery()
	reqID := newRequestID()
	ch := make(chan PatchActiveFlowReply, 1)
	h.query.mu.Lock()
	h.query.pendingPatch[reqID] = ch
	h.query.mu.Unlock()

	defer func() {
		h.query.mu.Lock()
		delete(h.query.pendingPatch, reqID)
		h.query.mu.Unlock()
	}()

	n := h.SendToUser(userID, NewEvent("editor.patch", EditorPatchPayload{
		RequestID:  reqID,
		FlowID:     flowID,
		IncludeDSL: includeDSL,
		Patch:      patch,
	}))
	if n == 0 {
		return nil, fmt.Errorf("no editor connected")
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case reply := <-ch:
		reply.RequestID = ""
		return &reply, nil
	case <-timer.C:
		return nil, fmt.Errorf("editor patch timeout")
	}
}
