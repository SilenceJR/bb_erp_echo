# Windows Client / Web 状态

> 基准日期：2026-09-05
> 范围：`web/` 共用 Vue 业务、`client/` Windows Tauri 壳与平台能力

当前按全新内网部署实现。Windows 10/11 Tauri 是正式产品，桌面 Web 是共用代码的备用入口；不维护公网、系统代理、macOS、旧页面、旧 API 或旧客户端双轨。

## 1. 当前结论

| 范围 | 状态 | 证据与边界 |
| --- | --- | --- |
| 启动状态机 | 已完成 | 桌面端先验证保存服务器；失败或未保存时才 Discovering → Validating → AutoConnected/SelectServer/ManualSetup → LoginReady |
| 局域网发现 | 已完成 | Tauri 按私网 IPv4 网卡向 UDP 39080 广播；严格校验 512 字节、未知字段、nonce、来源、协议、产品和 UUID |
| 服务验证 | 已完成 | 候选必须通过 `/ready` 与 `/api/v1/discovery/identity`；Vue 以规范化 `{origin, instance_id}` 复合键去重和渲染，同 ID 不同 origin 保留为多候选并禁止自动连接 |
| 登录入口 | 已完成 | 业务会话只在连接验证后挂载；登录页只显示已验证服务；更换服务回到启动流程 |
| 内网 HTTP 授权 | 已完成 | Tauri HTTP scope 以 URL Pattern 正则覆盖 `127/8`、`10/8`、`172.16/12` 与 `192.168/16`；已验证服务可发起登录及同源 API 请求，不开放 HTTPS、公网或任意域名 |
| 会话隔离 | 已完成 | 已保存、当前、手动和选择连接统一比较规范化 origin 与实例 ID；任一变化清除 access/refresh token，卸载工作台同步丢弃用户信息和领域缓存 |
| Web 备用入口 | 已完成 | 不执行 UDP；验证当前同源服务后进入共用登录页；平板/手机提供低频 Web 访问适配 |
| 原生文件保存 | 已完成 | Tauri 系统对话框、无代理/无重定向、同源受保护下载、临时文件和原子替换；saved/cancelled/error 三态 |
| 统一离开守卫 | 已完成 | 模块切换、登出、切服、安装更新、浏览器刷新与 Tauri 标题栏关闭共用 DirtyGuardRegistry；权限分配、改密、员工/部门状态和客户编码删除已纳入提交硬锁 |
| 窗口配置 | 已完成 | 首次最大化、最小 1024×680、后续恢复窗口状态 |
| Web/Tauri 共用 | 已完成 | 业务页面、状态、权限、DTO、校验和错误只维护一套 Vue 实现 |
| API canonical 收敛 | 已完成 | 共用客户端仅使用 `/api/v1/workorder`、`/api/v1/materials`、`/api/v1/products`、`/api/v1/molds` 与 inventory-documents/balances/ledgers；不依赖旧任务、单数资料或旧 inventory 别名 |
| 模块页拆分 | 已完成 | `ModulePage.vue` 只做注册表分发；统计、仓库、任务、模具、供应商和通用目录使用独立内容组件 |
| 模具重构 | 已完成，待运行态验收 | 新页面支持筛选、表格/卡片详情 Drawer、图片分组、DWG 文件、位置管理、跨页批量移位、ZIP 预览/人工修正/导入导出，以及 ZIP、图片、DWG 拖拽添加；不迁移旧生命周期 UI |
| 三栏客户端壳 | 已完成，本地浏览器已验收 | 左侧业务导航支持完整/图标/隐藏并本机持久化；中央保留 ERP 业务区；1440px 右侧详情停靠为 420px，1280px 使用覆盖式 Drawer，同一详情不会重复出现 |
| 主题与设置 | 已完成，本地浏览器已验收 | 默认博邦蓝亮色，可切换暗色及青绿/紫色；主题在 Vue 挂载前应用，连接与服务移入设置，健康状态不再长期占用首页顶部 |
| 管理权限体验 | 已完成，本地管理员流程已验收 | 用户写权限可读取角色选项，角色写权限可读取权限选项；前端不再额外要求冗余读取权限，并禁用修改自己的角色；系统托管 Silence 不出现在账号列表 |
| 暂缓模块状态 | 已完成，待真实旧库验收 | 任务单、仓库、供应商和统计入口保留；新库缺表时显示待重构说明并禁用写操作，统计不把不可用来源显示成真实零值 |
| 应用壳拆分 | 已完成 | `AppWorkspace.vue` 组合业务导航、中央工作区、设置和响应式详情面板 |
| 工作台控制器 | 已完成 | 配置、展示、DirtyGuard、模具、仓库、任务和通用目录均已下沉，3314 行降至 1516 行；根层负责会话、导航和领域组合 |
| 任务窄域接口 | 已完成 | 控制器内部直接由任务 state/operations 构造四切片 `workorderContext`，根只返回该单一对象；任务组件不再注入全量 `WorkspaceContext` |
| 样式治理 | 已完成 | `styles.css` 仅为分层入口；共享消费点全部使用 `--bb-*`，旧 `--brand/--muted/--line/--ink` 等兼容别名已删除 |
| 桌面单模板 | 已完成 | 固定侧栏、桌面表格和 1024×680 最小窗口使用单一实现，不保留并行页面模板 |
| CSS/Motion 分层 | 已完成，本地浏览器已复验 | 高频 hover/focus/颜色反馈使用 CSS；停靠详情用 160ms 网格与轻位移形成空间连续性，首帧不透明度不低于 `.92`；覆盖式 Drawer 由 Element Plus 管理遮罩、焦点和开闭；全局尊重 reduced motion |
| 平板/手机 Web | 低频适配已完成，本地窄屏已验收 | 1023px 以下使用覆盖导航、单列内容和表格横向滚动；导航具备 dialog 语义、焦点进入、Esc 关闭及焦点回归；Tauri 仍为桌面优先 |
| UI/UX 设计与独立审查 | 已完成 | 只读设计阶段按现代工业客户端原则约束外观；实现后由独立 UI 审查复验。Figma 因无可写个人 Draft 未写入，Eagle 素材只读筛选后未强制引入 |
| Windows 真机 | 待完成 | 需在 Windows 10/11、1920×1080、100%/125%/150% 缩放验收 |

