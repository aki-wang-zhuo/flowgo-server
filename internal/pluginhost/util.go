package pluginhost

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/flowgo/flowgo/api/types"
)

// ValidateUploadName 拒绝明确不支持的动态库扩展名。
func ValidateUploadName(fileName, locale string) error {
	locale = types.NormalizeLocale(locale)
	base := strings.ToLower(filepath.Base(fileName))
	switch {
	case strings.HasSuffix(base, ".so"), strings.HasSuffix(base, ".dll"), strings.HasSuffix(base, ".dylib"):
		return errDynamicLib(locale)
	case strings.HasSuffix(base, ".exe"):
		if runtime.GOOS != "windows" {
			return errExeOnNonWindows(locale, runtime.GOOS)
		}
		return nil
	case strings.HasSuffix(base, ".zip"):
		return nil
	default:
		if runtime.GOOS == "windows" {
			return errNeedExeOnWindows(locale)
		}
		return nil
	}
}

// ValidateBinaryForHost 粗检二进制是否匹配当前 OS。
func ValidateBinaryForHost(binPath, locale string) error {
	locale = types.NormalizeLocale(locale)
	fi, err := os.Stat(binPath)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("%s", msg(locale, "不是可执行文件: "+binPath, "not an executable: "+binPath))
	}
	name := strings.ToLower(filepath.Base(binPath))
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(name, ".exe") {
			return errNeedWindowsExe(locale, name)
		}
	} else if strings.HasSuffix(name, ".exe") {
		return errCannotLoadExe(locale, runtime.GOOS)
	}
	f, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := make([]byte, 4)
	n, _ := f.Read(hdr)
	if n < 2 {
		return fmt.Errorf("%s", msg(locale, "文件过小，不是有效可执行文件", "file too small to be a valid executable"))
	}
	if runtime.GOOS == "windows" {
		if hdr[0] != 'M' || hdr[1] != 'Z' {
			return errNotPE(locale)
		}
	} else if runtime.GOOS == "linux" {
		if n >= 4 && !(hdr[0] == 0x7f && hdr[1] == 'E' && hdr[2] == 'L' && hdr[3] == 'F') {
			return errNotELF(locale)
		}
	}
	return nil
}

func pluginIDFromName(fileName string) string {
	base := filepath.Base(fileName)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.TrimSpace(base)
	if base == "" {
		base = "plugin"
	}
	var b strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	id := b.String()
	if id == "" {
		id = "plugin"
	}
	return id
}

func writeManifest(dir string, man Manifest) error {
	b, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644)
}

func readManifest(dir string) (Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	var man Manifest
	if err := json.Unmarshal(b, &man); err != nil {
		return Manifest{}, err
	}
	if man.ID == "" {
		man.ID = filepath.Base(dir)
	}
	return man, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func findBinary(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var bins []string
	for _, e := range entries {
		if e.IsDir() || e.Name() == "manifest.json" {
			continue
		}
		name := strings.ToLower(e.Name())
		// 文档 Markdown、共享库不计入可执行文件
		if strings.HasSuffix(name, ".md") ||
			strings.HasSuffix(name, ".so") || strings.HasSuffix(name, ".dll") || strings.HasSuffix(name, ".dylib") {
			continue
		}
		bins = append(bins, filepath.Join(dir, e.Name()))
	}
	if len(bins) != 1 {
		return "", fmt.Errorf("plugin dir needs exactly one binary, got %d", len(bins))
	}
	return bins[0], nil
}

func unzipPlugin(zipPath, destDir string) (binName string, err error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	var candidates []string
	for _, f := range zr.File {
		name := f.Name
		if strings.Contains(name, "..") {
			continue
		}
		base := filepath.Base(name)
		if base == "" || base == "." || strings.HasSuffix(name, "/") {
			continue
		}
		target := filepath.Join(destDir, base)
		if err := writeZipFile(f, target); err != nil {
			return "", err
		}
		lower := strings.ToLower(base)
		if lower == "manifest.json" {
			continue
		}
		// 文档随 zip 解压保留，但不作为可执行候选
		if strings.HasSuffix(lower, ".md") ||
			strings.HasSuffix(lower, ".so") || strings.HasSuffix(lower, ".dll") || strings.HasSuffix(lower, ".dylib") {
			continue
		}
		candidates = append(candidates, base)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no executable in zip")
	}
	for _, c := range candidates {
		if strings.HasSuffix(strings.ToLower(c), ".exe") {
			return c, nil
		}
	}
	return candidates[0], nil
}

func writeZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	mode := os.FileMode(0o644)
	if f.Mode()&0o111 != 0 || runtime.GOOS != "windows" {
		mode = 0o755
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
