# API 同步说明

桌面端 `client` 不复制 Web 管理端代码，而是通过 `client/src/main.ts` 直接导入 `../web/src/main.ts`。

## 同步规则

- Web 生产环境使用相对 API 路径，推荐由 Go 同时托管 Web 静态文件，浏览器始终访问同源服务。
- 桌面端通过 `@tauri-apps/plugin-http` 从 Rust 层发送 HTTP 请求，不依赖 macOS/Windows WebView 的跨域行为。
- 共用请求层通过 `HttpTransport` 接口选择浏览器或桌面传输，业务 API 不感知运行平台。
- `npm run desktop:dev` 和生产安装包都通过同一套 Rust HTTP 传输访问 Go 后端。
- 首次启动默认连接 `http://127.0.0.1:8080`，可在登录页或登录后的顶栏设置运行 Go 服务的内网地址，例如 `http://192.168.1.20:8080`。
- `VITE_API_BASE_URL` 只用于指定安装包首次启动的默认地址；用户保存后的 ERP 服务器局域网地址优先。
- 服务地址只接受不带路径、参数和凭据的 `http://` 或 `https://` 源地址；每次实际请求仍只能使用站内 API 路径。
- 模块导航和接口路径仍在 `web/src/data/modules.ts` 中维护。
- Go 后端新增或调整接口时，优先更新 `web/src/api/*` 和 `web/src/data/modules.ts`，桌面端会自动复用。
- 图片创建接口 `POST /api/v1/files/images` 支持重复 `file` 字段一次上传多张图片，单图和多图均返回图片数组；替换接口仍使用单图 `PUT /api/v1/files/:id/content`。
- 登录响应同时返回 `access_token`、`refresh_token`、`expires_at` 和 `refresh_expires_at`；共用请求层会在 access token 临近/已经过期时调用 `/api/v1/auth/refresh`，轮换令牌后重试一次原请求。Web 与 Tauri 均持久化当前会话，连续 30 天未成功续期后回到登录页。
- 退出登录调用 `/api/v1/auth/logout` 撤销 refresh token；服务端撤销失败不阻止客户端清理本地会话。修改密码会撤销该账号全部 refresh token。
- 当前账号修改密码使用 `POST /api/v1/auth/change-password`；成功后旧 JWT 失效，Web 与 Client 都回到登录页并使用新密码登录。
- 员工是独立档案，可属于多个部门；登录账号仍只绑定一个当前部门。部门与员工使用专用页面维护，Web 和 Tauri 共用同一套页面、权限和 API 契约。
- 任务单及仓库/库存全部写操作先调用 `GET /api/v1/operator-employees` 加载当前账号部门的在职员工，只使用返回的 `id/name`。每次确认都必须显式提交 `operator_employee_id`，不自动选择、不跨操作记忆，成功后清空。即时库存写入把幂等键绑定到完整规范化请求快照；网络或服务端 5xx 导致结果不确定时重试复用原键，业务字段或操作人变化时才生成新键。
- 候选接口失败、账号无部门、部门停用或无候选时禁用提交；服务端返回 `409` 表示员工停用或成员关系已变化，客户端清空原选择并重新加载。其他校验错误保留当前表单，便于修正后重试。
- 新建生产单先通过 `GET /api/v1/warehouse/items?tab=product&q=<关键词>` 远程搜索仓库产品，选择后调用 `GET /api/v1/warehouse/items/product/:id` 获取默认仓库聚合库存；提交任务单时发送 `product_id`，`product_name` 与 `unit` 由服务端写入快照。通用任务不发送产品关联。
- 拥有 `workorder:write` 和 `workorder:temporary-product:write` 的账号可调用 `POST /api/v1/workorder/products` 临时建立正式产品档案；请求包含必填 `name`、手填唯一 `code`、默认“个”的可改 `unit` 与可选 `spec`。新产品库存为 0，建档不会生成入库流水。
- 产品选择和任务详情的库存采用“选择即查/打开即查 + 手动刷新”，不轮询；Web 与 Tauri 共用取消请求和请求序号保护，快速切换时旧响应不得覆盖当前产品。
- 操作日志、任务流转和库存历史以“员工｜动作”为主要信息，同时保留登录账号与终端责任信息；所选员工是登录账号对现场责任人的申报，不等同于员工本人完成二次认证。旧记录没有员工快照时显示“历史记录未记录员工”。
- 桌面端只负责 Rust/Tauri 壳、窗口配置、平台打包配置，不维护另一套业务 API。
- 桌面壳通过 `@tauri-apps/api/app` 的 `getVersion()` 读取真实安装版本；Rust 更新引擎计算当前 EXE SHA 并向 Go 请求更新计划。Vue 只触发检查/安装并展示状态，不接触本机路径、任意下载 URL或签名决策；Web 端不显示桌面自动安装按钮。

