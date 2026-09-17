package pluginhost

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/components"
	"github.com/flowgo/flowgo/engine"
	"github.com/flowgo/flowgo-node/sdk"
)

// Manifest 插件落盘元数据。
type Manifest struct {
	ID     string        `json:"id"`
	GOOS   string        `json:"goos"`
	GOARCH string        `json:"goarch"`
	Binary string        `json:"binary"`
	Types  []string      `json:"types"`
	// Disabled 为 true：不加载进程、面板不展示（文件保留）。缺省 false=启用。
	Disabled bool `json:"disabled,omitempty"`
	// Defs 安装时缓存的元数据，停用后仍可在节点管理中展示。
	Defs []sdk.WireDef `json:"defs,omitempty"`
}

// Manager 管理本地 RPC 插件进程与注册表挂载。
type Manager struct {
	mu     sync.Mutex
	dir    string
	reg    *engine.Registry
	loaded map[string]*loadedPlugin
	locale string // 最近一次操作语言（LoadAll 用默认）
}

type loadedPlugin struct {
	manifest Manifest
	client   *sdk.Client
	types    []string
}

// NewManager 创建插件管理器；dir 为插件根目录（如 data/plugins）。
func NewManager(dir string, reg *engine.Registry) *Manager {
	return &Manager{
		dir:     dir,
		reg:     reg,
		loaded:  make(map[string]*loadedPlugin),
		locale:  types.DefaultLocale,
	}
}

// Dir 返回插件根目录。
func (m *Manager) Dir() string { return m.dir }

// LoadAll 启动时扫描目录；跳过 Disabled 的插件。
func (m *Manager) LoadAll() {
	m.locale = types.DefaultLocale
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Printf("plugin: read dir: %v", err)
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := m.loadFromDir(filepath.Join(m.dir, e.Name()), false); err != nil {
			log.Printf("plugin: load %s: %v", e.Name(), err)
		}
	}
}

// InstallFile 安装并立即加载（启用）。
func (m *Manager) InstallFile(fileName string, r io.Reader, locale string) (id string, typeNames []string, err error) {
	locale = types.NormalizeLocale(locale)
	m.locale = locale
	man, err := m.installFile(fileName, r, locale)
	if err != nil {
		return "", nil, err
	}
	return man.ID, append([]string(nil), man.Types...), nil
}

// SetEnabled 停用（不加载、不删文件）或重新启用并加载。
func (m *Manager) SetEnabled(pluginID string, enabled bool, locale string) error {
	locale = types.NormalizeLocale(locale)
	m.locale = locale
	dir := filepath.Join(m.dir, pluginID)
	man, err := readManifest(dir)
	if err != nil {
		return errPluginNotFound(locale, pluginID)
	}
	man.Disabled = !enabled
	if err := writeManifest(dir, man); err != nil {
		return err
	}
	m.mu.Lock()
	if old, ok := m.loaded[pluginID]; ok {
		m.unloadLocked(old)
	}
	m.mu.Unlock()
	if !enabled {
		return nil
	}
	return m.loadFromDir(dir, true)
}

// Uninstall 卸载：停用进程并从磁盘删除。
func (m *Manager) Uninstall(pluginID string, locale string) error {
	locale = types.NormalizeLocale(locale)
	m.locale = locale
	dir := filepath.Join(m.dir, pluginID)
	if _, err := os.Stat(dir); err != nil {
		return errPluginNotFound(locale, pluginID)
	}
	m.mu.Lock()
	if old, ok := m.loaded[pluginID]; ok {
		m.unloadLocked(old)
	}
	m.mu.Unlock()
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return nil
}

// ForEachInstalled 遍历已安装插件（含停用）。
func (m *Manager) ForEachInstalled(fn func(id string, disabled bool, defs []types.ComponentDef)) {
	if fn == nil {
		return
	}
	for _, man := range m.listManifestsRaw() {
		defs := make([]types.ComponentDef, 0, len(man.Defs))
		for _, w := range man.Defs {
			d := sdk.FromWire(w)
			d.Category = components.NormalizeCategory(d.Category)
			defs = append(defs, d)
		}
		fn(man.ID, man.Disabled, defs)
	}
}

func (m *Manager) listManifestsRaw() []Manifest {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil
	}
	out := make([]Manifest, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		man, err := readManifest(filepath.Join(m.dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, man)
	}
	return out
}

