package componentdocs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flowgo/flowgo/api/types"
)

func TestGetPriority(t *testing.T) {
	dir := t.TempDir()
	builtin := map[string]map[string]string{
		"inject": {
			types.LocaleZhCN: "builtin-zh",
			types.LocaleEnUS: "builtin-en",
		},
	}
	s := New(dir, builtin)
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	def := types.ComponentDef{
		Type: "inject",
		Doc:  "wire-zh",
		Docs: map[string]string{types.LocaleEnUS: "wire-en"},
	}
	if got := s.Get("inject", types.LocaleZhCN, def); got != "builtin-zh" {
		t.Fatalf("want builtin, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "inject_zh.md"), []byte("file-zh"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := s.Get("inject", types.LocaleZhCN, def); got != "file-zh" {
		t.Fatalf("want file, got %q", got)
	}
	empty := types.ComponentDef{Type: "unknown"}
	if got := s.Get("unknown", types.LocaleZhCN, empty); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
	if got := s.Get("unknown", types.LocaleZhCN, types.ComponentDef{Type: "unknown", Doc: "only-wire"}); got != "only-wire" {
		t.Fatalf("want wire fallback, got %q", got)
	}
}