## 2. 目录职责

```text
web/src/App.vue                       启动门面、全局离开边界
web/src/components/app/
  StartupScreen.vue                   启动动画、发现结果、手动降级
  WorkspaceSession.vue                验证通过后才创建业务会话
  LoginScreen.vue                     已验证服务与账号登录
  AppWorkspace.vue                    桌面壳层
web/src/components/pages/module/      领域模块页与注册表
web/src/composables/
  useStartupConnection.ts             启动连接状态机
  useWorkspaceController.ts           会话、导航和跨领域协调
  useDirectoryOperations.ts           通用目录列表、分页、表单与供应商编辑
  useWorkorderOperations.ts           任务产品、临时建档、库存、日志与状态动作
  workorderContext.ts                 任务 Context 类型、注入键与消费入口
  useWarehouseOperations.ts           仓库详情、流水与出入库办理
  useModuleConfiguration.ts           表单/动作配置
  workspacePresentation.ts            展示格式化与纯计算
web/src/platform/
  appearance.ts                       主题、品牌色和左栏状态的本机持久化
  moduleAvailability.ts               暂缓模块错误识别与用户提示
  connection.ts                       Web/Tauri 连接与发现适配
  dirtyGuard.ts                       未保存离开注册表
  types.ts                            平台能力契约
web/src/styles/                        基础、Element、壳层、工具与业务模式
client/src/desktop-http.ts             Tauri 平台桥接
client/src-tauri/src/discovery.rs      UDP 发现与候选验证
client/src-tauri/src/save.rs           原生安全文件保存
```

