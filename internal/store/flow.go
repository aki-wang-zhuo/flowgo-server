package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/engine"
)

// FlowRecord 流程图元数据 + 草稿 DSL（dsl）+ 已发布快照状态。
type FlowRecord struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	OwnerID   string         `json:"ownerId"`
	GroupID   string         `json:"groupId"` // 空字符串表示未分组
	// PreviousGroupID 移入垃圾箱前的分组；恢复时写回（空=未分组）。
	PreviousGroupID string `json:"previousGroupId,omitempty"`
	Locked          bool   `json:"locked"` // 锁定后禁止改画布 / 保存 / 删除 / 发布（含 MCP）
	// HasLockPassword 是否设置了非空锁定密码（不回传密码本身）。
	HasLockPassword bool           `json:"hasLockPassword"`
	DSL             *types.FlowDSL `json:"dsl"` // 草稿；编辑器始终读写此份
	// PublishedDSL 已发布 DSL；列表/详情 JSON 不内嵌全文，只给状态位。
	PublishedDSL       *types.FlowDSL `json:"-"`
	Published          bool           `json:"published"`
	UnpublishedChanges bool           `json:"unpublishedChanges"`
	// HasPublishHistory 历史中是否仍有可恢复的已发布快照（下线后用于判断能否再上线）。
	HasPublishHistory bool   `json:"hasPublishHistory"`
	PublishedAt       string `json:"publishedAt,omitempty"`
	PublishedVersion  int    `json:"publishedVersion,omitempty"`
	UpdatedAt          string         `json:"updatedAt"`
	CreatedAt          string         `json:"createdAt"`
}

// ErrFlowLocked 流程已锁定，拒绝修改或删除。
var ErrFlowLocked = errors.New("flow is locked")

// ErrUnlockFailed 解锁密码不正确。
var ErrUnlockFailed = errors.New("unlock failed: wrong password")

// ErrNotPublished 尚未发布，正式执行 / HTTP 入口不可用。
var ErrNotPublished = errors.New("flow is not published")

// ErrNoPublished 没有已发布版本（无法放弃草稿或回滚）。
var ErrNoPublished = errors.New("flow has no published version")

// ErrHistoryNotFound 指定发布版本不存在。
var ErrHistoryNotFound = errors.New("publish history version not found")

// ErrCannotDeleteLive 不能删除当前线上已发布版本。
var ErrCannotDeleteLive = errors.New("cannot delete the live published version")

// hashLockPassword 锁定密码摘要；空密码存空串。
func hashLockPassword(password string) string {
	if password == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

// ListFlows 列出流程；若 user 非 admin，则按 FlowIDs 过滤。
func (s *Store) ListFlows(user *User) ([]*FlowRecord, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColFlows))
	if err != nil {
		return nil, err
	}
	out := make([]*FlowRecord, 0, len(docs))
	for _, d := range docs {
		rec := flowFromDoc(d)
		if err := s.migrateLegacyPublished(d, rec); err != nil {
			return nil, err
		}
		if user != nil && !user.CanAccessFlow(rec.ID) {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// GetFlow 按业务 flowId 获取流程。
func (s *Store) GetFlow(id string) (*FlowRecord, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(id)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	rec := flowFromDoc(docs[0])
	if err := s.migrateLegacyPublished(docs[0], rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// SaveFlow 新建或更新流程（以 dsl.id 为业务主键）。
func (s *Store) SaveFlow(ownerID string, dsl *types.FlowDSL) (*FlowRecord, error) {
	if dsl == nil || dsl.ID == "" {
		return nil, errors.New("flow id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	dslJSON, err := json.Marshal(dsl)
	if err != nil {
		return nil, err
	}

	existing, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(dsl.ID)))
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		doc := document.NewDocument()
		doc.Set("flowId", dsl.ID)
		doc.Set("name", dsl.Name)
		doc.Set("ownerId", ownerID)
		doc.Set("groupId", "")
		doc.Set("locked", false)
		doc.Set("lockPasswordHash", "")
		doc.Set("dslJSON", string(dslJSON))
		doc.Set("publishedJSON", "")
		doc.Set("publishedAt", "")
		doc.Set("publishedVersion", 0)
		doc.Set("historyJSON", "[]")
		doc.Set("createdAt", now)
		doc.Set("updatedAt", now)
		if _, err := s.db.InsertOne(ColFlows, doc); err != nil {
			return nil, err
		}
	} else {
		// 已锁定则拒绝覆盖 DSL（锁定开关走 SetFlowLocked）
		if docBool(existing[0], "locked", false) {
			return nil, ErrFlowLocked
		}
		// 更新时保留 groupId / locked，避免保存 DSL 冲掉元数据
		err = s.db.UpdateById(ColFlows, existing[0].ObjectId(), updateFields(map[string]interface{}{
			"name":      dsl.Name,
			"dslJSON":   string(dslJSON),
			"updatedAt": now,
		}))
		if err != nil {
			return nil, err
		}
	}
	return s.GetFlow(dsl.ID)
}

// SetFlowLocked 设置流程锁定状态。
// locked=true：写入 password（可空）为锁定密码；locked=false：若已设密码则必须匹配，否则 ErrUnlockFailed。
func (s *Store) SetFlowLocked(flowID string, locked bool, password string) (*FlowRecord, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(flowID)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	doc := docs[0]
	now := time.Now().UTC().Format(time.RFC3339)

	if locked {
		// 锁定：记录密码摘要（空表示无密码，解锁无需校验）
		if err := s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
			"locked":           true,
			"lockPasswordHash": hashLockPassword(password),
			"updatedAt":        now,
		})); err != nil {
			return nil, err
		}
		return s.GetFlow(flowID)
	}

	// 解锁：有密码则校验
	stored := DocString(doc, "lockPasswordHash")
	if stored != "" && stored != hashLockPassword(password) {
		return nil, ErrUnlockFailed
	}
	if err := s.db.UpdateById(ColFlows, doc.ObjectId(), updateFields(map[string]interface{}{
		"locked":           false,
		"lockPasswordHash": "",
		"updatedAt":        now,
	})); err != nil {
		return nil, err
	}
	return s.GetFlow(flowID)
}

