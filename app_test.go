package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiuzhanghua/gsetup-go/internal/pkgroot"
	gsrunner "github.com/qiuzhanghua/gsetup-go/internal/runner"
	pty "github.com/qiuzhanghua/portable-pty"
)

const manifestYAML = `id: ai-desk
name: AI Desk
version: 0.3.1
requires: [cot, tdp]
entry:
  darwin: { bundle: "AI Desk.app" }
  windows: { exe: ai-desk.exe }
  linux: { exe: ai-desk }
setup:
  darwin: { bundle: "GUI-Setup.app" }
  windows: { exe: GUI-Setup.exe }
  linux: { exe: GUI-Setup }
launch:
  cmd: ad
  mode: activate
`

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// fakePackage 造一个"解压出来的包"：清单 + install.sh + payload/AI Desk.app。
func fakePackage(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ad-manifest.yaml"), manifestYAML, 0o644)
	if script != "" {
		write(t, filepath.Join(dir, "install.sh"), script, 0o755)
	}
	if err := os.MkdirAll(filepath.Join(dir, "payload", "AI Desk.app", "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newTestApp(t *testing.T, pkgDir string) (*App, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv(pkgroot.EnvOverride, pkgDir)
	return &App{goos: "darwin", homeOverride: home}, home
}

func requirePty(t *testing.T) {
	t.Helper()
	m, err := pty.Native().OpenPty(pty.DefaultSize)
	if err != nil {
		t.Skipf("这台机器打不开伪终端（沙箱常见）：%v", err)
	}
	_ = m.Close()
}

// 下面几条要真在 pty 里起一个 #!/bin/sh 脚本（或者就着一份 sh 写的假 gpm）：
// Windows 上装进包里的是 install.cmd / gpm.exe，起不来 POSIX 脚本，那一支
// 由 TestStartWindowsRunsCmd 覆盖。
func requirePosixShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows 走的是 install.cmd / cmd.exe，pty 里起不来 #!/bin/sh")
	}
}

// collector 把事件收下来，顶替真正的窗口。
type collector struct {
	mu     sync.Mutex
	events []gsrunner.Event
	names  []string
	done   map[string]any
}

func (c *collector) hook(name string, payload any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.names = append(c.names, name)
	if ev, ok := payload.(gsrunner.Event); ok {
		c.events = append(c.events, ev)
	}
	if name == "done" {
		if m, ok := payload.(map[string]any); ok {
			c.done = m
		}
	}
}

func (c *collector) text(stream string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, ev := range c.events {
		if ev.Stream == stream {
			out = append(out, ev.Text)
		}
	}
	return out
}

func (c *collector) has(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, n := range c.names {
		if n == name {
			return true
		}
	}
	return false
}

func TestBootstrapReadsPackage(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)

	st := a.Bootstrap()
	if !st.OK {
		t.Fatalf("该认得出这个包，得到 %s", st.Message)
	}
	if st.ID != "ai-desk" || st.Name != "AI Desk" || st.Version != "0.3.1" || st.Cmd != "ad" {
		t.Fatalf("清单字段对不上：%+v", st)
	}
	if st.Short != "ad" {
		t.Fatalf("简称该是 ad，得到 %s", st.Short)
	}
	if !st.HasSetup {
		t.Fatal("清单里有 setup: darwin，该报 true")
	}
	// 这个 App 是钉在 darwin 上的（newTestApp），所以期望值也写成
	// 目标平台的字符串，不用 filepath.Join —— 那是宿主的规矩，
	// 在 Windows 上跑就会得到 \，而实现走的是 goos。
	if st.Script != pkg+"/install.sh" {
		t.Fatalf("Script = %s", st.Script)
	}
	if st.DefaultToolchain != "tdp" {
		t.Fatalf("缺省工具链该是 tdp（需求 §3.2.1），得到 %s", st.DefaultToolchain)
	}
	if st.DefaultHome != filepath.Join(home, "tdp") {
		t.Fatalf("缺省安装路径该是 ~/tdp，得到 %s", st.DefaultHome)
	}
	if len(st.Installs) != 2 {
		t.Fatalf("两个家都该列出来，得到 %+v", st.Installs)
	}
	for _, inst := range st.Installs {
		if inst.Installed {
			t.Fatalf("干净家里不该报已装：%+v", inst)
		}
	}
}

