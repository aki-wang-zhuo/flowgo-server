package store

import (
	"github.com/ostafen/clover/v2/document"
)

// McpPermissions MCP 工具权限开关（按当前登录用户生效）。
type McpPermissions struct {
	FlowCreate   bool `json:"flowCreate"`
	FlowRead     bool `json:"flowRead"`
	FlowUpdate   bool `json:"flowUpdate"`
	FlowDelete   bool `json:"flowDelete"`
	FlowExecute  bool `json:"flowExecute"`
	FlowUnlock   bool `json:"flowUnlock"` // 解锁流程；默认关闭
	NotifyEditor bool `json:"notifyEditor"`
}

// McpSettings 用户级 MCP 设置。
type McpSettings struct {
	UserID string `json:"userId"`
	// Enabled MCP 全局开关：关闭后禁用该用户的 MCP 与编辑器 WebSocket。
	Enabled     bool           `json:"enabled"`
	Permissions McpPermissions `json:"permissions"`
}

// DefaultMcpPermissions 默认权限；flowUnlock 默认关闭。
func DefaultMcpPermissions() McpPermissions {
	return McpPermissions{
		FlowCreate:   true,
		FlowRead:     true,
		FlowUpdate:   true,
		FlowDelete:   true,
		FlowExecute:  true,
		FlowUnlock:   false,
		NotifyEditor: true,
	}
}

// GetMcpSettings 读取用户 MCP 设置；不存在则返回默认全开。
func (s *Store) GetMcpSettings(userID string) (*McpSettings, error) {
	doc, err := s.findMcpSettingsDoc(userID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return &McpSettings{
			UserID:      userID,
			Enabled:     true,
			Permissions: DefaultMcpPermissions(),
		}, nil
	}
	return mcpSettingsFromDoc(doc, userID), nil
}

// IsMcpEnabled 该用户是否开启 MCP（含 WebSocket）。
func (s *Store) IsMcpEnabled(userID string) (bool, error) {
	cfg, err := s.GetMcpSettings(userID)
	if err != nil {
		return false, err
	}
	return cfg.Enabled, nil
}

// SaveMcpSettings 保存用户 MCP 设置（upsert）。
func (s *Store) SaveMcpSettings(userID string, enabled bool, perms McpPermissions) (*McpSettings, error) {
	existing, err := s.findMcpSettingsDoc(userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		doc := document.NewDocument()
		doc.Set("userId", userID)
		doc.Set("kind", "mcp")
		doc.Set("enabled", enabled)
		writeMcpPerms(doc, perms)
		if _, err := s.db.InsertOne(ColSettings, doc); err != nil {
			return nil, err
		}
		return &McpSettings{UserID: userID, Enabled: enabled, Permissions: perms}, nil
	}
	err = s.db.UpdateById(ColSettings, existing.ObjectId(), updateFields(map[string]interface{}{
		"userId":       userID,
		"kind":         "mcp",
		"enabled":      enabled,
		"flowCreate":   perms.FlowCreate,
		"flowRead":     perms.FlowRead,
		"flowUpdate":   perms.FlowUpdate,
		"flowDelete":   perms.FlowDelete,
		"flowExecute":  perms.FlowExecute,
		"flowUnlock":   perms.FlowUnlock,
		"notifyEditor": perms.NotifyEditor,
	}))
	if err != nil {
		return nil, err
	}
	return &McpSettings{UserID: userID, Enabled: enabled, Permissions: perms}, nil
}

func (s *Store) findMcpSettingsDoc(userID string) (*document.Document, error) {
	return s.findSettingsDoc(userID, "mcp")
}

func writeMcpPerms(doc *document.Document, p McpPermissions) {
	doc.Set("flowCreate", p.FlowCreate)
	doc.Set("flowRead", p.FlowRead)
	doc.Set("flowUpdate", p.FlowUpdate)
	doc.Set("flowDelete", p.FlowDelete)
	doc.Set("flowExecute", p.FlowExecute)
	doc.Set("flowUnlock", p.FlowUnlock)
	doc.Set("notifyEditor", p.NotifyEditor)
}

func mcpSettingsFromDoc(doc *document.Document, userID string) *McpSettings {
	uid := DocString(doc, "userId")
	if uid == "" {
		uid = userID
	}
	def := DefaultMcpPermissions()
	return &McpSettings{
		UserID:  uid,
		Enabled: docBool(doc, "enabled", true),
		Permissions: McpPermissions{
			FlowCreate:   docBool(doc, "flowCreate", def.FlowCreate),
			FlowRead:     docBool(doc, "flowRead", def.FlowRead),
			FlowUpdate:   docBool(doc, "flowUpdate", def.FlowUpdate),
			FlowDelete:   docBool(doc, "flowDelete", def.FlowDelete),
			FlowExecute:  docBool(doc, "flowExecute", def.FlowExecute),
			FlowUnlock:   docBool(doc, "flowUnlock", def.FlowUnlock),
			NotifyEditor: docBool(doc, "notifyEditor", def.NotifyEditor),
		},
	}
}

func docBool(doc *document.Document, key string, fallback bool) bool {
	v := doc.Get(key)
	if v == nil {
		return fallback
	}
	b, ok := v.(bool)
	if !ok {
		return fallback
	}
	return b
}
