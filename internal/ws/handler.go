package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/flowgo/flowgo-server/internal/auth"
	"github.com/flowgo/flowgo-server/internal/store"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 25 * time.Second
	sendBuf    = 32
)

var wsSessionSeq atomic.Uint64

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// 开发期允许任意 Origin；生产可由反向代理收紧
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client 单条编辑器连接。
type Client struct {
	hub       *Hub
	user      *store.User
	conn      *websocket.Conn
	send      chan []byte
	sessionID string
}

// Handler 返回 WebSocket 升级处理器：GET /api/ws?token=<JWT>
// 用户关闭 MCP 全局开关后拒绝升级。
func Handler(hub *Hub, authSvc *auth.Service, st *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := authenticateWS(r, authSvc)
		if err != nil || user == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if st != nil {
			enabled, err := st.IsMcpEnabled(user.ID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if !enabled {
				http.Error(w, "mcp disabled", http.StatusForbidden)
				return
			}
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade: %v", err)
			return
		}
		c := &Client{
			hub:       hub,
			user:      user,
			conn:      conn,
			send:      make(chan []byte, sendBuf),
			sessionID: fmt.Sprintf("ws-%d", wsSessionSeq.Add(1)),
		}
		hub.add(c)

		// 连接成功问候
		hello, _ := json.Marshal(NewEvent("server.hello", map[string]any{
			"userId":   user.ID,
			"username": user.Username,
			"online":   true,
		}))
		select {
		case c.send <- hello:
		default:
		}

		go c.writePump()
		go c.readPump()
	})
}

func authenticateWS(r *http.Request, authSvc *auth.Service) (*store.User, error) {
	// 优先 query token（浏览器 WS 无法自定义 Authorization 时常用）
	if tok := r.URL.Query().Get("token"); tok != "" {
		claims, err := authSvc.ParseToken(tok)
		if err != nil {
			return nil, err
		}
		return authSvc.UserByID(claims.UserID)
	}
	return authSvc.AuthenticateRequest(r)
}
