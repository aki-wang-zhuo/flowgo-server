package store

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ostafen/clover/v2/document"

	"github.com/flowgo/flowgo/api/types"
)

// KindHttpResponseTemplates settings 文档 kind：用户级 HTTP 响应体模板。
const KindHttpResponseTemplates = "httpResponseTemplates"

const (
	maxHttpResponseTemplates = 50
	maxTemplateNameLen       = 64
	maxTemplateBodyLen       = 16 * 1024
)

// ErrTemplateLimit 自定义模板数量超限。
var ErrTemplateLimit = errors.New("too many templates")

// ErrTemplateName 名称无效。
var ErrTemplateName = errors.New("template name is required")

// ErrTemplateStatus 状态码无效。
var ErrTemplateStatus = errors.New("statusCode out of range")

// ErrTemplateBody 响应体过长。
var ErrTemplateBody = errors.New("template body too large")

// HttpResponseTemplateItem 用户自定义的一条 HTTP 响应模板（内置模板不入库）。
type HttpResponseTemplateItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

// GetHttpResponseTemplates 读取当前用户自定义模板；无文档则空列表。
func (s *Store) GetHttpResponseTemplates(userID string) ([]HttpResponseTemplateItem, error) {
	doc, err := s.findSettingsDoc(userID, KindHttpResponseTemplates)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return []HttpResponseTemplateItem{}, nil
	}
	return parseHttpResponseTemplateItems(doc.Get("items")), nil
}

// SaveHttpResponseTemplates 整表覆盖保存（校验后 upsert settings 文档）。
func (s *Store) SaveHttpResponseTemplates(userID string, items []HttpResponseTemplateItem) ([]HttpResponseTemplateItem, error) {
	cleaned, err := normalizeHttpResponseTemplates(items)
	if err != nil {
		return nil, err
	}
	existing, err := s.findSettingsDoc(userID, KindHttpResponseTemplates)
	if err != nil {
		return nil, err
	}
	payload := templateItemsToMaps(cleaned)
	if existing == nil {
		doc := document.NewDocument()
		doc.Set("userId", userID)
		doc.Set("kind", KindHttpResponseTemplates)
		doc.Set("items", payload)
		if _, err := s.db.InsertOne(ColSettings, doc); err != nil {
			return nil, err
		}
		return cleaned, nil
	}
	err = s.db.UpdateById(ColSettings, existing.ObjectId(), updateFields(map[string]interface{}{
		"userId": userID,
		"kind":   KindHttpResponseTemplates,
		"items":  payload,
	}))
	if err != nil {
		return nil, err
	}
	return cleaned, nil
}

// AddHttpResponseTemplate 追加一条自定义模板并生成 id。
func (s *Store) AddHttpResponseTemplate(userID, name string, statusCode int, body string) (*HttpResponseTemplateItem, error) {
	list, err := s.GetHttpResponseTemplates(userID)
	if err != nil {
		return nil, err
	}
	item := HttpResponseTemplateItem{
		ID:         "c-" + types.NewID(),
		Name:       name,
		StatusCode: statusCode,
		Body:       body,
	}
	list = append(list, item)
	saved, err := s.SaveHttpResponseTemplates(userID, list)
	if err != nil {
		return nil, err
	}
	return &saved[len(saved)-1], nil
}

// UpdateHttpResponseTemplate 用当前内容覆盖指定自定义模板（名称不变）。
func (s *Store) UpdateHttpResponseTemplate(userID, id string, statusCode int, body string) (*HttpResponseTemplateItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	list, err := s.GetHttpResponseTemplates(userID)
	if err != nil {
		return nil, err
	}
	found := -1
	for i := range list {
		if list[i].ID == id {
			found = i
			break
		}
	}
	if found < 0 {
		return nil, ErrNotFound
	}
	list[found].StatusCode = statusCode
	list[found].Body = body
	saved, err := s.SaveHttpResponseTemplates(userID, list)
	if err != nil {
		return nil, err
	}
	for i := range saved {
		if saved[i].ID == id {
			return &saved[i], nil
		}
	}
	return &saved[found], nil
}

// DeleteHttpResponseTemplate 按 id 删除；不存在则当作已删除。
func (s *Store) DeleteHttpResponseTemplate(userID, id string) ([]HttpResponseTemplateItem, error) {
	id = strings.TrimSpace(id)
	list, err := s.GetHttpResponseTemplates(userID)
	if err != nil {
		return nil, err
	}
	next := make([]HttpResponseTemplateItem, 0, len(list))
	for _, it := range list {
		if it.ID == id {
			continue
		}
		next = append(next, it)
	}
	return s.SaveHttpResponseTemplates(userID, next)
}

func normalizeHttpResponseTemplates(items []HttpResponseTemplateItem) ([]HttpResponseTemplateItem, error) {
	if items == nil {
		items = []HttpResponseTemplateItem{}
	}
	if len(items) > maxHttpResponseTemplates {
		return nil, fmt.Errorf("%w (%d)", ErrTemplateLimit, maxHttpResponseTemplates)
	}
	out := make([]HttpResponseTemplateItem, 0, len(items))
	seen := map[string]struct{}{}
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		if name == "" || utf8.RuneCountInString(name) > maxTemplateNameLen {
			return nil, ErrTemplateName
		}
		if it.StatusCode < 100 || it.StatusCode > 599 {
			return nil, ErrTemplateStatus
		}
		if len(it.Body) > maxTemplateBodyLen {
			return nil, ErrTemplateBody
		}
		id := strings.TrimSpace(it.ID)
		if id == "" {
			id = "c-" + types.NewID()
		}
		if _, ok := seen[id]; ok {
			id = "c-" + types.NewID()
		}
		seen[id] = struct{}{}
		out = append(out, HttpResponseTemplateItem{
			ID:         id,
			Name:       name,
			StatusCode: it.StatusCode,
			Body:       it.Body,
		})
	}
	return out, nil
}

func templateItemsToMaps(items []HttpResponseTemplateItem) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]interface{}{
			"id":         it.ID,
			"name":       it.Name,
			"statusCode": it.StatusCode,
			"body":       it.Body,
		})
	}
	return out
}

func parseHttpResponseTemplateItems(raw interface{}) []HttpResponseTemplateItem {
	if raw == nil {
		return []HttpResponseTemplateItem{}
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return []HttpResponseTemplateItem{}
	}
	out := make([]HttpResponseTemplateItem, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		code := 200
		switch v := m["statusCode"].(type) {
		case int:
			code = v
		case int64:
			code = int(v)
		case float64:
			code = int(v)
		}
		out = append(out, HttpResponseTemplateItem{
			ID:         sprintAny(m["id"]),
			Name:       sprintAny(m["name"]),
			StatusCode: code,
			Body:       sprintAny(m["body"]),
		})
	}
	return out
}
