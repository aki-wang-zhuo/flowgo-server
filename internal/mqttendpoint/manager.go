/**
 * MQTT 入口 / 常驻发：按已发布 DSL 与草稿「连接并响应」管理客户端。
 * - 已发布：mqttIn 始终订阅；mqttOut 常驻/复用按配置建连
 * - 草稿：仅 mqttIn 且 connectAndRespond=true 时订阅（用草稿 DSL 响应）
 * 流程下线/删除时释放对应轨全部自有客户端。
 */
package mqttendpoint

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/components/iot"
	"github.com/flowgo/flowgo/engine"
	"github.com/flowgo/flowgo/utils/mqtt"
	"github.com/flowgo/flowgo-server/internal/store"
	"github.com/flowgo/flowgo-server/internal/ws"
)

const (
	kindIn         = "in"
	kindOutPersist = "out-persistent"
	kindOutReuse   = "out-reuse"

	trackPublished = "published"
	trackDraft     = "draft"
)

// clientHandle 一条活跃 MQTT 连接或复用映射。
type clientHandle struct {
	track   string
	flowID  string
	nodeID  string
	kind    string
	client  paho.Client
	topic   string
	owned   bool
	reuseOf string
}

// Manager 管理 MQTT 收订阅与发常驻/复用客户端。
type Manager struct {
	mu     sync.Mutex
	store  *store.Store
	engine *engine.Engine
	hub    *ws.Hub
	byNode map[string]*clientHandle
	byFlow map[string][]string // key = track+"\x00"+flowID → node keys

	// 草稿「连接并响应」仅绑定编辑器已打开的流程
	sessionMu sync.RWMutex
	sessions  map[string]map[string]struct{} // sessionID → flowIDs
	openCount map[string]int                 // flowID → 打开引用数
}

// NewManager 创建管理器并注册为 PublisherPool。
func NewManager(st *store.Store, eng *engine.Engine, hub *ws.Hub) *Manager {
	m := &Manager{
		store:     st,
		engine:    eng,
		hub:       hub,
		byNode:    make(map[string]*clientHandle),
		byFlow:    make(map[string][]string),
		sessions:  make(map[string]map[string]struct{}),
		openCount: make(map[string]int),
	}
	iot.SetPublisherPool(m)
	return m
}

