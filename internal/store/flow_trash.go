/**
 * 流程垃圾箱：系统分组、移入（软删）、恢复、组内硬删。
 */
package store

import (
	"errors"
	"time"

	"github.com/ostafen/clover/v2/query"
)

// TrashGroupID 系统「垃圾箱」分组固定业务 id。
const TrashGroupID = "__trash__"

// 垃圾箱排序靠后，列表中置于用户分组之后。
const trashGroupSort = 100000

// ErrSystemGroup 禁止改名 / 删除系统分组。
var ErrSystemGroup = errors.New("system group cannot be modified")

// ErrNotInTrash 流程不在垃圾箱，无法恢复。
var ErrNotInTrash = errors.New("flow is not in trash")

// ErrCannotMoveToTrash 请通过删除接口移入垃圾箱（以便下线并记录原分组）。
var ErrCannotMoveToTrash = errors.New("use delete to move flow into trash")

// ErrMustRestoreFromTrash 垃圾箱内流程须调用恢复接口，不能直接改分组。
var ErrMustRestoreFromTrash = errors.New("restore flow from trash instead of moving")

// SoftDeleteResult 软删 / 硬删结果，供 API 做下线与 WS 通知。
type SoftDeleteResult struct {
	// Flow 移入垃圾箱后的记录；硬删时为 nil。
	Flow *FlowRecord
	// WasPublished 移入垃圾箱前是否在线（已执行下线）。
	WasPublished bool
	// Purged true 表示已从库中彻底删除。
	Purged bool
}

// SoftDeleteOrPurgeFlow 组外删除 → 移入垃圾箱（已上线则先下线）；组内删除 → 硬删。
func (s *Store) SoftDeleteOrPurgeFlow(id string) (*SoftDeleteResult, error) {
	rec, err := s.GetFlow(id)
	if err != nil {
		return nil, err
	}
	if rec.Locked {
		return nil, ErrFlowLocked
	}
	if rec.GroupID == TrashGroupID {
		if err := s.purgeFlowUnlocked(id); err != nil {
			return nil, err
		}
		return &SoftDeleteResult{Purged: true}, nil
	}

	wasPublished := rec.Published
	if wasPublished {
		if _, err := s.UnpublishFlow(id); err != nil && !errors.Is(err, ErrNoPublished) {
			return nil, err
		}
	}

	prev := rec.GroupID
	if prev == TrashGroupID {
		prev = ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(id)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	if err := s.db.UpdateById(ColFlows, docs[0].ObjectId(), updateFields(map[string]interface{}{
		"groupId":         TrashGroupID,
		"previousGroupId": prev,
		"updatedAt":       now,
	})); err != nil {
		return nil, err
	}
	out, err := s.GetFlow(id)
	if err != nil {
		return nil, err
	}
	return &SoftDeleteResult{Flow: out, WasPublished: wasPublished, Purged: false}, nil
}

// RestoreFlow 从垃圾箱恢复到 previousGroupId（原分组已删则未分组）。
func (s *Store) RestoreFlow(id string) (*FlowRecord, error) {
	rec, err := s.GetFlow(id)
	if err != nil {
		return nil, err
	}
	if rec.Locked {
		return nil, ErrFlowLocked
	}
	if rec.GroupID != TrashGroupID {
		return nil, ErrNotInTrash
	}
	prev := rec.PreviousGroupID
	if prev == TrashGroupID {
		prev = ""
	}
	if prev != "" {
		if _, err := s.GetGroup(prev); err != nil {
			prev = ""
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(id)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	if err := s.db.UpdateById(ColFlows, docs[0].ObjectId(), updateFields(map[string]interface{}{
		"groupId":         prev,
		"previousGroupId": "",
		"updatedAt":       now,
	})); err != nil {
		return nil, err
	}
	return s.GetFlow(id)
}

// IsTrashGroupID 是否为系统垃圾箱 id。
func IsTrashGroupID(id string) bool {
	return id == TrashGroupID
}

// purgeFlowUnlocked 硬删（调用方已确认在垃圾箱且未锁定）。
func (s *Store) purgeFlowUnlocked(id string) error {
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("flowId").Eq(id)))
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return ErrNotFound
	}
	return s.db.DeleteById(ColFlows, docs[0].ObjectId())
}
