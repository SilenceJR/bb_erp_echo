# 博邦 ERP 开发交接与进度文档

更新时间：2026-07-27

## 1. 项目目标

实现博邦 ERP 管理系统。当前阶段目标是先搭好可运行的后台框架和模块化单体目录，为后续客户、仓库、库存、物料、产品、模具、任务单、统计报表等模块逐步实现业务流程。

## 2. 当前技术栈

后端：

- Go 1.26.5
- Echo v5
- GORM
- SQLite WAL
- Casbin
- Koanf + ENV
- slog
- validator

前端：

- `web/`：Vue 3 + Vite Web 管理端
- `client/`：Tauri 桌面端，复用 Web 管理端业务入口

## 3. 当前目录说明

```text
bb_erp_echo/
├── cmd/server/              # 后端启动入口
├── internal/app/            # 应用装配层
├── internal/config/         # 配置加载
├── internal/database/       # SQLite/GORM 初始化
├── internal/logger/         # 文件化日志系统
├── internal/middleware/     # JWT、权限、审计、访问日志中间件
├── internal/shared/         # 通用错误、请求、分页、响应工具
├── internal/auth/           # 登录、JWT、当前用户上下文
├── internal/user/           # 用户账号管理
├── internal/role/           # 角色、权限、Casbin 策略
├── internal/department/     # 组织、部门、终端管理
├── internal/audit/          # 操作审计查询
├── internal/customer/       # 客户与联系人模块骨架
├── internal/warehouse/      # 仓库模块骨架
├── internal/inventory/      # 库存模块骨架
├── internal/material/       # 物料模块骨架
├── internal/product/        # 产品模块骨架
├── internal/mold/           # 模具模块骨架
├── internal/workorder/      # 任务单与部门子任务模块骨架
├── internal/statistics/     # 统计报表模块骨架
├── internal/model/          # 当前 GORM 模型集中定义
├── migrations/              # 预留迁移目录
├── data/                    # SQLite 本地数据目录
├── logs/                    # 本地日志目录
├── configs/                 # 预留配置目录
├── web/                     # Web 管理端
└── client/                  # Tauri 桌面端
```

说明：当前 GORM 模型集中放在 `internal/model/`，这是为了避免早期拆分时产生循环依赖。后续业务稳定后，可以再把模型逐步迁到对应业务模块。

## 4. 已完成功能

后端基础设施：

- Echo v5 HTTP 服务和优雅关闭。
- Koanf 默认配置和 `BB_ERP_` 环境变量覆盖。
- GORM + SQLite WAL 初始化。
- GORM `AutoMigrate` 自动建表。
- validator 请求校验。
- 统一 JSON 错误响应。
- 文件化结构日志：`app`、`access`、`error` 三类日志。

认证与权限：

- `POST /api/v1/auth/login`
- `GET /api/v1/auth/me`
- JWT Bearer 认证。
- Casbin 接口权限和组织/部门数据范围模型。
- 默认超级管理员角色。
- 默认部门终端操作员角色。

系统管理：

- 组织列表和创建。
- 部门列表和创建。
- 终端列表和创建。
- 用户列表、创建、启停、重置密码、绑定角色。
- 角色列表、创建、绑定权限。
- 权限列表。
- 操作审计查询。

身份模型：

- `personal`：个人账号，审计日志记录具体人员。
- `department_terminal`：部门终端账号，审计日志记录部门和终端，具体人员为“未知”。
- 示例终端账号场景：`injection-terminal-01`，注塑车间电脑01。

业务模块：

- 客户与联系人、仓库、库存、物料、产品、模具、任务单、统计报表目前已经注册骨架路由。
- 骨架路由支持读占位和写占位，用于前端菜单、权限和审计联调。

## 5. 日志系统

默认日志目录：

```text
logs/
```

文件格式：

```text
logs/app-YYYY-MM-DD.log
logs/access-YYYY-MM-DD.log
logs/error-YYYY-MM-DD.log
```

默认保留 30 天，启动时自动清理过期日志。

日志配置环境变量：

```text
BB_ERP_LOG_LEVEL=info
BB_ERP_LOG_DIR=logs
BB_ERP_LOG_CONSOLE=true
BB_ERP_LOG_RETENTION_DAYS=30
```

排查建议：

