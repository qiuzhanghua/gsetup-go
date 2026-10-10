// GUI-Setup：给 gpm 打包出来的 GUI 应用配一个图形安装向导。
//
// 它自己什么也不装：找到包根、读清单、把用户的选择翻成一条命令，
// 然后在真 pty 里把 install.sh / install.cmd 跑起来，输出原样搬到窗口里。
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/qiuzhanghua/gsetup-go/internal/ledger"
	"github.com/qiuzhanghua/gsetup-go/internal/manifest"
	"github.com/qiuzhanghua/gsetup-go/internal/pkgroot"
	gsrunner "github.com/qiuzhanghua/gsetup-go/internal/runner"
	"github.com/qiuzhanghua/gsetup-go/internal/wizard"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是绑定给前端的那个对象。
type App struct {
	ctx context.Context

	mu   sync.Mutex
	busy bool

	root     string
	man      *manifest.Manifest
	userHome string
	goos     string
	loadErr  string

	// 这三个是给测试用的：真机上永远不设。
	homeOverride string
	emitHook     func(name string, payload any)
	envOverride  []string

	// pickFolder 也是给测试用的：真机上是 runtime 的目录选择框。
	pickFolder func(context.Context) (string, error)
}

// NewApp 建一个。
func NewApp() *App {
	return &App{goos: runtime.GOOS}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.ensure()
}

// ensure 只解析一次包根与清单；解析不了就把话记住，后面每个入口都把它报出来。
func (a *App) ensure() error {
	if a.loadErr != "" {
		return fmt.Errorf("%s", a.loadErr)
	}
	if a.man != nil {
		return nil
	}
	if a.homeOverride != "" {
		a.userHome = a.homeOverride
	} else if home, err := os.UserHomeDir(); err == nil {
		a.userHome = home
	}
	root, name, err := pkgroot.FindFromExecutable()
	if err != nil {
		exe, _ := os.Executable()
		a.loadErr = rootHint(a.goos, exe, err)
		return fmt.Errorf("%s", a.loadErr)
	}
	man, err := manifest.Parse(filepath.Join(root, name))
	if err != nil {
		a.loadErr = err.Error()
		return fmt.Errorf("%s", a.loadErr)
	}
	a.root, a.man = root, man
	return nil
}

// rootHint 把"找不着包根"翻成一句能照着做的话。
//
// 要紧的是分得清三种情形：
//
//  1. macOS 把 app 挪进只读临时目录了（App Translocation）—— 用户什么都没做错，
//     让他"放回去"没有用；
//  2. Windows 上"在压缩包里直接双击"—— 资源管理器只把这一个文件解到临时目录去跑，
//     用户同样什么都没做错，而且看不到自己跑到哪儿去了；
//  3. 他真的把 GUI-Setup 单独拷到别处去了 —— 这时"放回去"才是对的。
//
// goos 是参数而不是 runtime.GOOS：测试要在任何平台上都能验这三种话术。
func rootHint(goos, exe string, err error) string {
	if pkgroot.LooksTranslocated(exe) {
		return "macOS 把 GUI-Setup 挪到一个只读的临时目录里跑起来了（App Translocation —— 带" +
			"「下载」标记的 app 双击时系统就这么干），在那儿看不见解压出来的 install.sh 与清单。" +
			"点下面的「自己选包目录」，挑你解压出来的那个目录就行；或者在终端里" +
			"`xattr -dr com.apple.quarantine <解压出来的目录>` 之后再双击它。" +
			"（也可以直接跑 ./install.sh，命令行这条路不受影响。）\n原始错误：" + err.Error()
	}
	if goos == "windows" && pkgroot.LooksZipPreview(exe) {
		return "你是**在压缩包里直接双击** GUI-Setup 的：Windows 资源管理器只把这一个文件解到临时" +
			"目录里去运行，它身边既没有清单也没有 install.cmd，往上找是白找 —— 这不是你放错了。" +
			"请右键那个 zip →「全部解压缩…」，在解出来的文件夹里双击 GUI-Setup.exe" +
			"（它应该和 *-manifest.yaml、install.cmd 并排）。" +
			"要是你已经另外解压过一份，也可以点下面的「自己选包目录」直接挑那一层。" +
			"\n原始错误：" + err.Error()
	}
	return err.Error() + "。请把 GUI-Setup 放回解压出来的目录里再运行" +
		"（那一层应该有 *-manifest.yaml 与 " + wizard.ScriptName(goos) + "），" +
		"或者点下面的「自己选包目录」自己挑一下。"
}

