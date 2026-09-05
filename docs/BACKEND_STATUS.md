# Go 后端状态

## 本轮 UI 整改联验（2026-09-05）

本次仅调整共享前端及说明文档，没有修改 Go API、权限码、初始化账户或迁移契约，故无需重新生成未变化的 OpenAPI。`go test ./...` 已通过。浏览器隔离合成样本已检查新库暂缓模块不可用展示与统计降级；真实样本权限矩阵、已有业务表完整业务操作及 Windows 客户端联验仍待完成。不能把单元测试当作生产验收。最新问题、修复和证据见 [Astra 验收记录](ASTRA_ACCEPTANCE.md)。

> 基准日期：2026-09-05

## 本轮角色权限配置（2026-09-05）

角色权限替换继续沿用原有请求、响应和权限编码契约；后端新增内部固定角色注册表，当前仅定义 `super_admin`。固定角色权限由服务端维护并拒绝通过角色权限替换接口修改，普通角色仍可在事务内原子替换；操作者有效权限、组织边界、非法/重复 ID 和策略刷新行为保持原有安全约束。未新增 API 字段、数据库迁移或公开固定角色接口。

Go 后端是博邦 ERP 的业务、权限、审计和数据最终裁决者。当前按全新内网部署实现；本轮对已有数据库只保留非破坏、幂等的角色与暂缓模块兼容，不恢复旧 API 或旧客户端双轨。

## 1. 模块状态

