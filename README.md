# gsetup-go（GUI-Setup）

给 gpm 分发包用的**图形安装器**：双击就能装，不用先进终端。它把界面上的选择（装哪一家
工具链 cot / tdp、装到哪个家、要不要写 PATH）拼成一条 `install.sh` / `install.cmd` 命令，
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

## 发版

打 tag 就发（`.github/workflows/release.yml`），也可以在 Actions 页面手动指定 tag：

```sh
# 1. 先把版本号抬上去：wails.json 的 info.productVersion —— 徽标 / 关于里显示的就是它
# 2. 提交，再打带注释的 tag 推上去
git tag -a v1.1.0 -m "GUI-Setup v1.1.0：……"
git push gh main v1.1.0
```

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

ai-desk 那边目前是**就地编**（checkout `.gsetup-go`，按 `GSETUP_REF` 钉 tag），还没用这些
现成资产；将来想省那几分钟，可以改成下载这里的 zip / exe。

## 它怎么进到用户的包里

它不落家、不进账本 —— 它是"安装过程"本身。gpm-go 的清单为此多了一段 `setup:`（与
`entry:` 平行，但路径相对**包根**），`gpm pack --setup <路径>` 把这份 GUI-Setup 放在 zip
**顶层**、与 `install.sh` 并排（gpm v0.6.4 = 设计文档 v3.13 的 D43、FR-32）。
ai-desk 的 `tools/package.sh -s` 就是喂给它的那个参数；契约细节见 gpm-go 的
[`docs/PACKAGE-FORMAT.md`](https://github.com/qiuzhanghua/gpm-go/blob/main/docs/PACKAGE-FORMAT.md)。