// 找不到包根时那一句话要能让人知道怎么办。
func TestBootstrapWithoutPackage(t *testing.T) {
	a, _ := newTestApp(t, t.TempDir())
	st := a.Bootstrap()
	if st.OK {
		t.Fatal("空目录里不该认得出包")
	}
	if !strings.Contains(st.Message, "请把 GUI-Setup 放回解压出来的目录") {
		t.Fatalf("Message = %q", st.Message)
	}
}

// macOS 把下载来的 app 挪进只读临时目录（App Translocation）时，往上找是白找：
// 那句话得说清"这不是你放错了"，并且给两条能走的路。
func TestRootHintKnowsAboutTranslocation(t *testing.T) {
	exe := "/private/var/folders/cc/xx/T/AppTranslocation/2ED1/d/GUI-Setup.app/Contents/MacOS/GUI-Setup"
	msg := rootHint("darwin", exe, pkgroot.ErrNotFound)
	for _, want := range []string{"App Translocation", "xattr -dr com.apple.quarantine", "自己选包目录", pkgroot.ErrNotFound.Error()} {
		if !strings.Contains(msg, want) {
			t.Fatalf("被挪走时那句话里该有 %q，得到：%s", want, msg)
		}
	}
	if strings.Contains(msg, "请把 GUI-Setup 放回解压出来的目录") {
		t.Fatalf("挪走不是用户放错了，不该让他「放回去」：%s", msg)
	}
}

// Windows 的"在压缩包里直接双击"：资源管理器只把那一个文件解到临时目录去跑，
// 用户根本没解压过 —— 那句"放回解压出来的目录"对他毫无用处，得告诉他先解压。
func TestRootHintKnowsAboutWindowsZipPreview(t *testing.T) {
	exe := `C:\Users\q\AppData\Local\Temp\Temp1_ai-desk-0.3.4-windows-amd64.zip\GUI-Setup.exe`
	msg := rootHint("windows", exe, pkgroot.ErrNotFound)
	for _, want := range []string{"在压缩包里直接双击", "全部解压缩", "install.cmd", pkgroot.ErrNotFound.Error()} {
		if !strings.Contains(msg, want) {
			t.Fatalf("该说清是压缩包预览，缺 %q，得到：%s", want, msg)
		}
	}
	if strings.Contains(msg, "请把 GUI-Setup 放回解压出来的目录") {
		t.Fatalf("他没解压过，让他「放回去」没有用：%s", msg)
	}
}

// 真的把 app 单独拷出来放了时，原来那句话照旧（但要按平台提对入口脚本）。
func TestRootHintWhenJustMisplaced(t *testing.T) {
	msg := rootHint("darwin", "/tmp/somewhere/GUI-Setup.app/Contents/MacOS/GUI-Setup", pkgroot.ErrNotFound)
	if !strings.Contains(msg, "请把 GUI-Setup 放回解压出来的目录") {
		t.Fatalf("Message = %q", msg)
	}
	if strings.Contains(msg, "App Translocation") {
		t.Fatalf("没被挪走就别提 Translocation：%s", msg)
	}
}

// 提到"和谁并排"的地方，入口脚本名要按平台写对。
func TestRootHintNamesTheEntryScriptOfThatPlatform(t *testing.T) {
	if msg := rootHint("windows", `C:\Users\q\Downloads\pkg\GUI-Setup.exe`, pkgroot.ErrNotFound); !strings.Contains(msg, "install.cmd") {
		t.Fatalf("Windows 上该提 install.cmd：%s", msg)
	}
	if msg := rootHint("darwin", "/Users/q/Downloads/pkg/GUI-Setup.app/Contents/MacOS/GUI-Setup", pkgroot.ErrNotFound); !strings.Contains(msg, "install.sh") {
		t.Fatalf("macOS 上该提 install.sh：%s", msg)
	}
}