## 3. 维护规则

- Vue 业务组件不得直接导入 Tauri API；平台差异必须通过 Connection、Discovery、FileSave、WindowLeave 或 ClientUpdate 契约。
- 新页面必须进入模块注册表或专用入口，不向 `ModulePage.vue` 增加 `activeKey` 大分支。
- 业务状态优先放入领域 composable；`useWorkspaceController` 只保留会话、导航和跨领域协调。
- 设计令牌只在 `design-system.css` 定义；共享颜色、间距、圆角和阴影不得在业务文件重复硬编码。
- 产品最小窗口为 1024×680。紧凑桌面允许表格内部横向滚动，业务列表只维护一套桌面结构。
- 所有离开路径必须经过 DirtyGuardRegistry；保存中禁止离开，有未保存内容时默认保留。
- Tauri 文件保存只有 `saved` 可提示成功；`cancelled` 不提示成功，`error` 显示具体原因。
- Web 与 Tauri API、权限和错误语义保持一致，Go 服务端始终是最终裁决者。
- 外观本地键固定为 `bb_erp_theme_mode`、`bb_erp_theme_accent` 和 `bb_erp_sidebar_mode`；不与账号同步。

## 4. 历史基线验证与本轮验证

上一轮基线与本轮验证如下：

已通过：