// useRoot 把用户挑的目录当包根验一遍再收下（PickRoot 用）。
func (a *App) useRoot(dir string) error {
	names, err := pkgroot.Manifests(dir)
	if err != nil {
		return fmt.Errorf("%s 读不了：%w", dir, err)
	}
	switch len(names) {
	case 0:
		return fmt.Errorf("%s 里没有 *-manifest.yaml —— 要选解压出来、和 %s 并排的那一层", dir, wizard.ScriptName(a.goos))
	case 1:
	default:
		return fmt.Errorf("%s 里有 %d 份清单（%s），说不清该用哪一份", dir, len(names), strings.Join(names, "、"))
	}
	man, err := manifest.Parse(filepath.Join(dir, names[0]))
	if err != nil {
		return err
	}
	a.root, a.man, a.loadErr = dir, man, ""
	return nil
}

// PickRoot 让用户自己指一下包根：被 App Translocation 挪走之后，从自己的位置
// 往上找是找不着包的。挑中的目录验一遍，通过了就把整份底稿重新发给前端。
func (a *App) PickRoot() State {
	if a.pickFolder == nil {
		a.pickFolder = a.openDirectoryDialog
	}
	dir, err := a.pickFolder(a.ctx)
	if err != nil {
		return State{OK: false, Message: "选目录没成： " + err.Error(), Goos: a.goos, UserHome: a.userHome}
	}
	if strings.TrimSpace(dir) == "" {
		// 用户取消了：原来那句错话照旧报回去，界面不用动。
		return a.Bootstrap()
	}
	if err := a.useRoot(dir); err != nil {
		return State{OK: false, Message: err.Error(), Goos: a.goos, UserHome: a.userHome}
	}
	return a.Bootstrap()
}

// openDirectoryDialog 是正式实现；测试里换成 pickFolder 那个钩子。
func (a *App) openDirectoryDialog(ctx context.Context) (string, error) {
	return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{
		Title:                "选解压出来的那个目录（里面应该有 *-manifest.yaml 与 " + wizard.ScriptName(a.goos) + "）",
		DefaultDirectory:     a.defaultPickDir(),
		CanCreateDirectories: false,
	})
}

// defaultPickDir 让对话框从最可能的地方开：下载目录。
func (a *App) defaultPickDir() string {
	if a.userHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			a.userHome = home
		}
	}
	if a.userHome == "" {
		return ""
	}
	downloads := filepath.Join(a.userHome, "Downloads")
	if info, err := os.Stat(downloads); err == nil && info.IsDir() {
		return downloads
	}
	return a.userHome
}

// Installation 是界面 A 里的一行：某个家里装没装、装了哪个版本。
type Installation struct {
	Toolchain     string `json:"toolchain"`
	Home          string `json:"home"`
	HomeDisplay   string `json:"homeDisplay"`
	Installed     bool   `json:"installed"`
	Version       string `json:"version"`
	Cmd           string `json:"cmd"`
	NeedsUpgrade  bool   `json:"needsUpgrade"`
	Verified      bool   `json:"verified"`
	Uninstallable bool   `json:"uninstallable"`
}

// State 是窗口打开时的那份底稿。
type State struct {
	OK               bool           `json:"ok"`
	Message          string         `json:"message"`
	Root             string         `json:"root"`
	Short            string         `json:"short"`
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Version          string         `json:"version"`
	Cmd              string         `json:"cmd"`
	Goos             string         `json:"goos"`
	UserHome         string         `json:"userHome"`
	Toolchains       []string       `json:"toolchains"`
	DefaultToolchain string         `json:"defaultToolchain"`
	DefaultHome      string         `json:"defaultHome"`
	Installs         []Installation `json:"installs"`
	Script           string         `json:"script"`
	ScriptDisplay    string         `json:"scriptDisplay"`
	HasSetup         bool           `json:"hasSetup"`
}

