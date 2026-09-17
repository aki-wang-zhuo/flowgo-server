// 编辑器当前激活流程查询（MCP get_active_flow ↔ WebSocket 往返）。
package ws

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// EditorQueryPayload 服务端询问编辑器当前激活流程。
type EditorQueryPayload struct {
	RequestID  string `json:"requestId"`
	IncludeDSL bool   `json:"includeDsl"`
}

// ActiveFlowReply 编辑器对 editor.query 的应答（MCP 结果体）。
type ActiveFlowReply struct {
	RequestID string          `json:"requestId,omitempty"`
	Active    bool            `json:"active"`
	FlowID    string          `json:"flowId,omitempty"`
	Name      string          `json:"name,omitempty"`
	Dirty     bool            `json:"dirty,omitempty"`
	Locked    bool            `json:"locked,omitempty"`
	OpenIDs   []string        `json:"openIds,omitempty"`
	Revision  int64           `json:"revision,omitempty"`
	DSL       json.RawMessage `json:"dsl,omitempty"`
}

const defaultActiveQueryTimeout = 3 * time.Second

// Hub 查询相关状态（与 hub.go 同包扩展）。
type queryState struct {
	mu           sync.Mutex
	pending      map[string]chan ActiveFlowReply
	pendingPatch map[string]chan PatchActiveFlowReply
}

func (h *Hub) ensureQuery() {
	h.queryOnce.Do(func() {
		h.query = &queryState{
			pending:      make(map[string]chan ActiveFlowReply),
			pendingPatch: make(map[string]chan PatchActiveFlowReply),
		}
	})
}

// newRequestID 生成短随机请求 ID。
func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// SendToUser 向指定用户的所有编辑器连接推送事件，返回投递连接数。
func (h *Hub) SendToUser(userID string, ev Event) int {
	if h == nil || userID == "" {
		return 0
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for c := range h.clients {
		if c.user == nil || c.user.ID != userID {
			continue
		}
		select {
		case c.send <- data:
			n++
		default:
		}
	}
	return n
}

// CompleteActiveQuery 编辑器应答到达时调用。
func (h *Hub) CompleteActiveQuery(reply ActiveFlowReply) {
	if h == nil || reply.RequestID == "" {
		return
	}
	h.ensureQuery()
	h.query.mu.Lock()
	ch, ok := h.query.pending[reply.RequestID]
	if ok {
		delete(h.query.pending, reply.RequestID)
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

// QueryActiveFlow 向该用户已连接编辑器询问当前激活流程；超时或无连接则报错。
func (h *Hub) QueryActiveFlow(userID string, includeDSL bool, timeout time.Duration) (*ActiveFlowReply, error) {
	if h == nil {
		return nil, fmt.Errorf("websocket hub not ready")
	}
	if userID == "" {
		return nil, fmt.Errorf("unauthorized")
	}
	if timeout <= 0 {
		timeout = defaultActiveQueryTimeout
	}
	h.ensureQuery()
	reqID := newRequestID()
	ch := make(chan ActiveFlowReply, 1)
	h.query.mu.Lock()
	h.query.pending[reqID] = ch
	h.query.mu.Unlock()

	defer func() {
		h.query.mu.Lock()
		delete(h.query.pending, reqID)
		h.query.mu.Unlock()
	}()

	n := h.SendToUser(userID, NewEvent("editor.query", EditorQueryPayload{
		RequestID:  reqID,
		IncludeDSL: includeDSL,
	}))
	if n == 0 {
		return nil, fmt.Errorf("no editor connected")
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case reply := <-ch:
		reply.RequestID = "" // MCP 结果不必回传内部 ID
		return &reply, nil
	case <-timer.C:
		return nil, fmt.Errorf("editor query timeout")
	}
}
