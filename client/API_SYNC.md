# API 同步说明

## 2026-09-05 Astra 整改验证边界

本轮不改变 API、权限、账号初始化和模块过渡契约。详情、新增和编辑共用面板载体；查看 420px，表单/图片 520px；停靠条件统一为视口至少 1440px 且扣除导航、面板、分隔及中央 48px 内边距后至少保留 720px。详情和表单采用同一套空间过渡、焦点恢复和未保存保护。Tauri 前端与 macOS 应用构建已通过；macOS原生启动、发现及进入登录页已验证，但已认证原生业务流、Windows及脱敏真实样本验收未完成。详见 [逐页验收记录](../docs/ASTRA_ACCEPTANCE.md)，不得以构建代替成品运行验收。

桌面端 `client` 不复制 Web 管理端代码，而是通过 `client/src/main.ts` 直接导入 `../web/src/main.ts`。

## 同步规则

- 产品原则：Windows Tauri 客户端优先，Web 是桌面备用入口；业务页面、权限、DTO、校验和错误语义只维护一套 Vue 实现。局域网发现、连接验证、原生文件保存、窗口关闭和客户端更新通过平台能力端口分别适配。
- 登录后共用三栏客户端壳：左侧业务导航支持完整/图标/隐藏，中央保留 ERP 页面，右侧详情在宽屏停靠、1280px 以下覆盖。外观状态仅保存在当前电脑的 `bb_erp_theme_mode`、`bb_erp_theme_accent`、`bb_erp_sidebar_mode`，不与账号同步。
- 默认使用博邦蓝亮色，可切换暗色、青绿色或紫色。主题在 Vue 挂载前应用，Element Plus Teleport 浮层读取同一主题；连接成功状态收进“设置 / 连接与服务”，仅检测或故障时在全局突出。
- 用户写权限可读取用户列表和角色选项，角色写权限可读取角色与权限选项；客户端仅按该组合改善入口展示，最终授权、同组织、自身修改、权限子集和最后一个超级管理员保护由 Go 服务端裁决。
- 新建部门终端账号不自动获得角色，客户端创建成功后应提示管理员按岗位显式授权。
- 供应商、仓库/库存和任务单的新库数据结构暂缓时，API 返回 `503` 与 `code=module_not_initialized`；客户端保留页面框架、展示待重构说明并禁用新增、编辑、删除、导入等写操作。已有数据库表存在时保持原流程。
- 统计接口兼容新增 `data_status`、`unavailable_sources`、`message`。当 `data_status=sources_unavailable` 时，客户端必须标识不可用数据源并以“—”或空态显示其指标，不得把占位零值解释为真实经营数据。

