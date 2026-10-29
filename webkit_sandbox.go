package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// WebKitGTK 在 Linux 上用 bubblewrap 给 WebProcess 建沙箱，而 bwrap 需要
// 非特权 user namespace。Ubuntu 24.04 起默认用 AppArmor 限制它
// （kernel.apparmor_restrict_unprivileged_userns=1）：有 profile 的程序才放行。
// 我们的 GTK4（webkitgtk-6.0）构建没有 profile，bwrap 一失败 WebKit 直接
// abort（SIGTRAP），窗口都出不来。GTK3（webkit2gtk-4.1）则是静默降级成无沙箱。
//
// 处理：启动前自己跑一次 bwrap，失败就设 WebKit 的降级开关，让 GTK4 的行为
// 与 4.1 一致（继续启动、渲染进程无沙箱），并明确告警；如果调用方要求必须有
// 沙箱（GSETUP_REQUIRE_SANDBOX=1），则拒绝启动而不是悄悄降级。
const (
	// webkitSandboxEnv 是 WebKit 官方的降级开关（名字里的 DANGEROUS 是作者故意取的）。
	webkitSandboxEnv = "WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS"
	// strictSandboxEnv 设成 1 时，拿不到沙箱就拒绝启动。
	strictSandboxEnv = "GSETUP_REQUIRE_SANDBOX"
)

type sandboxAction int

const (
	sandboxKeep    sandboxAction = iota // 沙箱可用，什么都不做
	sandboxDisable                      // 沙箱不可用，设置降级开关继续启动
	sandboxRefuse                       // 沙箱不可用且要求严格，拒绝启动
)

// decideSandbox 是纯逻辑，平台与 bwrap 结果都由参数给，方便测试。
func decideSandbox(goos string, bwrapOK, strict bool) sandboxAction {
	if goos != "linux" {
		return sandboxKeep // macOS/Windows 不走 WebKitGTK 的 bwrap
	}
	if bwrapOK {
		return sandboxKeep
	}
	if strict {
		return sandboxRefuse
	}
	return sandboxDisable
}

// bwrapWorks 直接跑 WebKit 真正会跑的那一步；命令缺失、超时、非零退出都算不可用。
func bwrapWorks() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "bwrap", "--ro-bind", "/", "true").Run() == nil
}

// prepareWebKitSandbox 必须在 application.New 之前调用：WebKit 一初始化，
// 再改环境变量就来不及了。
func prepareWebKitSandbox() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	strict := os.Getenv(strictSandboxEnv) == "1"
	switch decideSandbox(runtime.GOOS, bwrapWorks(), strict) {
	case sandboxDisable:
		_ = os.Setenv(webkitSandboxEnv, "1")
		fmt.Fprintln(os.Stderr,
			"GUI-Setup: 本机限制了非特权 user namespace（bwrap 不可用），"+
				"WebKit 渲染沙箱用不了；已设置 "+webkitSandboxEnv+"=1 继续启动。"+
				"本程序只加载自带的本地页面。需要强制沙箱时设 GSETUP_REQUIRE_SANDBOX=1。")
	case sandboxRefuse:
		return fmt.Errorf("GUI-Setup: 本机无法启用 WebKit 渲染沙箱（bwrap 不可用），" +
			"而 GSETUP_REQUIRE_SANDBOX=1 要求必须有沙箱，拒绝启动。" +
			"可给程序装 AppArmor profile（需要 root-owned 路径），或取消该环境变量")
	}
	return nil
}
