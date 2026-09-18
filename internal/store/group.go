package store

import (
	"errors"
	"strings"
	"time"

	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"

	"github.com/flowgo/flowgo/api/types"
)

// GroupRecord 流程分组（仅元数据，不含流程列表）。
type GroupRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Sort      int    `json:"sort"`
	// System 系统分组（如垃圾箱），禁止改名 / 删除。
	System    bool   `json:"system,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ListGroups 列出全部流程分组，按 sort、名称排序。
func (s *Store) ListGroups() ([]*GroupRecord, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColGroups))
	if err != nil {
		return nil, err
	}
	out := make([]*GroupRecord, 0, len(docs))
	for _, d := range docs {
		out = append(out, groupFromDoc(d))
	}
	// 简单插入排序：sort 升序，同 sort 按名称
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && groupLess(out[j], out[j-1]) {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out, nil
}

func groupLess(a, b *GroupRecord) bool {
	if a.Sort != b.Sort {
		return a.Sort < b.Sort
	}
	return strings.ToLower(a.Name) < strings.ToLower(b.Name)
}

// GetGroup 按业务 groupId 获取分组。
func (s *Store) GetGroup(id string) (*GroupRecord, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColGroups).Where(query.Field("groupId").Eq(id)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	return groupFromDoc(docs[0]), nil
}

// CreateGroup 新建分组。
func (s *Store) CreateGroup(name string) (*GroupRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("group name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id := types.NewID()
	doc := document.NewDocument()
	doc.Set("groupId", id)
	doc.Set("name", name)
	doc.Set("sort", 0)
	doc.Set("createdAt", now)
	doc.Set("updatedAt", now)
	if _, err := s.db.InsertOne(ColGroups, doc); err != nil {
		return nil, err
	}
	return s.GetGroup(id)
}

// RenameGroup 修改分组名称。
func (s *Store) RenameGroup(id, name string) (*GroupRecord, error) {
	if IsTrashGroupID(id) {
		return nil, ErrSystemGroup
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("group name is required")
	}
	docs, err := s.db.FindAll(query.NewQuery(ColGroups).Where(query.Field("groupId").Eq(id)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	if docBool(docs[0], "system", false) {
		return nil, ErrSystemGroup
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.db.UpdateById(ColGroups, docs[0].ObjectId(), updateFields(map[string]interface{}{
		"name":      name,
		"updatedAt": now,
	})); err != nil {
		return nil, err
	}
	return s.GetGroup(id)
}

// DeleteGroup 删除分组，并将该组下流程的 groupId 清空（移入未分组）。
func (s *Store) DeleteGroup(id string) error {
	if IsTrashGroupID(id) {
		return ErrSystemGroup
	}
	docs, err := s.db.FindAll(query.NewQuery(ColGroups).Where(query.Field("groupId").Eq(id)))
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return ErrNotFound
	}
	if docBool(docs[0], "system", false) {
		return ErrSystemGroup
	}
	if err := s.ClearFlowsGroup(id); err != nil {
		return err
	}
	// 垃圾箱内流程的 previousGroupId 指向本分组时清空
	if err := s.clearPreviousGroupRefs(id); err != nil {
		return err
	}
	return s.db.DeleteById(ColGroups, docs[0].ObjectId())
}

// clearPreviousGroupRefs 删除分组后，清空仍指向该组的 previousGroupId。
func (s *Store) clearPreviousGroupRefs(groupID string) error {
	docs, err := s.db.FindAll(query.NewQuery(ColFlows).Where(query.Field("previousGroupId").Eq(groupID)))
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, d := range docs {
		if err := s.db.UpdateById(ColFlows, d.ObjectId(), updateFields(map[string]interface{}{
			"previousGroupId": "",
			"updatedAt":       now,
		})); err != nil {
			return err
		}
	}
	return nil
}

// EnsureTrashGroup 确保系统垃圾箱分组存在（启动时调用）。
func (s *Store) EnsureTrashGroup() error {
	if _, err := s.GetGroup(TrashGroupID); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	doc := document.NewDocument()
	doc.Set("groupId", TrashGroupID)
	doc.Set("name", "垃圾箱")
	doc.Set("sort", trashGroupSort)
	doc.Set("system", true)
	doc.Set("createdAt", now)
	doc.Set("updatedAt", now)
	_, err := s.db.InsertOne(ColGroups, doc)
	return err
}

func groupFromDoc(doc *document.Document) *GroupRecord {
	sortVal := 0
	if v, ok := doc.Get("sort").(int64); ok {
		sortVal = int(v)
	} else if v, ok := doc.Get("sort").(int); ok {
		sortVal = v
	} else if v, ok := doc.Get("sort").(float64); ok {
		sortVal = int(v)
	}
	return &GroupRecord{
		ID:        DocString(doc, "groupId"),
		Name:      DocString(doc, "name"),
		Sort:      sortVal,
		System:    docBool(doc, "system", false) || IsTrashGroupID(DocString(doc, "groupId")),
		CreatedAt: DocString(doc, "createdAt"),
		UpdatedAt: DocString(doc, "updatedAt"),
	}
}
