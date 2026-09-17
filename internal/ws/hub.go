package ws

import (
	"encoding/json"
	"sync"
	"time"
)

// Event 服务端推送给编辑器的统一信封。
type Event struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"ts"`
}

// NewEvent 构造带时间戳的事件。
func NewEvent(typ string, payload any) Event {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err == nil {
			raw = b
		}
	}
	return Event{
		Type:      typ,
		Payload:   raw,
		Timestamp: time.Now().UnixMilli(),
	}
}

// FlowChangedPayload 流程增删改通知。
type FlowChangedPayload struct {
	Action string `json:"action"` // saved | deleted | group
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Source string `json:"source"` // api | mcp
}

// EditorCommandPayload MCP/服务端请求编辑器执行的动作。
type EditorCommandPayload struct {
	Action string `json:"action"` // refresh_canvas | reload_flows | open_flow
	FlowID string `json:"flowId,omitempty"`
}

// Hub 管理所有编辑器 WebSocket 连接，并广播事件。
type Hub struct {
	mu        sync.RWMutex
	clients   map[*Client]struct{}
	queryOnce sync.Once
	query     *queryState
}

// NewHub 创建空 Hub。
func NewHub() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

func (h *Hub) add(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// ClientCount 当前连接数（状态排查用）。
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Broadcast 向所有已连接编辑器推送事件。
func (h *Hub) Broadcast(ev Event) {
	if h == nil {
		return
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			// 客户端发送队列满则跳过，避免阻塞
		}
	}
}

// NotifyFlowChanged 流程变更广播。
func (h *Hub) NotifyFlowChanged(action, id, name, source string) {
	h.Broadcast(NewEvent("flow.changed", FlowChangedPayload{
		Action: action,
		ID:     id,
		Name:   name,
		Source: source,
	}))
}

// NotifyEditorCommand 请求编辑器执行指定动作（如刷新画布）。
func (h *Hub) NotifyEditorCommand(action, flowID string) {
	h.Broadcast(NewEvent("editor.command", EditorCommandPayload{
		Action: action,
		FlowID: flowID,
	}))
}

// DisconnectUser 关闭指定用户的全部编辑器 WebSocket（MCP 关闭时调用）。
func (h *Hub) DisconnectUser(userID string) {
	if h == nil || userID == "" {
		return
	}
	var list []*Client
	h.mu.RLock()
	for c := range h.clients {
		if c.user != nil && c.user.ID == userID {
			list = append(list, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range list {
		_ = c.conn.Close()
	}
}
