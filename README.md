# gsetup-go（GUI-Setup）

给 gpm 分发包用的**图形安装器**：双击就能装，不用先进终端。它把界面上的选择（装哪一家
工具链 cot / tdp、装到哪个家、要不要写 PATH）拼成一条 `install.sh` / `install.cmd` / `install.ps1` 命令，
**先在窗口里只读地给你看一遍**，再用 [portable-pty](https://github.com/qiuzhanghua/portable-pty)
跑起来、实时显示输出（Unix 上 stderr 单独一根管道，是红字）。装完窗口自己停在"关闭"按钮上。

谁该出什么、验收标准怎么写，都在 [`docs/REQUIREMENTS.md`](docs/REQUIREMENTS.md)。

## 构建

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0   # 只需要装一次
wails build -clean                                          # 产物在 build/bin/
```

macOS 产出 `build/bin/GUI-Setup.app`，Windows 是 `GUI-Setup.exe`，Linux 是 `GUI-Setup`（裸
可执行文件）。前端产物（`frontend/dist` 与 `frontend/wailsjs`）都在仓库里，
`wails.json` 的 `frontend:build` 是空的，所以构建**不需要 npm**。

**Linux 上窗口起不来**，多半是缺 WebKitGTK（Wails 的运行时依赖，不是可选项）：

```sh
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev    # Ubuntu 24.04 起
wails build -clean -tags webkit2_41                           # 4.0 那一支在新镜像上已经没有了
```

## 开发与测试

```sh
go vet ./...
go test ./...                      # 五个内部包 + main 包
```

代码结构：

| 位置 | 干什么 |
| --- | --- |
| `main.go` / `app.go` | Wails 的启动与绑定：把界面上选的东西（哪家工具链、装到哪个家、要不要写 PATH）拼成一条 `install.sh` / `install.cmd` / `install.ps1` 命令行；Windows 上探一次 PowerShell（`pwsh` 7 优先），被策略拦就退回 `install.cmd` |
| `internal/pkgroot/` | 从自己的可执行文件往上最多 8 层找"恰好一份" `<简称>-manifest.yaml`；认得出 App Translocation 与 Windows 压缩包预览那两种路径 |
| `internal/wizard/` | 三平台的路径与命令拼接（`Sep` / `Join` / `Entry.Script` 都按**目标** goos 算，不按宿主）；`PickEntry` 是"跑哪条入口"的唯一真相（Windows：`pwsh` 7 > `powershell.exe` 5.1 > `install.cmd`） |
| `internal/runner/` | 用 portable-pty 跑那条命令、把输出实时送回窗口（Unix 上 stderr 单独一根管道） |
| `internal/manifest/` `internal/ledger/` | 读清单、读账本（"这个家里装过什么"就靠它） |
| `frontend/dist/` | **手写的**界面（`index.html` / `main.js` / `style.css`），不是构建产物 |
| `frontend/wailsjs/` | Wails 生成的绑定，提交进仓库；给 Go 侧加了新方法就顺手补这里 |
| `reveal_*.go` | 三平台"在文件管理器里显示这个文件" |

`e2e_test.go` 是**真机端到端**：拿一个真的解压出来的包（含真的 gpm、真的 `install.sh`、真的
`payload`）装一遍、再卸一遍。默认跳过（要 pty、要几十秒、会真的往临时家里写东西）：

```sh
GSETUP_E2E_PKG=/tmp/fwd/pk go test -count=1 -run TestEndToEndRealPackage -v .
```

## 发版

打 tag 就发（`.github/workflows/release.yml`），也可以在 Actions 页面手动指定 tag：

```sh
# 1. 先把版本号抬上去：wails.json 的 info.productVersion —— 徽标 / 关于里显示的就是它
# 2. 提交，再打带注释的 tag，两个远端都推
git tag -a v1.2.0 -m "GUI-Setup v1.2.0：界面 B 的缺省工具链从 cot 改成 tdp"
git push gh main v1.2.0        # GitHub：CI 与 Release 都在这儿
git push gitee main v1.2.0     # Gitee：镜像，同样推 main 与 tag
```

（远端：`gh` = GitHub，`gitee` = Gitee 镜像；CI 与发 Release 都在 GitHub，但两边都推。）

工作流开头会拿 tag 和 `wails.json` 的 `info.productVersion` 对一下，对不上直接报错，
所以"忘了抬版本号"这件事只会失败，不会发出一个名不副实的包。

Wails 是 CGO + 系统 webview，六个平台得各自原生编一份（gpm 那种一个 runner 编六份在
这儿不成立），矩阵就是六条腿：`macos-14`、`macos-26-intel`、`ubuntu-latest`、
`ubuntu-24.04-arm`、`windows-latest`、`windows-11-arm`。产物：

| 资产 | 说明 |
| --- | --- |
| `GUI-Setup-<版本>-darwin-<arch>.zip` | `ditto` 打的，里面是 `GUI-Setup.app` |
| `GUI-Setup-<版本>-windows-<arch>.exe` | |
| `GUI-Setup-<版本>-linux-<arch>` | 裸可执行文件，带 x 位 |

ai-desk 那边就是下这里的现成资产（按 `GSETUP_REF` 钉 tag 取对应的 zip / exe / 裸文件），
不再 checkout 本仓就地编 —— 装进用户包里的，必须就是这里发的那个二进制。

## CI

进 main 与每个 PR 走一遍 vet + test（`.github/workflows/ci.yml`），三个平台各自原生跑：
`ubuntu-latest`、`macos-latest`、`windows-latest`（Wails 是 CGO + 系统 webview，三套都得
真在那儿才编得过；Linux 还要 `libgtk-3-dev` 与 `libwebkit2gtk-4.1-dev`，并带上
`webkit2_41`）。这里**不**做六平台交叉编译 —— 那件事由 release.yml 的六条腿管。

测试本身也按平台写：`#!/bin/sh` 那几条 pty 用例在 Windows 上跳过（那边跑的是
`install.cmd` / `cmd.exe`），另有两条 Windows 专属用例：一条真起 `cmd.exe` 验退出码，一条真起
PowerShell（`pwsh` 7 优先，其次 `powershell.exe`）跑 `install.ps1`，验参数原样转交与退出码。

## 找不着包的时候

它从自己的可执行文件所在目录起，**往上最多 8 层**找一个"恰好一份"的
`<简称>-manifest.yaml`（这一层有两份就报错：说不清该用哪一份）。某一层读不了（没权限、
或者只读映像上 `readdir` 报 ERANGE）不算致命，记下来接着往上；一路没找着才说"没找到"，
并捎上哪几层没读成 —— 用户该看到的是"没找到包"，不是一句 errno。

macOS 上有个专门的坑：**App Translocation**。带 `com.apple.quarantine`（下载标记）的
`.app` 被双击时，Gatekeeper 会把整个 bundle 复制进一个只读临时目录再运行，于是路径变成
`…/T/AppTranslocation/<UUID>/d/GUI-Setup.app/Contents/MacOS/GUI-Setup` —— 在那儿**看不见**
用户解压出来的 `install.sh` 与清单，往上找是白找。认得出这种情况时错误页会说清是系统干的，
并给两条路：

- 点**「自己选包目录…」**，挑回解压出来的那一层（`pkgroot.Manifests` 验过恰好一份才收）；
- 或者先在终端里 `xattr -dr com.apple.quarantine <解压出来的目录>` 再双击。

Windows 上另有一个坑，而且更常见：**在压缩包预览里直接双击 `GUI-Setup.exe`**。资源管理器对
压缩包只做单文件临时解压 —— 把这一个文件解到 `%TEMP%\Temp<N>_<包名>.zip\` 再启动，进程退出后
连那份临时目录都一并清理，所以**用户根本看不到自己被解到哪儿去了**；它身边既没有清单也没有
`install.cmd` / `install.ps1`，往上找同样是白找。v1.0.2 起认得出这种路径（`Temp<N>_….zip` 那一段），错误页会
直接说：请右键那个 zip →「全部解压缩…」，在解出来的文件夹里再双击（那儿 `GUI-Setup.exe` 与
`ad-manifest.yaml`、`install.cmd` / `install.ps1` 并排）。macOS 的 App Translocation 是**系统**干的，这一条是
**资源管理器**干的 —— 现象一样，责任方完全不同。

直接跑 `./install.sh`（Windows：`install.cmd` 或 `install.ps1`）那条命令行路径一直不受影响。给自己调试用的
后门是环境变量 `GSETUP_ROOT=<包根>`。

## 它怎么进到用户的包里

它不落家、不进账本 —— 它是"安装过程"本身。gpm-go 的清单为此多了一段 `setup:`（与
`entry:` 平行，但路径相对**包根**），`gpm pack --setup <路径>` 把这份 GUI-Setup 放在 zip
**顶层**、与三条入口脚本并排（gpm v0.6.4 = 设计文档 v3.13 的 D43、FR-32）。
ai-desk 的 `tools/package.sh -s` 就是喂给它的那个参数；契约细节见 gpm-go 的
[`docs/PACKAGE-FORMAT.md`](https://github.com/qiuzhanghua/gpm-go/blob/main/docs/PACKAGE-FORMAT.md)。

## 文档

| 文件 | 是什么 |
| --- | --- |
| [`docs/REQUIREMENTS.md`](docs/REQUIREMENTS.md) | 需求与验收：定位与边界、分发形态（`setup:` 段）、三块界面的流程、"怎么执行"（调谁、烘进去的默认值、pty 是硬要求） |
| [gpm-go 的 `docs/PACKAGE-FORMAT.md`](https://github.com/qiuzhanghua/gpm-go/blob/main/docs/PACKAGE-FORMAT.md) | 分发包与 `setup:` 的**契约**（上文那些 v3.13 / D43 / FR-32 都出自这里） |
| [gpm-go 的 README](https://github.com/qiuzhanghua/gpm-go) | 上游：分发包长什么样、`install.sh` / `gpm` 各自干什么 |

## 许可证

MIT，© 太极计算机股份有限公司创新研究院。见 [`LICENSE`](LICENSE)。
