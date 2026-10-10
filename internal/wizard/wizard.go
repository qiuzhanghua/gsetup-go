// Package wizard 把界面上的选择翻成一条命令。
//
// 这里是"参数只有一处定义"的那个地方：窗口里显示给用户照抄的那一行，
// 和真正 exec 出去的 argv，都是从这个包算出来的。
package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Toolchains 是我们认识的两条命令行工具链。
var Toolchains = []string{"cot", "tdp"}

// KnownToolchain 判断名字认不认识。
func KnownToolchain(name string) bool {
	for _, t := range Toolchains {
		if t == name {
			return true
		}
	}
	return false
}

// Choice 是界面 B 上用户选完的那几件事。
type Choice struct {
	Home      string // 装到哪儿（绝对路径）
	Toolchain string // cot 或 tdp
	AddToPath bool   // "让终端里也能直接用 <cmd> 命令"——勾了就是 --yes
}

// Args 是交给 install.sh / install.cmd（最终交给 gpm install）的参数。
//
// 两条约定（用户裁决）：
//   - --dir 永远显式传（脚本里烘着的那个默认值只服务裸跑的人）；
//   - --with <一家> 把清单的 requires 收窄成一家，另一家的工具链不装。
//
// PATH 集成：勾了传 --yes，没勾传 --no-path。**不能都不传**——pty 里
// gpm 会判成交互终端然后停下来问，窗口就卡在那儿了（需求 §4.5）。
func (c Choice) Args() []string {
	args := []string{"--dir", c.Home, "--with", c.Toolchain}
	if c.AddToPath {
		args = append(args, "--yes")
	} else {
		args = append(args, "--no-path")
	}
	return args
}

// Sep 是按**目标平台**选的路径分隔符。
//
// 不能用 filepath.Join：那是跑这个程序的那台机器的规矩，而这里的 goos
// 是个参数（测试里就会拿别的平台调进来）。
func Sep(goos string) string {
	if goos == "windows" {
		return `\`
	}
	return "/"
}

// Join 按目标平台拼路径。
func Join(goos string, parts ...string) string {
	return strings.Join(parts, Sep(goos))
}

// ScriptName 是入口脚本的文件名。
func ScriptName(goos string) string {
	if goos == "windows" {
		return "install.cmd"
	}
	return "install.sh"
}

// Script 是包根里那个入口脚本。
func Script(root, goos string) string {
	return Join(goos, root, ScriptName(goos))
}

// Argv 是真正 exec 的 program + args。
//
// Windows 上多包一层 cmd /c，并在末尾塞 `< NUL`：install.cmd 末尾有一句
// pause（给双击的人看的"按任意键继续"），不给 EOF 它会一直等下去
// （需求 §4.4）。cmd.exe 会自己把 `< NUL` 当成重定向解析。
func Argv(root string, c Choice, goos string) (string, []string) {
	script := Script(root, goos)
	if goos == "windows" {
		args := append([]string{"/c", script}, c.Args()...)
		return "cmd.exe", append(args, "<", "NUL")
	}
	return script, c.Args()
}

// Display 是给人照抄的那一行（相对包根，和需求里的例子一个样子）。
func Display(root string, c Choice, goos string) string {
	var b strings.Builder
	if goos == "windows" {
		b.WriteString("install.cmd")
	} else {
		b.WriteString("./install.sh")
	}
	for _, a := range c.Args() {
		b.WriteString(" ")
		b.WriteString(quoteArg(a, c, goos))
	}
	if goos == "windows" {
		b.WriteString(" < NUL")
	}
	return b.String()
}

// quoteArg 让照抄的那一行在终端里真的能跑：家目录缩成 $HOME / %USERPROFILE%
// 并且加引号（路径里可能有空格）。
func quoteArg(arg string, c Choice, goos string) string {
	if arg == c.Home {
		if abbrev, ok := AbbrevHome(c.Home, goos); ok {
			return `"` + abbrev + `"`
		}
		if strings.ContainsAny(arg, " \t") {
			return `"` + arg + `"`
		}
		return arg
	}
	return arg
}

// AbbrevHome 把家目录缩成 $HOME/... / %USERPROFILE%\...，让人一眼看懂装到哪。
func AbbrevHome(path, goos string) (string, bool) {
	userHome, err := os.UserHomeDir()
	if err != nil || userHome == "" {
		return "", false
	}
	clean := filepath.Clean(path)
	if clean == userHome {
		if goos == "windows" {
			return "%USERPROFILE%", true
		}
		return "$HOME", true
	}
	sep := string(filepath.Separator)
	if strings.HasPrefix(clean, userHome+sep) {
		rest := strings.TrimPrefix(clean, userHome+sep)
		if goos == "windows" {
			return `%USERPROFILE%\` + strings.ReplaceAll(rest, "/", `\`), true
		}
		return "$HOME/" + rest, true
	}
	return "", false
}

// DefaultHome 是界面 B 的缺省安装路径：选 cot 就是 ~/cot，选 tdp 就是 ~/tdp。
func DefaultHome(userHome, toolchain string) string {
	if userHome == "" {
		return toolchain
	}
	return filepath.Join(userHome, toolchain)
}

// UninstallArgv 是卸载那条路：直接叫家里的 gpm，不经脚本（需求 §6）。
func UninstallArgv(home, id, goos string) (string, []string) {
	name := "gpm"
	if goos == "windows" {
		name += ".exe"
	}
	return Join(goos, home, "bin", name), []string{"uninstall", id, "--yes"}
}

// UninstallDisplay 是给人看的那一行。
func UninstallDisplay(home, id, goos string) string {
	display := Join(goos, home, "bin", "gpm")
	if abbrev, ok := AbbrevHome(home, goos); ok {
		display = abbrev + "/bin/gpm"
	}
	return fmt.Sprintf("%s uninstall %s --yes", display, id)
}

// VersionLess 判断 a 是不是比 b 旧——界面 A 靠它决定按钮写"升级"还是
// "已经是最新的"（需求 §9-2）。认不出来就退回字符串比较。
func VersionLess(a, b string) bool {
	as, aok := parseVersion(a)
	bs, bok := parseVersion(b)
	if !aok || !bok || len(as) == 0 || len(bs) == 0 {
		return a != b && a < b
	}
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av != bv {
			return av < bv
		}
	}
	return false
}

func parseVersion(v string) ([]int, bool) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if v == "" {
		return nil, false
	}
	// 去掉 -dev / +build 这种尾巴
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n := 0
		if p == "" {
			return nil, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return nil, false
			}
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out, true
}