func (m *Manager) installFile(fileName string, r io.Reader, locale string) (*Manifest, error) {
	if err := ValidateUploadName(fileName, locale); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "flowgo-plugin-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	_ = tmp.Close()

	id := pluginIDFromName(fileName)
	destDir := filepath.Join(m.dir, id)
	_ = os.RemoveAll(destDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}

	var binName string
	lower := strings.ToLower(fileName)
	if strings.HasSuffix(lower, ".zip") {
		binName, err = unzipPlugin(tmpPath, destDir)
		if err != nil {
			_ = os.RemoveAll(destDir)
			return nil, err
		}
	} else {
		binName = filepath.Base(fileName)
		destBin := filepath.Join(destDir, binName)
		if err := copyFile(tmpPath, destBin); err != nil {
			_ = os.RemoveAll(destDir)
			return nil, err
		}
		if runtime.GOOS != "windows" {
			_ = os.Chmod(destBin, 0o755)
		}
	}

	binPath := filepath.Join(destDir, binName)
	if err := ValidateBinaryForHost(binPath, locale); err != nil {
		_ = os.RemoveAll(destDir)
		return nil, err
	}

	man := Manifest{
		ID:       id,
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		Binary:   binName,
		Disabled: false,
	}
	if err := writeManifest(destDir, man); err != nil {
		_ = os.RemoveAll(destDir)
		return nil, err
	}
	if err := m.loadFromDir(destDir, true); err != nil {
		_ = os.RemoveAll(destDir)
		return nil, err
	}
	m.mu.Lock()
	lp := m.loaded[id]
	m.mu.Unlock()
	if lp != nil {
		man = lp.manifest
	}
	return &man, nil
}

// loadFromDir force=true 时忽略 Disabled（用于重新启用）。
func (m *Manager) loadFromDir(dir string, force bool) error {
	locale := m.locale
	man, err := readManifest(dir)
	if err != nil {
		bin, err2 := findBinary(dir)
		if err2 != nil {
			return err
		}
		man = Manifest{
			ID:     filepath.Base(dir),
			GOOS:   runtime.GOOS,
			GOARCH: runtime.GOARCH,
			Binary: filepath.Base(bin),
		}
	}
	if man.Disabled && !force {
		log.Printf("plugin: skip disabled %s", man.ID)
		return nil
	}
	if man.GOOS != "" && man.GOOS != runtime.GOOS {
		return errWrongOS(locale, man.GOOS, runtime.GOOS)
	}
	binPath := filepath.Join(dir, man.Binary)
	if err := ValidateBinaryForHost(binPath, locale); err != nil {
		return err
	}

	m.mu.Lock()
	if old, ok := m.loaded[man.ID]; ok {
		m.unloadLocked(old)
	}
	m.mu.Unlock()

	client, err := sdk.Start(binPath)
	if err != nil {
		return errStartPlugin(locale, err)
	}
	wireDefs := client.Defs()
	typesList := make([]string, 0, len(wireDefs))
	normDefs := make([]sdk.WireDef, 0, len(wireDefs))
	for _, w := range wireDefs {
		def := sdk.FromWire(w)
		def.Category = components.NormalizeCategory(def.Category)
		if def.Category == components.CategoryOther {
			def.CategoryLabel = "其他"
			def.CategoryLabels = map[string]string{types.LocaleEnUS: "Other"}
		}
		w = sdk.ToWire(def)
		if def.Type == "" {
			continue
		}
		if m.reg.Has(def.Type) {
			existing := findDef(m.reg, def.Type)
			if existing != nil && existing.Source != "" && existing.Source != types.ComponentSourcePlugin {
				_ = client.Close()
				return errTypeOccupied(locale, def.Type)
			}
			m.reg.Unregister(def.Type)
		}
		m.reg.Register(def, sdk.NewProxyFactory(client, def.Type))
		typesList = append(typesList, def.Type)
		normDefs = append(normDefs, w)
	}
	if len(typesList) == 0 {
		_ = client.Close()
		return errNoTypes(locale)
	}
	man.Types = typesList
	man.Defs = normDefs
	man.Disabled = false
	_ = writeManifest(dir, man)

	m.mu.Lock()
	m.loaded[man.ID] = &loadedPlugin{manifest: man, client: client, types: typesList}
	m.mu.Unlock()
	log.Printf("plugin: loaded %s types=%v", man.ID, typesList)
	return nil
}

func (m *Manager) unloadLocked(lp *loadedPlugin) {
	for _, t := range lp.types {
		m.reg.Unregister(t)
	}
	if lp.client != nil {
		_ = lp.client.Close()
	}
	delete(m.loaded, lp.manifest.ID)
}

func findDef(reg *engine.Registry, typeName string) *engineDefLite {
	for _, d := range reg.ListDefs() {
		if d.Type == typeName {
			return &engineDefLite{Source: d.Source}
		}
	}
	return nil
}

type engineDefLite struct{ Source string }