| 模块 | 状态 | 说明 |
| --- | --- | --- |
| 应用装配 | 已完成 | Echo、日志、SQLite WAL、迁移、路由、静态 Web 与优雅关闭 |
| 认证授权 | 已完成 | JWT、活跃会话续期、原子权限快照 provider、角色与权限、密码修改 |
| 组织人员 | 已完成 | 单组织、部门、员工多部门关系、终端、账号归属 |
| 操作责任 | 已完成 | 任务与库存写操作必须显式选择当前部门有效员工，并保存账号/终端/部门快照 |
| 客户 | 已完成 | 客户编码、多资料、Excel 预览令牌、原子导入、分页导出 |
| 基础资料 | 部分暂缓 | 物料、产品等共享基础表继续创建；新数据库暂不创建供应商、仓库和库位表，已有表与数据不删除 |
| 库存 | 数据结构暂缓 | 页面和 API 保留；新数据库不自动建库存表，访问返回统一 `503 module_not_initialized`，已有表时继续工作 |
| 任务单 | 数据结构暂缓 | 页面和 API 保留；新数据库不自动建任务/部门任务/流转表，写操作不可用，已有表时继续工作 |
| 模具 | 已完成 | 新模具、固定位置、图片分组/排序、DWG 文件、资料包导入导出和批量移位；模板下载为可直接回导的 `博邦模具导入模板.zip`，包含 `molds.xlsx`、`locations.json` 和标准空目录；资料包上限 2 GiB；不保留旧生命周期 |
| 统计审计 | 降级兼容已完成 | 缺少供应商、库存或任务数据源时仍返回 200，并用 `data_status`、`unavailable_sources`、`message` 明确标识；审计查询不受影响 |
| 文件图片 | 已完成，待 Windows 运行态验收 | 受保护批量上传、原图保留、扩展静态格式解码、JPEG 预览、替换和删除 |
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
- 管理写权限隐含读取完成管理所必需的数据：用户写可读用户和角色，角色写可读角色与权限；修改用户角色仍需用户写，修改角色权限仍需角色写。
- 管理员不能修改自己的角色、跨组织授权、授予自己未实际持有的角色或授予自身权限集合之外的权限；实际超级管理员可授予任意合法角色，普通管理员不能修改已被其他组织使用的共享角色。无效 ID 整体失败。部门终端不能成为超级管理员，停用、删除或降权不能使启用中的超级管理员数量归零。
- `super_admin` 是唯一锁定系统角色并拥有全部权限（含 `cost:view`）。新库不创建 `boss` 或终端操作员角色，新终端账号不自动分配角色；旧库同名历史角色解除系统锁定但保留关联。
- 全新数据库保留原有 `admin` 初始化规则，并额外创建 `Silence`；额外密码只从 `BB_ERP_SILENCE_PASSWORD` 配置读取且仅保存 bcrypt 哈希。缺少配置时新库初始化失败关闭，已有数据库不会补建或重置 Silence。系统注入的 Silence 带内部托管标记，不出现在账号管理列表、计数、搜索和分页中，账号管理写接口也不能按 ID 修改它；普通同名账号不受影响。
- 供应商、仓库、库位、库存和任务相关模型已从自动迁移移除；缺表接口返回稳定的 `503 module_not_initialized`，统计接口按可用数据源降级，不能把缺表解释为无引用、库存为零或真实经营零值。客户删除在库存单据或任务单表缺失时同样失败关闭。
- 任务与仓库/库存写操作必须提交 `operator_employee_id`；服务端在事务内重新验证账号当前部门、组织、部门/员工状态和成员关系。
- 库存幂等键绑定接口范围、账号、组织、规范化请求和操作员工；不匹配返回 `409`。
- 数量使用四位定点整数，金额/单价使用分；禁止把前端浮点值直接作为库存或金额事实。
- 图片权限继承业务对象权限；身份发现接口不得返回组织、账号、业务数据、更新地址或凭据。
- 图片上传支持 JPG/JFIF、PNG、GIF、WebP、HEIC/HEIF、AVIF、BMP、TIFF、SVG；动画只取静态封面，JPEG 应用 EXIF 方向。服务端保存原图并派生受保护 JPEG 预览，SVG 原图只允许附件下载。已取消 20 MiB 业务上限，但保留单批 100 张、单批预览 256 MiB、3200 万像素、HEIC/HEIF/AVIF 128 MiB、SVG 8 MiB、全局两个并发转换任务和全局请求上限等运行安全边界；Windows 正式服务构建固定使用 `nodynamic`，不探测外部图片解码 DLL。
- 模具资料包导入使用预览令牌和全量事务替换，仅清理模具、模具图片、预览文件、DWG 与位置字典；模板和正式导出均为 ZIP，模板固定包含一条 `MOLD-001` 示例行、`A1-1`/`B1-1` 位置和 `images/`、`drawings/` 标准空目录，下载响应统一使用安全的 UTF-8 `Content-Disposition`、`no-store` 和 `nosniff` 头；最多 2000 个源条目、共模复制后 5000 个资产，声明解压总量与实际落盘总量均限制为 4 GiB，工作簿/位置/修正参数另有 64/4/4 MiB 边界。导入忽略 Excel 的 ID/图片总数，图片按产品材料/补充图分组并支持共模复制，未知图片可在预览中人工指定分组和模具编号。资料包图片与图库共用扩展格式、预览阶段真实解码和静态预览规则，大小写扩展名均可识别，系统导出的新格式可回导；图片、DWG、整模删除和全量导入使用同一资产互斥边界。
- API 只保留当前 canonical 路径：任务单 `/api/v1/workorder`、物料 `/api/v1/materials`、产品 `/api/v1/products`、模具 `/api/v1/molds`、仓库管理 `/api/v1/warehouses`，以及库存单据/余额/流水和 `/api/v1/warehouse/items`、`/tabs` 路径；不注册旧任务、单数基础资料、`/api/v1/inventory` 或 `/api/v1/warehouse` 根别名。图片权限只校验当前业务对象权限。
- 新库直接由 GORM schema 创建非空幂等键部分唯一索引；账号和 JWT 的 `password_version` 明确从 `1` 开始，不执行旧库字段/索引修复。
- 管理员重置账号密码在同一事务递增 `password_version` 并撤销目标账号全部 refresh token，旧 access/refresh 会话均失效。
- 权限编码绑定采用 fail-closed：任何缺失或拼写错误都会失败，不会把空查询结果解释为全量授权；新建部门终端账号不再自动绑定角色，显式角色修改提交后同步刷新 Casbin 策略，失败不得虚报可用；外层 update manifest 拒绝重复 JSON key、未知字段和尾随 JSON。
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
- 本轮权限与模块过渡测试覆盖：管理写权限读取回退、自身/跨组织/越权授权拒绝、无效 ID 失败关闭、终端禁止超级管理员、最后一个超级管理员保护、新终端无默认角色，以及新库暂缓接口统一 503、统计 200 降级。
- 本轮初始化测试覆盖：原有 admin 与额外 Silence 同时创建、Silence 密码通过配置注入并以 bcrypt 哈希保存、已有数据库不补建或重置、历史角色幂等解除锁定且保留用户关联。
- 权限 provider 并发 `Enforce` + `ReloadPolicies` 无竞态；注入快照构建失败时旧权限仍可用。
- `git diff --check`
- 本轮导入导出专项：统一下载响应头由 `internal/spreadsheet` 提供，客户 XLSX 模板/导出契约保持不变；模具模板改为 `博邦模具导入模板.zip`，包含 `molds.xlsx`、`locations.json` 与标准空目录，模板和正式导出均通过现有 `readPackage` 回读测试；`go test ./internal/spreadsheet ./internal/customer ./internal/mold` 通过。
- 模具重构专项：`GOCACHE=/private/tmp/bb-erp-go-cache go test ./...`、`go vet ./...`、Web/Client `npm run build` 已通过；Swagger 三份产物已重新生成。
- 旧 API 路由、tasks/inventory 旧权限、图片权限回退、旧幂等索引升级和旧用户表密码版本迁移测试均已删除。
- `v0.0.10` 至 `v0.0.12` 按维护者要求设为 GitHub Actions 仅构建版本：完整 Windows 打包、签名和 Artifact 保存照常执行，`publish-gitee` 明确跳过，不向 Gitee 上传成品且不更新稳定 manifest。
- `v0.0.11` 标签对应 GitHub Actions `#91`（run `33458004272`）已成功：Go、Web、Tauri 前端、通用/Windows Rust 和完整 Windows 发布包作业全部通过，五组 GitHub Artifact 已生成；Gitee 发布作业为 `skipped`，`bb_erp_releases` 无 `v0.0.11` Release，稳定 manifest 保持 `0.0.9`。
- `v0.0.13` 待标签推送后由 GitHub Actions 执行完整打包；在获得实际作业结果前，不将 Artifact、Gitee Release 或 Windows 真机验收记为已完成。
- Windows 客户端启动优先验证上次保存的内网服务器；仅在保存服务器不可用、未就绪或身份不匹配时才执行 UDP 发现。构建与自动化测试不替代真实 Windows/局域网验收。
- 本轮前端弹层显示整改不改变 API、权限或数据契约；客户导入/导出、客户资料和任务操作弹层已移除 Teleport 内不可靠的 Motion 正文包装，待 Web/Tauri 与 Windows 真机完成首帧可见性复验。
- 启动重复排查确认属于开发 Vite 依赖发现重载现象，不涉及 Go 服务或认证续期；Tauri `--bundles app` 生产 `.app` 构建成功，完整 DMG 仅受当前 macOS 挂载脚本环境限制。
- 本轮扩展图片静态预览：`go test -count=1 ./...`、`go vet ./...`、`go test -race -count=1 ./internal/file ./internal/mold`、`go test -tags nodynamic -count=1 ./internal/file ./internal/mold ./internal/app`、Web 测试、Web/Client 生产构建、Windows amd64 `CGO_ENABLED=0` + `nodynamic` 服务端跨编译、`git diff --check` 和 Swagger 三份产物同步均通过；HEIC、AVIF 真实编解码器样本已在当前 macOS 环境生成 JPEG 预览。本轮最终复核时 Rust 工具链可用，`cargo fmt --check`、`cargo check --locked` 和 26 项 `cargo test --locked` 已通过；Windows NSIS 仍只能由 `windows-latest` 作业组装，Windows 真机上传/解码、极端高清图、磁盘不足和真实业务权限仍在目标环境验收。