// 自己选包目录：挑对了就把整份底稿重发一遍，别的入口也跟着能用。
func TestPickRootAdoptsTheChosenDirectory(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, t.TempDir()) // 一开始待的目录里没有包
	if st := a.Bootstrap(); st.OK {
		t.Fatal("空目录里不该认得出包")
	}
	a.pickFolder = func(context.Context) (string, error) { return pkg, nil }
	st := a.PickRoot()
	if !st.OK {
		t.Fatalf("挑对了该能用，得到 %s", st.Message)
	}
	if st.Root != pkg {
		t.Fatalf("Root = %s，想要 %s", st.Root, pkg)
	}
	if st.Short != "ad" || st.Name != "AI Desk" {
		t.Fatalf("清单该读出来了：%+v", st)
	}
	if st.DefaultHome != filepath.Join(home, "tdp") {
		t.Fatalf("DefaultHome = %s", st.DefaultHome)
	}
	if p := a.Plan("cot", "", false); !p.OK {
		t.Fatalf("收下之后 Plan 该能规划了，得到 %s", p.Message)
	}
}

// 挑错了（没清单 / 两份清单）要说清楚，别默默换一个。
func TestPickRootRejectsWrongDirectory(t *testing.T) {
	a, _ := newTestApp(t, t.TempDir())
	a.pickFolder = func(context.Context) (string, error) { return t.TempDir(), nil }
	if st := a.PickRoot(); st.OK || !strings.Contains(st.Message, "里没有 *-manifest.yaml") {
		t.Fatalf("空目录该被拒，得到 %+v", st)
	}

	two := t.TempDir()
	write(t, filepath.Join(two, "ad-manifest.yaml"), manifestYAML, 0o644)
	write(t, filepath.Join(two, "ac-manifest.yaml"), manifestYAML, 0o644)
	a.pickFolder = func(context.Context) (string, error) { return two, nil }
	if st := a.PickRoot(); st.OK || !strings.Contains(st.Message, "2 份清单") {
		t.Fatalf("两份清单该被拒，得到 %+v", st)
	}
}

// 用户点了取消：原来那句"找不着包"照旧报回去，界面不用动。
func TestPickRootCancelKeepsTheComplaint(t *testing.T) {
	a, _ := newTestApp(t, t.TempDir())
	a.pickFolder = func(context.Context) (string, error) { return "", nil }
	st := a.PickRoot()
	if st.OK || !strings.Contains(st.Message, "请把 GUI-Setup 放回解压出来的目录") {
		t.Fatalf("取消后该还是原来那句话，得到 %+v", st)
	}
}

// 对话框本身打不开（没有图形会话之类）也不能崩，得报出来。
func TestPickRootSurfacesDialogError(t *testing.T) {
	a, _ := newTestApp(t, t.TempDir())
	a.pickFolder = func(context.Context) (string, error) { return "", errors.New("没有图形会话") }
	if st := a.PickRoot(); st.OK || !strings.Contains(st.Message, "没有图形会话") {
		t.Fatalf("对话框出错该报出来，得到 %+v", st)
	}
}

