// Package pkgroot 定位"包根"：用户解压出来的那个目录。
//
// 包根 = 放着唯一一份 <简称>-manifest.yaml 的目录。GUI-Setup 是随包跑的，
// 从自己的可执行文件所在目录起逐级向上找，找到哪儿哪儿就是包根——
// 不写死层数，因为 macOS 的 .app 里可执行文件在 Contents/MacOS/ 下。
package pkgroot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Suffix 是清单文件名的后缀（与 gpm 的 manifest.Suffix 一致）。
const Suffix = "-manifest.yaml"

// EnvOverride 设了它就直接把它当包根（调试与测试用）。
//
// 用法：GSETUP_ROOT=/tmp/asm ./GUI-Setup
const EnvOverride = "GSETUP_ROOT"

// maxDepth 是向上找的层数上限。macOS 的 .app 里可执行文件往上第 4 层才是包根。
const maxDepth = 8

// ErrNotFound 表示从起点一路向上都没找到清单。
//
// 不写 `<简称>` 那种占位符：简称是**从清单里读出来**的，还没找着清单时根本不知道
// 它是什么，印一个尖括号只会让人以为要自己替换。
var ErrNotFound = errors.New("没找到包根（从自己所在的位置往上，一路都没有找着 *-manifest.yaml）")

// TranslocatedMark 是 macOS 把下载来的 app 挪进只读临时目录后，路径里必然有的那一段。
//
// Gatekeeper 的 App Translocation：带 com.apple.quarantine 的 .app 被双击时，
// 系统会把整个 bundle 复制到一个只读映像里再运行，于是 os.Executable() 报的是
//
//	/private/var/folders/…/T/AppTranslocation/<UUID>/d/GUI-Setup.app/Contents/MacOS/GUI-Setup
//
// 而用户解压出来的 install.sh 与清单**不在**那儿 —— 从这个位置往上找是白找。
const TranslocatedMark = "/AppTranslocation/"

// LooksTranslocated 看这个路径是不是 App Translocation 那个只读临时目录里的。
func LooksTranslocated(path string) bool {
	return strings.Contains(filepath.ToSlash(path), TranslocatedMark)
}

// tempZipMark 是 Windows 资源管理器"在压缩包里直接双击"留下的临时目录名：
// Temp<N>_<压缩包名>.zip。
//
// 资源管理器对压缩包只做**单文件**临时解压：双击里面的 GUI-Setup.exe 时，它把这一个
// 文件解到 %TEMP%\Temp1_<压缩包名>.zip\ 再启动，进程退出后连那份临时目录都清掉。于是
// exe 身边既没有清单也没有 install.cmd，往上爬到 C:\ 也是白爬 —— 用户看到的还是
// "没找到包根"，而且**看不到自己被解到哪儿去了**。
var tempZipMark = regexp.MustCompile(`(?i)^Temp\d+_.+\.zip$`)

// LooksZipPreview 报告这个路径是不是资源管理器为"在压缩包里双击"临时解出来的那一份。
//
// 与 LooksTranslocated 对称，但责任方完全不同：那边是 macOS 的**系统**行为
// （App Translocation），这边是 Windows **资源管理器**解压单个文件的行为 —— 系统
// 没碰过用户的包。
//
// 只看路径里有没有 Temp<N>_….zip 这一段，不要求它落在 os.TempDir() 之下：有人会把
// TEMP 指到别处，而这段名字本身已经够特征（正常解压不会解出这种目录名）。
//
// 故意**不**做"只要在临时目录下就算"那种更宽的判断：Go 自己编译测试时把二进制放在
// `%TEMP%\go-build<N>\…`，那会让本仓库自己的用例在 Windows 上误判成"压缩包预览"。
func LooksZipPreview(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		if tempZipMark.MatchString(part) {
			return true
		}
	}
	return false
}

// Manifests 列出 dir 下所有 <简称>-manifest.yaml 的文件名（只列一层）。
func Manifests(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, Suffix) && len(name) > len(Suffix) {
			out = append(out, name)
		}
	}
	return out, nil
}

// Find 从 start 起逐级向上找，返回包根与清单文件名。
//
// 某一层里有两份清单是**错误**（和 gpm 的 Discover 一个口径）：说明这不是
// 我们解压出来的那个目录，宁可说清楚也不要随便挑一个。
//
// 某一层读不了（没权限、或者被 macOS 挪进只读映像之后 getdirentries 报 ERANGE）
// 不算致命：记下来接着往上找，一路上都没找着才把它捎在 ErrNotFound 后面 ——
// 用户看到的应该是"没找到"，而不是一句 readdir 的 errno。
func Find(start string) (root string, manifestName string, err error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", "", err
	}
	var skipped []string
	for i := 0; i < maxDepth; i++ {
		names, err := Manifests(dir)
		switch {
		case err != nil && !os.IsNotExist(err):
			skipped = append(skipped, fmt.Sprintf("%s（%v）", dir, err))
		case len(names) == 1:
			return dir, names[0], nil
		case len(names) > 1:
			return "", "", fmt.Errorf("%s 里有 %d 份清单（%s），说不清该用哪一份", dir, len(names), strings.Join(names, "、"))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if len(skipped) > 0 {
		return "", "", fmt.Errorf("%w；另外有 %d 层没读成：%s", ErrNotFound, len(skipped), strings.Join(skipped, "、"))
	}
	return "", "", ErrNotFound
}

// FindFromExecutable 是正式入口：先看 GSETUP_ROOT，再顺着自己的可执行文件往上找。
//
// 符号链接要解开：有人会把 GUI-Setup 链接到别处。
func FindFromExecutable() (root string, manifestName string, err error) {
	if v := strings.TrimSpace(os.Getenv(EnvOverride)); v != "" {
		dir, aerr := filepath.Abs(v)
		if aerr != nil {
			return "", "", aerr
		}
		names, merr := Manifests(dir)
		if merr != nil {
			return "", "", fmt.Errorf("%s=%s 读不了：%w", EnvOverride, v, merr)
		}
		if len(names) != 1 {
			return "", "", fmt.Errorf("%s=%s 里有 %d 份清单，要恰好一份", EnvOverride, v, len(names))
		}
		return dir, names[0], nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}
	return Find(filepath.Dir(exe))
}
