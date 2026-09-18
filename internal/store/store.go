package store

import (
	"fmt"
	"os"
	"time"

	clover "github.com/ostafen/clover/v2"
	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"
	"golang.org/x/crypto/bcrypt"
)

// 集合名常量。
const (
	ColUsers    = "users"
	ColFlows    = "flows"
	ColGroups   = "flow_groups"
	ColAPIKeys  = "api_keys"
	ColSettings = "settings"
)

// Role 用户角色。
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Store 基于 CloverDB 的本地文档存储（不参与流程运行时）。
type Store struct {
	db *clover.DB
}

// Open 打开（或创建）数据库并确保集合与默认管理员存在。
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}
	db, err := clover.Open(path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.ensureCollections(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.ensureAdmin(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.EnsureTrashGroup(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) ensureCollections() error {
	for _, name := range []string{ColUsers, ColFlows, ColGroups, ColAPIKeys, ColSettings} {
		ok, err := s.db.HasCollection(name)
		if err != nil {
			return err
		}
		if !ok {
			if err := s.db.CreateCollection(name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) ensureAdmin() error {
	q := query.NewQuery(ColUsers).Where(query.Field("username").Eq("admin"))
	docs, err := s.db.FindAll(q)
	if err != nil {
		return err
	}
	if len(docs) > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	doc := document.NewDocument()
	doc.Set("username", "admin")
	doc.Set("passwordHash", string(hash))
	doc.Set("role", RoleAdmin)
	doc.Set("flowIDs", []string{})
	doc.Set("createdAt", time.Now().UTC().Format(time.RFC3339))
	_, err = s.db.InsertOne(ColUsers, doc)
	return err
}

// DB 暴露底层 clover 实例。
func (s *Store) DB() *clover.DB { return s.db }

// HashPassword 生成密码哈希。
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword 校验密码。
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// DocString 安全读取字符串字段。
func DocString(doc *document.Document, key string) string {
	if doc == nil {
		return ""
	}
	v := doc.Get(key)
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// updateFields 用 map 更新文档字段（适配 alpha UpdateById 回调 API）。
func updateFields(updates map[string]interface{}) func(doc *document.Document) *document.Document {
	return func(doc *document.Document) *document.Document {
		for k, v := range updates {
			doc.Set(k, v)
		}
		return doc
	}
}