// 命令预览：参数只有一处定义，显示的和 exec 的是同一份。
func TestPlanCommandLine(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)

	p := a.Plan("cot", "", true)
	if !p.OK {
		t.Fatalf("该能规划，得到 %s", p.Message)
	}
	if p.Program != pkg+"/install.sh" {
		t.Fatalf("Program = %s", p.Program)
	}
	want := []string{"--dir", filepath.Join(home, "cot"), "--with", "cot", "--yes"}
	if strings.Join(p.Args, " ") != strings.Join(want, " ") {
		t.Fatalf("Args = %v，想要 %v", p.Args, want)
	}
	if !strings.HasPrefix(p.Display, "./install.sh --dir ") {
		t.Fatalf("Display = %q", p.Display)
	}
	if !strings.Contains(p.Display, "--with cot --yes") {
		t.Fatalf("Display = %q", p.Display)
	}
	if p.Warning != "" {
		t.Fatalf("干净家里不该有警告，得到 %q", p.Warning)
	}
	if !filepath.IsAbs(p.Home) {
		t.Fatalf("家该是绝对路径，得到 %s", p.Home)
	}
}

func TestPlanDefaultsToDefaultToolchain(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)

	p := a.Plan("", "", false)
	if !p.OK || p.Toolchain != "cot" {
		t.Fatalf("空工具链该退回缺省，得到 %+v", p)
	}
	if p.Args[4] != "--no-path" {
		t.Fatalf("不勾 PATH 集成该传 --no-path，得到 %v", p.Args)
	}
	if p.Args[1] != filepath.Join(home, "cot") {
		t.Fatalf("家不对：%v", p.Args)
	}
}

func TestPlanRejectsBadInput(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, _ := newTestApp(t, pkg)

	if p := a.Plan("cargo", "", false); p.OK {
		t.Fatal("不认识的工具链该拒绝")
	}
	if p := a.Plan("cot", "cot", false); p.OK || !strings.Contains(p.Message, "绝对路径") {
		t.Fatalf("相对路径该拒绝，得到 %+v", p)
	}
}

func TestPlanWarnsOnMissingRequires(t *testing.T) {
	pkg := t.TempDir()
	write(t, filepath.Join(pkg, "ad-manifest.yaml"), "id: ai-desk\nname: AI Desk\nversion: 0.3.1\nrequires: [cot]\n", 0o644)
	write(t, filepath.Join(pkg, "install.sh"), "#!/bin/sh\nexit 0\n", 0o755)
	a, _ := newTestApp(t, pkg)

	p := a.Plan("tdp", "", false)
	if !p.OK {
		t.Fatalf("还能规划，得到 %s", p.Message)
	}
	if !strings.Contains(p.Warning, "requires 里没有 tdp") {
		t.Fatalf("该提醒一句，得到 %q", p.Warning)
	}
}

func TestPlanWarnsWhenAlreadyInstalled(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)
	write(t, filepath.Join(home, "cot", "cot-state.json"),
		`{"schemaVersion":1,"packages":[{"id":"ai-desk","version":"0.3.0"}]}`, 0o644)

	st := a.Bootstrap()
	if len(st.Installs) != 2 || !st.Installs[0].Installed || !st.Installs[0].NeedsUpgrade {
		t.Fatalf("该认出装过并且更旧：%+v", st.Installs)
	}
	p := a.Plan("cot", "", false)
	if !strings.Contains(p.Warning, "覆盖式升级") {
		t.Fatalf("该提醒会覆盖，得到 %q", p.Warning)
	}
}

// 选中的那个家算数：Plan.Installed 跟着用户填的 home 走，不跟 toolchain 的默认家走。
// （前端靠这个把已经装过的家的"开始安装"收起来。）
func TestPlanInstalledFollowsTheChosenHome(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)
	write(t, filepath.Join(home, "cot", "cot-state.json"),
		`{"schemaVersion":1,"packages":[{"id":"ai-desk","version":"0.3.1"}]}`, 0o644)

	if p := a.Plan("cot", "", false); !p.Installed {
		t.Fatalf("默认家 ~/cot 里装过，该报 installed：%+v", p)
	}
	if p := a.Plan("tdp", "", false); p.Installed {
		t.Fatalf("~/tdp 里没装过，不该报 installed：%+v", p)
	}
	if p := a.Plan("cot", "~/cot-two", false); p.Installed {
		t.Fatalf("~/cot-two 里没装过，不该报 installed：%+v", p)
	}
	// 选了 tdp 却把家指回装过的 ~/cot：按家算，照样报 installed。
	if p := a.Plan("tdp", "~/cot", false); !p.Installed {
		t.Fatalf("家是 ~/cot、那里装过，就该报 installed：%+v", p)
	}
	// 没装过的家不该多出一句"覆盖式升级"。
	if p := a.Plan("tdp", "", false); strings.Contains(p.Warning, "覆盖式升级") {
		t.Fatalf("空家不该提覆盖，得到 %q", p.Warning)
	}
}