- `go test -count=1 ./...`
- `go vet ./...`
- `go test -race ./internal/discovery ./internal/config ./internal/app`
- Web `npm run build`
- Web `npm test`（同 ID 不同 IP 不自动连接、origin 变化不复用旧认证会话）
- Client `npm run build`
- `cargo fmt --check`
- `cargo check --locked`
- `cargo test --locked`（26 项，包含 RFC1918 HTTP scope 回归测试）
- `git diff --check`
- 本轮客户端壳、主题、错误语义、对齐、焦点循环和详情交互：Web `npm test` 27 项通过，Web `npm run build` 通过，Client `npm run build` 通过。
- 本轮隔离 SQLite + 本地浏览器已完成真实登录与已认证页面检查：新库管理员登录、客户资料新增与详情、系统账号列表隐藏托管 Silence、仓库暂缓状态、统计数据源降级、设置与暗色切换均通过。
- 本轮桌面浏览器尺寸检查：1440×900 为 `224px / 796px / 420px` 三栏且停靠详情无遮罩；1280×800 自动改为覆盖式 Drawer；1024×680 保持桌面导航；1023×680 改为 dialog 导航。左栏三档实测 224/64/0，窄屏焦点进入、Esc 关闭和焦点回归通过。
- 本轮详情关闭与焦点：覆盖式 Drawer 点击遮罩空白区和 Esc 均可关闭；停靠态使用关闭按钮或 Esc，不将中央表格、筛选和文本区域绑定为关闭热区；查看切编辑会进入首个字段，关闭恢复触发点。真实 Tauri 窗口、Windows/WebView2 和 Windows 缩放仍未验收。
- 本轮 `web/npm run build`：通过；Vue 类型检查、Vite 生产构建通过，最大异步脚本约 159.67 kB。
- 本轮 `client/npm run build`：通过；Tauri 前端类型检查、Vite 生产构建通过；最大入口脚本约 409.60 kB，仅有既有 `@vueuse/core` Rollup 注释警告。
- 本轮 Tauri Rust 与打包验证：`cargo fmt --check`、`cargo check --locked`、`cargo test --locked` 全部通过，26 项 Rust 测试通过；`npx tauri build --bundles app` 成功生成 macOS `.app`。构建成功不能替代真实窗口流程和 Windows/WebView2 验收。
- 本轮打包后 Tauri 窗口：macOS `.app` 已实际启动到 `tauri://localhost`，启动页、品牌信息、未发现服务降级说明、手动服务器输入和重新发现入口均在原生窗口首帧可见；本次未在该窗口完成已认证业务流程，因此只记录为启动显示验证，不替代 Windows 或完整 Tauri 业务验收。
- 本轮模具页面：Web 与 Client `npm run build` 均通过；Windows 10/11、图片大批量上传、ZIP 导入和 DWG 下载仍需运行态验收。
- 本轮模具交互：图片组、ZIP 导入和 DWG 区域增加键盘可达的拖拽/点击入口；客户 Excel 导入同样提供拖放区，顶部按钮只打开弹窗，点击区域才打开系统文件选择器；Tauri 原生文件拖放路径已桥接为 WebView `File`，Windows 不依赖 HTML5 `dataTransfer.files`；详情加载失败、未保存关闭、编辑入口和窄屏 Drawer/空态显示已整改；Web/Tauri 生产构建与真实 Finder/资源管理器拖入仍需运行态验收。
- 本轮 `web/npm test`：通过，4 项启动连接/身份隔离回归测试通过。
- 本轮 `git diff --check`：通过。
- 本轮图片上传兼容：Web/Tauri 共用图库不再依赖 Windows WebView 的 `File.type`，前端按扩展名接受 JPG/JPEG/JFIF、PNG、GIF、WebP、HEIC/HEIF、AVIF、BMP、TIF/TIFF、SVG，并取消原 20 MiB 客户端拦截；单批统一最多 100 张。图片列表优先加载服务端 `preview_url` 静态预览，保留 `content_url` 原图；预览失败不自动下载高清原图，界面保留文件名、原因、刷新入口、处理建议和请求编号，上传错误只在持久提示中展示完整详情。手机 JPEG 静态预览应用 EXIF 方向。Web 测试、Web 与 Client 生产构建通过；Windows 真机选择/拖入、超大图片和各格式预览仍待运行态验收。
- 本轮弹层显示整改：移除 Element Plus Dialog/Drawer 内的 `AnimatePresence`、`motion.*` 初始透明度包装，覆盖客户导入/导出、客户资料 Drawer、客户编码、替代默认资料和任务操作 Dialog；Web/Tauri 构建需重新验证，Windows 真机首帧与焦点仍待验收。
- 启动重复排查：开发模式出现的 `optimized dependencies changed. reloading` 属于 Vite 依赖发现重载，不纳入生产问题；`npx tauri build --bundles app` 已成功生成内置静态资源的 macOS `.app`，生产包不运行 Vite `devUrl`。完整 DMG 打包仅在当前 macOS 的 `bundle_dmg.sh` 挂载步骤失败，应用编译与 `.app` 产物成功。
- 隔离 SQLite + 本地浏览器：同源身份验证后进入登录页，服务器名称/地址/版本正确，账号输入获得焦点，1920×1080 无页面横向溢出

模块页与业务详情已按异步入口拆包；本轮 Web 构建最大脚本约 150.99 kB（gzip 55.53 kB），未出现 500 KiB chunk 警告。任务组件已形成独立 `workorderContext` 异步块。

## 5. 未完成的外部验收

- Windows 10/11 原生安装与启动。
- 1920×1080 在 100%、125%、150% 缩放下的表格、Drawer、Dialog 和底部操作可达性。
- 保存服务器可用、保存服务器不可用后发现、零个/一个/多个 ERP 服务，以及 UDP 阻断/TCP 可用场景。
- 原生保存成功、取消、覆盖、无权限、磁盘不足和网络中断。
- 标题栏关闭、登出、切服、更新安装时的真实脏表单保护。
- 管理员、办公室、仓库、部门、只读账号的完整权限矩阵及客户、库存、任务、模具业务闭环。
- 已发现服务器、选择服务器、登录与至少一个已认证 API 请求的真实局域网桌面端验收。

这些项目不得由静态构建、本地浏览器或非 Windows Rust 测试冒充完成。平板/手机 Web 与 Windows 视觉、键盘、缩放和真实业务闭环仍待项目负责人亲自验收。
