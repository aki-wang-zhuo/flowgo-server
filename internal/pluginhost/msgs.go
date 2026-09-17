package pluginhost

import (
	"fmt"

	"github.com/flowgo/flowgo/api/types"
)

// 用户可见错误文案（中英）；按 Accept-Language 选取。
func msg(locale, zh, en string) string {
	return types.PickI18n(map[string]string{types.LocaleEnUS: en}, locale, zh)
}

func errDynamicLib(locale string) error {
	return fmt.Errorf("%s", msg(locale,
		"不支持 Go plugin 动态库（.so/.dll）；请上传本机可执行文件（Windows: .exe，Linux: 无后缀二进制）或包含该文件的 zip",
		"Go plugin shared libraries (.so/.dll) are not supported; upload a native executable (Windows: .exe, Linux: binary) or a zip containing it",
	))
}

func errExeOnNonWindows(locale, goos string) error {
	return fmt.Errorf("%s", msg(locale,
		fmt.Sprintf("当前服务器为 %s，不能加载 Windows .exe，请上传对应平台的插件二进制", goos),
		fmt.Sprintf("Server OS is %s; cannot load Windows .exe — upload a matching binary", goos),
	))
}

func errNeedExeOnWindows(locale string) error {
	return fmt.Errorf("%s", msg(locale,
		"Windows 服务器请上传 .exe 或包含 .exe 的 zip",
		"On Windows, upload a .exe or a zip that contains a .exe",
	))
}

func errWrongOS(locale, want, have string) error {
	return fmt.Errorf("%s", msg(locale,
		fmt.Sprintf("插件面向 %s，当前服务器为 %s，请上传匹配平台的可执行文件", want, have),
		fmt.Sprintf("Plugin targets %s but server is %s; upload a matching binary", want, have),
	))
}

func errNeedWindowsExe(locale, name string) error {
	return fmt.Errorf("%s", msg(locale,
		fmt.Sprintf("Windows 需要 .exe 插件，当前文件: %s", name),
		fmt.Sprintf("Windows requires a .exe plugin; got: %s", name),
	))
}

func errCannotLoadExe(locale, goos string) error {
	return fmt.Errorf("%s", msg(locale,
		fmt.Sprintf("当前服务器为 %s，不能加载 .exe", goos),
		fmt.Sprintf("Server OS is %s; cannot load .exe", goos),
	))
}

func errNotPE(locale string) error {
	return fmt.Errorf("%s", msg(locale,
		"不是有效的 Windows PE（.exe）文件",
		"Not a valid Windows PE (.exe) file",
	))
}

func errNotELF(locale string) error {
	return fmt.Errorf("%s", msg(locale,
		"不是有效的 Linux ELF 可执行文件",
		"Not a valid Linux ELF executable",
	))
}

func errStartPlugin(locale string, cause error) error {
	return fmt.Errorf("%s: %w", msg(locale, "启动插件失败", "failed to start plugin"), cause)
}

func errTypeOccupied(locale, typeName string) error {
	return fmt.Errorf("%s", msg(locale,
		fmt.Sprintf("类型 %s 已被内置节点占用，无法加载插件", typeName),
		fmt.Sprintf("Type %s is occupied by a built-in node", typeName),
	))
}

func errNoTypes(locale string) error {
	return fmt.Errorf("%s", msg(locale,
		"插件未声明任何节点类型",
		"Plugin declared no node types",
	))
}

func errPluginNotFound(locale, id string) error {
	return fmt.Errorf("%s", msg(locale,
		fmt.Sprintf("插件不存在: %s", id),
		fmt.Sprintf("Plugin not found: %s", id),
	))
}
