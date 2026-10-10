package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = `{
  "schemaVersion": 1,
  "self": "/tmp/home/cot/bin/gpm",
  "packages": [
    {
      "id": "ai-desk",
      "name": "AI Desk",
      "version": "0.3.1",
      "platform": "darwin_arm64",
      "installedAt": "2026-10-10T00:00:00Z",
      "dir": "/tmp/home/cot/AI Desk.app",
      "entry": "AI Desk.app",
      "entryKind": "bundle",
      "cmd": "ad",
      "launcher": "/tmp/home/cot/bin/ad",
      "links": [{"path": "/tmp/home/Applications/AI Desk.app", "target": "/tmp/home/cot/AI Desk.app", "kind": "app-symlink"}],
      "pathEdits": [{"path": "/tmp/home/.zshrc", "created": false}],
      "verified": true
    }
  ]
}`

// 名字规则：<家>/<家目录名>-state.json（gpm v3.7 起）。
func TestFileNameAndPath(t *testing.T) {
	home := filepath.Join("/tmp", "cot")
	if got := FileName(home); got != "cot-state.json" {
		t.Fatalf("FileName = %s", got)
	}
	if got := Path(home); got != filepath.Join(home, "cot-state.json") {
		t.Fatalf("Path = %s", got)
	}
}

// "这个家还没装过东西"不是错误。
func TestLoadMissingIsNotAnError(t *testing.T) {
	led, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("不该报错，得到 %v", err)
	}
	if led != nil {
		t.Fatalf("没有账本该返回 nil，得到 %+v", led)
	}
}

func TestLoadAndFind(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(Path(home), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	led, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if led == nil || led.SchemaVersion != 1 {
		t.Fatalf("账本没读出来：%+v", led)
	}
	pkg := led.Find("ai-desk")
	if pkg == nil {
		t.Fatal("没找着 ai-desk")
	}
	if pkg.Version != "0.3.1" || pkg.Cmd != "ad" || pkg.EntryKind != "bundle" {
		t.Fatalf("字段对不上：%+v", pkg)
	}
	if !pkg.Verified {
		t.Fatal("verified 该是 true")
	}
	if got := pkg.PathEditFiles(); len(got) != 1 || got[0] != "/tmp/home/.zshrc" {
		t.Fatalf("PathEditFiles = %v", got)
	}
	if got := pkg.LinkPaths(); len(got) != 1 || got[0] != "/tmp/home/Applications/AI Desk.app" {
		t.Fatalf("LinkPaths = %v", got)
	}
	if led.Find("nope") != nil {
		t.Fatal("不存在的 id 该返回 nil")
	}
}

// v3.6 及以前的名字还得能读。
func TestLoadLegacy(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(LegacyPath(home), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	led, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if led == nil || led.Find("ai-desk") == nil {
		t.Fatal("老名字的账本该照样读出来")
	}
}

// 新名字优先。
func TestNewNameWinsOverLegacy(t *testing.T) {
	home := t.TempDir()
	empty := `{"schemaVersion":1,"packages":[]}`
	both := `{"schemaVersion":1,"packages":[{"id":"ai-desk","version":"0.3.1"}]}`
	if err := os.WriteFile(LegacyPath(home), []byte(both), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(home), []byte(empty), 0o644); err != nil {
		t.Fatal(err)
	}
	led, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if led.Find("ai-desk") != nil {
		t.Fatal("两个都在时该读新名字那份")
	}
}

// nil 账本上找东西不能炸。
func TestFindOnNilLedger(t *testing.T) {
	var led *Ledger
	if led.Find("ai-desk") != nil {
		t.Fatal("nil 账本该返回 nil")
	}
}
