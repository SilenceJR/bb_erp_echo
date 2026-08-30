# Windows 本机打包与局域网发布

## 适用范围

本方案只支持 x86_64 Windows Server 2016 Desktop Experience 和 Windows 10 1909（build 18363）及以上版本。Server 2016 的角色是拉取源码、编译、发布客户端安装包、运行 ERP 服务端并向局域网客户端分发更新；它不作为日常 Web 或 Tauri 操作电脑。日常业务和桌面客户端更新在 Windows 10 工作站完成。

Gitee `main` 是唯一源码源。发布电脑必须使用专用、干净的 checkout，只有位于 `origin/main` HEAD 上、严格高于已安装版本的 `vMAJOR.MINOR.PATCH` 正式标签才会发布。普通提交、RC 标签、旧版本、移动标签或并发任务均不会进入发布事务。

旧 GitHub Actions、GitHub Artifact、Gitee Release 附件上传、双仓同步和差分包链路已移除。服务端和客户端均使用完整包。

## 脚本与工具链锁

统一入口是 `scripts/windows-release.ps1`，兼容系统自带的 Windows PowerShell 5.1：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows-release.ps1 -Mode Doctor
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows-release.ps1 -Mode Setup
powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File .\scripts\windows-release.ps1 -Mode Publish
```

`scripts/windows-toolchain.json` 固定 Go 1.27.0、Node.js 22.21.1、npm 10.9.4、Rust 1.97.1、`x86_64-pc-windows-msvc`、`rustfmt` 和仓库本地 Tauri CLI 2.11.4。Go 由管理员从 Go 官方页面手动安装；脚本只接受精确版本。Tauri CLI 不作全局安装，Rust crates、Web/Client npm 包和 Go 模块分别由 `Cargo.lock`、两个 `package-lock.json` 和 `go.sum` 恢复。

`Doctor` 只检查，不修改电脑；`Setup` 只在管理员显式运行时安装或修复环境；计划任务只运行 `Publish`。`Publish` 发现环境版本不符时直接失败，不会升级工具、修改系统、注册计划任务或重启电脑。

## 首次环境准备

1. 确认 Server 2016 使用 Desktop Experience，或 Windows 10 已达到 1909；确认系统为 64 位，并预留至少 30 GiB 可用空间。
2. 管理员手动安装锁定的 Go 版本，并重新打开 Windows PowerShell。
3. 将 Gitee 仓库克隆到专用目录。该目录不得混用日常开发改动。
4. 使用管理员 Windows PowerShell 运行 `-Mode Setup`。
5. 如脚本返回 3010，人工重启电脑后运行 `-Mode Doctor`；脚本绝不自动重启。
6. `Doctor` 全部通过后，再配置签名密钥和计划任务。

`Setup` 会按需安装 Git for Windows、Visual Studio Build Tools 2022 17.x 的 C++/MSVC/Windows 10 SDK、WebView2 Evergreen Runtime、锁定的 Node.js、rustup/Rust MSVC 工具链和 MSYS2 MinGW64 GCC。MSYS2 使用锁定的官方 2026-06-11 自解压基础包（而不是已不支持 Server 2016 的新版 GUI installer），随后通过 pacman 更新并安装 GCC。它不依赖 WinGet，也不安装 VS 2026 或全局 Tauri。

下载前会启用 TLS 1.2。安装器存入独立缓存目录（默认 `C:\BBERP\tool-cache`）；Node、Rust 和 MSYS2 使用各自官方发布的 SHA-256，Git 的官方发布元数据可提供哈希时也会固定。Git、Node、Visual Studio、WebView2 和当前锁定的 MSYS2 自解压包同时检查 Authenticode。上游 `rustup-init.exe` 目前不提供 Authenticode 签名，因此该文件只能使用 `static.rust-lang.org` 同路径官方 SHA-256 严格校验，脚本不会伪造“签名已通过”。Visual Studio 使用本地 layout，WebView2 使用完整 x64 Evergreen Standalone Installer，因此首次成功缓存后可以在断网环境复用；首次下载仍必须能够访问相应官方站点。缓存哈希或签名不一致时立即停止，不能带病发布。

## 签名账号与密钥

计划任务使用专用低权限账号，但必须授予运行服务、写入安装目录和以最高权限执行发布脚本所需的最小权限。以下变量配置在该账号或机器环境中，使计划任务启动时能够继承：

```powershell
[Environment]::SetEnvironmentVariable('TAURI_SIGNING_PRIVATE_KEY', '<私钥内容或受保护文件路径>', 'User')
[Environment]::SetEnvironmentVariable('TAURI_SIGNING_PRIVATE_KEY_PASSWORD', '<私钥密码>', 'User')
[Environment]::SetEnvironmentVariable('TAURI_UPDATER_PUBLIC_KEY', '<Tauri Minisign 公钥>', 'User')
```

私钥和密码不得写入仓库、脚本参数、批处理或日志。脚本启动后立即把签名变量从进程环境移除，因此 `git`、`go`、`npm`、`cargo` 和构建脚本均不能继承私钥；仅在每次调用仓库本地 Tauri signer 时短暂注入私钥和密码，调用结束立即恢复为空。Tauri 编译只注入公钥，最后一次签名后脚本内存中的私钥引用也会清除，updater 和新服务端不会继承签名材料。计划任务账号仍应使用 Windows 凭据保护和严格的目录 ACL。

## 发布流程

`Publish` 使用全局互斥锁并执行以下操作：

1. 取得全局互斥锁后，先检查追加式升级事务日志；如上次任务被强制结束或电脑断电，使用固定 `updates/recovery/bb-erp-updater.exe`（同时核对日志记录的 SHA-256）恢复旧程序、数据库和 stable manifest。该恢复发生在环境检查、联网、拉代码和版本门禁之前。
2. 运行完整环境检查，确认 checkout 干净且 `origin` 指向 Gitee。
3. `fetch --tags`、切换 `main`、`pull --ff-only`，确认本地 HEAD 等于 `origin/main`。
4. 校验 HEAD 正式标签和已安装版本；没有新版本时以成功状态跳过。
5. 恢复锁定依赖，执行 Go tidy 差异检查、Vet/Test、Web/Client 生产构建和 Rust fmt/check/test。
6. 使用 MinGW GCC/CGO 构建服务端，CGO 关闭构建 updater，并由仓库本地 Tauri 构建 NSIS 与 Portable 客户端。
7. 在 `updates/pending/<release-id>` 完成完整包、哈希、Minisign 和 v3 清单验证；服务端会再次验证 NSIS、Portable 和人工恢复 ZIP 各自的 Minisign、大小、SHA-256 及 ZIP 路径安全。任何失败都不会改变在线程序或 stable manifest。
8. 将不可变归档写入 `updates/releases/<version>`，客户端资源按 SHA-256 写入 `updates/artifacts/<sha256>`。
9. updater 将已验证的自身副本保存到固定恢复目录，持久化备份文件和 SQLite/WAL/SHM 后再推进事务阶段；随后停止目标 Windows Service 或普通进程，安装新文件并原子切换清单。
10. 通过 `/ready`、`/api/v1/version` 和客户端完整包计划验证目标版本；失败时恢复旧程序、数据库和清单，并再次验证旧服务。
11. 成功后记录标签、Gitee commit、工具链和制品哈希，保留当前 stable 与记录在部署状态中的上一成功版本。若脚本在 updater 成功返回后被终止，下次任务会在访问 Gitee 前补齐部署记录与安全清理。

默认安装目录是 `C:\BBERP\server`。可按现场目录覆盖：

```powershell
powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass `
  -File D:\BBERP-src\scripts\windows-release.ps1 `
  -Mode Publish `
  -RepositoryDir D:\BBERP-src `
  -InstallDir D:\BBERP\server `
  -CacheDir D:\BBERP\tool-cache `
  -WindowsServiceName BBERP `
  -HttpPort 8080 `
  -HealthBaseUrl http://127.0.0.1:8080 `
  -DatabasePath D:\BBERP\server\data\erp.db
