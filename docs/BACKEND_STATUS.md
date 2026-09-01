# Go 后端状态

> 基准日期：2026-09-01

Go 后端是博邦 ERP 的业务、权限、审计和数据最终裁决者。当前按全新内网部署实现，不为旧数据库、旧 API 或旧客户端保留迁移/兼容分支。

## 1. 模块状态

| 模块 | 状态 | 说明 |
| --- | --- | --- |
| 应用装配 | 已完成 | Echo、日志、SQLite WAL、迁移、路由、静态 Web 与优雅关闭 |
| 认证授权 | 已完成 | JWT、活跃会话续期、原子权限快照 provider、角色与权限、密码修改 |
| 组织人员 | 已完成 | 单组织、部门、员工多部门关系、终端、账号归属 |
| 操作责任 | 已完成 | 任务与库存写操作必须显式选择当前部门有效员工，并保存账号/终端/部门快照 |
| 客户 | 已完成 | 客户编码、多资料、Excel 预览令牌、原子导入、分页导出 |
| 基础资料 | 已完成 | 供应商、物料、产品、仓库与库位 |
| 库存 | 已完成 | 定点数量、单据、过账、冲销、禁止负库存、幂等、移动加权平均成本 |
| 任务单 | 已完成 | 生产/通用任务、部门任务、部分完成、待结案、暂停/恢复、加急和强制完成 |
| 模具 | 已完成 | 借出、归还、维修、保养、履历和状态约束 |
| 统计审计 | 已完成 | 统计聚合、成本权限与审计查询 |
| 文件图片 | 已完成 | 受保护上传、批量图片、替换和删除 |
| 客户端更新 | 已完成 | Windows full-only 更新、内网同源代理、签名、哈希、临时文件与失败恢复 |
| 局域网发现 | 已完成 | SQLite 稳定身份、匿名身份接口、UDP 39080、启动预检和 responder 生命周期 |

## 2. 局域网发现

服务身份首次创建时生成 RFC 4122 v4 UUID，并以固定单例记录保存在 SQLite。服务名默认使用主机名，可通过 `BB_ERP_DISCOVERY_SERVER_NAME` 覆盖；版本变化不会更换实例 ID。

匿名接口：

```text
GET /api/v1/discovery/identity
```

只返回：

```json
{
  "product": "bb-erp",
  "discovery_protocol": 1,
  "instance_id": "稳定 UUID",
  "server_name": "服务器名称",
  "server_version": "版本"
}
```

UDP 协议：

- 固定端口 `39080`，单包最多 512 字节。
- 严格拒绝未知字段、重复 JSON key、尾随 JSON、错误 nonce、协议和产品。
- 只接受 loopback/RFC1918 IPv4 来源，并按来源限速且限制缓存条目。
- 服务端启动预检按私网网卡广播，候选必须通过 `/ready` 与身份接口；任意已验证 peer 都会阻止当前实例启用。
- 身份响应设置 `Cache-Control: no-store`；客户端严格拒绝未知 DTO 字段，服务名/版本最多 120/64 字节且拒绝控制字符。
- 预检先收集最多 24 个去重候选，再以最多 4 个并发任务验证；默认 UDP 收集 2.5 秒，整个预检受 3 秒全局截止时间约束。
- HTTP 验证不使用代理、不跟随重定向，并限制超时和响应体。
- Tauri HTTP 插件 scope 使用 URL Pattern 正则覆盖 `127/8`、`10/8`、`172.16/12` 与 `192.168/16`，使已验证的内网服务可承接登录及同源业务请求；该配置不授权 HTTPS、公网或任意域名。
- HTTP 监听进入接收循环后才启动 UDP responder；关闭顺序为 discovery → HTTP → DB → logs。
- 预检是最佳努力约束。UDP 被阻断或同时启动竞态仍可能漏检，客户端多实例冲突页与端口占用是后续保护。

配置：

```text
BB_ERP_DISCOVERY_ENABLED
BB_ERP_DISCOVERY_SERVER_NAME
BB_ERP_DISCOVERY_BIND_HOST
BB_ERP_DISCOVERY_PORT
BB_ERP_DISCOVERY_SCAN_TIMEOUT
BB_ERP_DISCOVERY_PREFLIGHT_TIMEOUT
BB_ERP_DISCOVERY_HTTP_TIMEOUT
```

服务器 Windows 专用网络应放行 ERP TCP 端口和 UDP `39080`。

## 3. API 与权限约束

