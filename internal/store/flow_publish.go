package store

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"

	"github.com/flowgo/flowgo/api/types"
)

// maxPublishHistory 保留的已发布历史条数（含当前即将被替换的版本）。
const maxPublishHistory = 20

// PublishHistoryItem 一次发布快照（列表默认不含 DSL 正文）。
type PublishHistoryItem struct {
	Version     int            `json:"version"`
	Note        string         `json:"note,omitempty"`
	PublishedAt string         `json:"publishedAt"`
	DSL         *types.FlowDSL `json:"dsl,omitempty"`
}

type historyEntry struct {
	Version     int    `json:"version"`
	Note        string `json:"note,omitempty"`
	PublishedAt string `json:"publishedAt"`
	DSLJSON     string `json:"dslJSON"`
}

// migrateLegacyPublished 旧库只有 dslJSON：视为已发布且草稿相同，并写回 publishedJSON。
func (s *Store) migrateLegacyPublished(doc *document.Document, rec *FlowRecord) error {
	if rec == nil || rec.PublishedDSL != nil {
		return nil
	}
	if rec.DSL == nil {
		return nil
	}
	raw := DocString(doc, "dslJSON")
	if raw == "" {
		return nil
	}
	now := rec.UpdatedAt
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	if err := s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
		"publishedJSON":    raw,
		"publishedAt":      now,
		"publishedVersion": 1,
		"historyJSON":      "[]",
	})); err != nil {
		return err
	}
	rec.PublishedDSL = cloneFlowDSL(rec.DSL)
	rec.PublishedAt = now
	rec.PublishedVersion = 1
	applyPublishStatus(rec)
	return nil
}

// PublishFlow 将当前草稿复制为已发布；旧 published 写入历史。
func (s *Store) PublishFlow(flowID, note string) (*FlowRecord, error) {
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
	draftRaw := DocString(doc, "dslJSON")
	if strings.TrimSpace(draftRaw) == "" {
		return nil, errors.New("draft dsl is empty")
	}
	var draft types.FlowDSL
	if err := json.Unmarshal([]byte(draftRaw), &draft); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	nextVer := docInt(doc, "publishedVersion", 0) + 1
	hist := readHistory(doc)
	if old := DocString(doc, "publishedJSON"); old != "" {
		hist = append(hist, historyEntry{
			Version:     docInt(doc, "publishedVersion", 0),
			Note:        "",
			PublishedAt: DocString(doc, "publishedAt"),
			DSLJSON:     old,
		})
		if len(hist) > maxPublishHistory {
			hist = hist[len(hist)-maxPublishHistory:]
		}
	}
	histJSON, err := json.Marshal(hist)
	if err != nil {
		return nil, err
	}

	if err := s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
		"name":             draft.Name,
		"publishedJSON":    draftRaw,
		"publishedAt":      now,
		"publishedVersion": nextVer,
		"historyJSON":      string(histJSON),
		"updatedAt":        now,
	})); err != nil {
		return nil, err
	}
	// 发布说明写在「新版本」上：覆盖最后一次 history 条目前的 note 不便检索，
	// 单独把 note 存在 publishedNote 字段，并写入一条当前版本的虚记录不便。
	// 采用：history 存被替换的旧版；当前版本 note 存在 publishedNote。
	_ = s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
		"publishedNote": strings.TrimSpace(note),
	}))
	return s.GetFlow(flowID)
}

// DiscardDraft 用已发布 DSL 覆盖草稿。
func (s *Store) DiscardDraft(flowID string) (*FlowRecord, error) {
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
	pub := DocString(doc, "publishedJSON")
	if pub == "" {
		return nil, ErrNoPublished
	}
	var dsl types.FlowDSL
	if err := json.Unmarshal([]byte(pub), &dsl); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
		"name":      dsl.Name,
		"dslJSON":   pub,
		"updatedAt": now,
	})); err != nil {
		return nil, err
	}
	return s.GetFlow(flowID)
}