```

普通进程模式不传 `-WindowsServiceName`。Service 模式会验证服务实际指向目标安装目录的 `bb-erp-server.exe`，不允许停止同名但指向其他目录的服务。

普通进程由包内 `启动服务端.bat` 设置生产目录和本地 manifest；脚本会把 `-HttpPort` 与 `-DatabasePath` 写入该批处理，因此两者必须与 `-HealthBaseUrl` 和现场数据库一致。为兼容传统 CMD，普通进程的数据库路径需使用 ASCII 字符且不能包含 CMD 元字符。

Windows Service 必须直接指向安装目录的 `bb-erp-server.exe`，且正式方案统一使用机器级环境变量，不依赖批处理或无法核验的交互用户环境。发布前脚本会精确校验 `BB_ERP_APP_ENVIRONMENT=production`、`BB_ERP_HTTP_HOST=0.0.0.0`、`BB_ERP_HTTP_PORT`、`BB_ERP_DATABASE_PATH`、`BB_ERP_WEB_ENABLED=true`、`BB_ERP_WEB_DIST_DIR`、`BB_ERP_UPDATE_ENABLED=true`、`BB_ERP_UPDATE_MANIFEST_FILE` 和 `BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE` 是否与本次参数及安装目录一致；不一致时不会停止服务或开始升级。`HealthBaseUrl` 必须是与 `-HttpPort` 一致的本机 loopback 地址。

例如安装目录为 `D:\BBERP\server`、端口为 8080 时，管理员在首次创建 Service 前执行以下不含私钥的机器级配置：

```powershell
[Environment]::SetEnvironmentVariable('BB_ERP_APP_ENVIRONMENT', 'production', 'Machine')
[Environment]::SetEnvironmentVariable('BB_ERP_HTTP_HOST', '0.0.0.0', 'Machine')
[Environment]::SetEnvironmentVariable('BB_ERP_HTTP_PORT', '8080', 'Machine')
[Environment]::SetEnvironmentVariable('BB_ERP_DATABASE_PATH', 'D:\BBERP\server\data\erp.db', 'Machine')
[Environment]::SetEnvironmentVariable('BB_ERP_WEB_ENABLED', 'true', 'Machine')
[Environment]::SetEnvironmentVariable('BB_ERP_WEB_DIST_DIR', 'D:\BBERP\server\web\dist', 'Machine')
[Environment]::SetEnvironmentVariable('BB_ERP_UPDATE_ENABLED', 'true', 'Machine')
[Environment]::SetEnvironmentVariable('BB_ERP_UPDATE_MANIFEST_FILE', 'D:\BBERP\server\updates\stable\update-manifest.json', 'Machine')
[Environment]::SetEnvironmentVariable('BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE', 'D:\BBERP\server\update-public.key', 'Machine')
```

配置后人工重启一次该 Service，使新进程读取机器变量，再以完全相同的 `-WindowsServiceName`、`-InstallDir`、`-HttpPort`、`-HealthBaseUrl` 和 `-DatabasePath` 运行 `Doctor` 与首次值守 `Publish`。服务实际路径、机器变量或参数任一不一致都应修正配置，不能绕过校验。若现场不需要 Windows Service，使用包内 `启动服务端.bat` 的普通进程模式，并从计划任务参数中省略 `-WindowsServiceName`；两种模式只能选择一种，不能让同一安装目录同时运行 Service 和普通进程。

## 手工添加计划任务

脚本不会创建或修改计划任务。管理员在“任务计划程序”中手工创建：

- 使用专用发布账号，并选择“使用最高权限运行”。
- 触发频率按现场发布窗口设置；任务设置选择“如果任务已在运行，则不启动新实例”。
- 程序为 `powershell.exe`。
- 参数为 `-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "D:\BBERP-src\scripts\windows-release.ps1" -Mode Publish -RepositoryDir "D:\BBERP-src" -InstallDir "D:\BBERP\server" -WindowsServiceName "BBERP"`。
- “起始于”填写仓库目录。
- 不勾选会自动重启服务器的选项。

首次配置后用一个测试正式标签在人工值守窗口运行任务，并保存任务历史、发布日志和 Windows 事件日志作为验收证据。

## 局域网客户端更新

客户端在登录页填写 ERP 服务器局域网地址，例如 `http://192.168.1.20:8080`。登录、业务 API 和更新检查共用该地址。服务端从本机 `updates/stable/update-manifest.json` 读取清单，不请求 Gitee、GitHub或自身 HTTP 地址。