func TestPlanNeedsScriptInPackage(t *testing.T) {
	pkg := fakePackage(t, "") // 不放 install.sh
	a, _ := newTestApp(t, pkg)
	p := a.Plan("cot", "", false)
	if p.OK || !strings.Contains(p.Message, "包里没有") {
		t.Fatalf("少脚本该拒绝，得到 %+v", p)
	}
}

// 真跑一遍：pty 里输出按行走、stderr 在 Unix 上单独标红。
func TestStartRunsScriptInPty(t *testing.T) {
	script := "#!/bin/sh\necho \"装到 $2\"\necho oops 1>&2\nexit 0\n"
	pkg := fakePackage(t, script)
	a, home := newTestApp(t, pkg)
	requirePosixShell(t)
	requirePty(t)
	c := &collector{}
	a.emitHook = c.hook

	res := a.Start("cot", "", true)
	if !res.Success {
		t.Fatalf("该成功，得到 %+v", res)
	}
	if got := c.text(gsrunner.StreamStdout); len(got) != 1 || got[0] != "装到 "+filepath.Join(home, "cot") {
		t.Fatalf("stdout = %v", got)
	}
	if got := c.text(gsrunner.StreamStderr); len(got) != 1 || got[0] != "oops" {
		t.Fatalf("stderr = %v", got)
	}
	if !c.has("start") || !c.has("done") {
		t.Fatalf("start / done 事件该发出去，得到 %v", c.names)
	}
	if c.done == nil || c.done["result"] == nil {
		t.Fatalf("done 该带上结果：%+v", c.done)
	}
}

func TestStartReportsFailure(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 7\n")
	a, _ := newTestApp(t, pkg)
	requirePosixShell(t)
	requirePty(t)
	c := &collector{}
	a.emitHook = c.hook

	res := a.Start("cot", "", false)
	if res.Success || res.Code != 7 {
		t.Fatalf("该报失败 7，得到 %+v", res)
	}
	if res.Message() != "失败（退出码 7）" {
		t.Fatalf("Message = %q", res.Message())
	}
}

func TestUninstallCallsGpmInHome(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)
	requirePosixShell(t)
	requirePty(t)
	cot := filepath.Join(home, "cot")
	write(t, filepath.Join(cot, "cot-state.json"),
		`{"schemaVersion":1,"packages":[{"id":"ai-desk","version":"0.3.1"}]}`, 0o644)
	write(t, filepath.Join(cot, "bin", "gpm"), "#!/bin/sh\necho got: \"$@\"\nexit 0\n", 0o755)
	c := &collector{}
	a.emitHook = c.hook

	res := a.Uninstall(cot)
	if !res.Success {
		t.Fatalf("该成功，得到 %+v", res)
	}
	got := strings.Join(c.text(gsrunner.StreamStdout), "|")
	if got != "got: uninstall ai-desk --yes" {
		t.Fatalf("gpm 收到的参数不对：%q", got)
	}
	display := strings.Join(c.text(gsrunner.StreamSystem), "|")
	if !strings.Contains(display, "uninstall ai-desk --yes") {
		t.Fatalf("系统行该给人看命令：%q", display)
	}
}

