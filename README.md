# 博邦 ERP

博邦 ERP 是面向工厂内网使用的单组织、单仓库 ERP。后端采用 Go 1.27、Echo v5.3.1、GORM、SQLite WAL、Casbin 和标准库 `log/slog`；Web 端采用 Vue 3、TypeScript、Vite 与 Element Plus；桌面端通过 Tauri 复用同一套 Vue 界面和业务 API。

首版部署在内网电脑上，浏览器和桌面客户端通过服务端局域网 IP 访问。系统保留切换公网 HTTPS 地址的能力，不为旧版操作系统、IE 或浏览器兼容模式提供适配；桌面端支持现代 macOS(测试) 和 Windows 10/11(主要)。

## 已实现范围

- JWT 登录、个人/部门终端账号、Casbin 权限、角色、审计与结构化日志。
- 客户、联系人和供应商档案。
- 单仓库物品工作台，按产品、生产物资、常规产品、生活物资分类。
- 物品详情内办理采购入库、退货返工入库、客户出库、部门出库，支持立即过账、幂等、库存不足校验、冲销和移动加权平均成本。
- 模具台账、借出归还、维修保养及履历。
- 生产/通用任务单、多部门子任务流转、状态日志和办公室确认。
- 库存、任务、模具、审计等统计报表。
- 产品、模具、任务单和部门子任务图片的受保护上传、预览、替换与删除。
- Web 同源请求与 Tauri Rust HTTP 传输抽象，可动态保存内网或公网服务地址。
- 服务启动异步检查更新、周期检查、客户端安装包校验缓存，以及管理员“版本与更新”页面。
- Gitee 主仓库、GitHub 只读构建镜像和公开 Gitee Release 的 Windows 发布闭环。

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

默认管理员为 `admin` / `admin123456`。正式测试或部署前必须覆盖管理员密码和 JWT 密钥。

## 内网部署与网络方案

服务端默认监听 `0.0.0.0:8080`。本机可访问 `http://127.0.0.1:8080`，其他内网电脑应访问服务端的局域网 IP，例如 `http://192.168.1.20:8080`，并在操作系统防火墙放行 `8080/TCP`。

Web 生产环境由 Go 托管静态文件并使用同源相对路径。Tauri 通过 `HttpTransport` 接口选择 Rust HTTP 插件发送请求，避免依赖 macOS/Windows WebView 的跨域实现；业务 API 不感知运行平台。登录页和顶栏均可保存服务地址，后续部署公网时可直接切换为 HTTPS 源地址。详见 [client/API_SYNC.md](client/API_SYNC.md)。

常用环境变量：

```text
BB_ERP_HTTP_HOST=0.0.0.0
BB_ERP_HTTP_PORT=8080
BB_ERP_HTTP_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:5173
BB_ERP_DATABASE_PATH=data/erp.db
BB_ERP_LOG_DIR=logs
BB_ERP_FILES_ROOT_DIR=static/uploads
BB_ERP_WEB_ENABLED=true
BB_ERP_WEB_DIST_DIR=web/dist
BB_ERP_JWT_SECRET=change-me-in-production
BB_ERP_ADMIN_USERNAME=admin
BB_ERP_ADMIN_PASSWORD=change-me-in-production
BB_ERP_UPDATE_ENABLED=false
BB_ERP_UPDATE_MANIFEST_URL=https://gitee.com/SilenceJR/bb_erp_releases/raw/main/update-manifest.json
BB_ERP_UPDATE_CACHE_DIR=updates
BB_ERP_UPDATE_CHECK_INTERVAL=6h
BB_ERP_UPDATE_MANIFEST_TIMEOUT=20s
BB_ERP_UPDATE_DOWNLOAD_TIMEOUT=10m
# `tauri signer generate` 生成的整个 .pub 文件内容，或改用下方文件配置
BB_ERP_UPDATE_SIGNING_PUBLIC_KEY=
BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE=update-public.key
```

更新检查在服务启动后异步执行，失败不会阻止 ERP 启动，之后默认每 6 小时检查一次。旧版客户端 ZIP 保持兼容；v2 客户端资源还会验证签名 payload、文件 Minisign 签名、大小和 SHA-256，再按哈希原子写入缓存。服务端新版本只报告和提供下载，不会自动覆盖正在运行的服务。

Windows 10/11 x86_64 桌面端支持“上一版 → 最新版”的 zstd EXE 增量更新。来源版本、当前 EXE 哈希或布局不匹配时直接使用完整包；增量下载、校验或应用失败时自动切换签名 NSIS/便携 EXE，不要求用户再次确认。浏览器端不执行桌面安装，完整 ZIP 仅作为管理员故障恢复入口。`v0.1.0-rc.4` 是首次全量基线，`v0.1.0-rc.5` 用于验证首个真实差分；`rc.3` 及更旧客户端必须先完整安装 `rc.4`。

## 分支、CI 与发布

Gitee 是日常开发和标签发布的主仓库，GitHub 只接收分支与标签的 Push 镜像并运行 Actions。功能开发采用 `codex/<主题>` 分支，通过 Go、Web、Tauri 和相关发布配置验证后再合并主分支。

任意分支 push 和 GitHub PR 只执行验证；`main`/`master` 不生成 Windows 正式包；手动触发只生成保留 14 天的临时 Artifact。只有符合 `vMAJOR.MINOR.PATCH[-prerelease]` 的 Gitee 标签经镜像到 GitHub 后，才会构建并发布到独立的公开 Gitee 发布仓库。正式标签同时生成签名 NSIS、便携 EXE及满足阈值时的 zstd 差分；发布任务先匿名复验 manifest 声明的全部附件，最后才更新稳定 manifest。

仓库拓扑、remote 切换、Secrets/Variables、最小权限和首次预发布验收见 [docs/GITEE_RELEASE.md](docs/GITEE_RELEASE.md)。当前 Gitee 源码主仓库为 `SilenceJR/bb_erp_echo`，公开发布仓库为 `SilenceJR/bb_erp_releases`。

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

2026-08-26 已完成 Go 1.27 / Echo v5.3.1 迁移，`v0.1.0-rc.3` 已跑通 Gitee 主仓库、GitHub Windows 构建和 Gitee Release。Windows 增量升级分支已通过 `go mod tidy`、`go vet ./...`、`go test ./...`、Web/Client 生产构建、Rust 11 项测试、`cargo check --locked`、真实 Tauri 格式签名 smoke、发布脚本语法和 Workflow YAML 校验；独立 UI 审查结果为 P0/P1/P2 均为 0。`rc.4`/`rc.5` 尚需在签名私钥离线备份确认后发布，并在 Windows 10/11 真机完成 NSIS/便携安装、断网、磁盘不足、不可写目录、崩溃与 90 秒回滚验收。构建可能提示前端主包体积较大以及 `@vueuse/core` 注释位置，但不影响产物生成。

## 已知限制与路线图

- 首版为单组织、单仓库和局域网部署；公网 HTTPS 只保留扩展能力。
- SQLite 适合当前单机内网服务，部署前仍需建立可验证的备份与恢复流程。
- Tauri 的 Windows 10/11 与 macOS 运行态需要在对应真实系统持续回归。

路线图：

1. P0：恢复 Go、Web、Tauri 和 CI 全部绿色。
2. P1：完成现代浏览器、Windows 10/11 Tauri、macOS Tauri 的运行态验收。
3. P1：完成内网部署安全检查、SQLite 备份恢复。
4. P2：统计导出、任务实时提醒和办公室/部门权限细化。
