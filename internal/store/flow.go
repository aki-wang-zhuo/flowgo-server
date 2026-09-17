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
)

// FlowRecord 流程图元数据 + DSL 正文。
type FlowRecord struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	OwnerID   string         `json:"ownerId"`
	GroupID   string         `json:"groupId"` // 空字符串表示未分组
	Locked    bool           `json:"locked"`  // 锁定后禁止改画布 / 保存 / 删除（含 MCP）
	// HasLockPassword 是否设置了非空锁定密码（不回传密码本身）。
	HasLockPassword bool           `json:"hasLockPassword"`
	DSL             *types.FlowDSL `json:"dsl"`
	UpdatedAt       string         `json:"updatedAt"`
	CreatedAt       string         `json:"createdAt"`
}

// ErrFlowLocked 流程已锁定，拒绝修改或删除。
var ErrFlowLocked = errors.New("flow is locked")

// ErrUnlockFailed 解锁密码不正确。
var ErrUnlockFailed = errors.New("unlock failed: wrong password")

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
	return flowFromDoc(docs[0]), nil
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
func (s *Store) SetFlowGroup(flowID, groupID string) (*FlowRecord, error) {
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
func (s *Store) ClearFlowsGroup(groupID string) error {
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
		ID:              DocString(doc, "flowId"),
		Name:            DocString(doc, "name"),
		OwnerID:         DocString(doc, "ownerId"),
		GroupID:         DocString(doc, "groupId"),
		Locked:          docBool(doc, "locked", false),
		HasLockPassword: pwHash != "",
		CreatedAt:       DocString(doc, "createdAt"),
		UpdatedAt:       DocString(doc, "updatedAt"),
	}
	raw := DocString(doc, "dslJSON")
	if raw != "" {
		var dsl types.FlowDSL
		if err := json.Unmarshal([]byte(raw), &dsl); err == nil {
			rec.DSL = &dsl
		}
	}
	return rec
}
