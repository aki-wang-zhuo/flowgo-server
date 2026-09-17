/**
 * 节点编辑器文档内存库。
 * 读取优先级：磁盘 MD（data/docs）> 内置 embed（nodedocs）> 组件 Def/WireDef 内嵌 > 空。
 * 服务启动与刷新时 Reload；前端经 API 读内存，不落 localStorage。
 */
package componentdocs

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/flowgo/flowgo/api/types"
	"github.com/flowgo/flowgo/components/nodedocs"
)

// Store 进程内文档索引。
type Store struct {
	mu       sync.RWMutex
	docsDir  string
	fileDocs map[string]map[string]string // type -> locale -> body（磁盘 MD）
	builtin  map[string]map[string]string // type -> locale -> body（编译嵌入）
}

// New 创建文档库；builtin 通常为 nodedocs.BuiltinMap()。
func New(docsDir string, builtin map[string]map[string]string) *Store {
	if builtin == nil {
		builtin = map[string]map[string]string{}
	}
	s := &Store{
		docsDir:  docsDir,
		fileDocs: map[string]map[string]string{},
		builtin:  cloneLocaleMap(builtin),
	}
	return s
}

// DocsDir 返回文档落盘目录（如 data/docs）。
func (s *Store) DocsDir() string {
	if s == nil {
		return ""
	}
	return s.docsDir
}

// Reload 扫描 docsDir 下全部 *_zh.md / *_en.md 载入 file 层。
func (s *Store) Reload() error {
	if s == nil {
		return nil
	}
	_ = os.MkdirAll(s.docsDir, 0o755)
	next := map[string]map[string]string{}
	entries, err := os.ReadDir(s.docsDir)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			s.fileDocs = next
			s.mu.Unlock()
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		typeName, locale, ok := nodedocs.ParseDocFileName(e.Name())
		if !ok {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.docsDir, e.Name()))
		if err != nil {
			log.Printf("componentdocs: read %s: %v", e.Name(), err)
			continue
		}
		body := strings.TrimSpace(string(b))
		if body == "" {
			continue
		}
		if next[typeName] == nil {
			next[typeName] = map[string]string{}
		}
		next[typeName][locale] = body
	}
	s.mu.Lock()
	s.fileDocs = next
	s.mu.Unlock()
	return nil
}

// CopyDocsFromDir 将目录中的节点文档 MD 复制到 docsDir，再 Reload。
// 用于 zip 插件解压后把文档落到统一位置。
func (s *Store) CopyDocsFromDir(srcDir string) error {
	if s == nil || strings.TrimSpace(srcDir) == "" {
		return nil
	}
	_ = os.MkdirAll(s.docsDir, 0o755)
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	copied := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !nodedocs.IsDocMarkdown(e.Name()) {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(s.docsDir, e.Name())
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
		copied++
	}
	if copied > 0 {
		log.Printf("componentdocs: copied %d doc file(s) from %s", copied, srcDir)
	}
	return s.Reload()
}

// Get 按优先级取文档正文；无则返回空串（前端显示无文档）。
func (s *Store) Get(typeName, locale string, def types.ComponentDef) string {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return ""
	}
	locale = types.NormalizeLocale(locale)
	if s != nil {
		s.mu.RLock()
		if c := pickLocale(s.fileDocs[typeName], locale); c != "" {
			s.mu.RUnlock()
			return c
		}
		if c := pickLocale(s.builtin[typeName], locale); c != "" {
			s.mu.RUnlock()
			return c
		}
		s.mu.RUnlock()
	}
	// 二进制 / WireDef 内嵌（插件 Def.Doc/Docs）
	loc := types.LocalizeComponentDef(def, locale)
	return strings.TrimSpace(loc.Doc)
}

func pickLocale(m map[string]string, locale string) string {
	if m == nil {
		return ""
	}
	if c := strings.TrimSpace(m[locale]); c != "" {
		return c
	}
	// 回退：默认语言 → 任意非空
	if locale != types.DefaultLocale {
		if c := strings.TrimSpace(m[types.DefaultLocale]); c != "" {
			return c
		}
	}
	for _, c := range m {
		if t := strings.TrimSpace(c); t != "" {
			return t
		}
	}
	return ""
}

func cloneLocaleMap(in map[string]map[string]string) map[string]map[string]string {
	out := make(map[string]map[string]string, len(in))
	for t, m := range in {
		cp := make(map[string]string, len(m))
		for k, v := range m {
			cp[k] = v
		}
		out[t] = cp
	}
	return out
}
