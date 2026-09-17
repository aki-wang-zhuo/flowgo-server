package ws

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// clientInbound 编辑器上行消息信封。
type clientInbound struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// readPump 读客户端消息；解析 editor.active 应答；关闭时注销。
func (c *Client) readPump() {
	defer func() {
		c.hub.remove(c)
		_ = c.conn.Close()
	}()
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read: %v", err)
			}
			break
		}
		c.handleInbound(data)
	}
}

// handleInbound 处理编辑器上行 JSON（忽略无法识别的类型）。
func (c *Client) handleInbound(data []byte) {
	var msg clientInbound
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	switch msg.Type {
	case "editor.active":
		var reply ActiveFlowReply
		if err := json.Unmarshal(msg.Payload, &reply); err != nil {
			return
		}
		c.hub.CompleteActiveQuery(reply)
	case "editor.patched":
		var reply PatchActiveFlowReply
		if err := json.Unmarshal(msg.Payload, &reply); err != nil {
			return
		}
		c.hub.CompleteActivePatch(reply)
	default:
		// ping 正文或其它类型：忽略
	}
}

// writePump 将 hub 投递的消息写出，并定期 Ping。
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