// Bootstrap 是前端的第一句调用：报包根、清单、以及两个家里已经装了什么。
func (a *App) Bootstrap() State {
	if err := a.ensure(); err != nil {
		return State{OK: false, Message: err.Error(), Goos: a.goos, UserHome: a.userHome}
	}
	st := State{
		OK:         true,
		Root:       a.root,
		Short:      a.man.Short,
		ID:         a.man.ID,
		Name:       a.man.Name,
		Version:    a.man.Version,
		Cmd:        a.man.Launch.Cmd,
		Goos:       a.goos,
		UserHome:   a.userHome,
		HasSetup:   a.man.SetupEntry(a.goos).Rel() != "",
		Toolchains: wizard.Toolchains,
	}
	st.Script = wizard.Script(a.root, a.goos)
	st.ScriptDisplay = wizard.ScriptName(a.goos)

	def := a.man.DefaultToolchain()
	st.DefaultToolchain = def
	st.DefaultHome = wizard.DefaultHome(a.userHome, def)

	// 两个家都看一眼：清单说 requires: [cot, tdp]，而用户可能装在另一个家里。
	seen := map[string]bool{}
	for _, t := range append(a.man.Toolchains(), wizard.Toolchains...) {
		if seen[t] {
			continue
		}
		seen[t] = true
		st.Installs = append(st.Installs, a.inspect(t))
	}
	return st
}

func (a *App) inspect(toolchain string) Installation {
	home := wizard.DefaultHome(a.userHome, toolchain)
	inst := Installation{
		Toolchain:   toolchain,
		Home:        home,
		HomeDisplay: display(home, a.goos),
	}
	inst.Uninstallable = fileExists(gpmPath(home, a.goos))

	led, err := ledger.Load(home)
	if err != nil {
		return inst
	}
	p := led.Find(a.man.ID)
	if p == nil {
		return inst
	}
	inst.Installed = true
	inst.Version = p.Version
	inst.Cmd = p.Cmd
	inst.Verified = p.Verified
	inst.NeedsUpgrade = wizard.VersionLess(p.Version, a.man.Version)
	return inst
}

// Plan 是"将要执行什么"的完整答案：给人看的那一行，和真正 exec 的 argv。
type Plan struct {
	OK          bool     `json:"ok"`
	Message     string   `json:"message"`
	Toolchain   string   `json:"toolchain"`
	Home        string   `json:"home"`
	HomeDisplay string   `json:"homeDisplay"`
	AddToPath   bool     `json:"addToPath"`
	Installed   bool     `json:"installed"`
	Program     string   `json:"program"`
	Args        []string `json:"args"`
	Display     string   `json:"display"`
	Dir         string   `json:"dir"`
	Script      string   `json:"script"`
	Warning     string   `json:"warning"`
}

// Plan 只在内存里算，不动磁盘（需求 §4.3：先把要执行的命令只读地摊给用户看）。
func (a *App) Plan(toolchain, home string, addToPath bool) Plan {
	if err := a.ensure(); err != nil {
		return Plan{OK: false, Message: err.Error()}
	}
	toolchain = strings.TrimSpace(toolchain)
	if toolchain == "" {
		toolchain = a.man.DefaultToolchain()
	}
	if !wizard.KnownToolchain(toolchain) {
		return Plan{OK: false, Message: "工具链只认 cot 或 tdp，收到的是「" + toolchain + "」"}
	}
	home = a.resolveHome(home, toolchain)
	if !filepath.IsAbs(home) {
		return Plan{OK: false, Message: "安装路径要写绝对路径（比如 ~/cot），收到的是「" + home + "」"}
	}
	c := wizard.Choice{Home: home, Toolchain: toolchain, AddToPath: addToPath}
	p := Plan{
		OK:          true,
		Toolchain:   toolchain,
		Home:        home,
		HomeDisplay: display(home, a.goos),
		AddToPath:   addToPath,
		Display:     wizard.Display(a.root, c, a.goos),
		Dir:         a.root,
		Script:      wizard.Script(a.root, a.goos),
	}
	p.Program, p.Args = wizard.Argv(a.root, c, a.goos)

	if !fileExists(p.Script) {
		p.OK = false
		p.Message = "包里没有 " + p.Script + "——是不是只解压了一半？"
		return p
	}
	if !contains(a.man.Requires, toolchain) {
		p.Warning = "清单的 requires 里没有 " + toolchain + "，--with " + toolchain + " 可能会被 gpm 拒绝。"
	}
	// 看的是用户选的那个家（不是 toolchain 的默认家）：前端据此把"开始安装"收起来，
	// 已经装过的家该走 A 区的升级 / 重新安装 / 卸载。
	if led, err := ledger.Load(home); err == nil {
		if pkg := led.Find(a.man.ID); pkg != nil {
			p.Installed = true
			msg := fmt.Sprintf("这个家里已经装过 %s %s，再装一次就是覆盖式升级。", a.man.Name, pkg.Version)
			if p.Warning != "" {
				p.Warning += " "
			}
			p.Warning += msg
		}
	}
	return p
}

