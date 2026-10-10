package pkgroot

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 这两条用例靠 chmod 000 造出"读不了的目录"：Windows 上 chmod 对目录没有这个效果，
// root 又什么都不怕 —— 两种情形都跳过，别让它们假装测过了。
func skipIfChmodIsMeaningless(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上 chmod 000 不会让目录读不了")
	}
	if os.Geteuid() == 0 {
		t.Skip("root 眼里没有读不了的目录")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 包根 = 放着清单的那一层；从 .app 里往外爬靠的就是这个。
func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ad-manifest.yaml"), "id: ai-desk\n")
	deep := filepath.Join(root, "payload", "AI Desk.app", "Contents", "MacOS")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	got, name, err := Find(deep)
	if err != nil {
		t.Fatal(err)
	}
	if got != root || name != "ad-manifest.yaml" {
		t.Fatalf("Find = (%s, %s)，想要 (%s, ad-manifest.yaml)", got, name, root)
	}
}

// 同一层两份清单是错误：说不清该用哪一份，宁可说清楚。
func TestFindRefusesTwoManifests(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ad-manifest.yaml"), "id: ai-desk\n")
	writeFile(t, filepath.Join(root, "ac-manifest.yaml"), "id: ai-code\n")

	if _, _, err := Find(root); err == nil {
		t.Fatal("两份清单该报错")
	}
}

func TestFindNotFound(t *testing.T) {
	root := t.TempDir()
	_, _, err := Find(root)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("想要 ErrNotFound，得到 %v", err)
	}
}

// 往上爬的时候某一层读不了（EACCES / ERANGE 之类）不该一票否决：继续往上找，
// 找着了就用；真没找着才报错，并且说清楚哪几层没读成。
func TestFindSkipsUnreadableDirectory(t *testing.T) {
	skipIfChmodIsMeaningless(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ad-manifest.yaml"), "id: ai-desk\n")
	blocked := filepath.Join(root, "blocked")
	deep := filepath.Join(blocked, "inside", "GUI-Setup.app", "Contents", "MacOS")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	got, name, err := Find(deep)
	if err != nil {
		t.Fatal(err)
	}
	if got != root || name != "ad-manifest.yaml" {
		t.Fatalf("Find = (%s, %s)，想要 (%s, ad-manifest.yaml)", got, name, root)
	}
}

// 一路爬到头都没有清单：报的还是 ErrNotFound，但捎上"哪几层没读成"。
func TestFindReportsWhatItCouldNotRead(t *testing.T) {
	skipIfChmodIsMeaningless(t)
	empty := t.TempDir()
	blocked := filepath.Join(empty, "blocked")
	deep := filepath.Join(blocked, "inside")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	_, _, err := Find(deep)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("想要 ErrNotFound，得到 %v", err)
	}
	if !strings.Contains(err.Error(), "没读成") || !strings.Contains(err.Error(), blocked) {
		t.Fatalf("该说清哪一层没读成，得到 %v", err)
	}
}

// App Translocation：macOS 把下载来的 app 挪进只读临时目录时，exe 路径里带这个记号。
func TestLooksTranslocated(t *testing.T) {
	yes := "/private/var/folders/cc/xx/T/AppTranslocation/2ED1/d/GUI-Setup.app/Contents/MacOS/GUI-Setup"
	if !LooksTranslocated(yes) {
		t.Fatalf("%s 该认出来", yes)
	}
	for _, no := range []string{
		"/Users/q/Downloads/pkg/GUI-Setup.app/Contents/MacOS/GUI-Setup",
		"/opt/AppTranslocationSucks/x",
	} {
		if LooksTranslocated(no) {
			t.Fatalf("%s 不该认成被挪走", no)
		}
	}
}

// 后缀一样但名字就是后缀本身的不算（免得把 -manifest.yaml 当简称）。
func TestManifestsIgnoresBareSuffix(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, Suffix), "id: x\n")
	names, err := Manifests(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("不该认 %s，得到 %v", Suffix, names)
	}
}

// GSETUP_ROOT 是给自己调试用的后门。
func TestFindFromExecutableHonorsOverride(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ad-manifest.yaml"), "id: ai-desk\n")
	t.Setenv(EnvOverride, root)

	got, name, err := FindFromExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if got != root || name != "ad-manifest.yaml" {
		t.Fatalf("FindFromExecutable = (%s, %s)", got, name)
	}
}

func TestFindFromExecutableOverrideNeedsExactlyOne(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvOverride, root)
	if _, _, err := FindFromExecutable(); err == nil {
		t.Fatal("GSETUP_ROOT 指着一个没有清单的目录该报错")
	}
}