// Publisher 实现 iot.PublisherPool：优先已发布轨，其次草稿轨（连接并响应）。
func (m *Manager) Publisher(flowID, outNodeID string) (paho.Client, bool) {
	if m == nil || flowID == "" || outNodeID == "" {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if h := m.byNode[nodeKey(trackPublished, flowID, outNodeID)]; h != nil && h.client != nil {
		return h.client, true
	}
	if h := m.byNode[nodeKey(trackDraft, flowID, outNodeID)]; h != nil && h.client != nil {
		return h.client, true
	}
	return nil, false
}

// StartAll 仅恢复已发布流程的 MQTT 入口；草稿「连接并响应」等编辑器打开流程后再挂载。
func (m *Manager) StartAll() {
	list, err := m.store.ListFlows(&store.User{Role: store.RoleAdmin})
	if err != nil {
		log.Printf("mqttendpoint: list flows: %v", err)
		return
	}
	for _, rec := range list {
		if rec == nil || rec.PublishedDSL == nil {
			continue
		}
		if err := m.SyncFlow(rec); err != nil {
			log.Printf("mqttendpoint: sync published %s: %v", rec.ID, err)
		}
	}
	// 启动时清掉未打开的草稿轨，避免热更新/旧逻辑残留连接
	m.closeUnopenedDrafts()
}

// SyncFlow 重建已发布 MQTT 资源；同时清除草稿轨避免双连。
func (m *Manager) SyncFlow(rec *store.FlowRecord) error {
	if rec == nil {
		return nil
	}
	m.RemoveDraft(rec.ID)
	dsl := rec.PublishedDSL
	if dsl == nil {
		m.RemoveFlow(rec.ID)
		return nil
	}
	dsl = ensureDSLID(dsl, rec.ID)
	m.RemoveFlow(rec.ID)

	for _, node := range dsl.Nodes {
		if node.Type != iot.TypeMqttIn {
			continue
		}
		cfg, err := iot.ParseMqttInConfig(node.Configuration)
		if err != nil {
			m.RemoveFlow(rec.ID)
			return fmt.Errorf("mqttIn %s: %w", node.ID, err)
		}
		if err := m.addIn(trackPublished, rec.ID, node.ID, cfg, cloneDSL(dsl)); err != nil {
			m.RemoveFlow(rec.ID)
			return err
		}
	}
	for _, node := range dsl.Nodes {
		if node.Type != iot.TypeMqttOut {
			continue
		}
		cfg, err := iot.ParseMqttOutConfig(node.Configuration)
		if err != nil {
			m.RemoveFlow(rec.ID)
			return fmt.Errorf("mqttOut %s: %w", node.ID, err)
		}
		if !cfg.NeedsManagedClient() {
			continue
		}
		if err := m.addOut(trackPublished, rec.ID, node.ID, cfg); err != nil {
			m.RemoveFlow(rec.ID)
			return err
		}
	}
	return nil
}

// SyncDraft 未发布时：为 connectAndRespond 的 mqttIn 建草稿订阅，并拉起依赖的 mqttOut 常驻/复用。
// 已发布则只清草稿轨。未在编辑器打开时拒绝挂载并确保已释放。
func (m *Manager) SyncDraft(rec *store.FlowRecord) error {
	if rec == nil {
		return nil
	}
	if !m.IsDraftOpen(rec.ID) {
		m.RemoveDraft(rec.ID)
		return nil
	}
	if rec.PublishedDSL != nil {
		m.RemoveDraft(rec.ID)
		return nil
	}
	dsl := rec.DSL
	if dsl == nil {
		m.RemoveDraft(rec.ID)
		return nil
	}
	dsl = ensureDSLID(dsl, rec.ID)
	m.RemoveDraft(rec.ID)

	activeIn := map[string]bool{}
	for _, node := range dsl.Nodes {
		if node.Type != iot.TypeMqttIn {
			continue
		}
		cfg, err := iot.ParseMqttInConfig(node.Configuration)
		if err != nil {
			m.RemoveDraft(rec.ID)
			return fmt.Errorf("mqttIn %s: %w", node.ID, err)
		}
		if !cfg.ConnectAndRespond {
			continue
		}
		if err := m.addIn(trackDraft, rec.ID, node.ID, cfg, cloneDSL(dsl)); err != nil {
			m.RemoveDraft(rec.ID)
			return err
		}
		activeIn[node.ID] = true
	}
	// 草稿下：复用/常驻发节点也要托管，否则收消息触发发会报 managed client unavailable
	for _, node := range dsl.Nodes {
		if node.Type != iot.TypeMqttOut {
			continue
		}
		cfg, err := iot.ParseMqttOutConfig(node.Configuration)
		if err != nil {
			m.RemoveDraft(rec.ID)
			return fmt.Errorf("mqttOut %s: %w", node.ID, err)
		}
		if !cfg.NeedsManagedClient() {
			continue
		}
		if cfg.ReuseFrom != "" && !activeIn[cfg.ReuseFrom] {
			// 复用的收节点未开启「连接并响应」，跳过托管（运行时会失败并提示）
			continue
		}
		if err := m.addOut(trackDraft, rec.ID, node.ID, cfg); err != nil {
			m.RemoveDraft(rec.ID)
			return err
		}
	}
	return nil
}

// RemoveFlow 卸载已发布轨。
func (m *Manager) RemoveFlow(flowID string) {
	m.removeTrack(trackPublished, flowID)
}

// RemoveDraft 卸载草稿轨。
func (m *Manager) RemoveDraft(flowID string) {
	m.removeTrack(trackDraft, flowID)
}

func (m *Manager) removeTrack(track, flowID string) {
	if flowID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	fk := flowKey(track, flowID)
	keys := m.byFlow[fk]
	delete(m.byFlow, fk)
	for _, key := range keys {
		h := m.byNode[key]
		if h == nil {
			continue
		}
		m.stopLocked(h)
		delete(m.byNode, key)
	}
}

func (m *Manager) addIn(track, flowID, nodeID string, cfg iot.MqttInConfig, dsl *types.FlowDSL) error {
	broker := cfg.Broker
	if strings.TrimSpace(broker.ClientID) == "" {
		prefix := "flowgo"
		if track == trackDraft {
			prefix = "flowgo-draft"
		}
		broker.ClientID = fmt.Sprintf("%s-%s-%s", prefix, shortID(flowID), shortID(nodeID))
	}
	client, err := mqtt.Connect(broker, 15*time.Second)
	if err != nil {
		return fmt.Errorf("mqttIn %s connect: %w", nodeID, err)
	}
	h := &clientHandle{
		track:  track,
		flowID: flowID,
		nodeID: nodeID,
		kind:   kindIn,
		client: client,
		topic:  cfg.Topic,
		owned:  true,
	}
	cacheTrack := engine.CacheTrackPublished
	if track == trackDraft {
		cacheTrack = engine.CacheTrackDraft
	}
	handler := func(_ paho.Client, pm paho.Message) {
		m.onMessage(dsl, flowID, nodeID, cacheTrack, pm)
	}
	token := client.Subscribe(cfg.Topic, cfg.QoS, handler)
	if !token.WaitTimeout(10 * time.Second) {
		client.Disconnect(250)
		return fmt.Errorf("mqttIn %s subscribe timeout topic=%s", nodeID, cfg.Topic)
	}
	if err := token.Error(); err != nil {
		client.Disconnect(250)
		return fmt.Errorf("mqttIn %s subscribe: %w", nodeID, err)
	}
	m.mu.Lock()
	m.registerLocked(h)
	m.mu.Unlock()
	log.Printf("mqttendpoint: mqttIn 已订阅 track=%s flow=%s node=%s topic=%s", track, flowID, nodeID, cfg.Topic)
	return nil
}

func (m *Manager) addOut(track, flowID, nodeID string, cfg iot.MqttOutConfig) error {
	if cfg.ReuseFrom != "" {
		m.mu.Lock()
		in := m.byNode[nodeKey(track, flowID, cfg.ReuseFrom)]
		m.mu.Unlock()
		if in == nil || in.kind != kindIn || in.client == nil {
			return fmt.Errorf("mqttOut %s: reuseFrom %q not found or not mqttIn", nodeID, cfg.ReuseFrom)
		}
		h := &clientHandle{
			track:   track,
			flowID:  flowID,
			nodeID:  nodeID,
			kind:    kindOutReuse,
			client:  in.client,
			owned:   false,
			reuseOf: cfg.ReuseFrom,
		}
		m.mu.Lock()
		m.registerLocked(h)
		m.mu.Unlock()
		log.Printf("mqttendpoint: mqttOut 复用 mqttIn track=%s flow=%s out=%s in=%s", track, flowID, nodeID, cfg.ReuseFrom)
		return nil
	}
	if cfg.SessionMode != iot.SessionPersistent {
		return nil
	}
	broker := cfg.Broker
	if strings.TrimSpace(broker.ClientID) == "" {
		broker.ClientID = fmt.Sprintf("flowgo-%s-%s", shortID(flowID), shortID(nodeID))
	}
	client, err := mqtt.Connect(broker, 15*time.Second)
	if err != nil {
		return fmt.Errorf("mqttOut %s connect: %w", nodeID, err)
	}
	h := &clientHandle{
		track:  track,
		flowID: flowID,
		nodeID: nodeID,
		kind:   kindOutPersist,
		client: client,
		owned:  true,
	}
	m.mu.Lock()
	m.registerLocked(h)
	m.mu.Unlock()
	log.Printf("mqttendpoint: mqttOut 常驻已连接 track=%s flow=%s node=%s", track, flowID, nodeID)
	return nil
}

func (m *Manager) registerLocked(h *clientHandle) {
	key := nodeKey(h.track, h.flowID, h.nodeID)
	m.byNode[key] = h
	fk := flowKey(h.track, h.flowID)
	m.byFlow[fk] = append(m.byFlow[fk], key)
}

func (m *Manager) stopLocked(h *clientHandle) {
	if h == nil {
		return
	}
	if !h.owned || h.client == nil {
		log.Printf("mqttendpoint: 释放映射 track=%s flow=%s node=%s kind=%s", h.track, h.flowID, h.nodeID, h.kind)
		return
	}
	if h.topic != "" {
		token := h.client.Unsubscribe(h.topic)
		_ = token.WaitTimeout(3 * time.Second)
	}
	h.client.Disconnect(250)
	log.Printf("mqttendpoint: 已断开 track=%s flow=%s node=%s kind=%s", h.track, h.flowID, h.nodeID, h.kind)
}
