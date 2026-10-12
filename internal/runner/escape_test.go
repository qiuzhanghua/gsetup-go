package runner

import (
	"runtime"
	"strings"
	"testing"
)

// conptyNoise 是探针实测抓到的形状：ConPTY 握手 + cmd.exe 的标题 OSC + 真实内容。
// （把 ESC 写成 \x1b，BEL 写成 \x07 —— 两者在正常字体里都不可见，所以用户看到的是
// 一行"乱码"。）
const conptyNoise = "\x1b[?9001h\x1b[?1004h\x1b[?25l\x1b[2J\x1b[m\x1b[HHIHI" +
	"\x1b]0;C:\\Windows\\system32\\cmd.exe\x07\x1b[?25h\x1b[?9001l\x1b[?1004l"

func TestStripRemovesControlSequences(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ConPTY 握手 + 标题 OSC + 内容", conptyNoise, "HIHI"},
		{"OSC 用 ST（ESC \\）收尾", "A\x1b]0;title\x1b\\B", "AB"},
		// 两字节序列连"参数字节"一起吃掉：ESC ( B 与 ESC = 各占两个字节，
		// 所以两头那两个 B 是内容，中间那个 B 属于 ESC = 被吃掉。
		{"两个两字节序列", "A\x1b(B\x1b=B", "ABB"},
		{"裸的两字节序列（ESC B）", "A\x1bB", "A"},
		{"SGR 颜色", "\x1b[32;1m绿\x1b[m", "绿"},
		{"内容里的方括号不受影响", "echo [ok] 你好", "echo [ok] 你好"},
		{"纯文本原样", "装好了\n", "装好了\n"},
	}
	for _, c := range cases {
		var s stripper
		if got := string(s.strip([]byte(c.in))); got != c.want {
			t.Errorf("%s：strip(%q) = %q，想要 %q", c.name, c.in, got, c.want)
		}
	}
}

// 最容易漏的一种：序列正好被 pty 的多次 Read 切开。逐个字节喂，结果必须和
// 一次性喂完全一样（标题那一段实测就是单独一批到的）。
func TestStripSurvivesEverySplitPoint(t *testing.T) {
	for cut := 0; cut <= len(conptyNoise); cut++ {
		var s stripper
		got := string(s.strip([]byte(conptyNoise[:cut]))) + string(s.strip([]byte(conptyNoise[cut:])))
		if got != "HIHI" {
			t.Fatalf("在第 %d 字节切开后得到 %q，想要 %q", cut, got, "HIHI")
		}
	}
}

// 畸形输入：一个永远不收尾的 CSI 不能把后面的输出吞掉（上限之后回到正常状态）。
func TestStripGivesUpOnMalformedSequence(t *testing.T) {
	var s stripper
	in := "\x1b[" + strings.Repeat("x", maxCSI+10) + "OK"
	got := string(s.strip([]byte(in)))
	if !strings.Contains(got, "OK") {
		t.Fatalf("畸形序列后面那点内容不该被吞掉，得到 %q", got)
	}
}

// lineSink 一级：剥完再切行，粘在内容里的序列不会跑进事件，纯序列的行也不会
// 变成一行"空事件"。
func TestLineSinkStripsBeforeSplitting(t *testing.T) {
	var events []Event
	sink := &lineSink{stream: StreamStdout, emit: func(ev Event) { events = append(events, ev) }}

	// 故意切在序列中间，模仿 pty 的分批读
	for _, chunk := range []string{"A\x1b[?25", "l\x1b]0;ti", "tle\x07B\n", "\x1b[2J\x1b[H\n", "C\n"} {
		if _, err := sink.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	sink.Flush()

	var got []string
	for _, ev := range events {
		got = append(got, ev.Text)
	}
	want := []string{"AB", "C"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("事件 = %v，想要 %v", got, want)
	}
}

// Windows 上真起一个 cmd.exe：ConPTY 的握手与 cmd 的标题都不该进事件，而内容
// 必须还在（这条是探针的"回归版"，只在 ConPTY 存在的平台上跑）。
func TestRunStripsConPtyNoiseOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("这条只在 Windows 上有 ConPTY")
	}
	var events []Event
	res := Run(Options{
		Dir:     t.TempDir(),
		Command: "cmd.exe",
		Args:    []string{"/c", "echo HI"},
		Emit:    func(ev Event) { events = append(events, ev) },
	})
	if strings.Contains(res.Err, "伪终端") {
		t.Skipf("这台机器开不了 ConPTY：%v", res.Err)
	}
	if !res.Success {
		t.Fatalf("该成功，得到 %+v", res)
	}
	var b strings.Builder
	for _, ev := range events {
		b.WriteString(ev.Text)
		b.WriteByte('\n')
	}
	joined := b.String()
	if strings.ContainsRune(joined, escByte) || strings.ContainsRune(joined, belByte) {
		t.Fatalf("控制序列没剥干净：%q", joined)
	}
	if !strings.Contains(joined, "HI") {
		t.Fatalf("真实内容丢了：%q", joined)
	}
}