## 5. 待完成

- Windows 10/11 真机防火墙、零/一/多服务、UDP 阻断和同时启动场景。
- 自动发现或手动验证后，在 Windows 桌面端登录并请求至少一个已认证 API 的真实局域网验收。
- 完整管理员/办公室/仓库/部门/只读权限矩阵。
- 模具 ZIP 导入失败回滚、真实大包上传、Windows/Tauri 视觉与受保护文件下载仍需运行态验收。
- 模具独立图片文件夹的“只追加图片”导入端点尚未开放；当前可用路径是 ZIP 全量资料包导入，补充该端点时复用同一命名识别和预览校验。
- 客户、库存、任务、模具在断网、并发冲突、重复提交和磁盘异常下的端到端验收。
- 安装目录、数据库、上传、日志和更新缓存的权限与备份恢复演练。

这些外部项目不得由单元测试、本机浏览器或非 Windows 构建冒充完成。

## 6. 文档同步规则

- API、权限、字段或错误语义变化时同步 Handler Swagger 注释、`docs/API.md`、Swagger 三份产物和必要的 `test.http`。
- Web/Tauri 共用 API 或运行约束变化时同步 `docs/WEB_STATUS.md`、`client/API_SYNC.md` 和构建规则。
- 每次代码变更记录实际执行的验证，不把未执行项目写成完成。
