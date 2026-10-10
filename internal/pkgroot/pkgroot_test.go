package pkgroot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
