# 博邦 ERP

博邦 ERP 是面向工厂局域网的单组织、单仓库系统。Go/Echo/SQLite 负责业务、权限和数据；Vue/Element Plus 提供共用业务界面；Windows 10/11 Tauri 客户端是正式入口，桌面 Web 是备用入口。

当前按全新部署设计，不迁移或兼容旧数据库、旧 API、旧客户端和旧页面；不支持公网、系统代理、macOS 或移动端原生客户端。Windows/Tauri 与桌面 Web 是主入口，平板和手机仅支持低频 Web 访问适配。主要验收环境为 1920×1080，Tauri 窗口最小尺寸为 1024×680。

后端只提供当前 canonical API：任务单 `/api/v1/workorder`、物料 `/api/v1/materials`、产品 `/api/v1/products`、模具 `/api/v1/molds`，库存使用 `/api/v1/inventory-documents`、`/api/v1/inventory-balances` 和 `/api/v1/inventory-ledgers`；旧任务、单数基础资料和 `/api/v1/inventory` 路径不再注册。

## 主要能力

- 客户编码与多资料、Excel 预览导入和分页导出。
- 部门、员工多部门成员关系、终端、账号、角色和权限。
- 物料、产品、供应商、仓库、库存单据、过账、冲销、幂等和移动加权平均成本。
- 生产/通用任务、部门处理、部分完成、待结案和强制完成。
- 模具借出、归还、维修、保养与履历。
- 操作员工 + 登录账号 + 终端双重责任审计。
- 统计报表、受保护图片和 Windows 客户端更新。
- 客户端启动动画、局域网自动发现、服务身份验证、原生文件保存和未保存离开保护。

## 客户端优先架构

```text
Windows Tauri                         桌面 Web
  Rust 平台适配器                       浏览器适配器
        \                                 /
         Connection / Discovery / FileSave
         WindowLeave / ClientUpdate
                       |
              Vue 共用业务与 UI
                       |
               Go API / 权限 / 状态机
                       |
                    SQLite
```

业务页面、DTO、权限、校验和错误语义只维护一套 Vue 实现。局域网发现、连接验证、原生保存、窗口生命周期和客户端更新才属于 Tauri/Rust。

客户端启动时向每个私网 IPv4 网卡的 UDP `39080` 广播。候选必须通过 nonce、来源 IP、协议、产品、UUID、`GET /ready` 和 `GET /api/v1/discovery/identity` 校验。唯一实例自动连接，多实例要求明确选择，未发现时保留手动内网 IPv4 连接。

Go 服务启动前执行同协议预检；发现另一有效实例时拒绝启动。这是内网最佳努力约束，不是跨 VLAN、防火墙或同时启动竞态下的绝对分布式锁；客户端仍会在多实例时阻止静默连接。

## 目录

```text
cmd/server/                Go 服务入口
internal/                  领域模块、权限、发现、更新
web/                       共用 Vue 业务与桌面 Web
client/                    Windows Tauri 壳和 Rust 平台能力
docs/                      API、架构、产品、状态和用户文档
scripts/                   发布与维护脚本
```

关键文档：

- [客户端优先整改设计](docs/CLIENT_FIRST_REMEDIATION.md)
- [Web/Tauri 架构](docs/WEB_CLIENT_ARCHITECTURE.md)
- [Web/Tauri 状态](docs/WEB_STATUS.md)
- [API 说明](docs/API.md)
- [后端状态](docs/BACKEND_STATUS.md)
- [首次使用帮助](docs/USER_GUIDE.md)
- [客户端 API 同步](client/API_SYNC.md)

## 本地启动

需要 Go 1.27、Node.js 22、npm；Windows 客户端构建还需要 Rust 和 Tauri 对应平台依赖。

启动 Go：

```bash
go run ./cmd/server
```

默认监听 `0.0.0.0:8080`，数据库为 `data/erp.db`，Web 静态目录为 `web/dist`。

启动 Web 开发服务：

```bash
cd web
npm install
npm run dev
```

启动 Tauri 开发客户端：

```bash
cd client
npm install
npm run desktop:dev
```

首次登录账号为 `admin` / `admin123456`。登录后应立即修改密码，再创建部门、员工、终端、账号和角色。

服务器 Windows 专用网络需要放行 ERP TCP 端口（默认 `8080`）和发现 UDP 端口 `39080`。UDP 被阻断时仍可手动连接。

## 常用配置

配置使用 `BB_ERP_` 环境变量覆盖，例如：

```bash
BB_ERP_HTTP_HOST=0.0.0.0
BB_ERP_HTTP_PORT=8080
BB_ERP_DATABASE_PATH=data/erp.db
BB_ERP_LOG_DIR=logs
BB_ERP_FILES_ROOT_DIR=static/uploads
BB_ERP_DISCOVERY_ENABLED=true
BB_ERP_DISCOVERY_SERVER_NAME=生产服务器
BB_ERP_DISCOVERY_PORT=39080
```

连接范围仅允许 loopback 与 RFC1918 IPv4 的 HTTP 地址。

## 验证

```bash
go test -count=1 ./...
go vet ./...
cd web && npm run build
cd ../client && npm run build
cd src-tauri && cargo fmt --check && cargo check --locked && cargo test --locked
```

本地自动化与浏览器检查不能替代 Windows 10/11 真机验收。交付前还需在 1920×1080 的 100%/125%/150% 缩放下验证发现、连接、登录、权限、文件保存、窗口关闭、更新和核心业务流程。

## 分支与提交

功能、修复和发布工程变更在 `codex/<主题>` 分支完成验证后再合并主分支。代码变化必须同步相关 API、Swagger、Web/Tauri 和状态文档；提交时只纳入本工作项文件，不覆盖维护者的本地改动。