func TestUninstallWithoutLedger(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)
	res := a.Uninstall(filepath.Join(home, "cot"))
	if res.Err == "" || !strings.Contains(res.Err, "没记账") {
		t.Fatalf("干净家该说没记账，得到 %+v", res)
	}
}

func TestUninstallWithoutGpm(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)
	cot := filepath.Join(home, "cot")
	write(t, filepath.Join(cot, "cot-state.json"),
		`{"schemaVersion":1,"packages":[{"id":"ai-desk","version":"0.3.1"}]}`, 0o644)
	res := a.Uninstall(cot)
	if res.Err == "" || !strings.Contains(res.Err, "家里没有") {
		t.Fatalf("没有 gpm 该说清楚，得到 %+v", res)
	}
}

// 前端装完 / 卸完要重新问一次 A 区，靠的就是 Bootstrap 可以反复调：
// 在同一个 App 上第二次调用必须反映账本和 bin/gpm 的最新样子。
func TestBootstrapIsRepeatableAfterUninstall(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)
	cot := filepath.Join(home, "cot")
	write(t, filepath.Join(cot, "bin", "gpm"), "#!/bin/sh\nexit 0\n", 0o755)
	write(t, filepath.Join(cot, "cot-state.json"),
		`{"schemaVersion":1,"packages":[{"id":"ai-desk","version":"0.3.1"}]}`, 0o644)

	cotInstall := func(st State) Installation {
		t.Helper()
		for _, i := range st.Installs {
			if i.Toolchain == "cot" {
				return i
			}
		}
		t.Fatalf("Bootstrap 里该有 cot 那一条：%+v", st.Installs)
		return Installation{}
	}

	before := cotInstall(a.Bootstrap())
	if !before.Installed || before.Version != "0.3.1" || !before.Uninstallable {
		t.Fatalf("装着的家该报 installed / 0.3.1 / uninstallable：%+v", before)
	}

	// 卸载之后的样子：账本里没有这条了，bin/gpm 也没了。
	write(t, filepath.Join(cot, "cot-state.json"), `{"schemaVersion":1,"packages":[]}`, 0o644)
	if err := os.Remove(filepath.Join(cot, "bin", "gpm")); err != nil {
		t.Fatal(err)
	}

	after := cotInstall(a.Bootstrap())
	if after.Installed || after.Version != "" || after.Uninstallable {
		t.Fatalf("卸完之后该报没装、也没有可卸载的入口：%+v", after)
	}
}

// 一次只能跑一条：第二条要当场被拒，不能两条命令抢同一个终端。
// 需求 §6：卸载之前要把 gpm 即将回放删掉的东西先摊开。
func TestPlanUninstallListsWhatWillGo(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)
	cot := filepath.Join(home, "cot")
	app := filepath.Join(cot, "AI Desk.app")
	launcher := filepath.Join(cot, "bin", "ad")
	link := filepath.Join(home, "Applications", "AI Desk.app")
	rc := filepath.Join(home, ".zshrc")

	write(t, filepath.Join(cot, "bin", "gpm"), "#!/bin/sh\nexit 0\n", 0o755)
	write(t, filepath.Join(app, "Contents", "Info.plist"), "x", 0o644)
	write(t, launcher, "#!/bin/sh\nexit 0\n", 0o755)
	write(t, rc, "# >>> gpm >>>\n", 0o644)
	// 图形入口故意不建：界面上该画成"已经不在"。
	write(t, filepath.Join(cot, "cot-state.json"), `{"schemaVersion":1,"packages":[{
		"id":"ai-desk","name":"AI Desk","version":"0.3.1",
		"dir":`+jsonString(app)+`,"entry":"AI Desk.app","entryKind":"bundle","cmd":"ad",
		"launcher":`+jsonString(launcher)+`,
		"links":[{"path":`+jsonString(link)+`,"kind":"app-symlink"}],
		"pathEdits":[{"path":`+jsonString(rc)+`,"created":false}],
		"verified":true}]}`, 0o644)

	p := a.PlanUninstall(cot)
	if !p.OK {
		t.Fatalf("该能规划卸载，得到 %s", p.Message)
	}
	if p.Version != "0.3.1" || p.Home != cot {
		t.Fatalf("头部不对：%+v", p)
	}
	want := []struct {
		kind    string
		path    string
		present bool
	}{
		{"应用（图形入口）", app, true},
		{"启动器", launcher, true},
		{"图形入口", link, false},
		{"PATH 块", rc, true},
	}
	if len(p.Items) != len(want) {
		t.Fatalf("清单该有 %d 项，得到 %+v", len(want), p.Items)
	}
	for i, w := range want {
		got := p.Items[i]
		if got.Kind != w.kind || got.Path != w.path || got.Present != w.present {
			t.Fatalf("第 %d 项不对：%+v，想要 %+v", i, got, w)
		}
		if got.Display == "" {
			t.Fatalf("第 %d 项该有给人看的形式：%+v", i, got)
		}
	}
	if !strings.Contains(p.Display, "uninstall ai-desk --yes") {
		t.Fatalf("Display = %q", p.Display)
	}
	if !strings.HasSuffix(p.Program, "bin/gpm") {
		t.Fatalf("Program = %s", p.Program)
	}
	if p.Warning == "" {
		t.Fatal("该说明工具链自己不会被卸掉")
	}
}

