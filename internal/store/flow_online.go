package store

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/ostafen/clover/v2/query"

	"github.com/flowgo/flowgo/api/types"
)

// ErrNoPublishHistory 无历史已发布快照，无法上线。
var ErrNoPublishHistory = errors.New("no publish history to restore")

// ErrAlreadyOnline 流程已在线上，无需再上线。
var ErrAlreadyOnline = errors.New("flow is already online")

// UnpublishFlow 下线：将当前已发布快照归档进历史，清空线上 published，草稿不变。
// 调用方需随后 SyncFlow / Invalidate，以从内存卸载 HTTP 入口。
func (s *Store) UnpublishFlow(flowID string) (*FlowRecord, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(flowID)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	doc := docs[0]
	if docBool(doc, "locked", false) {
		return nil, ErrFlowLocked
	}
	curJSON := DocString(doc, "publishedJSON")
	if curJSON == "" {
		return nil, ErrNoPublished
	}
	curVer := docInt(doc, "publishedVersion", 0)

	hist := readHistory(doc)
	hist = append(hist, historyEntry{
		Version:     curVer,
		Note:        DocString(doc, "publishedNote"),
		PublishedAt: DocString(doc, "publishedAt"),
		DSLJSON:     curJSON,
	})
	if len(hist) > maxPublishHistory {
		hist = hist[len(hist)-maxPublishHistory:]
	}
	histJSON, err := json.Marshal(hist)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
		"publishedJSON":    "",
		"publishedAt":      "",
		"publishedNote":    "",
		"publishedVersion": 0,
		"historyJSON":      string(histJSON),
		"updatedAt":        now,
	})); err != nil {
		return nil, err
	}
	return s.GetFlow(flowID)
}

// GoOnlineFromLatest 上线：从历史中取出最近一条已发布快照恢复为当前线上版，草稿不变。
// 无历史则 ErrNoPublishHistory；已在线上则 ErrAlreadyOnline。
func (s *Store) GoOnlineFromLatest(flowID string) (*FlowRecord, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(flowID)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	doc := docs[0]
	if docBool(doc, "locked", false) {
		return nil, ErrFlowLocked
	}
	if DocString(doc, "publishedJSON") != "" {
		return nil, ErrAlreadyOnline
	}
	hist := readHistory(doc)
	if len(hist) == 0 {
		return nil, ErrNoPublishHistory
	}
	// 最近已发布 = 历史末尾（下线时追加的那条）
	last := hist[len(hist)-1]
	rest := hist[:len(hist)-1]
	histJSON, err := json.Marshal(rest)
	if err != nil {
		return nil, err
	}
	var dsl types.FlowDSL
	if err := json.Unmarshal([]byte(last.DSLJSON), &dsl); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	pubAt := last.PublishedAt
	if pubAt == "" {
		pubAt = now
	}
	if err := s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
		"publishedJSON":    last.DSLJSON,
		"publishedAt":      pubAt,
		"publishedNote":    last.Note,
		"publishedVersion": last.Version,
		"historyJSON":      string(histJSON),
		"updatedAt":        now,
	})); err != nil {
		return nil, err
	}
	return s.GetFlow(flowID)
}

// PeekLatestPublishDSL 预览历史上最近一条已发布 DSL（不上线），供 HTTP 路由冲突校验。
func (s *Store) PeekLatestPublishDSL(flowID string) (*types.FlowDSL, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(flowID)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	hist := readHistory(docs[0])
	if len(hist) == 0 {
		return nil, ErrNoPublishHistory
	}
	last := hist[len(hist)-1]
	var dsl types.FlowDSL
	if err := json.Unmarshal([]byte(last.DSLJSON), &dsl); err != nil {
		return nil, err
	}
	return &dsl, nil
}