// ListPublishHistory 返回历史（旧版本，不含当前 published）；includeDSL 为 false 时去掉正文。
func (s *Store) ListPublishHistory(flowID string, includeDSL bool) ([]PublishHistoryItem, *FlowRecord, error) {
	rec, err := s.GetFlow(flowID)
	if err != nil {
		return nil, nil, err
	}
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(flowID)))
	if err != nil {
		return nil, rec, err
	}
	if len(docs) == 0 {
		return nil, rec, ErrNotFound
	}
	entries := readHistory(docs[0])
	out := make([]PublishHistoryItem, 0, len(entries)+1)
	// 当前已发布作为列表第一项
	if rec.PublishedDSL != nil {
		cur := PublishHistoryItem{
			Version:     rec.PublishedVersion,
			Note:        DocString(docs[0], "publishedNote"),
			PublishedAt: rec.PublishedAt,
		}
		if includeDSL {
			cur.DSL = cloneFlowDSL(rec.PublishedDSL)
		}
		out = append(out, cur)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		item := PublishHistoryItem{
			Version:     e.Version,
			Note:        e.Note,
			PublishedAt: e.PublishedAt,
		}
		if includeDSL && e.DSLJSON != "" {
			var dsl types.FlowDSL
			if err := json.Unmarshal([]byte(e.DSLJSON), &dsl); err == nil {
				item.DSL = &dsl
			}
		}
		out = append(out, item)
	}
	return out, rec, nil
}

// RollbackPublish 将已发布 DSL 恢复为历史中的某版本；草稿不变。
func (s *Store) RollbackPublish(flowID string, version int) (*FlowRecord, error) {
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

	now := time.Now().UTC().Format(time.RFC3339)
	curVer := docInt(doc, "publishedVersion", 0)
	curJSON := DocString(doc, "publishedJSON")
	targetJSON := ""
	targetNote := ""
	if version == curVer {
		return s.GetFlow(flowID)
	}
	for _, e := range readHistory(doc) {
		if e.Version == version {
			targetJSON = e.DSLJSON
			targetNote = e.Note
			break
		}
	}
	if targetJSON == "" {
		return nil, ErrHistoryNotFound
	}

	hist := readHistory(doc)
	if curJSON != "" && curVer > 0 {
		hist = append(hist, historyEntry{
			Version:     curVer,
			Note:        DocString(doc, "publishedNote"),
			PublishedAt: DocString(doc, "publishedAt"),
			DSLJSON:     curJSON,
		})
		if len(hist) > maxPublishHistory {
			hist = hist[len(hist)-maxPublishHistory:]
		}
	}
	histJSON, err := json.Marshal(hist)
	if err != nil {
		return nil, err
	}
	nextVer := curVer + 1
	var rolled types.FlowDSL
	if err := json.Unmarshal([]byte(targetJSON), &rolled); err != nil {
		return nil, err
	}
	note := strings.TrimSpace(targetNote)
	if note == "" {
		note = "rollback"
	}
	if err := s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
		"publishedJSON":    targetJSON,
		"publishedAt":      now,
		"publishedVersion": nextVer,
		"publishedNote":    note,
		"historyJSON":      string(histJSON),
		"updatedAt":        now,
	})); err != nil {
		return nil, err
	}
	return s.GetFlow(flowID)
}

// DeletePublishHistory 从历史中删除指定版本；禁止删除当前线上版本。
func (s *Store) DeletePublishHistory(flowID string, version int) (*FlowRecord, error) {
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
	curVer := docInt(doc, "publishedVersion", 0)
	if version <= 0 {
		return nil, ErrHistoryNotFound
	}
	if version == curVer {
		return nil, ErrCannotDeleteLive
	}
	hist := readHistory(doc)
	next := make([]historyEntry, 0, len(hist))
	found := false
	for _, e := range hist {
		if e.Version == version {
			found = true
			continue
		}
		next = append(next, e)
	}
	if !found {
		return nil, ErrHistoryNotFound
	}
	histJSON, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
		"historyJSON": string(histJSON),
		"updatedAt":   now,
	})); err != nil {
		return nil, err
	}
	return s.GetFlow(flowID)
}

func readHistory(doc *document.Document) []historyEntry {
	raw := DocString(doc, "historyJSON")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var entries []historyEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	return entries
}