- Web 生产环境使用相对 API 路径，推荐由 Go 同时托管 Web 静态文件，浏览器始终访问同源服务。
- 桌面端通过 Rust 发送 HTTP 请求，不依赖 WebView 跨域行为或系统代理。
- 共用请求层通过 `HttpTransport` 接口选择浏览器或桌面传输，业务 API 不感知运行平台。
- `npm run desktop:dev` 和生产安装包都通过同一套 Rust HTTP 传输访问 Go 后端。
- 开发模式的 Vite 依赖发现可能触发页面 reload，但不进入生产包；生产 Tauri 使用 `frontendDist` 内嵌静态资源，不访问 `devUrl`，启动状态机不会因 Vite 依赖优化重复执行。
- Element Plus Dialog/Drawer 通过 Teleport 挂载时，正文使用稳定的普通 Vue 容器，不嵌套 `motion-v` 的初始透明度或 `AnimatePresence`；这样 Web 与 Tauri 首帧均可读，弹层开闭、焦点和滚动由 Element Plus 管理。
- 桌面端启动时优先核对上次保存的 origin 与实例 ID；通过 `/ready` 与 `/api/v1/discovery/identity` 后直接进入登录页，不发送 UDP 广播。保存地址不可达、服务未就绪、身份接口失败或实例变化时不复用旧认证，才向 UDP `39080` 发送发现请求。
- 发现候选以规范化 `{origin, instance_id}` 作为复合唯一键。只有一个复合候选时自动连接；同 ID 不同 IP 保留为多个候选并要求明确选择，零候选进入手动设置。登录页只展示已经验证的服务。
- 服务地址只接受不带路径、参数和凭据的 loopback/RFC1918 IPv4 `http://` 源地址；每次实际请求仍只能使用站内 API 路径。
- Tauri HTTP 插件 scope 使用 URL Pattern 正则匹配 `127/8`、`10/8`、`172.16/12` 和 `192.168/16`；它与服务身份验证共同约束桌面端请求，不授权 HTTPS、公网地址或任意域名。
- 模块导航和接口路径仍在 `web/src/data/modules.ts` 中维护。
- Go 后端新增或调整接口时，优先更新 `web/src/api/*` 和 `web/src/data/modules.ts`，桌面端会自动复用。
- 业务 API 只使用当前 canonical 路径：任务单 `/api/v1/workorder`、物料 `/api/v1/materials`、产品 `/api/v1/products`、模具 `/api/v1/molds`、仓库管理 `/api/v1/warehouses`，库存单据/余额/流水使用各自复数路径，库存浏览/操作使用 `/api/v1/warehouse/items` 和 `/tabs`；客户端不回退旧任务、单数资料、`/api/v1/inventory` 或 `/api/v1/warehouse` 根路径。
- 图片创建接口 `POST /api/v1/files/images` 支持重复 `file` 字段一次上传多张图片，单图和多图均返回图片数组；单批最多 100 张，替换接口仍使用单图 `PUT /api/v1/files/:id/content`。图片元数据同时保留原图 `content_url` 和静态预览 `preview_url`，图库优先读取预览；新记录预览失败时不自动加载高清原图，而是显示文件名、原因、刷新入口和请求编号。共用客户端按文件扩展名预检查 JPG/JPEG/JFIF、PNG、GIF、WebP、HEIC/HEIF、AVIF、BMP、TIF/TIFF、SVG，不依赖 WebView 提供的 `File.type`，也不执行原 20 MiB 客户端拦截；真实内容、格式、全局请求上限、像素和解码资源边界由 Go 服务端最终校验，GIF 和动态照片只显示静态封面。失败界面展示文件名、具体或大概原因、处理建议和可用的请求编号；Tauri 点击选择和原生拖入均为图片传输保留最长两小时，降低服务端已完成但客户端先超时的误报概率。
- 模具使用 `/api/v1/molds`；位置字典使用 `/api/v1/mold-locations`，批量移位使用 `/api/v1/molds/bulk-location`。模具模板通过 `/api/v1/molds/import-template` 下载 ZIP，资料包通过 `/api/v1/molds/import/preview`、`/api/v1/molds/import/commit` 预览确认导入，通过 `/api/v1/molds/export` 导出 ZIP；客户模板与导出分别使用 `/api/v1/customers/import-template`、`/api/v1/customers/export`。这些固定文件传输契约在 `web/src/data/transferRegistry.ts` 维护，模型字段、目录或接口变更必须同步更新注册项与回归测试。模具图片沿用受保护图片接口，DWG 使用 `/api/v1/molds/:id/drawings` 上传、下载和删除。Web 与 Tauri 共用同一套页面和 DTO，ZIP、图片、DWG 和客户 Excel 支持 HTML5 拖入及点击选择；Tauri Windows 原生拖放直接由 Rust 按文件流上传，避免大文件进入 WebView 内存。
- 登录响应同时返回 `access_token`、`refresh_token`、`expires_at` 和 `refresh_expires_at`；共用请求层会在 access token 临近/已经过期时调用 `/api/v1/auth/refresh`，轮换令牌后重试一次原请求。Web 与 Tauri 均持久化当前会话，连续 30 天未成功续期后回到登录页。
- 退出登录调用 `/api/v1/auth/logout` 撤销 refresh token；服务端撤销失败不阻止客户端清理本地会话。修改密码会撤销该账号全部 refresh token。
- 管理员重置其他账号密码会递增其密码版本，并在同一事务撤销该账号全部 refresh token；旧 access token 和 refresh token 均不可继续使用，账号必须用新密码重新登录。
- 当前账号修改密码使用 `POST /api/v1/auth/change-password`；成功后旧 JWT 失效，Web 与 Client 都回到登录页并使用新密码登录。
- 员工是独立档案，可属于多个部门；登录账号仍只绑定一个当前部门。部门与员工使用专用页面维护，Web 和 Tauri 共用同一套页面、权限和 API 契约。
- 任务单及仓库/库存全部写操作先调用 `GET /api/v1/operator-employees` 加载当前账号部门的在职员工，只使用返回的 `id/name`。每次确认都必须显式提交 `operator_employee_id`，不自动选择、不跨操作记忆，成功后清空。即时库存写入把幂等键绑定到完整规范化请求快照；网络或服务端 5xx 导致结果不确定时重试复用原键，业务字段或操作人变化时才生成新键。
- 候选接口失败、账号无部门、部门停用或无候选时禁用提交；服务端返回 `409` 表示员工停用或成员关系已变化，客户端清空原选择并重新加载。其他校验错误保留当前表单，便于修正后重试。
- 新建生产单先通过 `GET /api/v1/warehouse/items?tab=product&q=<关键词>` 远程搜索仓库产品，选择后调用 `GET /api/v1/warehouse/items/product/:id` 获取默认仓库聚合库存；提交任务单时发送 `product_id`，`product_name` 与 `unit` 由服务端写入快照。通用任务不发送产品关联。
- 拥有 `workorder:write` 和 `workorder:temporary-product:write` 的账号可调用 `POST /api/v1/workorder/products` 临时建立正式产品档案；请求包含必填 `name`、手填唯一 `code`、默认“个”的可改 `unit` 与可选 `spec`。新产品库存为 0，建档不会生成入库流水。
- 产品选择和任务详情的库存采用“选择即查/打开即查 + 手动刷新”，不轮询；Web 与 Tauri 共用取消请求和请求序号保护，快速切换时旧响应不得覆盖当前产品。
- 操作日志、任务流转和库存历史以“员工｜动作”为主要信息，同时保留登录账号与终端责任信息；所选员工是登录账号对现场责任人的申报，不等同于员工本人完成二次认证。旧记录没有员工快照时显示“历史记录未记录员工”。
- 桌面端不维护另一套业务 API；Rust 仅实现平台能力。
- 客户模板、客户导出和服务端升级包统一调用 `FileSave`。Tauri 使用系统保存对话框、同源受保护下载、无代理/无重定向、同目录临时文件和原子替换；取消不会提示成功。Web 备用入口保留浏览器 Blob 下载。
- 其他业务模块选择客户时调用 `GET /api/v1/customers/options`，选项 ID 是具体客户资料 ID，标签由客户编码与简称/名称组成，默认资料优先显示但不限制选择其他资料。
- 桌面壳通过 `@tauri-apps/api/app` 的 `getVersion()` 读取真实安装版本。Vue 只触发检查/安装并展示状态，不接触本机路径、任意下载 URL 或签名决策；Web 端不显示桌面自动安装按钮。

