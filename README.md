# 博邦 ERP

博邦 ERP 是面向工厂内网使用的单组织、单仓库 ERP。后端采用 Go 1.27、Echo v5.3.1、GORM、SQLite WAL、Casbin 和标准库 `log/slog`；Web 端采用 Vue 3、TypeScript、Vite 与 Element Plus；桌面端通过 Tauri 复用同一套 Vue 界面和业务 API。

首版部署在内网电脑上，浏览器和桌面客户端通过服务端局域网 IP 访问。系统保留切换公网 HTTPS 地址的能力，不为旧版操作系统、IE 或浏览器兼容模式提供适配；桌面端支持现代 macOS 和 Windows 10/11。

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
- Windows 服务端、桌面客户端、全量便携包和升级包的 CI 构建流程。

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
```

## 本地启动

需要 Go 1.27、Node.js 22、npm；开发桌面端还需要 Rust 和当前平台的 Tauri 依赖。

后端：

```bash
go run ./cmd/server
```

也可使用 Air：

```bash
air
```

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
```

## 权限与仓库原则

- `warehouse:read/write` 控制仓库与物品资料；`inventory:documents:read/write` 控制物品流水和四类出入库操作。
- `suppliers:read/write`、`customers:read`、`system:departments:read` 控制表单所需关联资料；缺少依赖权限时前端隐藏不可完成的操作，后端仍执行权限校验。
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

2026-08-26 已完成 Go 1.27 / Echo v5.3.1 迁移，并通过 `go mod tidy` 差异检查、`go vet ./...`、`go test ./...`、Web 与 Tauri 前端生产构建及 `cargo check --locked`。真实浏览器已验证登录、权限导航、仓库物品新增、详情抽屉、四类出入库入口、图片上传、Bearer Blob 预览、替换、删除确认保护，以及桌面/移动响应式切换；图片删除接口和四类业务归属边界由 Go 测试覆盖。远端 CI 在下一次推送成功前标记为“待复验”。构建可能提示前端主包体积较大以及 `@vueuse/core` 注释位置，但不影响产物生成。

## 已知限制与路线图

- 首版为单组织、单仓库和局域网部署；公网 HTTPS 只保留扩展能力。
- SQLite 适合当前单机内网服务，部署前仍需建立可验证的备份与恢复流程。
- Tauri 的 Windows 10/11 与 macOS 运行态需要在对应真实系统持续回归。

路线图：

1. P0：恢复 Go、Web、Tauri 和 CI 全部绿色。
2. P1：完成现代浏览器、Windows 10/11 Tauri、macOS Tauri 的运行态验收。
3. P1：完成内网部署安全检查、SQLite 备份恢复。
4. P2：统计导出、任务实时提醒和办公室/部门权限细化。
