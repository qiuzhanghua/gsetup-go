package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const sample = `id: ai-desk
name: AI Desk
version: 0.3.1
requires: [cot, tdp, node]
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

func parse(t *testing.T, body string) *Manifest {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ad-manifest.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestParseBasics(t *testing.T) {
	m := parse(t, sample)
	if m.ID != "ai-desk" || m.Name != "AI Desk" || m.Version != "0.3.1" {
		t.Fatalf("基本字段对不上：%+v", m)
	}
	if m.Short != "ad" {
		t.Fatalf("简称该从文件名里来，得到 %q", m.Short)
	}
	if m.Launch.Cmd != "ad" || m.Launch.Mode != "activate" {
		t.Fatalf("launch 对不上：%+v", m.Launch)
	}
}

// cmd 不写就跟着简称走（和 gpm 一个口径）。
func TestParseDefaultsCmdToShort(t *testing.T) {
	m := parse(t, "id: ai-desk\nname: AI Desk\nversion: 0.3.1\n")
	if m.Launch.Cmd != "ad" {
		t.Fatalf("Launch.Cmd 该缺省成 ad，得到 %q", m.Launch.Cmd)
	}
}

func TestParseWithoutIDFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ad-manifest.yaml")
	if err := os.WriteFile(path, []byte("name: AI Desk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(path); err == nil {
		t.Fatal("没有 id 该报错")
	}
}

// 只认 cot / tdp，别的 requires 当没看见——但顺序要保住。
func TestToolchainsFiltersAndKeepsOrder(t *testing.T) {
	m := parse(t, sample)
	if got := m.Toolchains(); !reflect.DeepEqual(got, []string{"cot", "tdp"}) {
		t.Fatalf("Toolchains = %v", got)
	}
	if got := m.DefaultToolchain(); got != "cot" {
		t.Fatalf("DefaultToolchain = %s，想要 cot", got)
	}
}

func TestDefaultToolchainWhenNoRequires(t *testing.T) {
	m := parse(t, "id: x\nname: X\nversion: 1.0.0\n")
	if got := m.DefaultToolchain(); got != "" {
		t.Fatalf("没有 requires 时该返回空串，得到 %q", got)
	}
	if got := m.Toolchains(); len(got) != 0 {
		t.Fatalf("Toolchains 该是空的，得到 %v", got)
	}
}

// setup: 与 entry: 平行，按 GOOS 分键——GUI-Setup 自己就是这么被声明的。
func TestSetupEntryPerOS(t *testing.T) {
	m := parse(t, sample)
	if got := m.SetupEntry("darwin").Rel(); got != "GUI-Setup.app" {
		t.Fatalf("darwin 的 setup 该是 GUI-Setup.app，得到 %q", got)
	}
	if got := m.SetupEntry("darwin").Kind(); got != "bundle" {
		t.Fatalf("darwin 的 setup 该是 bundle，得到 %q", got)
	}
	if got := m.SetupEntry("windows").Rel(); got != "GUI-Setup.exe" {
		t.Fatalf("windows 的 setup 该是 GUI-Setup.exe，得到 %q", got)
	}
	if got := m.SetupEntry("freebsd").Rel(); got != "" {
		t.Fatalf("没有的平台上该给空 Entry，得到 %q", got)
	}
}

func TestSetupEntryWhenNoSetupSection(t *testing.T) {
	m := parse(t, "id: x\nname: X\nversion: 1.0.0\n")
	if got := m.SetupEntry("darwin").Rel(); got != "" {
		t.Fatalf("没有 setup: 段时该给空 Entry，得到 %q", got)
	}
}

func TestEntryPerOS(t *testing.T) {
	m := parse(t, sample)
	if got := m.Entry["linux"]; got.Kind() != "exe" || got.Rel() != "ai-desk" {
		t.Fatalf("linux 的 entry 对不上：%+v", got)
	}
}

func TestShortFromName(t *testing.T) {
	if got := ShortFromName("/x/ad-manifest.yaml"); got != "ad" {
		t.Fatalf("ShortFromName = %q", got)
	}
}