## 更新请求

- Rust 调用 `/api/v1/updates/client/plan`，只传真实安装版本、`windows-x86_64` 和 `nsis|portable` 安装模式。当前契约固定返回完整更新，不保留差分计划、旧 ZIP、协议降级或外部下载分支。
- 更新资源必须通过来源限制、签名、哈希和大小验证；写入临时文件并原子替换，启动失败时恢复原客户端。
- 安装前先调用统一离开守卫，存在未保存业务内容时默认取消安装。
- 管理页读取 `/api/v1/system/updates/status`；拥有 `system:updates:write` 时可调用 `POST /api/v1/system/updates/check`。服务端升级包通过同源受保护接口 `/api/v1/system/updates/server/download` 下载，由 Go 服务端使用已部署可信公钥流式验证 Minisign 签名、1 字节至 512 MiB 大小、SHA-256 和 ZIP 结构后分发，Tauri 不直接打开外部更新地址；桌面传输对此大文件接口使用 12 分钟超时，比服务端默认下载时限多保留 2 分钟，以便接收并反馈服务端具体错误。
- Web 更新中心不展示没有有效下载路径的客户端 ZIP 卡片；真实客户端更新能力只通过 Tauri 原生更新面板提供。

## 调试说明

- 先启动 Go 后端：`go run ./cmd/server`
- 再启动桌面调试：`cd client && npm run desktop:dev`
- 本轮只验收 Windows 10/11；其他桌面平台不在产品范围。
- Windows 支持通过 `npm run desktop:build` 构建安装包，前提是本机已安装 Rust、Node.js 和 Tauri 对应平台依赖。
