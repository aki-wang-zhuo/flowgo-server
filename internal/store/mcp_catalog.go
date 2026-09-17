package store

import "github.com/flowgo/flowgo/api/types"

// McpToolInfo 单个 MCP 工具的元数据（供设置页与文档展示）。
type McpToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// McpCapability 一项权限开关及其覆盖的工具列表。
// key 与 McpPermissions JSON 字段名一致。
type McpCapability struct {
	Key         string        `json:"key"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Tools       []McpToolInfo `json:"tools"`
}

// mcpToolDef 工具条目的多语言定义（内部）。
type mcpToolDef struct {
	Name         string
	Description  string
	Descriptions map[string]string
}

// mcpCapDef 能力项的多语言定义（内部）。
type mcpCapDef struct {
	Key          string
	Title        string
	Titles       map[string]string
	Description  string
	Descriptions map[string]string
	Tools        []mcpToolDef
}

// mcpCatalogDefs 后端唯一数据源；新增/变更 MCP 工具时请同步 gateway 注册。
var mcpCatalogDefs = []mcpCapDef{
	{
		Key:   "flowRead",
		Title: "查看流程与节点",
		Titles: map[string]string{
			types.LocaleEnUS: "View flows & components",
		},
		Description: "列出/读取流程 DSL、获取编辑器当前激活流程（含 revision），以及列出节点与查看节点文档",
		Descriptions: map[string]string{
			types.LocaleEnUS: "List/read flow DSL, get the editor's active flow (with revision), list components, and view component docs",
		},
		Tools: []mcpToolDef{
			{
				Name: "list_flows", Description: "列出当前用户可访问的流程图",
				Descriptions: map[string]string{types.LocaleEnUS: "List flows accessible to the current user"},
			},
			{
				Name: "get_flow", Description: "获取流程草稿 DSL 与发布状态（published / unpublishedChanges）",
				Descriptions: map[string]string{types.LocaleEnUS: "Get draft DSL and publish status (published / unpublishedChanges)"},
			},
			{
				Name: "list_publish_history", Description: "列出流程发布历史（含当前已发布版本）",
				Descriptions: map[string]string{
					types.LocaleEnUS: "List publish history (including the current published version)",
				},
			},
			{
				Name: "get_active_flow", Description: "获取编辑器中正在编辑的流程（含未保存画布与 revision）；需编辑器在线",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Get the flow currently open in the editor (incl. unsaved canvas and revision); editor must be online",
				},
			},
			{
				Name: "list_components", Description: "列出当前用户可用的流程节点（已启用）",
				Descriptions: map[string]string{types.LocaleEnUS: "List enabled components available to the current user"},
			},
			{
				Name: "get_component_doc", Description: "获取指定节点的完整文档（description、usage、configFields、relationTypes）",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Get full docs for a component (description, usage, configFields, relationTypes)",
				},
			},
		},
	},
	{
		Key:   "flowCreate",
		Title: "新建流程",
		Titles: map[string]string{
			types.LocaleEnUS: "Create flow",
		},
		Description: "通过 save_flow 创建尚不存在的流程图",
		Descriptions: map[string]string{
			types.LocaleEnUS: "Create a new flow via save_flow when it does not exist yet",
		},
		Tools: []mcpToolDef{
			{
				Name: "save_flow", Description: "保存流程草稿（不发布）；目标不存在时需本权限",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Save a flow draft (does not publish); required when the target does not exist",
				},
			},
		},
	},
	{
		Key:   "flowUpdate",
		Title: "修改流程",
		Titles: map[string]string{
			types.LocaleEnUS: "Update flow",
		},
		Description: "保存草稿、发布/回滚线上版本、放弃草稿，或增量修改编辑器中正在编辑的流程",
		Descriptions: map[string]string{
			types.LocaleEnUS: "Save draft, publish/rollback live version, discard draft, or patch the editor's active flow",
		},
		Tools: []mcpToolDef{
			{
				Name: "save_flow", Description: "保存流程草稿（不发布）；目标已存在时需本权限",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Save a flow draft (does not publish); required when the target already exists",
				},
			},
			{
				Name: "publish_flow", Description: "将已保存草稿发布为线上版本并同步 HTTP 入口",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Publish the saved draft as the live version and sync HTTP endpoints",
				},
			},
			{
				Name: "discard_draft", Description: "放弃草稿，用当前已发布版本覆盖",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Discard the draft and restore it from the published version",
				},
			},
			{
				Name: "rollback_publish", Description: "将线上已发布版本回滚到历史 version（草稿不变）",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Roll the live published version back to a history version (draft unchanged)",
				},
			},
			{
				Name: "delete_publish_history", Description: "删除发布历史中的某 version（不能删当前线上版本）",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Delete a publish-history version (cannot delete the live version)",
				},
			},
			{
				Name: "patch_active_flow", Description: "增量修改编辑器当前流程（只传变更节点/边）；返回 revision；需编辑器在线",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Incrementally patch the editor's active flow (send only changes); returns revision; editor must be online",
				},
			},
		},
	},
	{
		Key:   "flowDelete",
		Title: "删除流程",
		Titles: map[string]string{
			types.LocaleEnUS: "Delete flow",
		},
		Description: "删除指定流程图",
		Descriptions: map[string]string{
			types.LocaleEnUS: "Delete a flow by id",
		},
		Tools: []mcpToolDef{
			{
				Name: "delete_flow", Description: "删除流程图",
				Descriptions: map[string]string{types.LocaleEnUS: "Delete a flow"},
			},
		},
	},
	{
		Key:   "flowExecute",
		Title: "执行流程",
		Titles: map[string]string{
			types.LocaleEnUS: "Execute flow",
		},
		Description: "执行【已发布】版本；未发布则失败。画布调试不走本工具",
		Descriptions: map[string]string{
			types.LocaleEnUS: "Run the published version; fails if unpublished. Canvas debug does not use these tools",
		},
		Tools: []mcpToolDef{
			{
				Name: "execute_flow", Description: "从已发布流程入口执行（可传输入 JSON 与消息类型）",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Execute the published flow from the entry node (optional input JSON and message type)",
				},
			},
			{
				Name: "execute_from_node", Description: "从已发布流程的指定节点执行（需 nodeId）",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Execute the published flow from a specific node (nodeId required)",
				},
			},
		},
	},
	{
		Key:   "flowUnlock",
		Title: "解锁流程",
		Titles: map[string]string{
			types.LocaleEnUS: "Unlock flow",
		},
		Description: "通过 MCP 解锁已锁定的流程（需正确密码；本能力默认关闭）",
		Descriptions: map[string]string{
			types.LocaleEnUS: "Unlock a locked flow via MCP (correct password required; off by default)",
		},
		Tools: []mcpToolDef{
			{
				Name: "unlock_flow", Description: "解锁流程；password 可空。若提示密码错误，须向用户请求密码后重试，且 AI 不得记录该密码",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Unlock a flow; password may be empty. On wrong password, ask the user and retry; never store the password",
				},
			},
		},
	},
	{
		Key:   "notifyEditor",
		Title: "通知编辑器",
		Titles: map[string]string{
			types.LocaleEnUS: "Notify editor",
		},
		Description: "通过 WebSocket 驱动已打开的编辑器刷新画布、重载列表或打开流程",
		Descriptions: map[string]string{
			types.LocaleEnUS: "Drive open editors over WebSocket: refresh canvas, reload list, or open a flow",
		},
		Tools: []mcpToolDef{
			{
				Name: "notify_editor", Description: "动作：refresh_canvas / reload_flows / open_flow",
				Descriptions: map[string]string{
					types.LocaleEnUS: "Actions: refresh_canvas / reload_flows / open_flow",
				},
			},
		},
	},
}

// McpCapabilityCatalog 返回默认语言（中文）的能力目录。
func McpCapabilityCatalog() []McpCapability {
	return McpCapabilityCatalogLocale(types.DefaultLocale)
}

// McpCapabilityCatalogLocale 按语言返回能力目录（title / description / 工具说明已本地化）。
func McpCapabilityCatalogLocale(locale string) []McpCapability {
	locale = types.NormalizeLocale(locale)
	out := make([]McpCapability, 0, len(mcpCatalogDefs))
	for _, d := range mcpCatalogDefs {
		tools := make([]McpToolInfo, 0, len(d.Tools))
		for _, t := range d.Tools {
			tools = append(tools, McpToolInfo{
				Name:        t.Name,
				Description: types.PickI18n(t.Descriptions, locale, t.Description),
			})
		}
		out = append(out, McpCapability{
			Key:         d.Key,
			Title:       types.PickI18n(d.Titles, locale, d.Title),
			Description: types.PickI18n(d.Descriptions, locale, d.Description),
			Tools:       tools,
		})
	}
	return out
}