func TestPlanUninstallWithoutLedger(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)
	p := a.PlanUninstall(filepath.Join(home, "cot"))
	if p.OK || !strings.Contains(p.Message, "没记账") {
		t.Fatalf("干净家该说没记账，得到 %+v", p)
	}
}

func TestPlanUninstallWithoutGpm(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nexit 0\n")
	a, home := newTestApp(t, pkg)
	cot := filepath.Join(home, "cot")
	write(t, filepath.Join(cot, "cot-state.json"),
		`{"schemaVersion":1,"packages":[{"id":"ai-desk","version":"0.3.1"}]}`, 0o644)
	p := a.PlanUninstall(cot)
	if p.OK || !strings.Contains(p.Message, "家里没有") {
		t.Fatalf("没有 gpm 该说清楚，得到 %+v", p)
	}
}

func jsonString(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestStartRejectsConcurrentRun(t *testing.T) {
	pkg := fakePackage(t, "#!/bin/sh\nsleep 2\nexit 0\n")
	a, _ := newTestApp(t, pkg)
	requirePosixShell(t)
	requirePty(t)

	done := make(chan gsrunner.Result, 1)
	go func() { done <- a.Start("cot", "", false) }()

	// 等它就位再打第二条
	busy := false
	for i := 0; i < 200; i++ {
		a.mu.Lock()
		busy = a.busy
		a.mu.Unlock()
		if busy {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !busy {
		t.Fatal("第一条该占住位置")
	}
	second := a.Start("cot", "", false)
	if second.Err == "" || !strings.Contains(second.Err, "还在跑") {
		t.Fatalf("第二条该被拒，得到 %+v", second)
	}
	<-done
}

// Windows 那一支：pty 里起的是 cmd.exe /c install.cmd … < NUL（wizard.Argv 拼的），
// 退出码要原样传回来。这条只在 Windows 上跑 —— 它要的就是一个真的 cmd.exe。
func TestStartWindowsRunsCmd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("只在 Windows 上跑（要真的 cmd.exe）")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ad-manifest.yaml"), manifestYAML, 0o644)
	// %2 就是 --dir 后面那个家：cmd 的 %0 是脚本本身。
	write(t, filepath.Join(dir, "install.cmd"), "@echo off\r\necho 装到 %2\r\nexit /b 7\r\n", 0o644)
	if err := os.MkdirAll(filepath.Join(dir, "payload", "AI Desk.app", "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv(pkgroot.EnvOverride, dir)
	a := &App{goos: "windows", homeOverride: home}
	requirePty(t)
	c := &collector{}
	a.emitHook = c.hook

	res := a.Start("cot", "", true)
	if res.Success || res.Code != 7 {
		t.Fatalf("该报失败 7，得到 %+v", res)
	}
	if res.Message() != "失败（退出码 7）" {
		t.Fatalf("Message = %q", res.Message())
	}
	lines := append(c.text(gsrunner.StreamStdout), c.text(gsrunner.StreamStderr)...)
	if !strings.Contains(strings.Join(lines, "|"), home) {
		t.Fatalf("install.cmd 该收到 --dir %s，pty 里只看到 %q", home, lines)
	}
}

// Windows 上"有 PowerShell、且包里确实有 install.ps1"时走的那条路（用户裁决：
// pwsh 7 > powershell 5.1 > install.cmd）：真拿 PowerShell 起脚本、参数照传、
// 退出码照传回来。这条要一个真的 PowerShell —— CI 的 windows 腿上有。
func TestStartWindowsRunsInstallPS1(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("只在 Windows 上跑（要真的 PowerShell）")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ad-manifest.yaml"), manifestYAML, 0o644)
	write(t, filepath.Join(dir, "install.ps1"),
		"foreach ($a in $args) { Write-Host \"arg=<$a>\" }\nexit 7\n", 0o644)
	if err := os.MkdirAll(filepath.Join(dir, "payload", "AI Desk.app", "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewApp() // 真探测一次 pwsh / powershell.exe
	if a.shells.Pwsh == "" && a.shells.Win == "" {
		t.Skip("这台机器没有 PowerShell")
	}
	home := t.TempDir()
	t.Setenv(pkgroot.EnvOverride, dir)
	a.homeOverride = home
	requirePty(t)

	p := a.Plan("cot", "", false)
	if !p.OK || !strings.HasSuffix(p.Script, "install.ps1") {
		t.Fatalf("该挑 install.ps1，得到 %+v", p)
	}
	if !strings.Contains(p.Display, "-ExecutionPolicy Bypass") {
		t.Fatalf("给人照抄的那一行该带 -ExecutionPolicy Bypass：%q", p.Display)
	}

	c := &collector{}
	a.emitHook = c.hook
	res := a.Start("cot", "", false)
	lines := append(c.text(gsrunner.StreamStdout), c.text(gsrunner.StreamStderr)...)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"arg=<--dir>", "arg=<" + p.Home + ">", "arg=<--with>", "arg=<cot>", "arg=<--no-path>"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("脚本没收到 %q：\n%s", want, joined)
		}
	}
	if res.Code != 7 {
		t.Fatalf("退出码该原样传回来（7），得到 %+v\n%s", res, joined)
	}
}

// 被策略拦住时要认得出这种失败（真正的"退回 install.cmd"动作在 Start 里，
// 正常路径由上面那条真 PowerShell 用例覆盖）。中英两种输出都算 —— 输出语言跟着系统走。
func TestExecPolicyRefused(t *testing.T) {
	yes := []string{
		"SecurityError: 无法加载文件 C:\\pkg\\install.ps1，因为此系统上已禁用脚本运行。有关详细信息，请参阅 https://go.microsoft.com/fwlink/?LinkID=135170 处的 about_Execution_Policies。",
		"文件 C:\\pkg\\install.ps1 未进行数字签名。无法在当前系统上运行此脚本。",
		"File C:\\pkg\\install.ps1 cannot be loaded because running scripts is disabled on this system.",
		"File C:\\pkg\\install.ps1 is not digitally signed.",
	}
	for _, s := range yes {
		if !execPolicyRefused(s) {
			t.Errorf("该认出来：%q", s)
		}
	}
	for _, s := range []string{
		"",
		"gpm: mkdir C:\\x: The system cannot find the path specified.",
		"AI Desk 0.3.7 装好了。",
	} {
		if execPolicyRefused(s) {
			t.Errorf("不该认成策略问题：%q", s)
		}
	}
}
