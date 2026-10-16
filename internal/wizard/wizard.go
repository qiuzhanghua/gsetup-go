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

// PreferredToolchain 是界面 B 单选组的缺省选中项。
//
// 需求 §3.2.1 的原话是「用户选择 cot/tdp，缺省是 tdp」——所以缺省**不是**清单
// requires 的第一家：那一条是账本探测与「没写家时兜底」的规矩（§9-1）。
const PreferredToolchain = "tdp"

// DefaultChoice 从清单提供的那几家里挑界面 B 的缺省项：有 tdp 就选它，否则退回
// 第一家。缺省必须落在 requires 里 —— 用户不动它时 --with 拿的就是这个名字，
// 挑一个这家包里没有的工具链，gpm 那边只会报错。
func DefaultChoice(offered []string) string {
	for _, t := range offered {
		if t == PreferredToolchain {
			return t
		}
	}
	if len(offered) > 0 {
		return offered[0]
	}
	return ""
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

// 入口脚本的文件名。三条并存（gpm v0.6.6 / 契约 v3.15、D44）：非 Windows 只有 .sh；
// Windows 上有 .cmd（双击、以及执行策略禁脚本时的退路）与 .ps1（终端与 GUI 用）。
const (
	InstallSH  = "install.sh"
	InstallCMD = "install.cmd"
	InstallPS1 = "install.ps1"
)

// EntryNames 是"这个平台的包里该有哪几条入口"，给提示语用（用户会看到什么）。
func EntryNames(goos string) string {
	if goos == "windows" {
		return InstallCMD + " / " + InstallPS1
	}
	return InstallSH
}

// Shells 是这台机器上有哪些 PowerShell（探测结果；测试里直接喂）。
type Shells struct {
	Pwsh string // pwsh（PowerShell 7+）的路径；没有则空
	Win  string // powershell.exe（Windows PowerShell 5.1）的路径；没有则空
}

// pick 按用户裁决的顺序挑：pwsh 7 优先，其次 5.1。
func (s Shells) pick() (string, bool) {
	if s.Pwsh != "" {
		return s.Pwsh, true
	}
	if s.Win != "" {
		return s.Win, true
	}
	return "", false
}

// Entry 是这次要跑哪条入口脚本，以及怎么起它。
type Entry struct {
	Name    string   // install.sh / install.cmd / install.ps1
	Display string   // 给人照抄那一行的开头（相对包根）
	Shell   string   // 解释器可执行文件；空 = 直接 exec 脚本本身
	Args    []string // 解释器自己的参数（脚本路径接在它们后面）
	Tail    []string // 命令末尾固定要加的（cmd 需要 `< NUL`）
}

// PickEntry 按需求 §3.1 选入口：非 Windows 是 install.sh；Windows 上优先 install.ps1
// （这台机器上有 PowerShell、且包里确实有那一份），否则退回 install.cmd。
//
// 为什么 hasPS1 要当入参：包不一定是新版 gpm 打的（v0.6.6 起才有 .ps1），老包里没有它
// 就别硬挑 —— 挑一条不存在的脚本只会变成一句看不懂的报错。
func PickEntry(goos string, hasPS1 bool, sh Shells) Entry {
	if goos != "windows" {
		return Entry{Name: InstallSH, Display: "./" + InstallSH}
	}
	if interp, ok := sh.pick(); ok && hasPS1 {
		// 从下载的 zip 里解出来的 .ps1 带着下载标记（MOTW），RemoteSigned 会直接拒绝
		// 执行 —— 所以必须显式给 Bypass，这也是文档里推荐用户照抄的那一条命令
		// （实测：不加 Bypass 时报"未进行数字签名…无法在当前系统上运行此脚本"）。
		return Entry{
			Name:    InstallPS1,
			Display: shellName(interp) + " -NoProfile -ExecutionPolicy Bypass -File " + InstallPS1,
			Shell:   interp,
			Args:    []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File"},
		}
	}
	// Windows 的退路（也是"双击就能跑"的那条）：末尾塞 `< NUL` 吃掉 install.cmd 的
	// pause（给双击的人看的"按任意键继续"），不给 EOF 它会一直等下去（需求 §4.4）。
	return Entry{
		Name:    InstallCMD,
		Display: InstallCMD,
		Shell:   "cmd.exe",
		Args:    []string{"/c"},
		Tail:    []string{"<", "NUL"},
	}
}

// Script 是包根里那条入口脚本的完整路径。
func (e Entry) Script(root, goos string) string {
	return Join(goos, root, e.Name)
}

// Argv 是真正 exec 的 program + args。
//
// 三条入口的差别只在这里：`.sh` 直接 exec 自己（脚本里自己会 cd），`.cmd` 多包一层
// `cmd /c` 并吃掉 pause，`.ps1` 用 PowerShell 起、带 `-ExecutionPolicy Bypass`。
func Argv(root string, c Choice, goos string, e Entry) (string, []string) {
	script := e.Script(root, goos)
	if e.Shell == "" {
		return script, c.Args()
	}
	args := append([]string{}, e.Args...)
	args = append(args, script)
	args = append(args, c.Args()...)
	return e.Shell, append(args, e.Tail...)
}

// Display 是给人照抄的那一行（相对包根，和需求里的例子一个样子）。
//
// PowerShell 那条**照实写全**（`pwsh -NoProfile -ExecutionPolicy Bypass -File …`）：
// 只写 `.\install.ps1` 在默认策略下会被拒（同一台机器上实测过），照抄的人会以为脚本坏了。
func Display(root string, c Choice, goos string, e Entry) string {
	var b strings.Builder
	b.WriteString(e.Display)
	for _, a := range c.Args() {
		b.WriteString(" ")
		b.WriteString(quoteArg(a, c, goos))
	}
	for _, t := range e.Tail {
		b.WriteString(" " + t)
	}
	return b.String()
}

// shellName 把解释器的路径缩成命令名（`C:\Program Files\PowerShell\7\pwsh.exe` → `pwsh`）。
//
// 不用 filepath.Base：那是跑这个程序的机器的规矩，而这些路径可能来自另一个平台
// （测试里就会这样喂进来）。
func shellName(p string) string {
	s := strings.ReplaceAll(p, `\`, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSuffix(s, ".exe")
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
		// 缩出来的这一行是给人照抄**目标平台**的命令，所以分隔符也按 goos 走：
		// 宿主是 Windows 而目标是 darwin 时（测试里就会这么调），也得是
		// $HOME/cot/bin 而不是 $HOME/cot\bin。
		if sep != Sep(goos) {
			rest = strings.ReplaceAll(rest, sep, Sep(goos))
		}
		if goos == "windows" {
			return `%USERPROFILE%\` + rest, true
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
		display = Join(goos, abbrev, "bin", "gpm")
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