// SetFlowGroup 将流程移入指定分组；groupID 为空表示未分组。
// 禁止直接移入垃圾箱（须走 SoftDeleteOrPurgeFlow）；禁止从垃圾箱用本接口移出（须走 RestoreFlow）。
func (s *Store) SetFlowGroup(flowID, groupID string) (*FlowRecord, error) {
	if IsTrashGroupID(groupID) {
		return nil, ErrCannotMoveToTrash
	}
	cur, err := s.GetFlow(flowID)
	if err != nil {
		return nil, err
	}
	if cur.GroupID == TrashGroupID {
		return nil, ErrMustRestoreFromTrash
	}
	if groupID != "" {
		if _, err := s.GetGroup(groupID); err != nil {
			return nil, err
		}
	}
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(flowID)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.db.UpdateById(ColFlows, docs[0].ObjectId(), updateFields(map[string]interface{}{
		"groupId":   groupID,
		"updatedAt": now,
	})); err != nil {
		return nil, err
	}
	return s.GetFlow(flowID)
}

// ClearFlowsGroup 清空指定分组下所有流程的 groupId（删除分组时调用）。
// 不会清空垃圾箱（系统分组不可删）。
func (s *Store) ClearFlowsGroup(groupID string) error {
	if IsTrashGroupID(groupID) {
		return ErrSystemGroup
	}
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("groupId").Eq(groupID)))
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, d := range docs {
		if err := s.db.UpdateById(ColFlows, d.ObjectId(), updateFields(map[string]interface{}{
			"groupId":   "",
			"updatedAt": now,
		})); err != nil {
			return err
		}
	}
	return nil
}

// DeleteFlow 删除流程；已锁定则拒绝。
func (s *Store) DeleteFlow(id string) error {
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(id)))
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return ErrNotFound
	}
	if docBool(docs[0], "locked", false) {
		return ErrFlowLocked
	}
	return s.db.DeleteById(ColFlows, docs[0].ObjectId())
}

func flowFromDoc(doc *document.Document) *FlowRecord {
	pwHash := DocString(doc, "lockPasswordHash")
	rec := &FlowRecord{
		ID:               DocString(doc, "flowId"),
		Name:             DocString(doc, "name"),
		OwnerID:          DocString(doc, "ownerId"),
		GroupID:          DocString(doc, "groupId"),
		PreviousGroupID:  DocString(doc, "previousGroupId"),
		Locked:           docBool(doc, "locked", false),
		HasLockPassword:  pwHash != "",
		CreatedAt:        DocString(doc, "createdAt"),
		UpdatedAt:        DocString(doc, "updatedAt"),
		PublishedAt:      DocString(doc, "publishedAt"),
		PublishedVersion: docInt(doc, "publishedVersion", 0),
	}
	if raw := DocString(doc, "dslJSON"); raw != "" {
		var dsl types.FlowDSL
		if err := json.Unmarshal([]byte(raw), &dsl); err == nil {
			rec.DSL = &dsl
		}
	}
	if raw := DocString(doc, "publishedJSON"); raw != "" {
		var dsl types.FlowDSL
		if err := json.Unmarshal([]byte(raw), &dsl); err == nil {
			rec.PublishedDSL = &dsl
		}
	}
	rec.HasPublishHistory = len(readHistory(doc)) > 0
	applyPublishStatus(rec)
	return rec
}

func applyPublishStatus(rec *FlowRecord) {
	if rec == nil {
		return
	}
	rec.Published = rec.PublishedDSL != nil
	if !rec.Published {
		rec.UnpublishedChanges = rec.DSL != nil
		return
	}
	if rec.DSL == nil {
		rec.UnpublishedChanges = false
		return
	}
	fa, errA := engine.DslFingerprint(rec.DSL)
	fb, errB := engine.DslFingerprint(rec.PublishedDSL)
	if errA != nil || errB != nil {
		rec.UnpublishedChanges = true
		return
	}
	rec.UnpublishedChanges = fa != fb
}

func cloneFlowDSL(in *types.FlowDSL) *types.FlowDSL {
	if in == nil {
		return nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		out := *in
		return &out
	}
	var out types.FlowDSL
	if err := json.Unmarshal(raw, &out); err != nil {
		cp := *in
		return &cp
	}
	return &out
}

func docInt(doc *document.Document, key string, def int) int {
	if doc == nil {
		return def
	}
	v := doc.Get(key)
	if v == nil {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return def
	}
}
