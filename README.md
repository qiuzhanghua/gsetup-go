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

## 它怎么进到用户的包里

它不落家、不进账本 —— 它是"安装过程"本身。gpm-go 的清单为此多了一段 `setup:`（与
`entry:` 平行，但路径相对**包根**），`gpm pack --setup <路径>` 把这份 GUI-Setup 放在 zip
**顶层**、与 `install.sh` 并排（gpm v0.6.4 = 设计文档 v3.13 的 D43、FR-32）。
ai-desk 的 `tools/package.sh -s` 就是喂给它的那个参数；契约细节见 gpm-go 的
[`docs/PACKAGE-FORMAT.md`](https://github.com/qiuzhanghua/gpm-go/blob/main/docs/PACKAGE-FORMAT.md)。
