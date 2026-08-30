# Go 后端状态、进度与维护台账

> 基准日期：2026-08-30
> 适用范围：`cmd/`、`internal/`、Go 测试、配置、部署与 Windows 发布支撑。
> “已实现”不等于“Windows 真机已验收”；未取得目标系统证据的项目必须明确标为待验收。

## 当前结论

Go 后端已经形成可运行的模块化单体：Echo HTTP 服务、SQLite WAL、JWT/Casbin、部门与员工、客户/供应商、库存、模具、任务单、统计、受保护图片和更新 API 均已接入应用装配。

发布链路已切换为 Gitee `main` + Windows 本机脚本 + 局域网完整包。服务端读取本机 stable manifest，不请求外部发布站点或自身 HTTP 地址；Server 2016 Desktop Experience 只承担构建、分发和服务端运行，不作为主要 Web/Tauri 使用电脑。旧双仓、云端 Artifact、Release 上传和差分生成已删除。

当前不能称为正式可用。仓库测试通过只能证明代码层结果；Server 2016/Windows 10 的环境安装、打包、服务重启、SQLite 回滚和局域网更新仍需真机证据。

## 进度总表

| 领域 | 状态 | 当前说明 |
| --- | --- | --- |
| 服务启动与配置 | 已完成 | 环境变量覆盖、健康/就绪检查、优雅关闭、Web 静态资源和更新服务已装配。 |
| SQLite 数据层 | 已完成 | 自动迁移、WAL、外键、单写连接、业务模型和 refresh 会话已接入。 |
| 登录、权限与审计 | 已完成 | JWT/refresh 轮换、Casbin、组织/部门边界、账号与实际操作员工快照已实现。 |
| 客户/供应商/仓库/库存 | 已完成 | 主数据、四类出入库、过账、冲销、幂等、库存不足和成本权限已实现。 |
| 模具/任务单/统计 | 已完成 | 生命周期、部门子任务、办公室确认、业务履历和统计聚合已实现。 |
| 图片文件 | 已完成 | 受保护单图/多图、原子批量保存、预览、替换和删除已实现。 |
| 本地更新源 | 已实现，待真机 | `BB_ERP_UPDATE_MANIFEST_FILE` 读取 `updates/stable/update-manifest.json`；正式运行不发起公网 manifest 请求。 |
| 客户端 v3 完整包 | 已实现，待真机 | payload 绑定类型、大小、SHA-256 和 Minisign；资源从本地内容寻址目录经受控 API 分发，`deltas` 为空。 |
| 服务端升级事务 | 已实现，待真机 | updater 使用可耐受半写入的追加式事务日志，持久化备份程序、Web、公钥、版本、stable manifest 和 SQLite/WAL/SHM；验证失败或下次计划任务启动时恢复旧版本。 |
| Windows 环境/发布脚本 | 已实现，待真机 | PowerShell 5.1 `Doctor/Setup/Publish`、工具链锁、正式标签门禁、全量构建和计划任务入口已提供。 |
| OpenAPI 与维护文档 | 已同步 | Windows 局域网 v3 更新接口、Swagger/Markdown、状态、架构与 `test.http` 已同步；后续 API 行为变化仍须同步维护。 |
| 生产运行态验收 | 待完成 | 真实账号、权限矩阵、内网访问、计划任务、更新与异常恢复尚未闭环。 |

## 更新与发布契约

- `BB_ERP_UPDATE_MANIFEST_FILE` 默认指向安装目录内 `updates/stable/update-manifest.json`。
- `/api/v1/updates/client/plan` 只为新版本返回完整 NSIS 或 Portable 计划；正式桌面自动更新固定请求 Portable，NSIS 只用于首次安装或人工恢复，不生成或选择差分。
- `/api/v1/updates/client/artifacts/{sha256}` 只按已验证清单中的哈希提供资源，禁止目录列表和任意路径读取。
- 服务端完整 ZIP、客户端 NSIS/Portable 和人工恢复 ZIP 均通过可信公钥验证签名、大小、SHA-256；ZIP 同时验证路径安全和必需文件。
- updater 必须精确停止目标 Windows Service 或安装目录普通进程。Service 模式先验证服务 EXE 指向目标 `bb-erp-server.exe`。
- 新服务启动后验证 `/ready`、`/api/v1/version` 目标版本和客户端完整包计划；任一步失败均恢复旧程序、数据库和 stable manifest，再验证旧服务。
- 发布脚本启动即从进程环境清除签名变量，仅为单次 signer 子进程短暂注入私钥和密码；依赖恢复、构建、updater 和新服务均不继承私钥。

