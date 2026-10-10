# gsetup-go 需求文档

> 本文 = **用户原稿（§0，一字未改）+ 讨论结论（§1 起）**。2026-10-10 与用户逐条过了一遍。
>
> 上游是 [`gpm-go`](https://github.com/qiuzhanghua/gpm-go)：**命令行那一半已经做完**
> （`gpm` + `install.sh` / `install.cmd` + 清单契约 + 账本 + PATH 集成 + 工具链自举）。
> 本项目只补**图形**那一半：让不碰终端的人也能把 GUI 程序装好。
> 下面凡引号里的句子都是用户原话，出处写在句子里。

## 0. 原稿（一字未改）

```
# 用途

github.com/qiuzhanghua/gpm-go已经做好命令行安装GUI程序到Linux/Windows/Mac的功能。

本项目(GUI-SETUP)补足GUI安装的部分，编译成的执行文件名字为GUI-Setup, 该文件被打包到ai-desk中。

# 应用方式
1. 用户选择cot/tdp,缺省是tdp
2. 根据选择，提醒用户将安装到~/cot或者~/tdp中，此处允许用户修改安装路径
3. 根据操作系统和支持的Shell，向用户只读地显示将执行的命令
   3.1 Windows下如果有pwsh 7.x或者powershell则执行install.ps1,否则执行install.cmd（gpm-go和ai-desk也要据此相应调整）
   3.2 非Windows(如Linux/Mac)，则是install.sh
4. 执行相应的命令
   4.1 使用qiuzhanghua/portable-pty来执行命令和交互
   4.2 stderr的输出用红字
   4.3 安装错误也要用红字提示
5. 有按钮提示关闭自身


# 技术考虑
1. go wails?
```

原稿里 3.1 那条已经被讨论推翻（见 §8.2）：**不做 `install.ps1`**。

## 1. 定位与边界

- **一句话**：图形安装向导。用户解压 `ad-<版本>-<平台>.zip` 之后**双击 `GUI-Setup`**，
  在窗口里选几下，就把 AI Desk（以及它带的命令行工具链）装好；看不懂的黑框输出留在同一个窗口里给他看。
- **可执行文件名**：`GUI-Setup`（Windows 上 `GUI-Setup.exe`）。用户原稿：「编译成的执行文件名字为GUI-Setup」。
- **语言**：Go（上游 gpm-go 同语言，直接复用 `internal/` 里的判断与路径逻辑）。
- **分工**：

  | | 干什么 | 不干什么 |
  |---|---|---|
  | `gpm`（已有） | 解包、校验 SHA256、落入口、建启动器、写 PATH 块、自举 cot/tdp、记账本、卸载回放 | 不弹窗、不问人、不画界面 |
  | `GUI-Setup`（本项目） | 问清楚"装哪家、装到哪儿"，把参数交给 `install.sh` / `install.cmd`，把子进程输出搬进窗口 | **不自己装任何文件**——一行安装逻辑都不许有，否则两个入口会各自跑偏 |

- **复用面**：不止 AI Desk。任何用 `gpm pack` 打出来的包（清单里声明了 `setup:`，见 §2.2）都能带上它。

## 2. 分发形态

### 2.1 放在 zip 顶层（不另起目录）

`GUI-Setup` 与 `install.sh` / `install.cmd`、`gpm` **并排放在 zip 顶层**
（用户裁决："GUI-Setup 放 zip 顶层、与 install.sh 同级"）。

**不另起 `setup/` 那一层目录**——用户问："每个平台一个zip包，setup 这个目录存在的意义是什么? 要做成所有平台一个zip包吗？"
问得对，去掉：

- 包本来就是**一个平台一个 zip**（顶层的 `gpm` / `gpm.exe`、`tools/<os>_<arch>/`、清单里按 GOOS 挑的 `entry`，
  整条契约都是按平台切的），单平台包里再套一层目录只让用户多双击一次。
- **也不改成"一个 zip 通吃三平台"**：下载体积会变成三倍（三个平台的 app 与两套工具链都塞进去），
  而 `SHA256SUMS`、`entry` 那种"按平台挑一项"的语义都得跟着重做，收益远小于代价。发布矩阵照旧六个资产。
- 所以顶层放的就是**这个平台自己的形态**，与 `gpm` / `gpm.exe` 完全平行：

  | 平台 | 顶层多出来的东西 |
  |---|---|
  | macOS | `GUI-Setup.app`——**必须是 `.app`**：Finder 里双击裸 Mach-O 不会有 Dock 图标、也拿不到前台焦点；Wails 在 darwin 上产出的本来就是 `.app` |
  | Windows | `GUI-Setup.exe` |
  | Linux | `GUI-Setup`（要不要再配一个 AppImage 另说） |

- macOS 上于是顶层有两个 `.app`：`GUI-Setup.app`（安装器）与 `payload/AI Desk.app`（真应用）。README 里要写清"双击哪个"。
- zip 里这些顶层文件要**带可执行位**（由 `gpm pack` 负责），否则 macOS / Linux 解压出来双击不动。

### 2.2 清单新增 `setup:` 字段

用清单声明它（用户裁决："用 manifest 新增 `setup:` 字段声明 GUI-Setup（与 `entry:` 平行）"）。
形状**直接复用 `entry:` 那个类型**（gpm 侧 `internal/manifest/manifest.go` 的 `Entry map[string]Entry`，按 GOOS 挑一项，`bundle` 与 `exe` 二选一）：

```yaml
setup:
  darwin: { bundle: "GUI-Setup.app" }
  windows: { exe: GUI-Setup.exe }
  linux: { exe: GUI-Setup }
```

- 清单本来就是**按平台生成**的：ai-desk 的 `tools/package.sh` 在 `case $os` 里只写当前平台那一块（`entry:` 如此，`setup:` 照抄）。
- 字段缺省 = 这个包不带图形安装器，一切照旧（老包不受影响）。
- `gpm pack` 多一个 `--setup <路径>` 参数（对照 `--gpm`）：清单声明了 `setup:` 却没给 `--setup`、或给了却没声明 → 报错，
  口径和"`payload/` 与 entry 对不上"时一致。
- **两个相对根不一样，必须写进 PACKAGE-FORMAT**：`entry:` 里的路径相对 `payload/`；`setup:` 不在 payload 里，它的路径只相对 **zip 顶层**。
- `gpm install` **不读**这个字段：它只是给人和给 GUI-Setup 看的声明；装完之后顶层那份也不必留在家里。

### 2.3 终端入口照旧

`install.sh` / `install.cmd` **不动**：下载完愿意自己开终端跑的人，还是照旧用它们（用户裁决，GUI 调它们也正是为了这条不变成两条路）。

## 3. 界面与流程

窗口很小就够（一个 800×600 的宿主应用旁边，向导 520×460 那种量级），流程是**一条直线**：

### 3.1 界面 A：先看家里有没有装过（用户裁决："管卸载/升级"）

启动时用账本判断（唯一真相是 `<家>/<家目录名>-state.json`，比如 `~/cot/cot-state.json`）：

- 找到本应用（按清单 `id`）→ 摆出：装的是哪个版本、装在哪、包里是哪个版本，然后给三个按钮
  **升级**（版本更新）/ **卸载** / **取消**。同一个 id 再跑一次 `gpm install` 就是覆盖式升级（D30），所以"升级"不需要新命令。
- 没找到 → 直接进界面 B。
- 家在哪儿得先猜（缺省 `~/cot`），因为账本按家分；找不到就问用户，或直接走界面 B 让用户选。

### 3.2 界面 B：选装哪家、装到哪儿

1. **选 cot 还是 tdp**（用户原稿缺省是 tdp；用户补充裁决："**一家**"）——
   这是个二选一，不提供"都不装"：装 AI Desk 就得有命令行的家。
2. **显示安装路径**，并**允许改**（用户原稿第 2 条）：缺省 `~/cot`（选 cot）或 `~/tdp`（选 tdp），旁边一个"更改…"。
3. **只读地显示即将执行的命令**（原稿第 3 条）——让人能照抄到终端里自己跑：

   ```sh
   ./install.sh --dir "$HOME/cot" --with cot --yes
   ```

   （Windows 上显示 `install.cmd --dir "%USERPROFILE%\cot" --with cot --yes` 那一版。）

### 3.3 界面 C：跑起来，输出就在窗口里

一个带滚动条的**白色**输出区（用户裁决：白底，不用黑底——"免得非技术人员对黑色 Terminal 的恐惧"；红字在白底上对比度也更好），实时显示子进程的输出；底下是**关闭**按钮（原稿第 5 条：装完/失败后给个按钮把窗口关掉）。
`GUI-Setup` 自己**不弹系统对话框**去报错——错误就是窗口里的红字。

## 4. 怎么执行（已定）

### 4.1 调谁

调 `install.sh` / `install.cmd`（用户裁决："GUI 调 install.sh/cmd"），**不是**直接调 `./gpm install`：
参数与默认值只有一处定义，用户自己到终端跑和 GUI 跑才是同一条路。

gpm v3.12 起这两个脚本会把参数原样转交给 gpm（`"$@"` / `%*`，见 gpm-go D42），所以下面这些参数真的能到达 gpm：

| 界面上的选择 | 传的参数 |
|---|---|
| 选 cot | `--dir <家> --with cot` |
| 选 tdp | `--dir <家> --with tdp` |
| 用户在界面里改了路径 | `--dir` 用改过的那个（**永远显式传**，见 4.3） |
| 不弹 gpm 自己的询问 | `--yes`（PATH 集成同意） |
| 卸载 | `<家>/bin/gpm uninstall <id> --yes`（不经脚本） |

`--with cot` 有两个作用：把清单的 `requires: [cot, tdp]`（AI Desk 现在是两家都带）**收窄成一家**，
另一家的工具链不装（用户裁决："另一家工具链不装"）。

### 4.2 非 Windows：`install.sh`

直接 `exec` 它（它自己有 `#!/bin/sh` + `chmod +x` 兜底）。

### 4.3 烘进去的默认值不用管

`install.sh` 里烘着的 `--dir "${COT_HOME:-$HOME/cot}"` 只服务**裸跑 `./install.sh` 的人**
（用户裁决："烘进去的默认值只管裸跑 ./install.sh 的人"）。
因此 **ai-desk 的打包参数不改**，GUI-Setup **永远显式传 `--dir`**——按 Go flag"后出现者覆盖先出现者"，
显式那个赢。

### 4.4 Windows：`cmd /c install.cmd < NUL`

不写 `install.ps1`（用户裁决："不要 install.ps1"）。改用现成的 `install.cmd`，但注意它**末尾有一句 `pause`**
（给双击的人看的"按任意键继续"）——GUI 调它会卡在等按键上。处理方式（用户选了 (c)）：

```
cmd /c install.cmd --dir "%USERPROFILE%\cot" --with cot --yes < NUL
```

`< NUL` 让 `pause` 立刻拿到 EOF 自己走完，既不改 `install.cmd`、也不改 gpm。

### 4.5 portable-pty 是硬要求

用户裁决："portable-pty 是硬要求（我想让用户看到安装的过程/问题）"。用的就是
[`github.com/qiuzhanghua/portable-pty`](https://github.com/qiuzhanghua/portable-pty)（v0.2.1，从 wezterm 移植）。

- 它给的是**真 pty**，所以子进程看到的是终端：彩色、进度、"y/N"这些行为跟人手敲一样。
- 代价：gpm 的 `isTerminal` 会判为交互终端，于是它**会问** PATH 集成的事——所以必须传 `--yes`
  （或 `--no-path`），否则窗口会停在那儿等输入。这是我们自己要答的问题，不该丢给用户。
- 不打算给 `--yes` 之外的"图形化 PATH 询问"：PATH 集成就是"要不要写那三行 rc"，界面 B 上用一句
  "让终端里也能直接用 `ad` 命令"当勾选框，勾了就等于 `--yes`。

### 4.6 它从哪儿跑、包根怎么找

GUI-Setup 是**随包跑的**（在解压出来的目录里双击），不是装进家里的东西（§6）。
它从自己的可执行文件所在目录起**逐级向上找那唯一一个 `<简称>-manifest.yaml`**，找到哪儿，哪儿就是包根：

- 裸二进制（Windows / Linux）：包根就是自己所在目录。
- macOS 的 `.app`：可执行文件在 `GUI-Setup.app/Contents/MacOS/GUI-Setup`，往上第四层才是包根——**别写死层数**，用"找到清单为止"的循环。

找到包根之后：清单读 `<包根>/<简称>-manifest.yaml`，要跑的命令拼 `<包根>/install.sh`（Windows：`<包根>\install.cmd`）。
找不到就直说："请把 GUI-Setup 放回解压出来的目录里再运行"——有人会把 `.app` 单独拷到桌面。

## 5. 输出与红字（平台不一样，要如实处理）

用 pty 就绕不开一个事实：**Unix 上三个流能分开，Windows 上分不开**。

- **macOS / Linux**：调用方自己设 `cmd.Stderr` 会被 portable-pty 尊重（Unix 上 slave 只填调用方没设的流），
  所以 stderr 单独接一根管道 → **只有 stderr 整段红字**，stdout 用正常色。顺序可能和真实终端略有出入（两条管道各走各的）。
- **Windows**：走 ConPTY，三个流全由伪控制台供给，调用方设的会被忽略 → stdout / stderr **合流**，
  做不到"只有错误是红的"。退路：整段按正常色显示，另外在**退出码非 0**时把"安装失败"那几行用红字标出来（我们自己加的提示，不是给子进程输出染色）。

两条底线（原稿 4.2 / 4.3）：**stderr 的用途是红字提示**、**安装错误也要红字提示**。窗口里同时给一行
可照抄的"完整命令"和退出码，方便用户贴给我们排查。

## 6. 卸载与升级（已定）

- **入口只有 GUI-Setup 和终端两处**：界面 A 的"卸载"按钮直接跑
  `<家>/bin/gpm uninstall <id> --yes`（用户选 (a)）。**不做 `uninstall.sh`**（用户裁决），
  也不把 `GUI-Setup` 装进家里——它是随包跑的临时工具，用完就扔。
  - 之所以不用脚本：卸载不需要"环境还没建立"那套 bootstrap，gpm 自己已经在家里的 `bin/` 下。
  - 终端用户的路子照旧：`<家>/bin/gpm uninstall ai-desk --yes`。
- **升级**：界面 A 的"升级"就是再跑一次 `install.sh`（同 id 覆盖式安装，D30 已有）。**没有版本回退**。
- 卸载界面要**把 gpm 即将删的东西先列出来**（入口 / 启动器 / 图形入口 / PATH 块），再让用户点"确认卸载"——
  这正是 gpm v3.9 的 `askUninstall` 干的事，GUI 端只是把清单先画出来，实际仍由 `gpm uninstall --yes` 执行。

## 7. 需要 gpm-go / ai-desk 配合的改动

原稿 3.1 那句"（gpm-go和ai-desk也要据此相应调整）"具体是这些：

| 仓 | 改动 | 状态 |
|---|---|---|
| `gpm-go` | 生成的 `install.sh` / `install.cmd` 把参数原样转交（`"$@"` / `%*`，拼在烘进去的 `--dir` 之后） | ✅ v3.12 已完成 |
| `gpm-go` | `gpm pack` 多一个 `--setup <路径>` 参数 + 清单解析 `setup:`（复用 `Entry`，按 GOOS 挑一项，bundle/exe 二选一），把顶层那份打进 zip 并保住可执行位；PACKAGE-FORMAT 里写清两个相对根 | ✅ v3.13 已完成（gpm v0.6.4） |
| `gpm-go` | （可选）`gpm list` 输出里带上版本，方便 GUI 端判断"能不能升级" | ⬜ 待确认（账本自己读也行） |
| `ai-desk` | CI 先构建 `gsetup-go`，把 `GUI-Setup` / `GUI-Setup.exe` 与 `install.sh` 并排放在装配目录**顶层**（不另起 `setup/`，见 §2.1）；`GPM_REF` 跟到带 v3.12 的 gpm | ✅ 已完成（`tools/package.sh` 新增 `-s`；`release.yml` 检出 gsetup-go 就地编，`GSETUP_REF: v0.1.0`、`GPM_REF: v0.6.4`；六平台 CI 全绿） |
| `ai-desk` | 清单 `ad-manifest.yaml` 加 `setup:` 段（`darwin: { bundle: "GUI-Setup.app" }`、`windows: { exe: GUI-Setup.exe }`） | ✅ 已完成（由 `package.sh -s` 按平台生成，见 v0.6.4 包实测） |

## 8. 明确不做

1. **不做签名 / 公证**（用户裁决：方案 (i)，接受首次由用户手动放行），但**说明的口径要按 macOS 15 的实情写**：

   - 右键→「打开」在新系统上基本失效（ai-desk 自己的
     [README.md](https://github.com/qiuzhanghua/ai-desk/blob/main/README.md) 已经记着这条），
     真正能走的是**系统设置 → 隐私与安全性 → 找到被拦的那条提示点「仍要打开」**。
     gsetup-go 的 README / 首次运行提示里不能只写"右键打开"，否则用户照做会发现没用。
   - Windows 上是 SmartScreen（"更多信息 → 仍要运行"）。
   - **代价被这条路放大了**：现在"第一次运行的东西"是终端里的 `install.sh`（Shell 脚本不受 Gatekeeper 拦），
     改成双击 `GUI-Setup.app` 之后，第一次遇到的就是"未签名 + 带 `com.apple.quarantine`"的应用；
     而 gpm 的 `xattr -dr`（v3.10、D40）只在**安装时**清标记，救不了这一次。
     这是"给非开发人员双击装"最脆弱的一环；已裁决：**不改签名，就走方案 (i)**。

2. **不做 `install.ps1`**（推翻原稿 3.1）：改走 `cmd /c install.cmd < NUL`。当初 gpm v3.5 特意选 `.cmd` 就是因为
   PowerShell 默认 ExecutionPolicy 会拦脚本。
3. **不做 `uninstall.sh`**（用户选 (a)）。
4. **GUI-Setup 自己不装任何文件**，也不进 `payload/`、不装进家。
5. **不做联网**：不下载、不检查更新、不回传（跟 gpm §0.5 的身份宣言一致）。

## 9. 已定的小事（原「待确认」，2026-10-10 逐条答完）

- **9-1 界面 A 什么时候出现？** **已定**：启动时先无条件探一次账本——缺省家取清单 `requires` 的第一家（缺省 `~/cot`），
  `~/tdp` 也顺带看一眼；探到就进界面 A，没探到直接进界面 B。
- **9-2 "升级"比不比版本？** **已定**：比。包里版本**严格更新**才把按钮显示成"升级"；
  否则显示"已经是最新的（<已装版本>）"并且不写盘——避免按钮点下去没反应。
- **9-3 tdp 该测了**（用户："现在该测了"）：界面保留 tdp 选项，并写进验收标准（A1 走 tdp、A2 验 cot 没被动）。
  **底气**：v3.12 那轮真机 e2e 已经用正式产物跑通过 tdp 27.0.6 的自举（两家各自落位、各有一本账）；
  要新验的是"只装一家"时另一家确实不动。
- **9-4 要不要也能装"没有 GUI 的包"？** **不做**（用户："不做，那是 cot/tdp 的职责"）。
- **9-5 技术选型**：**用 Go**（用户："毕竟 golang 开发时更方便"；走 Tauri 就得改用 Rust，那就不做 gsetup-go 了）。
  框架见 §11。
- **9-6 Windows 上"装到哪儿"的默认值**：`%USERPROFILE%\cot`（用户确认）。
  gpm 只在"清单没有 `requires`"时才用平台数据目录；这里两家工具链都有固定老家。
- **9-7 语言**：先中文（用户确认）。
- **9-8 签名**：**已定：方案 (i)，接受不签名**。第一次双击由用户手动放行（系统设置 → 隐私与安全性 → 仍要打开），
  这条写成用户可见的明文步骤（§8.1、A1）；不做签名 / 公证。

## 10. 验收标准

- **A1 干净机器（macOS）**：解压 `ad-*.zip`，双击 `GUI-Setup.app`（首次：系统设置 → 隐私与安全性 → 仍要打开）→ 选 tdp → 装完
  → `~/tdp/AI Desk.app` 存在、`command -v ad` 有输出、三份 rc 各恰一个 `# >>> gpm >>>` 块、
  账本 `~/tdp/tdp-state.json` 里 `packages` 有 ai-desk 且 `verified: true`。
- **A2 一家**：上面那次装完，`~/cot/bin/cot` **不应该**被这次安装动过（清单的两家被 `--with tdp` 收窄成一家）。
- **A3 界面里改路径**：把路径改成 `/tmp/x` → 装完 `/tmp/x/AI Desk.app` 存在，
  而烘在脚本里的默认家（`$HOME/cot`）**没有**冒出 `AI Desk.app`。
- **A4 红字（macOS/Linux）**：故意让安装失败（家里预放同名的别人文件）→ 窗口里 stderr 那几行是红的、退出码非 0、有"安装失败"的红色提示。
- **A5 Windows 不卡 `pause`**：`cmd /c install.cmd ... < NUL` 跑完自己退出，窗口停在"关闭"按钮上。
- **A6 卸载**：装好后再开 GUI-Setup → 界面 A 认出已装 → 点卸载 → 先列出将删项、确认后
  `<家>/bin/gpm uninstall <id> --yes` 跑完，入口 / 启动器 / 软链 / PATH 块全清、账本 `packages: []`，
  工具链的 `bin/` 一个字节没动。
- **A7 幂等**：连点两次"升级" → 第二次打印"已经装好 …，跳过"（或"已经是最新的"），rc 文件不被写第二遍。
- **A8 只读展示的命令与实际执行的一致**：窗口里显示的命令，复制到终端里跑，结果一样。

**已经真机验证过的（macOS，2026-10-10）**：① 拿 gpm v0.6.4 打的**真的** ai-desk 0.3.1 包跑端到端
（`GSETUP_E2E_PKG=… go test -count=1 -run TestEndToEndRealPackage -v .`）：界面上那条只读命令与
真跑的命令一致，装完 `<家>/cot/AI Desk.app` + `bin/ad` + 账本一条（`entryKind: bundle`、`cmd: ad`），
家里没有 `ai-desk_*`、`lib/` 空；再开一次能认出已装，卸载把入口 / 启动器 / 账本清干净。
② ai-desk 六平台 CI 的产物里，`GUI-Setup.app` 就躺在 `install.sh` 旁边（macOS 主可执行文件 0755、
**不在** `SHA256SUMS` 里），与清单的 `setup:` 段对得上；把那个 zip 解到干净家里跑 `./install.sh --yes`
能装好，而 `find <家> -iname "*GUI-Setup*"` 一处也没有 —— 图形安装器是"安装过程"，不落家、不进账本。
③ **在真机上被人手点完了两遍**（macOS 27.0.1 / arm64，2026-10-10，用的正是 AI Desk **v0.3.2 的正式 Release 资产**）：
第一遍界面 A 选 cot、家 `~/cot`、勾了 PATH ⇒ 实跑 `./install.sh --dir ~/cot --with cot --yes`，
`~/cot/AI Desk.app` + `bin/ad`（里设 `COT_HOME`）+ `bin/gpm` + `~/Applications` 软链 + 三份 rc 块 +
新建的 `~/cot/cot-state.json`（`verified: true`）全套落位，而 `~/cot/bin/cot` 与 `lib/` 那 49 个目录一位没动；
第二遍选 tdp（家自动变 `~/tdp`）⇒ 包里那份 `tools/darwin_arm64/tdp` 自举成 `tdp 27.0.6`，应用落
`~/tdp/AI Desk.app`、账本新建 `~/tdp/tdp-state.json`（`bin/ad` 里设的是 `TDP_HOME`），而 **`~/cot` 全程一位没动**
—— 清单的 `requires: [cot, tdp]` 确实被 `--with` 收窄成了一家；新开一个交互 zsh，
`command -v ad tdp gpm` 三条全落在 `~/tdp/bin`，PATH 块当场生效（这就是界面 A 那个勾选框的效果）。
④ 两遍之后都在终端 `gpm uninstall ai-desk --yes` 回放干净：入口 / 启动器 / 软链 / 三份 rc 块 / `bin/gpm`
全清，账本只剩 `packages: []`，`~/cot` 与 `~/tdp/bin/tdp` 一个字节没动，三个 rc 文件的**末字节逐字节还原**
（原本末行没有尾换行，回放后也没有）—— 卸载不带走工具链（这正是 §6 的边界：要清那个家得自己 `rm -rf`）。
⑤ 顺带验到"写不动"时的行为：同一条卸载命令若所有外部写都被拒（受限环境里实测），它**一位都没删**、
连 `cot-state.json.tmp` 都没留下 —— 账本最后落盘，要么全做要么全不做。

## 11. 技术选型

**语言定了：Go**（§9 的 9-5）。剩下的是 GUI 框架。

| 方案 | 好处 | 代价 |
|---|---|---|
| **Wails v2**（稳定线） | 独立可执行文件；Go 直接调 portable-pty；界面是 HTML/CSS（我们熟）；中文由系统 webview 的字体渲染；macOS 上产出的就是 `.app` | 每个平台要 webview 运行时：Windows 要 WebView2（Win10/11 一般随 Edge 预装）、**Linux 要 WebKitGTK（缺了窗口起不来）**、macOS 用系统 WKWebView |
| **Wails v3**（beta） | 同 v2，API 更新、多窗口更顺 | v3 目前仍标 beta（[v3 beta 公告](https://v3.wails.io/blog/wails-v3-beta/)，pkg.go.dev 上还是 `v3.0.0-alpha2.x` 的版本号），API 还会动 |
| **Gio / Fyne**（纯 Go，无 webview） | 单二进制、零 webview 依赖，最像"安装器" | **Fyne 自带字体没有中文字形**——纯中文界面得自己嵌一套中文字体（几 MB 起）；看日志、排版都不如 HTML 顺手 |

**倾向**：Wails。理由是这个向导的价值全在"看得清、点得对"——HTML/CSS + 系统字体最省事，
webview 依赖在 Windows / macOS 上由系统兜底，只有 Linux 需要用户装一个 WebKitGTK（README 里给一行包名提示）。
要把依赖压到零就选 Gio，代价是嵌中文字体。

**已定（用户，2026-10-10）：Wails v2**（稳定线，不用 v3 beta）。接受它带来的 webview 依赖；
Linux 上缺 WebKitGTK 时窗口起不来的情况，在 README 里给一行装包提示。

**另一个前置条件**：签名 + 公证（§8.1 与 9-8）。不签的话，"双击就装"在 macOS 上第一步就被拦；
签了才谈得上"非开发人员无障碍"。这两件事最好一起定。
