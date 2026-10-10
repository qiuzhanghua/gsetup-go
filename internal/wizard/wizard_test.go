package wizard

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// 这一条钉的是"两个 --with 之前的那两个参数"：--dir 永远显式传。
func TestArgsAlwaysPassDirAndWith(t *testing.T) {
	c := Choice{Home: "/tmp/home/cot", Toolchain: "cot", AddToPath: true}
	want := []string{"--dir", "/tmp/home/cot", "--with", "cot", "--yes"}
	if got := c.Args(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Args() = %v，想要 %v", got, want)
	}
}

// 不勾"让终端里也能用"就是 --no-path，**不能什么都不传**：pty 里 gpm 会
// 判成交互终端然后停下来问（需求 §4.5）。
func TestArgsNoPathWhenUnchecked(t *testing.T) {
	c := Choice{Home: "/tmp/home/tdp", Toolchain: "tdp"}
	want := []string{"--dir", "/tmp/home/tdp", "--with", "tdp", "--no-path"}
	if got := c.Args(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Args() = %v，想要 %v", got, want)
	}
}

func TestScriptPerOS(t *testing.T) {
	if got := Script("/pkg", "darwin"); got != filepath.Join("/pkg", "install.sh") {
		t.Fatalf("darwin 该走 install.sh，得到 %s", got)
	}
	if got := Script("/pkg", "linux"); got != filepath.Join("/pkg", "install.sh") {
		t.Fatalf("linux 该走 install.sh，得到 %s", got)
	}
	if got := Script(`C:\pkg`, "windows"); got != `C:\pkg\install.cmd` {
		t.Fatalf("windows 该走 install.cmd，得到 %s", got)
	}
}

// Windows 上多包一层 cmd /c，末尾塞 < NUL：install.cmd 有一句 pause。
func TestArgvWindowsWrapsWithCmdAndNul(t *testing.T) {
	c := Choice{Home: `C:\Users\me\cot`, Toolchain: "cot"}
	prog, args := Argv(`C:\pkg`, c, "windows")
	if prog != "cmd.exe" {
		t.Fatalf("prog = %s，想要 cmd.exe", prog)
	}
	want := []string{"/c", `C:\pkg\install.cmd`, "--dir", `C:\Users\me\cot`, "--with", "cot", "--no-path", "<", "NUL"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %v，想要 %v", args, want)
	}
}

// Unix 上直接 exec 脚本本身（install.sh 里自己会 cd 到包根）。
func TestArgvUnixRunsScriptDirectly(t *testing.T) {
	c := Choice{Home: "/tmp/home/cot", Toolchain: "cot"}
	prog, args := Argv("/pkg", c, "darwin")
	if prog != filepath.Join("/pkg", "install.sh") {
		t.Fatalf("prog = %s", prog)
	}
	want := []string{"--dir", "/tmp/home/cot", "--with", "cot", "--no-path"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %v，想要 %v", args, want)
	}
}

// 给人照抄的那一行：家目录缩成 $HOME，路径带引号。
func TestDisplayAbbreviatesHome(t *testing.T) {
	userHome, err := os.UserHomeDir()
	if err != nil || userHome == "" {
		t.Skip("没有家目录")
	}
	c := Choice{Home: filepath.Join(userHome, "cot"), Toolchain: "cot", AddToPath: true}
	got := Display(userHome, c, "darwin")
	want := `./install.sh --dir "$HOME/cot" --with cot --yes`
	if got != want {
		t.Fatalf("Display() = %q，想要 %q", got, want)
	}
}

func TestDisplayWindows(t *testing.T) {
	c := Choice{Home: `/tmp/other/cot`, Toolchain: "cot", AddToPath: true}
	got := Display(`C:\pkg`, c, "windows")
	want := `install.cmd --dir /tmp/other/cot --with cot --yes < NUL`
	if got != want {
		t.Fatalf("Display() = %q，想要 %q", got, want)
	}
}

func TestAbbrevHome(t *testing.T) {
	userHome, err := os.UserHomeDir()
	if err != nil || userHome == "" {
		t.Skip("没有家目录")
	}
	cases := []struct {
		path string
		goos string
		want string
		ok   bool
	}{
		{userHome, "darwin", "$HOME", true},
		{userHome, "windows", "%USERPROFILE%", true},
		{filepath.Join(userHome, "cot"), "darwin", "$HOME/cot", true},
		{filepath.Join(userHome, "cot", "bin"), "darwin", "$HOME/cot/bin", true},
		{"/opt/somewhere", "darwin", "", false},
	}
	for _, tc := range cases {
		got, ok := AbbrevHome(tc.path, tc.goos)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("AbbrevHome(%q, %s) = (%q, %v)，想要 (%q, %v)", tc.path, tc.goos, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDefaultHome(t *testing.T) {
	if got := DefaultHome("/Users/me", "cot"); got != filepath.Join("/Users/me", "cot") {
		t.Fatalf("DefaultHome = %s", got)
	}
	if got := DefaultHome("", "tdp"); got != "tdp" {
		t.Fatalf("没有家目录时该退回简称，得到 %s", got)
	}
}

func TestUninstall(t *testing.T) {
	prog, args := UninstallArgv("/tmp/home/cot", "ai-desk", "darwin")
	if prog != filepath.Join("/tmp/home/cot", "bin", "gpm") {
		t.Fatalf("prog = %s", prog)
	}
	if !reflect.DeepEqual(args, []string{"uninstall", "ai-desk", "--yes"}) {
		t.Fatalf("args = %v", args)
	}
	if prog, _ := UninstallArgv(`C:\home\cot`, "ai-desk", "windows"); prog != `C:\home\cot\bin\gpm.exe` {
		t.Fatalf("windows 上的 gpm 该带 .exe，得到 %s", prog)
	}
	if got := UninstallDisplay("/tmp/home/cot", "ai-desk", "darwin"); got != "/tmp/home/cot/bin/gpm uninstall ai-desk --yes" {
		t.Fatalf("UninstallDisplay = %s", got)
	}
}

// 界面 A 靠它决定按钮写"升级"还是"已经是最新的"。
func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.3.0", "0.3.1", true},
		{"0.3.1", "0.3.1", false},
		{"0.3.1", "0.3.0", false},
		{"0.9.0", "0.10.0", true},
		{"v0.3.1", "0.3.2", true},
		{"v0.3.1", "0.3.1", false},
		{"0.3.1-dev", "0.3.1", false},
		{"1.0", "1.0.0", false},
	}
	for _, tc := range cases {
		if got := VersionLess(tc.a, tc.b); got != tc.want {
			t.Fatalf("VersionLess(%q, %q) = %v，想要 %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestKnownToolchain(t *testing.T) {
	if !KnownToolchain("cot") || !KnownToolchain("tdp") {
		t.Fatal("cot / tdp 该认")
	}
	if KnownToolchain("cargo") || KnownToolchain("") {
		t.Fatal("别的名字不该认")
	}
}