// Start 真的跑起来。输出通过 "output" 事件一行一行推给前端，结束时发 "done"。
func (a *App) Start(toolchain, home string, addToPath bool) gsrunner.Result {
	p := a.Plan(toolchain, home, addToPath)
	if !p.OK {
		return gsrunner.Result{Err: p.Message}
	}
	if !a.lock() {
		return gsrunner.Result{Err: "上一条命令还在跑，等它结束。"}
	}
	defer a.unlock()

	a.emit("start", map[string]any{"display": p.Display, "home": p.Home, "toolchain": p.Toolchain})
	a.emit("output", gsrunner.Event{Stream: gsrunner.StreamSystem, Text: "$ " + p.Display})
	if p.Warning != "" {
		a.emit("output", gsrunner.Event{Stream: gsrunner.StreamSystem, Text: "注意：" + p.Warning})
	}

	res := gsrunner.Run(gsrunner.Options{
		Dir:     p.Dir,
		Command: p.Program,
		Args:    p.Args,
		Env:     a.envOverride,
		Emit:    func(ev gsrunner.Event) { a.emit("output", ev) },
	})
	a.emit("done", map[string]any{"action": "install", "result": res, "message": res.Message()})
	return res
}

// Removal 是卸载清单里的一行：即将被回放删掉的东西。
type Removal struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Display string `json:"display"`
	Present bool   `json:"present"`
}

