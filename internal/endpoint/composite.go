/**
 * HTTP 与 MQTT 入口管理器的组合实现，满足 api.EndpointSync。
 * SyncFlow / SyncDraft / RemoveFlow 同时驱动两类入口；路由冲突校验仅走 HTTP。
 */
package endpoint

import (
	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo-server/internal/mqttendpoint"
	"github.com/flowgo/flowgo-server/internal/store"
)

// Composite 聚合 HTTP Manager 与 MQTT Manager。
type Composite struct {
	HTTP *Manager
	MQTT *mqttendpoint.Manager
}

// NewComposite 创建组合入口同步器。
func NewComposite(httpMgr *Manager, mqttMgr *mqttendpoint.Manager) *Composite {
	return &Composite{HTTP: httpMgr, MQTT: mqttMgr}
}

// CheckHTTPRoutes 委托 HTTP 管理器。
func (c *Composite) CheckHTTPRoutes(flowID string, dsl *types.FlowDSL) error {
	if c == nil || c.HTTP == nil {
		return nil
	}
	return c.HTTP.CheckHTTPRoutes(flowID, dsl)
}

// SyncFlow 先同步 HTTP，再同步 MQTT 已发布轨。
func (c *Composite) SyncFlow(rec *store.FlowRecord) error {
	if c == nil {
		return nil
	}
	if c.HTTP != nil {
		if err := c.HTTP.SyncFlow(rec); err != nil {
			return err
		}
	}
	if c.MQTT != nil {
		if err := c.MQTT.SyncFlow(rec); err != nil {
			return err
		}
	}
	return nil
}

// SyncDraftIfOpen 仅当编辑器打开该流程时同步草稿 MQTT。
func (c *Composite) SyncDraftIfOpen(rec *store.FlowRecord) error {
	if c == nil || c.MQTT == nil {
		return nil
	}
	return c.MQTT.SyncDraftIfOpen(rec)
}

// SyncDraft 保留给会话打开时直接调用（等同 SyncDraft 本体）。
func (c *Composite) SyncDraft(rec *store.FlowRecord) error {
	if c == nil || c.MQTT == nil {
		return nil
	}
	return c.MQTT.SyncDraft(rec)
}

// RemoveFlow 同时卸载 HTTP、MQTT 已发布轨与草稿轨。
func (c *Composite) RemoveFlow(flowID string) {
	if c == nil {
		return
	}
	if c.HTTP != nil {
		c.HTTP.RemoveFlow(flowID)
	}
	if c.MQTT != nil {
		c.MQTT.RemoveFlow(flowID)
		c.MQTT.RemoveDraft(flowID)
	}
}

// StartAll 恢复 HTTP 入口与已发布 MQTT；草稿 MQTT 等编辑器打开流程后再挂载。
func (c *Composite) StartAll() {
	if c == nil {
		return
	}
	if c.HTTP != nil {
		c.HTTP.StartAll()
	}
	if c.MQTT != nil {
		c.MQTT.StartAll()
	}
}
