// Package runner 在真 pty 里跑一条命令，把它的输出一行一行搬出来。
//
// 为什么非得是 pty（用户裁决）：gpm 看见的是终端，彩色、进度、"y/N"这些
// 行为跟人手敲一模一样；用户也就能在窗口里看见安装的过程和问题。
//
// 流的处理如实反映平台差异（需求 §5）：
//   - macOS / Linux：调用方自己设的 cmd.Stderr 会被 portable-pty 尊重，
//     所以 stderr 单独走一根管道 → 只有它标红；
//   - Windows：ConPTY 三条流全由伪控制台供给，调用方设的会被忽略 →
//     stdout / stderr 合流，窗口那边只能整段同色。
package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	pty "github.com/qiuzhanghua/portable-pty"
)

// 流名字。
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
	StreamSystem = "system" // 我们自己加的说明（不是子进程输出）
)

// drainGrace 是"子进程已经退了，但输出通道还没关"时最多再等多久。
const drainGrace = 3 * time.Second

// Event 是一行输出（或我们的一句说明）。
type Event struct {
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

// Options 是跑一条命令要的东西。
type Options struct {
	Dir     string   // 工作目录（包根；也是 install.sh 里 cd 的那个地方）
	Command string   // 可执行文件
	Args    []string // 参数
	Env     []string // nil = 用 portable-pty 给的那份环境快照
	Emit    func(Event)
}

// Result 是运行结果。
type Result struct {
	Code    int    `json:"code"`
	Signal  string `json:"signal,omitempty"`
	Success bool   `json:"success"`
	Err     string `json:"error,omitempty"`
}

// Message 是给用户看的一句话。
func (r Result) Message() string {
	switch {
	case r.Err != "":
		return r.Err
	case r.Signal != "":
		return "被信号 " + r.Signal + " 终止了"
	case r.Code == 0:
		return "成功"
	default:
		return fmt.Sprintf("失败（退出码 %d）", r.Code)
	}
}

// Run 跑一条命令，阻塞到它结束。
func Run(opt Options) Result {
	if opt.Emit == nil {
		opt.Emit = func(Event) {}
	}

	m, err := pty.Native().OpenPty(pty.DefaultSize)
	if err != nil {
		return Result{Err: "打不开伪终端：" + err.Error()}
	}

	cmd := pty.Command(opt.Command, opt.Args...)
	if opt.Dir != "" {
		cmd.Dir = opt.Dir
	}
	if opt.Env != nil {
		cmd.Env = opt.Env
	}

	out := &lineSink{stream: StreamStdout, emit: opt.Emit}
	errOut := &lineSink{stream: StreamStderr, emit: opt.Emit}

	// Unix：给 stderr 单独一根管道。父进程 spawn 之后必须把自己的写端关掉，
	// 不然子进程退出后读端也等不到 EOF。
	var stderrRead, stderrWrite *os.File
	if runtime.GOOS != "windows" {
		if pr, pw, perr := os.Pipe(); perr == nil {
			cmd.Stderr = pw
			stderrRead, stderrWrite = pr, pw
		}
	}

	child, err := m.Spawn(cmd)
	if stderrWrite != nil {
		_ = stderrWrite.Close()
	}
	if err != nil {
		_ = m.Close()
		return Result{Err: "起不来 " + opt.Command + "：" + err.Error()}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(out, m)
	}()
	if stderrRead != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = io.Copy(errOut, stderrRead)
		}()
	}

	status, waitErr := child.Wait()

	// 子进程走了，把主端关掉让读的那一头收工；万一还有孙进程攥着不放，
	// 只再等一小会儿，不把窗口挂死。
	_ = m.Close()
	drained := make(chan struct{})
	go func() { wg.Wait(); close(drained) }()
	select {
	case <-drained:
	case <-time.After(drainGrace):
		opt.Emit(Event{Stream: StreamSystem, Text: "（输出通道没在 3 秒内关闭，剩下的就不等了）"})
	}
	out.Flush()
	errOut.Flush()

	res := Result{Code: status.Code, Signal: status.Signal, Success: status.Success()}
	if waitErr != nil && res.Code == 0 && res.Signal == "" {
		res.Err = "等子进程结束时出错：" + waitErr.Error()
	}
	return res
}

// lineSink 把字节流切成一行一行往外送；pty 的 CRLF 在这里归一成 LF。
//
// 切行之前先剥掉控制序列（见 escape.go）：ConPTY 的会话握手、cmd.exe 的标题
// OSC、清屏与光标这些都不是"内容"，而窗口那边只是把文本塞进 <pre>、不解释
// ANSI —— 原样送过去，用户看到的就是一行乱码。剥在这里而不是前端：序列可能被
// 多次 Read 切开，而且剥完事件里就只剩内容了（前端一行都不用改）。
type lineSink struct {
	stream string
	buf    []byte
	strip  stripper
	emit   func(Event)
}

func (s *lineSink) Write(p []byte) (int, error) {
	s.buf = append(s.buf, s.strip.strip(p)...)
	for {
		i := bytes.IndexByte(s.buf, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSuffix(string(s.buf[:i]), "\r")
		s.buf = s.buf[i+1:]
		if line != "" {
			s.emit(Event{Stream: s.stream, Text: line})
		}
	}
	return len(p), nil
}

func (s *lineSink) Flush() {
	if len(s.buf) == 0 {
		return
	}
	line := strings.TrimRight(string(s.buf), "\r\n")
	s.buf = nil
	if line != "" {
		s.emit(Event{Stream: s.stream, Text: line})
	}
}
