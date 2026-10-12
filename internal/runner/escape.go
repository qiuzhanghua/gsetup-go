// 剥掉终端控制序列：GUI-Setup 的输出面板是普通 <pre>，不解释 ANSI，
// 原样显示出来就是用户眼里的"一行乱码"。
package runner

// escByte 与 belByte 是控制序列的两个关键字节。
const (
	escByte = 0x1b // ESC
	belByte = 0x07 // BEL：OSC 的一种收尾
)

// 序列长度上限：畸形输入下别把后面整段输出吞掉。
//
// CSI 正常的参数最多几十字节；OSC 里通常只有一条窗口标题（一个路径）。真碰上
// 超长的畸形序列就回到正常状态继续输出 —— 宁可漏出几个怪字符，也不能让用户的
// 安装日志被吃掉。
const (
	maxCSI = 64
	maxOSC = 4096
)

type stripState int

const (
	stGround stripState = iota // 正常文本
	stEsc                      // 刚读到 ESC，等下一个字节定性
	stCSI                      // ESC [ …
	stOSC                      // ESC ] … 到 BEL 或 ST（ESC \）
)

// stripper 从字节流里剥掉控制序列，并跨 Write 保留没读完的那半截。
//
// 为什么需要它：GUI-Setup 故意用真 pty 跑子进程（需求 §5），Windows 上就是
// ConPTY —— 会话建立与结束时会发 ESC[?9001h（Win32 Input Mode）、ESC[?1004h
// （焦点上报）、ESC[?25l/h（光标）、ESC[2J ESC[m ESC[H（初始化屏幕），cmd.exe
// 还会发 ESC]0;<自己的路径> BEL 去设窗口标题。这些都不是"内容"，而窗口那边只是
// 把文本塞进 <pre>，于是它们被当成普通字符显示出来。
//
// 为什么要状态机而不是正则：这些序列里**没有换行**，会和真实内容粘在同一行
// （实测 `HI\x1b]0;…\x07` 就是这样），而且会被 pty 的多次 Read 切开（实测标题
// 那一段是单独一批到的）—— 逐块替换必然漏出半截。
//
// 换 shell 不解决问题（实测三种 shell 在同一个 pty 里头尾序列逐字节相同，只有
// 标题文字变成各自的 exe 路径）：这些序列是 pty 层的握手，不是 cmd.exe 打的。
type stripper struct {
	state stripState
	esc   bool // OSC 里刚读到 ESC，等一个 '\' 收尾（ST）
	n     int  // 当前序列已读字节数（防畸形输入把输出吞光）
}

// strip 吃掉 p 里的控制序列，返回剩下的文本。返回值是新切片，不改 p。
func (s *stripper) strip(p []byte) []byte {
	out := make([]byte, 0, len(p))
	for _, b := range p {
		switch s.state {
		case stGround:
			if b == escByte {
				s.state, s.n = stEsc, 0
				continue
			}
			out = append(out, b)

		case stEsc:
			s.n++
			switch {
			case b == '[':
				s.state = stCSI
			case b == ']':
				s.state, s.esc = stOSC, false
			default:
				// 其余两字节序列（ESC ( B、ESC = 之类）：整个丢掉
				s.state = stGround
			}

		case stCSI:
			s.n++
			// 参数与中间字节（0x20–0x3F）全吃掉；最终字节（0x40–0x7E）收尾。
			if (b >= 0x40 && b <= 0x7e) || s.n > maxCSI {
				s.state = stGround
			}

		case stOSC:
			s.n++
			if s.esc {
				// 上一个字节是 ESC：这一个若是 '\' 就是 ST，序列到此为止。
				s.esc = false
				if b == '\\' {
					s.state = stGround
				}
				continue // OSC 里的字节永远不往外输出
			}
			switch {
			case b == belByte:
				s.state = stGround
			case b == escByte:
				s.esc = true
			case s.n > maxOSC:
				s.state = stGround
			}
		}
	}
	return out
}
