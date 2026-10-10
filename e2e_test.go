package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiuzhanghua/gsetup-go/internal/pkgroot"
)

// 这条是真机端到端：拿一个**真的解压出来的包**（含真的 gpm、真的 install.sh、
// 真的 payload），让 GUI-Setup 的代码路径去装一遍、再卸一遍。
//
// 默认跳过，因为它要几十秒、要 pty、还会真的往临时家里写东西：
//
//	GSETUP_E2E_PKG=/tmp/fwd/pk go test -count=1 -run TestEndToEndRealPackage -v .
func TestEndToEndRealPackage(t *testing.T) {
	src := os.Getenv("GSETUP_E2E_PKG")
	if src == "" {
		t.Skip("设 GSETUP_E2E_PKG=<解压出来的包目录> 才跑这条真机端到端")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("这条钉的是 macOS 上的真实路径")
	}
	requirePty(t)

	pkg := filepath.Join(t.TempDir(), "pkg")
	if err := copyTree(src, pkg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pkg, "gpm")); err != nil {
		t.Fatalf("这个包里没有 gpm：%v", err)
	}

	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(pkgroot.EnvOverride, pkg)

	a := &App{goos: runtime.GOOS, homeOverride: home}
	// 子进程用自己的家：绝不能让测试碰到真的 ~/cot。
	a.envOverride = cleanEnv(home)

	st := a.Bootstrap()
	if !st.OK {
		t.Fatalf("认不出这个包：%s", st.Message)
	}
	if st.ID != "ai-desk" || st.DefaultToolchain != "cot" {
		t.Fatalf("清单读得不对：%+v", st)
	}

	p := a.Plan("cot", "", true)
	if !p.OK {
		t.Fatalf("规划失败：%s", p.Message)
	}
	t.Logf("将要执行：%s", p.Display)

	res := a.Start("cot", "", true)
	if !res.Success {
		t.Fatalf("安装失败：%+v", res.Message())
	}

	cot := filepath.Join(home, "cot")
	app := filepath.Join(cot, "AI Desk.app")
	mustExist(t, app)
	mustExist(t, filepath.Join(cot, "bin", "ad"))
	mustExist(t, filepath.Join(cot, "cot-state.json"))

	// v3.6 起的布局：GUI 应用落顶层，lib/ 里不再有它。
	entries, err := os.ReadDir(cot)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
		if strings.HasPrefix(e.Name(), "ai-desk_") {
			t.Fatalf("v3.6 起不该再有 ai-desk_* 目录，家目录里是 %v", names)
		}
	}
	if lib, err := os.ReadDir(filepath.Join(cot, "lib")); err == nil && len(lib) != 0 {
		t.Fatalf("lib/ 该是空的，得到 %d 项", len(lib))
	}

	led := readLedger(t, filepath.Join(cot, "cot-state.json"))
	pkgRec := led.Packages[0]
	if pkgRec.ID != "ai-desk" || pkgRec.EntryKind != "bundle" || pkgRec.Cmd != "ad" {
		t.Fatalf("账本记得不对：%+v", pkgRec)
	}
	if pkgRec.Dir != app {
		t.Fatalf("账本 dir = %s，想要 %s", pkgRec.Dir, app)
	}

	// 装完之后界面 A 该认出"已经装过了"，而且不比包里的旧。
	st2 := a.Bootstrap()
	if !st2.Installs[0].Installed || st2.Installs[0].Version != st.Version {
		t.Fatalf("装完该认出来：%+v", st2.Installs)
	}
	if st2.Installs[0].NeedsUpgrade {
		t.Fatal("刚装的版本不该报「可以升级」")
	}

	// 卸掉：走家里的 gpm，不经脚本。
	un := a.Uninstall(cot)
	if !un.Success {
		t.Fatalf("卸载失败：%+v", un.Message())
	}
	mustNotExist(t, app)
	mustNotExist(t, filepath.Join(cot, "bin", "ad"))
	led2 := readLedger(t, filepath.Join(cot, "cot-state.json"))
	if len(led2.Packages) != 0 {
		t.Fatalf("卸完账本该是空的：%+v", led2.Packages)
	}
}

// cleanEnv 是给子进程的环境：把家的三个变量换掉，别的照抄。
func cleanEnv(home string) []string {
	var out []string
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "HOME="),
			strings.HasPrefix(kv, "COT_HOME="),
			strings.HasPrefix(kv, "TDP_HOME="):
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HOME="+home)
}

type ledgerFile struct {
	Packages []struct {
		ID        string `json:"id"`
		Version   string `json:"version"`
		Dir       string `json:"dir"`
		EntryKind string `json:"entryKind"`
		Cmd       string `json:"cmd"`
	} `json:"packages"`
}

func readLedger(t *testing.T, path string) ledgerFile {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var l ledgerFile
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("该存在却没有：%s（%v）", path, err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("该没有了却还在：%s", path)
	}
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		return os.WriteFile(target, raw, mode)
	})
}