- `/health` 只表示进程存活；`/ready` 同时检查数据库。
- 前端权限隐藏只用于体验，Handler/Service/事务层仍必须验证权限和业务状态。
- 任务与仓库/库存写操作必须提交 `operator_employee_id`；服务端在事务内重新验证账号当前部门、组织、部门/员工状态和成员关系。
- 库存幂等键绑定接口范围、账号、组织、规范化请求和操作员工；不匹配返回 `409`。
- 数量使用四位定点整数，金额/单价使用分；禁止把前端浮点值直接作为库存或金额事实。
- 图片权限继承业务对象权限；身份发现接口不得返回组织、账号、业务数据、更新地址或凭据。
- API 只保留当前 canonical 路径：任务单 `/api/v1/workorder`、物料 `/api/v1/materials`、产品 `/api/v1/products`、模具 `/api/v1/molds`、仓库管理 `/api/v1/warehouses`，以及库存单据/余额/流水和 `/api/v1/warehouse/items`、`/tabs` 路径；不注册旧任务、单数基础资料、`/api/v1/inventory` 或 `/api/v1/warehouse` 根别名。图片权限只校验当前业务对象权限。
- 新库直接由 GORM schema 创建非空幂等键部分唯一索引；账号和 JWT 的 `password_version` 明确从 `1` 开始，不执行旧库字段/索引修复。
- 管理员重置账号密码在同一事务递增 `password_version` 并撤销目标账号全部 refresh token，旧 access/refresh 会话均失效。
- 权限编码绑定采用 fail-closed：任何缺失或拼写错误都会失败，不会把空查询结果解释为全量授权；部门终端账号的用户写入与默认角色绑定同一事务，提交后同步刷新 Casbin 策略，绑定失败完整回滚，策略刷新失败返回 `503` 而不虚报可用；外层 update manifest 拒绝重复 JSON key、未知字段和尾随 JSON。
- 权限刷新先在临时 Casbin 引擎中构建数据库完整快照，全部成功后原子切换；HTTP 权限中间件、文件权限和角色服务禁止直接读写当前引擎。并发 `Enforce` 期间不会读到清空或半成品策略，构建失败保留上一份可用快照。

## 4. 本轮验证

已通过：

- `go test -count=1 ./...`
- `go vet ./...`
- `go test -race ./internal/discovery ./internal/config ./internal/app`
- `go test -race ./internal/update`
- `cargo fmt --check`、`cargo check --locked --all-targets`、`cargo test --locked`
- Web 与 Tauri 前端生产构建；主入口均低于 500 kB
- Windows 日常 CI 增加 `windows-latest` Rust 检查与测试，不再只在标签发布时编译 Windows 条件代码
- 发布签名使用规范化客户端公钥执行真实文件验签；密钥不匹配与载荷篡改均失败关闭
- discovery 协议、重复字段、UUID、身份持久化、广播/候选验证、重定向、响应体、限速和生命周期测试
- discovery 慢候选并发验证、候选上限、身份 DTO 严格字段/文本校验、`no-store` 和并发 Start/Shutdown 竞态回归测试
- Tauri RFC1918 HTTP scope 回归测试：允许 loopback、`10/8`、`172.16/12`、`192.168/16`，拒绝公网、超出 `172.16/12` 的地址和 HTTPS。
- Swagger `docs.go`、JSON、YAML 同步
- 真实 app 集成验证部门终端账号通过 `POST /api/v1/system/users` 创建后可直接登录；`workorder` 读写和 `warehouse` 读取成功，`warehouse` 写入按默认角色返回 `403`，测试未手工调用 `ReloadPolicies`。
- 权限 provider 并发 `Enforce` + `ReloadPolicies` 无竞态；注入快照构建失败时旧权限仍可用。
- `git diff --check`
- 旧 API 路由、tasks/inventory 旧权限、图片权限回退、旧幂等索引升级和旧用户表密码版本迁移测试均已删除。
- `v0.0.10` 至 `v0.0.12` 按维护者要求设为 GitHub Actions 仅构建版本：完整 Windows 打包、签名和 Artifact 保存照常执行，`publish-gitee` 明确跳过，不向 Gitee 上传成品且不更新稳定 manifest。
- `v0.0.11` 标签对应 GitHub Actions `#91`（run `33458004272`）已成功：Go、Web、Tauri 前端、通用/Windows Rust 和完整 Windows 发布包作业全部通过，五组 GitHub Artifact 已生成；Gitee 发布作业为 `skipped`，`bb_erp_releases` 无 `v0.0.11` Release，稳定 manifest 保持 `0.0.9`。
- `v0.0.12` 待标签推送后由 GitHub Actions 执行完整打包；在获得实际作业结果前，不将 Artifact、Gitee 跳过状态或 Windows 真机验收记为已完成。

## 5. 待完成

- Windows 10/11 真机防火墙、零/一/多服务、UDP 阻断和同时启动场景。
- 自动发现或手动验证后，在 Windows 桌面端登录并请求至少一个已认证 API 的真实局域网验收。
- 完整管理员/办公室/仓库/部门/只读权限矩阵。
- 客户、库存、任务、模具在断网、并发冲突、重复提交和磁盘异常下的端到端验收。
- 安装目录、数据库、上传、日志和更新缓存的权限与备份恢复演练。

这些外部项目不得由单元测试、本机浏览器或非 Windows 构建冒充完成。

## 6. 文档同步规则

- API、权限、字段或错误语义变化时同步 Handler Swagger 注释、`docs/API.md`、Swagger 三份产物和必要的 `test.http`。
- Web/Tauri 共用 API 或运行约束变化时同步 `docs/WEB_STATUS.md`、`client/API_SYNC.md` 和构建规则。
- 每次代码变更记录实际执行的验证，不把未执行项目写成完成。