客户端通过以下受控 API 发现和下载更新：

- `GET /api/v1/updates/client/plan`
- `GET /api/v1/updates/client/artifacts/{sha256}`
- Tauri 完整包兼容接口

发布目录不会作为静态目录暴露，也不开启目录列表。v3 签名 payload 将资源类型、大小、SHA-256 和 Minisign 签名绑定；客户端下载后重新验证。人工恢复 ZIP 也在清单中携带独立 Minisign，服务端在分发前复验，但浏览器通过局域网 HTTP 保存的文件不具备自动端到端验签能力，因此恢复 ZIP 只能由管理员在受信任内网人工处理，不能自动执行。自动更新固定使用用户已确认的 Portable 单 EXE，在客户端目录内备份、替换并通过 ready marker 验证新版本启动，失败时恢复旧 EXE；NSIS 只用于首次安装或人工恢复，避免安装阶段再次读取 stable 清单而越过用户确认。自动更新要求客户端安装目录可写，不满足时会停止并提示下载恢复 ZIP 或重新安装到可写目录。用户必须先确认，确认后才会下载、安装并重启；“稍后处理”不会更改当前客户端。Server 2016 上不需要日常打开 Tauri 客户端。

防火墙只向受信任的私有网络配置文件放行 ERP 端口。不要将 HTTP 服务直接暴露到互联网；跨网段或公网访问需要另行设计 HTTPS、身份验证和网络边界。

