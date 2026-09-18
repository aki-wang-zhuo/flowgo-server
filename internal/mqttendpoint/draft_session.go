/**
 * 草稿 MQTT「连接并响应」仅随编辑器打开的流程挂载：
 * 有任一编辑器会话打开该流程才 SyncDraft；全部关闭则 RemoveDraft。
 */
package mqttendpoint

import (
	"log"
	"strings"

	"github.com/flowgo/flowgo-server/internal/store"
)

// UpdateSessionOpenFlows 更新某编辑器会话当前打开的流程 id 列表。
// 新打开的未发布流程会尝试 SyncDraft；不再打开的流程在引用计数归零后释放草稿 MQTT。
func (m *Manager) UpdateSessionOpenFlows(sessionID string, openIDs []string) {
	if m == nil || sessionID == "" {
		return
	}
	wanted := uniqueNonEmpty(openIDs)

	m.sessionMu.Lock()
	old := m.sessions[sessionID]
	if old == nil {
		old = map[string]struct{}{}
	}
	added, removed := diffFlowSets(old, wanted)

	// 先更新会话集合与引用计数
	next := make(map[string]struct{}, len(wanted))
	for _, id := range wanted {
		next[id] = struct{}{}
	}
	m.sessions[sessionID] = next
	for _, id := range removed {
		m.openCount[id]--
		if m.openCount[id] <= 0 {
			delete(m.openCount, id)
		}
	}
	for _, id := range added {
		m.openCount[id]++
	}
	// 复制需要动作的 id，避免持锁调 Sync/Remove
	toClose := make([]string, 0)
	for _, id := range removed {
		if m.openCount[id] == 0 {
			toClose = append(toClose, id)
		}
	}
	toOpen := make([]string, 0)
	for _, id := range added {
		if m.openCount[id] == 1 {
			toOpen = append(toOpen, id)
		}
	}
	m.sessionMu.Unlock()

	for _, id := range toClose {
		m.RemoveDraft(id)
		log.Printf("mqttendpoint: 编辑器已关闭流程，释放草稿 MQTT flow=%s", id)
	}
	for _, id := range toOpen {
		if err := m.syncDraftByID(id); err != nil {
			log.Printf("mqttendpoint: 打开流程草稿 MQTT 失败 flow=%s: %v", id, err)
		}
	}
	// 兜底：不在任何会话打开列表中的草稿连接一律释放
	m.closeUnopenedDrafts()
}

// ReleaseSession 编辑器断开时释放其打开的全部草稿引用。
func (m *Manager) ReleaseSession(sessionID string) {
	if m == nil || sessionID == "" {
		return
	}
	m.sessionMu.Lock()
	old := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	toClose := make([]string, 0)
	for id := range old {
		m.openCount[id]--
		if m.openCount[id] <= 0 {
			delete(m.openCount, id)
			toClose = append(toClose, id)
		}
	}
	m.sessionMu.Unlock()
	for _, id := range toClose {
		m.RemoveDraft(id)
		log.Printf("mqttendpoint: 会话断开，释放草稿 MQTT flow=%s", id)
	}
	m.closeUnopenedDrafts()
}

// IsDraftOpen 是否有编辑器正在打开该流程（草稿 MQTT 允许挂载）。
func (m *Manager) IsDraftOpen(flowID string) bool {
	if m == nil || flowID == "" {
		return false
	}
	m.sessionMu.RLock()
	defer m.sessionMu.RUnlock()
	return m.openCount[flowID] > 0
}

// SyncDraftIfOpen 仅当流程被编辑器打开时同步草稿 MQTT；未打开则确保已释放。
func (m *Manager) SyncDraftIfOpen(rec *store.FlowRecord) error {
	if rec == nil {
		return nil
	}
	if !m.IsDraftOpen(rec.ID) {
		m.RemoveDraft(rec.ID)
		return nil
	}
	return m.SyncDraft(rec)
}

// closeUnopenedDrafts 释放 openCount 为零的全部草稿轨连接（含历史残留）。
func (m *Manager) closeUnopenedDrafts() {
	if m == nil {
		return
	}
	m.mu.Lock()
	ids := make([]string, 0)
	prefix := trackDraft + "\x00"
	for fk := range m.byFlow {
		if strings.HasPrefix(fk, prefix) {
			ids = append(ids, strings.TrimPrefix(fk, prefix))
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		if !m.IsDraftOpen(id) {
			m.RemoveDraft(id)
			log.Printf("mqttendpoint: 清理未打开草稿 MQTT flow=%s", id)
		}
	}
}

func (m *Manager) syncDraftByID(flowID string) error {
	if m.store == nil {
		return nil
	}
	rec, err := m.store.GetFlow(flowID)
	if err != nil {
		return err
	}
	return m.SyncDraft(rec)
}

func uniqueNonEmpty(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func diffFlowSets(old map[string]struct{}, wanted []string) (added, removed []string) {
	want := make(map[string]struct{}, len(wanted))
	for _, id := range wanted {
		want[id] = struct{}{}
		if _, ok := old[id]; !ok {
			added = append(added, id)
		}
	}
	for id := range old {
		if _, ok := want[id]; !ok {
			removed = append(removed, id)
		}
	}
	return added, removed
}