## 当前待完成

1. 在 Server 2016 Desktop Experience 和 Windows 10 x64 分别验证全新 Setup、正确环境复用、错误版本、无管理员权限、3010 重启提示与断网缓存。
2. 验证正式标签、无标签跳过、RC、版本倒退、移动标签、脏 checkout、并发和中途终止。
3. 在 Windows Service 与普通进程两种模式验证成功升级，以及数据库迁移失败、端口占用、错误签名、损坏 ZIP、磁盘不足、启动超时和完整回滚。
4. 验证 NSIS、Portable、服务端 ZIP 的版本、大小、SHA-256、Minisign 和 ZIP 路径安全。
5. 完成真实业务账号、权限、库存、模具、任务、图片、备份恢复和长时间运行验收。

## 已知风险

| 风险 | 当前控制 |
| --- | --- |
| 默认管理员密码未修改 | 首次部署后必须在系统内修改；生产验收记录账号处理结果。 |
| SQLite 不适合多实例高并发 | 首版固定单机服务；扩展前重新评估数据库和文件存储。 |
| 安装器或工具链漂移 | `Doctor` 精确验证锁文件；只有管理员显式 `Setup` 可修改工具链，`Publish` 只失败提示。 |
| 私钥泄露 | 计划任务专用账号、环境变量、目录 ACL；签名后清除子进程继承值，私钥不得入库。 |
| 更新中断破坏数据 | staging 完成后才切换；备份逐文件 Sync 并校验，追加式阶段日志保留最后完整记录，固定 recovery updater 在下一次环境/网络检查前恢复；仍需 Windows 断电真机验收。 |
| 本地测试冒充 Windows 验收 | 验证记录分开报告，未取得真机证据不得标记正式可用。 |

## 验证记录

本次 Windows 局域网发布改造应执行并记录：

```text
go mod tidy 差异检查
go vet ./...
go test ./...
web: npm ci && npm run build
client: npm ci && npm run build
client/src-tauri: cargo fmt --check, cargo check --locked, cargo test --locked
Windows PowerShell 5.1 Doctor/Setup/Publish 真机用例（待执行）
```

2026-08-30 本机验证已通过：`go mod tidy -diff`、`go vet ./...`、`go test -count=1 ./...`、Web/Client 生产构建、Rust `fmt/check/test --locked`（18 项 Rust 测试）和 `git diff --check`。事务测试覆盖半写入日志、`backed_up/installed/activated/started` 回滚、SQLite 主文件与 WAL/SHM、首次安装有/无预置数据库及重复恢复。前端构建仅有既有的大 chunk 与依赖注释提示，不是构建失败。

非 Windows 开发机无法证明 PowerShell 安装器、MSVC/SDK、WebView2、NSIS、Windows Service、进程停止、重启和回滚行为。最终交付必须单独列出这些未执行项。

## 文档同步规则

1. Go API、权限、字段或错误行为变化，同步 Handler Swagger 注释、`docs/API.md`、`docs/docs.go`、`docs/swagger.json`、`docs/swagger.yaml`，必要时更新 `test.http`。
2. 配置、数据库、部署或发布行为变化，同步 README、本文档、`docs/WINDOWS_LAN_RELEASE.md`、架构图和客户端同步文档。
3. Web/Tauri 共用 API 变化，同时检查 `docs/WEB_STATUS.md` 和 `client/API_SYNC.md`。
4. 代码、对应文档和验证证据放在同一工作项和同一提交；个人 `.codex` 配置不纳入业务提交。

## 相关文档

- [Windows 本机打包与局域网发布](WINDOWS_LAN_RELEASE.md)
- [API](API.md)
- [Go 后端产品架构与业务时序图](BACKEND_ARCHITECTURE.md)
- [Web 与 Tauri Client 产品架构与交互时序图](WEB_CLIENT_ARCHITECTURE.md)
