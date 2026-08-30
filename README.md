# 博邦 ERP

博邦 ERP 是面向工厂内网使用的单组织、单仓库 ERP。后端采用 Go 1.27、Echo v5.3.1、GORM、SQLite WAL、Casbin 和标准库 `log/slog`；Web 端采用 Vue 3、TypeScript、Vite 与 Element Plus；桌面端通过 Tauri 复用同一套 Vue 界面和业务 API。

首版部署在工厂局域网。Windows Server 2016 Desktop Experience 主要负责从 Gitee 拉取源码、编译、分发更新包和运行 Go 服务端，不作为日常 Web/Tauri 操作电脑；Windows 10 1909 及以上工作站通过服务器局域网 IP 使用浏览器或桌面客户端。

## 已实现范围

- JWT 登录、个人/部门终端账号、Casbin 权限、角色、员工档案、员工多部门成员关系、双重责任审计与结构化日志。
- 客户、联系人和供应商档案。
- 单仓库物品工作台，按产品、生产物资、常规产品、生活物资分类。
- 物品详情内办理采购入库、退货返工入库、客户出库、部门出库，支持立即过账、幂等、库存不足校验、冲销和移动加权平均成本。
- 模具台账、借出归还、维修保养及履历。
- 生产/通用任务单、多部门子任务流转、状态日志和办公室确认；任务及仓库/库存写操作显式记录本次实际操作员工。
- 库存、任务、模具、审计等统计报表。
- 产品、模具、任务单和部门子任务图片的受保护单图/多图上传、预览、替换与删除。
- Web 同源请求与 Tauri Rust HTTP 传输抽象，可保存 ERP 服务器局域网地址。
- 本机 stable manifest、v3 完整包签名校验、内容寻址分发、管理员“版本与更新”页面，以及客户端确认后安装重启。
- Gitee `main` 唯一源码源、Windows PowerShell 5.1 环境引导和本机计划任务发布闭环。

## 目录职责

```text
cmd/server/              Go 服务入口
cmd/updater/             Windows 服务端升级器
internal/app/            应用装配与路由
internal/config/         配置和环境变量
internal/model/          GORM 数据模型及增量迁移入口
internal/middleware/     认证、权限、审计和请求日志
internal/{业务模块}/     客户、仓库、库存、模具、任务单等模块
docs/                    API Markdown 与 OpenAPI 产物
web/                     Vue Web 应用
client/                  Tauri 桌面壳和请求传输
scripts/                 发布与仓库工程脚本
```

## 状态与维护文档

- [Go 后端状态、进度与维护台账](docs/BACKEND_STATUS.md)：记录后端已完成、待完成、未完成、待修改、待修复、已发现问题、验证结果和后续计划。
- [Go 后端产品架构与业务时序图](docs/BACKEND_ARCHITECTURE.md)：用中文图示说明 Go 后端模块、数据流和核心业务过程。
- [Web 与 Tauri Client 产品架构与交互时序图](docs/WEB_CLIENT_ARCHITECTURE.md)：分别说明 Web、桌面 Client 和两者共用的页面与交互过程。
- [Web 与 Tauri Client 端进度与维护规范](docs/WEB_STATUS.md)：记录共用前端结构、Client 复用和弹窗样式改造进度。
- 每次代码更新必须同步受影响文档，并与代码一起提交；API 变更还必须同步 Swagger/OpenAPI 和必要的调试请求。

## 本地启动

需要 Go 1.27、Node.js 22、npm；开发桌面端还需要 Rust 和当前平台的 Tauri 依赖。

后端：

```bash
go run ./cmd/server
```

后端开发推荐使用 Air 快速编译和热重启。Go 1.27 可直接安装当前 Air：

```bash
go install github.com/air-verse/air@latest
air -c .air.toml
```