- 看服务启动、关闭、初始化问题：查 `app-*.log`
- 看接口请求、状态码、耗时：查 `access-*.log`
- 看统一错误和系统错误：查 `error-*.log`
- 看业务操作责任归属：查数据库表 `audit_logs`

## 6. 配置与环境变量

常用环境变量：

```text
BB_ERP_HTTP_HOST=127.0.0.1
BB_ERP_HTTP_PORT=8080
BB_ERP_HTTP_ALLOWED_ORIGINS=http://localhost:3000,http://localhost:5173
BB_ERP_DATABASE_PATH=data/erp.db
BB_ERP_JWT_SECRET=change-me-in-production
BB_ERP_JWT_EXPIRES_IN=24h
BB_ERP_ADMIN_USERNAME=admin
BB_ERP_ADMIN_PASSWORD=admin123456
BB_ERP_ADMIN_NAME=系统管理员
```

注意：

- `BB_ERP_JWT_SECRET` 和 `BB_ERP_ADMIN_PASSWORD` 在生产或正式测试环境必须覆盖。
- 默认数据库路径是 `data/erp.db`。
- `data/*.db` 和 `logs/*.log` 是运行产物，不应提交。

## 7. 当前 API 概览

公开接口：

```text
GET  /health
GET  /ready
POST /api/v1/auth/login
```

登录后接口：

```text
GET /api/v1/auth/me
```

系统接口：

```text
GET  /api/v1/system/organizations
POST /api/v1/system/organizations
GET  /api/v1/system/departments
POST /api/v1/system/departments
GET  /api/v1/system/terminals
POST /api/v1/system/terminals
GET  /api/v1/system/users
POST /api/v1/system/users
PATCH /api/v1/system/users/:id/status
POST /api/v1/system/users/:id/reset-password
POST /api/v1/system/users/:id/roles
GET  /api/v1/system/roles
POST /api/v1/system/roles
POST /api/v1/system/roles/:id/permissions
GET  /api/v1/system/permissions
GET  /api/v1/system/audits
```

业务骨架接口：

```text
GET  /api/v1/customers
POST /api/v1/customers
GET  /api/v1/warehouse
POST /api/v1/warehouse
GET  /api/v1/inventory
POST /api/v1/inventory
GET  /api/v1/material
POST /api/v1/material
GET  /api/v1/product
POST /api/v1/product
GET  /api/v1/mold
POST /api/v1/mold
GET  /api/v1/workorder
POST /api/v1/workorder
GET  /api/v1/statistics
POST /api/v1/statistics
```

兼容旧路径：

```text
GET  /api/v1/tasks
POST /api/v1/tasks
```

## 8. 当前测试状态

已覆盖：

- 应用初始化、健康检查、就绪检查、SQLite WAL。
- 登录成功、密码错误、当前用户信息。
- JWT 缺失、Casbin 无权限拒绝、组织数据边界。
- 个人账号与部门终端账号的审计差异。
- 默认日志配置、环境变量覆盖。
- 日志目录创建、三类日志写入、控制台和文件双输出。
- 按天日志切分和 30 天历史清理。

运行命令：

```bash
go test ./...
```

最近一次验证结果：通过。

## 9. 下一步建议

建议按这个顺序继续迭代：

1. 维护 `.gitignore`，确认不提交 `data/*.db`、`logs/*.log`、`node_modules/`、`dist/`、`target/`。
2. 给每个业务模块补 `model.go`、`repository.go`、`service.go`、`handler.go`，逐步从 `internal/model` 拆出业务模型。
3. 先实现客户与联系人模块 CRUD，因为它通常是销售、任务单和报表的前置数据。
4. 再实现物料、产品、仓库、库存的基础数据与库存流水。
5. 然后实现模具档案、任务单、部门子任务。
6. 最后做统计报表和前端页面联调。

## 10. 给下一位 Codex 的注意事项

- 代码注释必须使用中文，公开结构体、函数、参数和非显然业务规则都要说明。
- 不要把部门终端账号当作具体人员账号；审计里具体人员必须是“未知”。
- 不要绕过 Casbin 权限中间件新增后台接口。
- 业务写操作必须走审计中间件。
- 当前 `client/` 和 `web/` 目录已有前端内容，后续改 API 时要同步检查前端接口路径。
- 本地数据库和日志只是运行产物，不代表业务迁移文件。