## macOS 开发机交叉编译边界

macOS 开发机可继续用于日常开发、Go/Web/Rust 静态检查和纯 Go Windows 程序的预编译验证，但不能代替本方案的 Windows 发布电脑。当前 macOS 26 arm64 设备实测可用 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0` 生成 Windows x64 的 `bb-erp-updater.exe`。

正式服务端不能按相同方式关闭 CGO：仓库的 SQLite 驱动依赖 `github.com/mattn/go-sqlite3`，发布脚本固定使用 MinGW GCC 和 `CGO_ENABLED=1`。当前 Mac 未安装 `x86_64-w64-mingw32-gcc`，因此不能直接生成正式服务端；即使后续安装 MinGW 并成功交叉编译，也只能视为候选制品，仍须在 Windows 上验证 SQLite、Windows Service、普通进程升级和回滚。

客户端已安装 `x86_64-pc-windows-msvc` Rust target 仍不足以完成 Tauri Windows 构建。当前 Mac 实测在 Windows target 的 `cargo check --locked` 中因缺少 Windows C 头文件/SDK而在 `ring`、`zstd-sys` 等原生依赖处失败，也不具备 Visual Studio MSVC、Windows SDK、`llvm-rc` 和 Windows 安装器工具。Tauri 虽提供从 macOS/Linux 交叉生成 NSIS 的备选路径，但限制较多、测试覆盖较弱，MSI 仍只能在 Windows 构建；本仓库不把该路径作为正式发布链路。

因此正式 NSIS、Portable、服务端 ZIP、签名、安装、重启和回滚仍统一在 Windows Server 2016 Desktop Experience 或 Windows 10 x64 上完成。Mac 生成的 updater 或未来实验性交叉编译产物不得直接写入 stable manifest。

## 故障处理与验收边界

- `Doctor` 失败：按输出修复版本、PATH、MSVC/SDK、WebView2、GCC、长路径或磁盘空间，再重跑；不要让计划任务自动修复环境。
- 返回 3010：人工重启后重跑 `Doctor`。
- 无新标签：任务成功跳过，这是正常状态。
- checkout 脏、标签倒退/移动或并发：任务停止；先修复专用 checkout 和 Gitee 标签，不得强行覆盖。
- 新服务健康检查失败：updater 自动回滚；检查发布日志、服务日志和 Windows 事件日志，确认旧版本 `/ready` 恢复后再处理新标签。
- 上次发布被强制结束或断电：保持原计划任务参数不变再次运行 `Publish`。脚本会先读取最后一条完整事务记录，忽略半写入尾记录，校验固定 recovery updater 后恢复；不要手工删除 `updates/pending/server-upgrade-transaction.json`。
- 客户端无法检查：核对服务器 IP、同一局域网、防火墙私网规则、服务运行状态、stable manifest 和计划任务日志。

本仓库在非 Windows 环境完成的静态检查、Go/Web/Rust 测试不能代替真机验收。Windows Server 2016 Desktop Experience 与 Windows 10 x64 上的 Setup、断网缓存、构建、Service/普通进程升级、SQLite 回滚、NSIS/Portable 安装、重启及错误包测试证据齐全之前，不得标记为正式可用。