Air 监听 Go、模板和 HTML 文件，构建 `./cmd/server` 到忽略提交的 `tmp` 目录；保存代码后会先优雅中断旧进程，再启动新进程。`data`、`logs`、上传目录、前端依赖和 Tauri target 均排除监听，避免无关文件触发后端重启。可用以下命令确认重启后的服务：

```bash
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/ready
```

Air 只负责 Go 服务热重启。Web 热更新仍在另一个终端运行 `cd web && npm run dev`；Tauri 调试运行 `cd client && npm run desktop:dev`。Windows 10/11 使用同一份 `.air.toml`，平台覆盖会构建 `tmp\\bb-erp-dev.exe`。

Web 开发端：

```bash
cd web
npm ci
npm run dev
```

桌面端：

```bash
cd client
npm ci
npm run desktop:dev
```

默认管理员为 `admin` / `admin123456`。首次进入管理系统后，应在“系统 / 用户”中将管理员密码修改为正式密码；JWT 密钥由系统内部使用，无需额外配置。默认 access token 有效期为 2 小时，客户端会使用 refresh token 自动续期；连续 30 天未成功续期后需要重新登录。可通过 `BB_ERP_JWT_EXPIRES_IN` 和 `BB_ERP_JWT_REFRESH_EXPIRES_IN` 调整期限。

## 内网部署与网络方案

服务端默认监听 `0.0.0.0:8080`。本机可访问 `http://127.0.0.1:8080`，其他内网电脑应访问服务端的局域网 IP，例如 `http://192.168.1.20:8080`，并在操作系统防火墙放行 `8080/TCP`。

Web 生产环境由 Go 托管静态文件并使用同源相对路径。Tauri 通过 `HttpTransport` 接口选择 Rust HTTP 插件发送请求，避免依赖 Windows WebView 的跨域实现；业务 API 不感知运行平台。登录页和顶栏均可保存 ERP 服务器局域网地址。详见 [client/API_SYNC.md](client/API_SYNC.md)。

常用环境变量：

```text
BB_ERP_HTTP_HOST=0.0.0.0
BB_ERP_HTTP_PORT=8080
BB_ERP_HTTP_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:5173
BB_ERP_DATABASE_PATH=data/erp.db
BB_ERP_JWT_EXPIRES_IN=2h
BB_ERP_JWT_REFRESH_EXPIRES_IN=720h
BB_ERP_LOG_DIR=logs
BB_ERP_FILES_ROOT_DIR=static/uploads
BB_ERP_WEB_ENABLED=true
BB_ERP_WEB_DIST_DIR=web/dist
BB_ERP_ADMIN_USERNAME=admin
BB_ERP_UPDATE_ENABLED=false
BB_ERP_UPDATE_MANIFEST_FILE=updates/stable/update-manifest.json
BB_ERP_UPDATE_CACHE_DIR=updates
BB_ERP_UPDATE_CHECK_INTERVAL=6h
BB_ERP_UPDATE_MANIFEST_TIMEOUT=20s
BB_ERP_UPDATE_DOWNLOAD_TIMEOUT=10m
# `tauri signer generate` 生成的整个 .pub 文件内容，或改用下方文件配置
BB_ERP_UPDATE_SIGNING_PUBLIC_KEY=
BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE=update-public.key
```

服务端只读取本机 `updates/stable/update-manifest.json`，不会访问 Gitee、GitHub 或通过 HTTP 请求自身。客户端 v3 完整包资源由类型、大小、SHA-256 和 Minisign 签名共同绑定，并通过受控同源 API 下载；更新目录不作为静态目录公开。用户必须先确认，确认后客户端才使用精确 Portable EXE 完成备份、替换、启动验证和失败回滚；NSIS 只用于首次安装或人工恢复。浏览器端只查看发布状态和管理员恢复包。

