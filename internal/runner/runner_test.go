package runner

import (
	"runtime"
	"strings"
	"sync"
	"testing"

	pty "github.com/qiuzhanghua/portable-pty"
)

// requirePty 在打不开伪终端的机器上跳过——沙箱里 /dev/ptmx 常常不许开，
// 那不是这个包的错（真机上照样跑）。
func requirePty(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("这个包的用例只跑 Unix 的 /bin/sh")
	}
	m, err := pty.Native().OpenPty(pty.DefaultSize)
	if err != nil {
		t.Skipf("这台机器打不开伪终端：%v", err)
	}
	_ = m.Close()
}

func collect(t *testing.T, script string) (Result, []Event) {
	t.Helper()
	requirePty(t)
	var mu sync.Mutex
	var events []Event
	res := Run(Options{
		Dir:     t.TempDir(),
		Command: "/bin/sh",
		Args:    []string{"-c", script},
		Emit: func(ev Event) {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		},
	})
	return res, events
}

func lines(events []Event, stream string) []string {
	var out []string
	for _, ev := range events {
		if ev.Stream == stream {
			out = append(out, ev.Text)
		}
	}
	return out
}

func TestRunCapturesStdoutLineByLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("这个用例只跑 Unix 的 /bin/sh")
	}
	res, events := collect(t, `echo one; echo two; printf 'tail-without-newline'`)
	if !res.Success || res.Code != 0 {
		t.Fatalf("该成功，得到 %+v", res)
	}
	got := lines(events, StreamStdout)
	want := []string{"one", "two", "tail-without-newline"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("stdout 行 = %v，想要 %v", got, want)
	}
}

func TestRunReportsNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("这个用例只跑 Unix 的 /bin/sh")
	}
	res, _ := collect(t, `echo boom 1>&2; exit 3`)
	if res.Success {
		t.Fatal("退出码 3 不该算成功")
	}
	if res.Code != 3 {
		t.Fatalf("退出码该是 3，得到 %+v", res)
	}
	if res.Message() != "失败（退出码 3）" {
		t.Fatalf("Message = %q", res.Message())
	}
}

// Unix 上 stderr 是单独一根管道（Windows 的 ConPTY 会把三条流合起来）。
func TestRunSeparatesStderrOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ConPTY 下 stdout / stderr 合流，这是平台事实（需求 §5）")
	}
	res, events := collect(t, `echo to-out; echo to-err 1>&2`)
	if !res.Success {
		t.Fatalf("该成功，得到 %+v", res)
	}
	if got := lines(events, StreamStdout); len(got) != 1 || got[0] != "to-out" {
		t.Fatalf("stdout = %v", got)
	}
	if got := lines(events, StreamStderr); len(got) != 1 || got[0] != "to-err" {
		t.Fatalf("stderr = %v", got)
	}
}

// 起不来的命令不能把窗口挂死。
func TestRunBadCommand(t *testing.T) {
	requirePty(t)
	res := Run(Options{Dir: t.TempDir(), Command: "/definitely/not/here", Args: nil})
	if res.Success || res.Err == "" {
		t.Fatalf("该报错，得到 %+v", res)
	}
}

// 空行不出事件：gpm 的输出里空行多，全搬过去窗口会很吵。
func TestEmptyLinesAreSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("这个用例只跑 Unix 的 /bin/sh")
	}
	_, events := collect(t, `echo a; echo; echo b`)
	if got := lines(events, StreamStdout); len(got) != 2 {
		t.Fatalf("空行该被跳过，得到 %v", got)
	}
}
