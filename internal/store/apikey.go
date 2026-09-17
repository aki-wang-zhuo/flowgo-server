package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"
)

// APIKey API 密钥记录（明文仅创建时返回一次，库中存哈希）。
type APIKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	UserID    string `json:"userId"`
	KeyPrefix string `json:"keyPrefix"`
	KeyHash   string `json:"-"`
	CreatedAt string `json:"createdAt"`
}

// CreateAPIKey 为用户创建 API Key，返回明文 key（仅此一次）。
func (s *Store) CreateAPIKey(userID, name string) (record *APIKey, plain string, err error) {
	plain, err = generateAPIKey()
	if err != nil {
		return nil, "", err
	}
	hash, err := HashPassword(plain)
	if err != nil {
		return nil, "", err
	}
	prefix := plain
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	doc := document.NewDocument()
	doc.Set("name", name)
	doc.Set("userId", userID)
	doc.Set("keyPrefix", prefix)
	doc.Set("keyHash", hash)
	doc.Set("createdAt", time.Now().UTC().Format(time.RFC3339))
	id, err := s.db.InsertOne(ColAPIKeys, doc)
	if err != nil {
		return nil, "", err
	}
	rec, err := s.getAPIKeyByDocID(id)
	return rec, plain, err
}

// ListAPIKeys 列出用户的 API Key（不含哈希）。
func (s *Store) ListAPIKeys(userID string) ([]*APIKey, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColAPIKeys).Where(query.Field("userId").Eq(userID)))
	if err != nil {
		return nil, err
	}
	out := make([]*APIKey, 0, len(docs))
	for _, d := range docs {
		out = append(out, apiKeyFromDoc(d))
	}
	return out, nil
}

// DeleteAPIKey 删除指定 API Key。
func (s *Store) DeleteAPIKey(id, userID string, isAdmin bool) error {
	doc, err := s.db.FindById(ColAPIKeys, id)
	if err != nil {
		return err
	}
	if doc == nil {
		return ErrNotFound
	}
	if !isAdmin && DocString(doc, "userId") != userID {
		return ErrNotFound
	}
	return s.db.DeleteById(ColAPIKeys, id)
}

// FindUserByAPIKey 用明文 API Key 反查用户。
func (s *Store) FindUserByAPIKey(plain string) (*User, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColAPIKeys))
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		hash := DocString(d, "keyHash")
		if CheckPassword(hash, plain) {
			uid := DocString(d, "userId")
			return s.FindUserByID(uid)
		}
	}
	return nil, ErrNotFound
}

func (s *Store) getAPIKeyByDocID(id string) (*APIKey, error) {
	doc, err := s.db.FindById(ColAPIKeys, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, ErrNotFound
	}
	return apiKeyFromDoc(doc), nil
}

func apiKeyFromDoc(doc *document.Document) *APIKey {
	return &APIKey{
		ID:        doc.ObjectId(),
		Name:      DocString(doc, "name"),
		UserID:    DocString(doc, "userId"),
		KeyPrefix: DocString(doc, "keyPrefix"),
		KeyHash:   DocString(doc, "keyHash"),
		CreatedAt: DocString(doc, "createdAt"),
	}
}

func generateAPIKey() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("fg_%s", hex.EncodeToString(b[:])), nil
}
