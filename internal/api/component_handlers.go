package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/components"
	"github.com/flowgo/flowgo/engine"
)

// HandleListComponents GET /api/components
func (s *Server) HandleListComponents(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	prefs, err := s.Store.GetComponentPrefs(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	defs := components.FilterDefs(engine.DefaultRegistry.ListDefs(), prefs.IsComponentEnabled)
	writeJSON(w, http.StatusOK, types.ComponentsResponse{
		Groups: components.BuildGroups(defs, locale),
	})
}

// HandleListComponentDocs GET /api/components/docs
// 返回当前用户可见节点的编辑器 Markdown 文档（不含 MCP Usage）。
// 正文来自服务端内存 DocStore（磁盘 MD > 内置 embed > Def 内嵌）。
func (s *Server) HandleListComponentDocs(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	prefs, err := s.Store.GetComponentPrefs(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	defs := components.FilterDefs(engine.DefaultRegistry.ListDefs(), prefs.IsComponentEnabled)
	items := make([]types.ComponentDocItem, 0, len(defs))
	for _, d := range defs {
		doc := ""
		if s.Docs != nil {
			doc = s.Docs.Get(d.Type, locale, d)
		} else {
			doc = strings.TrimSpace(types.LocalizeComponentDef(d, locale).Doc)
		}
		items = append(items, types.ComponentDocItem{
			Type: d.Type,
			Doc:  doc,
		})
	}
	writeJSON(w, http.StatusOK, types.ComponentDocsResponse{Items: items})
}

// HandleGetComponentDoc GET /api/components/{type}/doc
// 单节点编辑器文档；禁用节点返回 404。无文档时 doc 为空串。
func (s *Server) HandleGetComponentDoc(w http.ResponseWriter, r *http.Request) {
	typeName := strings.TrimSpace(r.PathValue("type"))
	if typeName == "" {
		locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
		writeError(w, http.StatusBadRequest, types.PickI18n(map[string]string{
			types.LocaleEnUS: "type required",
		}, locale, "缺少组件类型"))
		return
	}
	user := UserFromContext(r.Context())
	prefs, err := s.Store.GetComponentPrefs(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !prefs.IsComponentEnabled(typeName) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	def, ok := engine.DefaultRegistry.GetDef(typeName)
	if !ok {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	doc := ""
	if s.Docs != nil {
		doc = s.Docs.Get(typeName, locale, def)
	} else {
		doc = strings.TrimSpace(types.LocalizeComponentDef(def, locale).Doc)
	}
	writeJSON(w, http.StatusOK, types.ComponentDocResponse{
		Type: typeName,
		Doc:  doc,
	})
}

// HandleReloadComponentDocs POST /api/components/docs/reload
// 重新扫描 data/docs 载入内存（供面板「刷新文档」）。
func (s *Server) HandleReloadComponentDocs(w http.ResponseWriter, r *http.Request) {
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if s.Docs == nil {
		writeError(w, http.StatusNotImplemented, types.PickI18n(map[string]string{
			types.LocaleEnUS: "Doc store is not enabled",
		}, locale, "文档库未启用"))
		return
	}
	if err := s.Docs.Reload(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"message": types.PickI18n(map[string]string{
			types.LocaleEnUS: "Component docs reloaded into memory",
		}, locale, "节点文档已重新载入内存"),
	})
}

// HandleGetComponentManage GET /api/settings/components
// 合并：注册表中的节点 + 已停用插件的缓存 Def；插件启用状态以插件清单为准。
func (s *Server) HandleGetComponentManage(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	prefs, err := s.Store.GetComponentPrefs(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))

	type keyed struct {
		def      types.ComponentDef
		enabled  bool
		pluginID string
	}
	byType := map[string]keyed{}

	for _, d := range engine.DefaultRegistry.ListDefs() {
		if d.HideInPalette {
			continue
		}
		d.Category = components.NormalizeCategory(d.Category)
		src := d.Source
		if src == "" {
			src = types.ComponentSourceBuiltin
		}
		d.Source = src
		en := prefs.IsComponentEnabled(d.Type)
		if src == types.ComponentSourcePlugin {
			en = true
		}
		byType[d.Type] = keyed{def: d, enabled: en, pluginID: ""}
	}

	if s.Plugins != nil {
		s.Plugins.ForEachInstalled(func(id string, disabled bool, defs []types.ComponentDef) {
			for _, d := range defs {
				d.Category = components.NormalizeCategory(d.Category)
				d.Source = types.ComponentSourcePlugin
				if disabled {
					byType[d.Type] = keyed{def: d, enabled: false, pluginID: id}
					continue
				}
				if k, ok := byType[d.Type]; ok {
					k.pluginID = id
					k.enabled = true
					byType[d.Type] = k
				} else {
					byType[d.Type] = keyed{def: d, enabled: true, pluginID: id}
				}
			}
		})
	}

	byCat := map[string][]keyed{}
	for _, k := range byType {
		cat := components.NormalizeCategory(k.def.Category)
		k.def.Category = cat
		byCat[cat] = append(byCat[cat], k)
	}
	for cat, list := range byCat {
		sort.Slice(list, func(i, j int) bool {
			if list[i].def.Order != list[j].def.Order {
				return list[i].def.Order < list[j].def.Order
			}
			return list[i].def.Type < list[j].def.Type
		})
		byCat[cat] = list
	}

	groups := make([]types.ComponentManageGroup, 0, len(components.Categories))
	for _, c := range components.Categories {
		items := make([]types.ComponentManageItem, 0, len(byCat[c.ID]))
		for _, k := range byCat[c.ID] {
			d := types.LocalizeComponentDef(k.def, locale)
			catLabel := d.CategoryLabel
			if catLabel == "" {
				catLabel = components.CategoryLabelLocale(d.Category, locale)
			}
			items = append(items, types.ComponentManageItem{
				Type:          d.Type,
				Label:         d.Label,
				Category:      d.Category,
				CategoryLabel: catLabel,
				Description:   d.Description,
				Source:        d.Source,
				Enabled:       k.enabled,
				PluginID:      k.pluginID,
			})
		}
		groups = append(groups, types.ComponentManageGroup{
			ID:    c.ID,
			Label: types.PickI18n(c.Labels, locale, c.Label),
			Items: items,
		})
	}
	writeJSON(w, http.StatusOK, types.ComponentManageResponse{Groups: groups})
}

// HandleSaveComponentManage PUT /api/settings/components
// 仅保存内置节点的用户级禁用列表（插件启停走独立接口）。
func (s *Server) HandleSaveComponentManage(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	var body struct {
		Disabled []string `json:"disabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// 过滤掉插件类型，避免误写入用户偏好
	pluginTypes := map[string]bool{}
	if s.Plugins != nil {
		s.Plugins.ForEachInstalled(func(_ string, _ bool, defs []types.ComponentDef) {
			for _, d := range defs {
				pluginTypes[d.Type] = true
			}
		})
	}
	filtered := make([]string, 0, len(body.Disabled))
	for _, t := range body.Disabled {
		t = strings.TrimSpace(t)
		if t == "" || pluginTypes[t] {
			continue
		}
		filtered = append(filtered, t)
	}
	prefs, err := s.Store.SaveComponentPrefs(user.ID, filtered)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prefs)
}

// HandleListMarketplaceComponents GET /api/components/marketplace
func (s *Server) HandleListMarketplaceComponents(w http.ResponseWriter, r *http.Request) {
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  []interface{}{},
		"status": "not_implemented",
		"message": types.PickI18n(map[string]string{
			types.LocaleEnUS: "Marketplace is not open yet; API reserved",
		}, locale, "节点市场尚未开放，接口已预留"),
	})
}

// HandleInstallMarketplaceComponent POST /api/components/marketplace/install
func (s *Server) HandleInstallMarketplaceComponent(w http.ResponseWriter, r *http.Request) {
	locale := types.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"status": "not_implemented",
		"message": types.PickI18n(map[string]string{
			types.LocaleEnUS: "Installing from marketplace is not implemented yet",
		}, locale, "从市场安装节点尚未实现"),
	})
}