## 更新请求

- `/api/v1/version` 保持启动兼容信息。
- 兼容状态接口仍为 `/api/v1/updates/client/status?current_version=<安装版本>`；当前没有已部署旧客户端，不执行旧协议迁移。历史 v2/delta 运行时代码暂留但正式 v3 发布不生成、不选择也不展示差分。
- 新客户端由 Rust 调用 `/api/v1/updates/client/plan`，传入真实版本、当前 EXE SHA、`windows-x86_64` 和固定的 `install_mode=portable`。返回 `204` 表示已是最新。
- v3 计划只允许 `full`。签名 payload 绑定资源 `kind`、大小、SHA-256 和 Minisign 签名，不包含或信任公网 URL。自动更新只使用用户确认的 Portable 单 EXE，由本地 helper 原子替换并在 Ready 超时后回滚；NSIS 仅用于首次安装或人工恢复。
- `/api/v1/updates/client/tauri/windows/x86_64/<当前版本>` 提供官方 updater JSON；`/api/v1/updates/client/artifacts/<sha256>` 只分发当前已验签 manifest 中的内容寻址资源，支持 ETag 和 Range。
- 管理页读取 `/api/v1/system/updates/status`；拥有 `system:updates:write` 时可调用 `POST /api/v1/system/updates/check`。服务端升级包通过同源受保护接口 `/api/v1/system/updates/server/download` 下载，由 Go 服务端使用已部署可信公钥流式验证 Minisign 签名、1 字节至 512 MiB 大小、SHA-256 和 ZIP 结构后分发，Tauri 不直接打开外部更新地址；桌面传输对此大文件接口使用 12 分钟超时，比服务端默认下载时限多保留 2 分钟，以便接收并反馈服务端具体错误。
- 管理员恢复 ZIP 从需要 `system:updates:write` 权限的 `/api/v1/system/updates/client/download` 下载，并在每次分发前重验独立 Minisign、大小、SHA-256 和 ZIP 路径安全；正式清单来自服务端本机 `updates/stable/update-manifest.json`，制品来自 `updates/artifacts/<sha256>`，服务端不会请求 Gitee、GitHub或自身 HTTP 地址。
- 局域网可使用 HTTP，但 Rust 只接受 loopback/私网 ERP 服务器地址；所有自动更新资源仍须通过端到端签名、大小和哈希验证。当前协议不面向公网。
- 客户端不检测、不选择 RC 渠道；自动更新只使用服务器计划任务激活的正式 stable manifest。

## 调试说明

- 先启动 Go 后端：`go run ./cmd/server`
- 再启动桌面调试：`cd client && npm run desktop:dev`
- macOS 调试只用于开发，不作为 Windows 发布验收。

## Windows 构建与验收

- 正式 Windows 包只由 `scripts/windows-release.ps1 -Mode Publish` 在锁定环境中构建；不得全局安装 Tauri 或执行 `latest` 依赖安装。
- 便携恢复目录必须保持 `bb-erp-client-windows-x86_64.exe` 与 `bb-erp-portable.json` 同目录。
- Windows 10 真机必须验证服务器地址、更新前确认、稍后处理、完整包下载安装重启、断网、截断下载、错误签名和不可写目录。
- Server 2016 只承担构建、分发和服务端运行，不作为主要 Tauri 使用电脑。