服务端发布由计划任务调用 updater 完成。updater 停止目标 Windows Service 或安装目录普通进程，持久化备份 EXE、Web、公钥、版本信息、stable manifest 和 SQLite/WAL/SHM，安装后验证 `/ready`、目标版本和客户端完整包计划；失败时恢复旧程序、数据库和清单。追加式事务日志及固定 recovery updater 允许下一次计划任务在环境检查和联网前恢复被强制中止的升级。Service 模式还会校验服务实际指向目标 `bb-erp-server.exe`，并要求机器级运行配置与发布参数一致。

## 分支与 Windows 发布

Gitee 是唯一源码源。功能开发采用 `codex/<主题>` 分支，通过 Go、Web、Tauri 和发布配置验证后再合并 `main`。GitHub Workflow、Artifact、Gitee Release 上传、双仓同步和差分生成脚本均已删除。

正式发布只接受 `origin/main` HEAD 上严格递增的 `vMAJOR.MINOR.PATCH` 标签。Windows Server 2016 Desktop Experience 或 Windows 10 的专用 checkout 运行 `scripts/windows-release.ps1`：管理员首次显式执行 `Setup` 安装或修复环境，计划任务日常只执行 `Publish`，不会自动升级工具链或重启电脑。完整的环境、密钥、计划任务、局域网分发和回滚说明见 [Windows 本机打包与局域网发布](docs/WINDOWS_LAN_RELEASE.md)。

## 权限与仓库原则

- `warehouse:read/write` 控制仓库与物品资料；`inventory:documents:read/write` 控制物品流水和四类出入库操作。
- `suppliers:read/write`、`customers:read`、`system:departments:read` 控制表单所需关联资料；缺少依赖权限时前端隐藏不可完成的操作，后端仍执行权限校验。
- `system:updates:read` 控制版本状态页面，`system:updates:write` 控制管理员立即检查；已有 write 角色在升级时自动补充 read。
- `cost:view` 独立控制采购单价、库存成本和金额，列表、详情、流水与统计接口均不得向无权限用户泄露成本字段。
- 所有新库存业务由服务端自动使用默认仓库，不接受用户选择任意仓库。每次只办理当前物品并立即过账。
- 采购入库关联供应商；退货返工可关联客户或部门及可选原出库记录；出库关联客户或目标部门。

## API 与开发约定

API 权威说明见 [docs/API.md](docs/API.md)，运行服务后也可访问 `/swagger/index.html`。修改 API 时必须同步 handler、测试、`docs/API.md`、`docs/docs.go`、`docs/swagger.json`、`docs/swagger.yaml` 和 `test.http`；Web 业务请求优先通过可替换的请求/传输接口复用，不在组件内新增平台专属网络实现。

本地完整校验：

```bash
go mod tidy
go vet ./...
go test ./...
cd web && npm ci && npm run build
cd ../client && npm ci && npm run build
cd src-tauri && cargo check --locked
```

## 验证状态

2026-08-30 已切换为 Gitee main + Windows 本机全量发布设计。仓库内静态检查和跨平台测试只证明代码与构建配置，不证明 Windows 正式可用；Server 2016 Desktop Experience/Windows 10 真机的 Setup、断网缓存、NSIS/Portable、计划任务、Service/普通进程重启、SQLite 回滚和局域网下载证据完整前，发布状态仍为“待真机验收”。

## 已知限制与路线图

- 首版为单组织、单仓库和局域网部署；当前发布协议不面向公网。
- SQLite 适合当前单机内网服务，部署前仍需建立可验证的备份与恢复流程。
- Tauri 的 Windows 10 运行态需要在真实系统持续回归；Server 2016 只承担构建、分发和服务端运行。

路线图：

1. P0：保持 Go、Web、Tauri 和 Windows 发布脚本验证全部绿色。
2. P1：完成现代浏览器和 Windows 10 Tauri 的运行态验收。
3. P1：完成内网部署安全检查、SQLite 备份恢复。
4. P2：统计导出、任务实时提醒和办公室/部门权限细化。