// UninstallPlan 是"卸载会删掉什么"的答案。
//
// 需求 §6：卸载界面要把 gpm 即将删的东西**先列出来**（入口 / 启动器 / 图形入口 /
// PATH 块），再让用户点"确认卸载"——清单就是 gpm v3.9 `askUninstall` 会摊开的那一份，
// 来源是账本（D28：账本是所有外部副作用的唯一真相）。
type UninstallPlan struct {
	OK          bool      `json:"ok"`
	Message     string    `json:"message"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Home        string    `json:"home"`
	HomeDisplay string    `json:"homeDisplay"`
	Items       []Removal `json:"items"`
	Program     string    `json:"program"`
	Args        []string  `json:"args"`
	Display     string    `json:"display"`
	Warning     string    `json:"warning"`
}

// PlanUninstall 只看账本、只碰磁盘的读操作，不动任何东西。真正的删除交给家里的 gpm。
func (a *App) PlanUninstall(home string) UninstallPlan {
	if err := a.ensure(); err != nil {
		return UninstallPlan{OK: false, Message: err.Error()}
	}
	home = a.resolveHome(home, a.man.DefaultToolchain())
	p := UninstallPlan{
		ID:          a.man.ID,
		Name:        a.man.Name,
		Version:     a.man.Version,
		Home:        home,
		HomeDisplay: display(home, a.goos),
	}
	led, err := ledger.Load(home)
	if err != nil {
		p.Message = "账本读不了：" + err.Error()
		return p
	}
	pkg := led.Find(a.man.ID)
	if pkg == nil {
		p.Message = "这个家（" + p.HomeDisplay + "）里没记账装过 " + a.man.Name + "。"
		return p
	}
	p.Version = pkg.Version

	add := func(kind, path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		p.Items = append(p.Items, Removal{
			Kind:    kind,
			Path:    path,
			Display: display(path, a.goos),
			Present: pathExists(path),
		})
	}
	add(kindLabel(pkg.EntryKind), pkg.Dir)
	add("启动器", pkg.Launcher)
	for _, l := range pkg.LinkPaths() {
		add("图形入口", l)
	}
	for _, f := range pkg.PathEditFiles() {
		add("PATH 块", f)
	}

	prog, args := wizard.UninstallArgv(home, a.man.ID, a.goos)
	p.Program, p.Args = prog, args
	p.Display = wizard.UninstallDisplay(home, a.man.ID, a.goos)

	if !fileExists(prog) {
		p.Message = "家里没有 " + prog + "，得在终端里自己卸：" + p.Display
		return p
	}
	p.OK = true
	// 清单之外的别乱猜：工具链自己（cot / tdp）不在这个包的账上，不会被卸掉。
	p.Warning = "只删这本账记着的东西。cot / tdp 自己是另一套账，不会被卸掉。"
	return p
}

// Uninstall 走"家里的 gpm"，不经脚本：卸载不需要包，需要的只是账本。
func (a *App) Uninstall(home string) gsrunner.Result {
	if err := a.ensure(); err != nil {
		return gsrunner.Result{Err: err.Error()}
	}
	home = a.resolveHome(home, a.man.DefaultToolchain())
	led, err := ledger.Load(home)
	if err != nil {
		return gsrunner.Result{Err: "账本读不了：" + err.Error()}
	}
	pkg := led.Find(a.man.ID)
	if pkg == nil {
		return gsrunner.Result{Err: "这个家（" + display(home, a.goos) + "）里没记账装过 " + a.man.Name + "。"}
	}
	prog, args := wizard.UninstallArgv(home, a.man.ID, a.goos)
	if !fileExists(prog) {
		return gsrunner.Result{Err: "家里没有 " + prog + "，得在终端里自己卸。"}
	}
	if !a.lock() {
		return gsrunner.Result{Err: "上一条命令还在跑，等它结束。"}
	}
	defer a.unlock()

	a.emit("start", map[string]any{"display": wizard.UninstallDisplay(home, a.man.ID, a.goos), "home": home, "toolchain": ""})
	a.emit("output", gsrunner.Event{Stream: gsrunner.StreamSystem, Text: "$ " + wizard.UninstallDisplay(home, a.man.ID, a.goos)})
	res := gsrunner.Run(gsrunner.Options{
		Dir:     a.root,
		Command: prog,
		Args:    args,
		Env:     a.envOverride,
		Emit:    func(ev gsrunner.Event) { a.emit("output", ev) },
	})
	a.emit("done", map[string]any{"action": "uninstall", "result": res, "message": res.Message()})
	return res
}

// Quit 是界面 C 上那个"关掉它"的按钮。
func (a *App) Quit() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}

// OpenFolder 在文件管理器里打开这个家（顺手给的）。
func (a *App) OpenFolder(path string) string {
	if path == "" {
		return ""
	}
	if err := a.ensure(); err != nil {
		return err.Error()
	}
	if err := reveal(path); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) resolveHome(home, toolchain string) string {
	home = strings.TrimSpace(home)
	switch {
	case home == "":
		return wizard.DefaultHome(a.userHome, toolchain)
	case home == "~":
		return a.userHome
	case strings.HasPrefix(home, "~/"):
		return filepath.Join(a.userHome, home[2:])
	default:
		return filepath.Clean(home)
	}
}

func (a *App) emit(name string, payload any) {
	if a.emitHook != nil {
		a.emitHook(name, payload)
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, name, payload)
	}
}

func (a *App) lock() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busy {
		return false
	}
	a.busy = true
	return true
}

func (a *App) unlock() {
	a.mu.Lock()
	a.busy = false
	a.mu.Unlock()
}

func gpmPath(home, goos string) string {
	name := "gpm"
	if goos == "windows" {
		name += ".exe"
	}
	return wizard.Join(goos, home, "bin", name)
}

func display(path, goos string) string {
	if abbrev, ok := wizard.AbbrevHome(path, goos); ok {
		return abbrev
	}
	return path
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// pathExists 连软链本身也算"在"（账本里记的图形入口就是软链）。
func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// kindLabel 把账本里的 entryKind 翻成人话。
func kindLabel(kind string) string {
	switch kind {
	case "bundle":
		return "应用（图形入口）"
	case "exe":
		return "程序"
	case "":
		return "安装内容"
	default:
		return kind
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
