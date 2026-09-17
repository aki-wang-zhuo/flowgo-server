package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"
)

// User 用户模型（存储于 Clover，不参与流程执行）。
type User struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	PasswordHash string   `json:"-"`
	Role         string   `json:"role"`
	FlowIDs      []string `json:"flowIds"`
	CreatedAt    string   `json:"createdAt"`
}

// ErrNotFound 资源不存在。
var ErrNotFound = errors.New("not found")

// ErrConflict 资源冲突（如用户名已存在）。
var ErrConflict = errors.New("conflict")

// FindUserByUsername 按用户名查找。
func (s *Store) FindUserByUsername(username string) (*User, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColUsers).Where(query.Field("username").Eq(username)))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNotFound
	}
	return userFromDoc(docs[0]), nil
}

// FindUserByID 按文档 ID 查找。
func (s *Store) FindUserByID(id string) (*User, error) {
	doc, err := s.db.FindById(ColUsers, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, ErrNotFound
	}
	return userFromDoc(doc), nil
}

// ListUsers 列出全部用户。
func (s *Store) ListUsers() ([]*User, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColUsers))
	if err != nil {
		return nil, err
	}
	out := make([]*User, 0, len(docs))
	for _, d := range docs {
		out = append(out, userFromDoc(d))
	}
	return out, nil
}

// CreateUser 创建用户。
func (s *Store) CreateUser(username, password, role string, flowIDs []string) (*User, error) {
	if _, err := s.FindUserByUsername(username); err == nil {
		return nil, ErrConflict
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if role == "" {
		role = RoleUser
	}
	if flowIDs == nil {
		flowIDs = []string{}
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	doc := document.NewDocument()
	doc.Set("username", username)
	doc.Set("passwordHash", hash)
	doc.Set("role", role)
	doc.Set("flowIDs", flowIDs)
	doc.Set("createdAt", time.Now().UTC().Format(time.RFC3339))
	id, err := s.db.InsertOne(ColUsers, doc)
	if err != nil {
		return nil, err
	}
	return s.FindUserByID(id)
}

// UpdatePassword 修改密码。
func (s *Store) UpdatePassword(userID, newPassword string) error {
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.db.UpdateById(ColUsers, userID, updateFields(map[string]interface{}{
		"passwordHash": hash,
	}))
}

// UpdateUserFlows 更新用户可访问的流程 ID 列表。
func (s *Store) UpdateUserFlows(userID string, flowIDs []string) error {
	if flowIDs == nil {
		flowIDs = []string{}
	}
	return s.db.UpdateById(ColUsers, userID, updateFields(map[string]interface{}{
		"flowIDs": flowIDs,
	}))
}

// CanAccessFlow 判断用户是否可访问指定流程。
func (u *User) CanAccessFlow(flowID string) bool {
	if u == nil {
		return false
	}
	if u.Role == RoleAdmin {
		return true
	}
	for _, id := range u.FlowIDs {
		if id == flowID {
			return true
		}
	}
	return false
}

func userFromDoc(doc *document.Document) *User {
	u := &User{
		ID:           doc.ObjectId(),
		Username:     DocString(doc, "username"),
		PasswordHash: DocString(doc, "passwordHash"),
		Role:         DocString(doc, "role"),
		CreatedAt:    DocString(doc, "createdAt"),
		FlowIDs:      []string{},
	}
	if raw := doc.Get("flowIDs"); raw != nil {
		switch t := raw.(type) {
		case []string:
			u.FlowIDs = t
		case []interface{}:
			for _, item := range t {
				u.FlowIDs = append(u.FlowIDs, fmt.Sprint(item))
			}
		}
	}
	return u
}
