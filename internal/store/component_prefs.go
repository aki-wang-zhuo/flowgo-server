package store

import (
	"fmt"

	"github.com/ostafen/clover/v2/document"
	"github.com/ostafen/clover/v2/query"
)

// ComponentPrefs 用户级节点启用偏好（未出现在 Disabled 中的类型默认启用）。
type ComponentPrefs struct {
	UserID   string   `json:"userId"`
	Disabled []string `json:"disabled"`
}

// GetComponentPrefs 读取用户节点开关；不存在则全部启用。
func (s *Store) GetComponentPrefs(userID string) (*ComponentPrefs, error) {
	doc, err := s.findSettingsDoc(userID, "components")
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return &ComponentPrefs{UserID: userID, Disabled: []string{}}, nil
	}
	return &ComponentPrefs{
		UserID:   userID,
		Disabled: docStringSlice(doc, "disabled"),
	}, nil
}

// SaveComponentPrefs 保存用户禁用的节点类型列表。
func (s *Store) SaveComponentPrefs(userID string, disabled []string) (*ComponentPrefs, error) {
	if disabled == nil {
		disabled = []string{}
	}
	existing, err := s.findSettingsDoc(userID, "components")
	if err != nil {
		return nil, err
	}
	if existing == nil {
		doc := document.NewDocument()
		doc.Set("userId", userID)
		doc.Set("kind", "components")
		doc.Set("disabled", disabled)
		if _, err := s.db.InsertOne(ColSettings, doc); err != nil {
			return nil, err
		}
		return &ComponentPrefs{UserID: userID, Disabled: disabled}, nil
	}
	err = s.db.UpdateById(ColSettings, existing.ObjectId(), updateFields(map[string]interface{}{
		"userId":   userID,
		"kind":     "components",
		"disabled": disabled,
	}))
	if err != nil {
		return nil, err
	}
	return &ComponentPrefs{UserID: userID, Disabled: disabled}, nil
}

// IsComponentEnabled 判断类型是否启用（默认 true）。
func (p *ComponentPrefs) IsComponentEnabled(typeName string) bool {
	if p == nil {
		return true
	}
	for _, d := range p.Disabled {
		if d == typeName {
			return false
		}
	}
	return true
}

func (s *Store) findSettingsDoc(userID, kind string) (*document.Document, error) {
	docs, err := s.db.FindAll(query.NewQuery(ColSettings).Where(
		query.Field("userId").Eq(userID).And(query.Field("kind").Eq(kind)),
	))
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, nil
	}
	return docs[0], nil
}

func docStringSlice(doc *document.Document, key string) []string {
	raw := doc.Get(key)
	if raw == nil {
		return []string{}
	}
	switch t := raw.(type) {
	case []string:
		return append([]string{}, t...)
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, sprintAny(item))
		}
		return out
	default:
		return []string{}
	}
}

func sprintAny(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
