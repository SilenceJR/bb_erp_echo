# Web 与 Tauri Client 端进度与维护规范

> 基准日期：2026-08-27
> 适用范围：`web/` Vue Web 应用、`client/` Tauri 桌面壳及两者共用的前端源码

## 1. 状态结论

Web 模块化代码改造、统一弹窗样式改造以及 Tauri Client 对共用源码的构建接入已经完成，Web 与 Tauri Client 的生产构建均已通过。

完整浏览器运行态验收尚未完成，尤其是需要真实 Go API、登录账号和业务数据的流程。业务专属 CSS 仍有一部分保留在 `web/src/styles.css`，用于维持跨页面、跨子组件和响应式样式的一致性；后续可以继续按组件拆分，但不应直接删除全局规则。

`0.0.2`、`0.0.3` 和 `0.0.4` 的 Windows 构建均已完成，但 Gitee 大附件接口持续出现 0 字节响应超时。`0.0.5` 已建立完整更新基线，`0.0.6` 已完成相邻版本发布；`0.0.7` 已修复公钥部署边界，`0.0.8` 已完成从 0.0.7 出发的相邻版本正式发布。两版均通过七个正式附件、匿名 SHA-256、签名差分基线和稳定 manifest 技术验收。版本之间不改变 Web/Tauri 业务 API，Windows 真机的手动恢复、差分或完整包回退、重启和数据保留仍待用户验收。

## 2. 进度清单

| 范围 | 状态 | 说明 |
| --- | --- | --- |
| 应用入口收敛 | 已完成 | `App.vue` 只负责 Element Plus 配置、登录态分支、工作台挂载和页面插槽编排。 |
| 登录与服务器设置 | 已完成 | 登录、登出、2 小时 access token、活跃 30 天 refresh token 自动轮换、401 补偿重试、健康检查、服务器地址设置和客户端更新状态由共用认证/请求层承载。 |
| 当前账号修改密码 | 已完成 | 顶栏账号菜单提供修改密码弹窗；修改成功后旧 JWT 失效并自动回到登录页。 |
| 应用壳层与导航 | 已完成 | 顶栏、侧边栏、移动端抽屉导航独立于 `AppWorkspace.vue`，继续使用 `activeKey`。 |
| 首页仪表盘 | 已完成 | 首页指标、快捷入口、业务分组和更新提示拆至 `DashboardPage.vue`。 |
| 通用资料模块 | 已完成 | 列表、分页、搜索、筛选、新增、编辑和空状态由 `ModulePage.vue` 编排。 |
| 统计报表 | 已完成 | 统计数据加载和展示已纳入模块页与工作台控制器。 |
| 库存模块 | 已完成 | 库存详情、流水、出入库、快速新增供应商和关闭未提交表单确认已拆至库存抽屉及 `useWarehouse` 状态组合式函数。 |
| 模具模块 | 已完成 | 详情、借出、归还、维修、保养、履历和请求过期保护已拆至模具抽屉及 `useMold` 状态组合式函数。 |
| 任务单模块 | 已完成 | 任务详情、办公室操作、部门子任务和流转日志已拆至任务单抽屉及 `useWorkorder` 状态组合式函数。 |
| 角色与账号权限 | 已完成 | 权限配置、角色分配、选项缓存、权限禁用规则由 `useAssignment` 管理。 |
| 图片管理 | 已完成 | `ImageGallery.vue` 支持一次选择多张图片并通过共用请求层批量提交；删除确认继续使用统一弹窗封装。 |
| Web/Tauri 复用 | 已完成 | `client/src/main.ts` 继续直接复用 `web/src/main.ts`、同一套业务组件和认证续期逻辑。 |
| Client HTTP 传输 | 已完成 | Tauri 通过 `desktop-http.ts` 注入 Rust HTTP 传输，业务请求继续使用共用的 `HttpTransport` 抽象。 |
| Client 服务器地址 | 已完成 | 首次默认地址、登录页设置、顶栏设置和保存后的地址优先级遵循 `client/API_SYNC.md`。 |
| Client 更新入口 | 已完成 | Vue 仅展示更新状态并触发桌面更新流程；版本、EXE 哈希、签名和本机路径由 Tauri/Rust 负责。 |
| Client Tauri 构建配置 | 已完成 | `tauri.conf.json` 保持 Web 产物作为 `frontendDist`，窗口、CSP、更新器和安装模式继续由 Client 管理。 |
| MessageBox 样式 | 已完成 | 显式引入 Element Plus MessageBox 样式，补充遮罩、居中、不透明卡片、层级、按钮和移动端宽度规则。 |
| 业务 CSS 全量下沉 | 待完成 | 当前仍有较多跨组件基础和页面样式位于 `web/src/styles.css`，后续拆分需要配合 `:deep`、子组件样式和响应式规则验证。 |
| 浏览器全流程验收 | 待完成 | 需要启动 Go API 并使用测试账号完成登录、权限、库存、模具、任务单和图片流程。 |
| 功能分支与提交 | 待完成 | 当前修复在 `codex/fix-start-system-client-path` 分支，尚待提交、远端推送和合并。 |
| Client 桌面运行态 | 待完成 | 需要在 macOS、Windows 10/11 真机完成登录、窗口、服务器切换、弹窗和更新流程验收。 |
| Client 安装包验证 | 待完成 | 需要执行 `npm run desktop:build`，并完成正式版 Windows 安装、升级、回滚和断网场景验证；RC/手动构建使用独立便携客户端 Artifact，确认 EXE 与 `bb-erp-portable.json` 同目录即可启动；全量便携包必须确认 `启动系统.bat` 能从 `client\bb_erp_client.exe` 启动客户端。 |

## 3. 目录职责

```text
web/src/App.vue
  应用入口，只做根级配置、登录态分支和页面编排

web/src/components/app/
  LoginScreen.vue       登录和桌面端服务器设置入口
  AppWorkspace.vue      登录后壳层、顶栏、侧边栏、移动端导航

web/src/components/pages/
  DashboardPage.vue     首页仪表盘
  ModulePage.vue        通用资料、库存列表、统计和系统页面
  ServerSettingsDialog.vue
  WarehouseDrawer.vue   库存详情、流水和出入库办理
  MoldDrawer.vue        模具详情和生命周期操作
  WorkorderDrawer.vue   任务单详情、部门子任务和日志
  DetailPanels.vue      页面抽屉和设置弹窗的组合入口

web/src/composables/
  useWorkspaceController.ts  工作台跨领域编排、API 流程和页面计算属性
  useAuth.ts                 登录态和服务器连接状态
  useModuleData.ts           导航、列表查询、通用表单和缓存状态
  useAssignment.ts           角色/权限配置状态
  useWarehouse.ts            库存详情和出入库表单状态
  useMold.ts                 模具详情和生命周期状态
  useWorkorder.ts            任务详情和日志状态
  useAppMessageBox.ts        统一 confirm/prompt 入口
  workspaceContext.ts        页面与工作台之间的类型化注入键

client/
  src/main.ts                注入桌面 HTTP 传输后复用 Web 入口
  src/desktop-http.ts        Tauri Rust HTTP 传输适配
  src-tauri/                 窗口、CSP、更新器、安装包和 Rust 桌面能力
  API_SYNC.md                Web/Tauri API、传输和更新同步规则
```

## 4. 维护规范

### 4.1 组件和状态

- 新增页面优先放入 `web/src/components/pages/`，不要继续扩大 `App.vue`。
- 应用根组件只做组合，不在其中直接放业务 API、列表状态或抽屉状态。
- 页面通过工作台上下文、插槽、props 和事件协作；不新增全局状态库，不新增路由系统。
- 导航继续使用 `activeKey`，因为 Web 和 Tauri 共用同一套入口和组件。
- 业务状态应放入对应领域组合式函数，跨领域动作才放到 `useWorkspaceController.ts`。

### 4.2 API、权限和字段

- 不修改现有 API 路径、权限编码、请求方法和后端字段格式。
- `warehouse_records` 是库存单据视图，`warehouses` 是库存主数据列表；两者的路径映射集中维护在工作台控制器中。
- 前端权限判断只负责隐藏不可用导航和操作，后端仍是最终授权边界。
- `cost:view` 缺失时不得主动展示平均成本、单价、金额和余额金额等成本字段。
- 数量按万分之一固定精度传输，金额和单价按分传输；转换集中在 API 边界，避免浮点计算误差。
- 图片权限继承产品、模具、任务单或部门子任务的业务权限，不单独创建一套图片权限。

### 4.3 请求生命周期

- 抽屉关闭、切换记录或重置状态时，必须取消可取消请求并清理请求引用。
- `AbortController` 不能单独作为过期保护；必须同时检查请求序号和当前抽屉记录。
- `finally` 中只允许当前请求修改 loading 状态，避免旧请求覆盖新请求的加载状态。
- 关闭库存抽屉时必须清理未提交的出入库表单，防止重新打开后显示上一条记录或提交旧字段。

### 4.4 弹窗

- 业务代码和 `ImageGallery.vue` 使用 `appMessageBox.confirm` 或 `appMessageBox.prompt`。
- 只有 `useAppMessageBox.ts` 可以直接导入 Element Plus 的 `ElMessageBox`。
- 默认按钮为“确认”和“取消”；删除、放弃、继续等操作可以按业务覆盖文案。
- MessageBox 必须保留 `bb-message-box` 自定义类，以便统一控制遮罩、居中、背景、边框、阴影和移动端布局。
- 新增弹窗场景时，应验证键盘关闭、焦点回收、遮罩点击策略和移动端按钮换行。

### 4.5 注释和文档

- 公共组合式函数、组件入口和上下文类型使用 JSDoc 说明用途与约束。
- 实现注释重点解释“为什么”，包括 API 字段映射、权限原因、请求取消/序号保护、固定精度和状态清理。
- 不写与代码重复的注释；接口名称、字段名和命令保持英文原文，说明文字使用中文。
- 修改 Web API、运行方式或用户可见交互时，同步检查 `docs/API.md`、`client/API_SYNC.md` 和本文件。

### 4.6 Tauri Client

- `client/src/main.ts` 只负责注入桌面传输并复用 Web 入口，不复制页面和业务 API。
- Web 请求只能依赖 `HttpTransport` 抽象，不能在业务组件中直接判断浏览器或 Tauri 平台。
- 服务地址只能保存合法的 HTTP/HTTPS 源地址；API 请求继续使用站内路径。
- 版本读取、EXE 哈希、签名校验、更新计划、本机文件路径和安装替换属于 Rust/Tauri 层，Vue 只负责触发和展示状态。
- 修改共用 Web 源码后必须同时执行 Web 和 Client 构建；修改 Rust、窗口、CSP 或更新器配置后还需要补充对应平台检查。
- Client 专属能力、窗口配置和打包规则写入 `client/`，不要混入 Web 组件或共用 API 层。

## 5. 验证记录

已执行并通过：

```bash
cd web && npm run build
cd client && npm run build
git diff --check
```

构建结果包括：

- Web：`vue-tsc --noEmit` 和 Vite 构建通过。
- Tauri Client：`vue-tsc --noEmit -p tsconfig.json` 和 Vite 构建通过。
- Web 与 Tauri 产物均包含 `.el-message-box`、`.el-overlay-message-box` 和 `.v-modal` 相关样式。
- Web 开发服务器入口返回 HTTP 200。
- 现有 chunk 体积提示属于构建警告，不作为本次失败处理。

尚未完成：

- 真实浏览器中的登录、登出、服务器设置、移动端导航和权限隐藏验收。
- 分页、搜索、筛选、通用新增/编辑、角色权限配置的真实 API 交互验收。
- 库存出入库、未提交表单关闭确认、模具生命周期、任务单流转、图片删除的真实 API 交互验收。
- 桌面宽度和移动宽度下 MessageBox 的视觉截图及键盘焦点验收。
- Tauri Client 在 macOS、Windows 10/11 的真实窗口、HTTP 传输、服务器地址保存和更新入口验收。
- Windows 正式版安装包、NSIS/便携版升级、断网、损坏资源、不可写目录和回滚验收；RC/手动便携客户端 Artifact 的独立启动验收。

## 6. 后续工作顺序

1. 启动 Go 服务和测试数据，按“尚未完成”清单执行浏览器和 Tauri Client 运行态验收。
2. 修复验收中发现的交互或响应式问题，并再次运行 Web/Client 构建。
3. 在不改变视觉结果的前提下，将 `styles.css` 中的业务专属规则逐步移动到对应组件的 `<style scoped>`，共用规则保留在设计系统或基础组件中。
4. 在 Windows 10/11 完成安装包、升级和回滚测试，并把结果记录到发布验收文档。
5. 解决本地 Git 分支权限问题后创建 `codex/web-modularization-dialog` 等功能分支，再进行提交和合并。

## 7. 范围边界

- 本次记录覆盖 Web 结构、交互样式、Tauri Client 复用边界、构建和验收状态。
- 不调整 Go 后端接口、权限定义、数据库结构或业务数据格式。
- 不把工作区已有的 `.codex`、`README.md`、`docs/USER_GUIDE.md`、`output/` 等无关变更纳入本次状态结论。
### 2026-08-29 正式发布 CI 第 59 次重试问题记录

- Go、Web、Tauri 前端、Rust 和 Windows 打包均通过；发布作业在 manifest 资源文件校验阶段失败。
- 失败文件为签名 NSIS 安装器，实际已随 `bb-erp-client-windows.zip` 放在 `installer/` 目录；发布作业此前没有解包客户端 ZIP。
- 当前工作流改为复用已有客户端 Artifact，在发布前提取安装器，避免额外的 NSIS 中转 Artifact；该修复未改变 all-in-one 内容或客户端更新协议。
- Gitee 正式发布和 Windows 用户升级仍待下一次同标签重试完成后验收。

### 2026-08-29 正式发布 CI 第 61 次重试问题记录

- Web、Tauri、Rust 和 Windows 正式包生成全部通过，客户端 ZIP 内 NSIS 提取也已通过。
- Gitee 成功保存四个附件，但 all-in-one 与独立便携 EXE 上传在 900 秒后无响应，不能标记正式发布完成。
- 0.0.5 改为串行可恢复上传：已存在的自动附件直接复用，请求超时后查询服务端实际状态，同名冲突停止；0.0.6 将验证从 0.0.5 的差分或完整包回退。
- Windows 10/11 的真实安装、自动更新、断网、损坏资源、不可写目录和数据保留仍待用户验收。

### 2026-08-29 正式版本 0.0.5 发布验收

- GitHub Actions #63 的 Web、Tauri、Rust、Windows 正式构建与签名通过，公开 Gitee Release 六个附件齐全。
- 独立便携 EXE 和 all-in-one 均通过首次上传及匿名 SHA-256 复验；稳定 manifest 已独立读取确认为 `0.0.5` 并包含签名 v2 更新信息。
- 历史 RC 不含 v2 payload，0.0.5 因此只提供完整更新基线；0.0.6 将验证从 0.0.5 的差分或完整包回退。
- Windows 10/11 的真实安装、升级、断网、损坏资源、不可写目录、重启和数据保留仍待用户验收。

### 2026-08-29 正式版本 0.0.6 首次发布问题

- GitHub Actions #66 已从稳定 0.0.5 生成约 3.88 MB 的 0.0.6 客户端差分，Windows 构建和签名通过。
- 首次 Gitee 发布在上传前停止，因为现有 update-manifest Artifact 未携带 manifest 引用的 `.zstpatch`；公开 0.0.5 未受影响。
- 修复为把 JSON 和可选差分放入同一内部 Artifact，不增加中转包；0.0.6 正式 Release 与 Windows 用户升级仍待重试验收。

### 2026-08-29 正式版本 0.0.6 发布验收

- GitHub Actions #69 的 Web、Tauri、Rust、Windows 构建和签名通过，公开 Gitee Release 七个附件全部通过首次上传与匿名 SHA-256 复验。
- 稳定 manifest 已独立确认为 `0.0.6`；签名 v2 payload 明确包含从 `0.0.5` 出发的约 3.88 MB 差分，以及 NSIS/便携 EXE 完整回退资源。
- 技术发布链路已完成，源码 `v0.0.6` 标签未改写；Windows 10/11 真机升级、重启、异常恢复和数据保留仍待用户验收。

### 2026-08-29 Windows 更新公钥故障与 0.0.7 / 0.0.8 验收计划

- `0.0.5` 的更新页显示公钥 Base64 解析失败，因此“最新版本”为空且尚未开始下载 `0.0.6`；这不是 Web 页面或稳定 manifest 地址错误。
- `0.0.7` 已通过启动脚本锁定包内公钥、服务端无效直接值回退和升级器替换发布文件完成修复。由于旧版检查链路已在验签入口失败，需手动部署 `0.0.7` 全量包一次。
- `0.0.7` 真机成功读取稳定 manifest 后，使用 `0.0.8` 验收检查状态、差分/完整包回退、重启版本显示和数据保留；未运行的 Windows 结果不标记为已完成。

### 2026-08-29 正式版本 0.0.7 发布验收

- GitHub Actions #73 已完成 Web、Tauri、Rust、Windows 构建和签名，Gitee 七个正式附件全部通过匿名大小与 SHA-256 复验，稳定 manifest 已确认为 `0.0.7`。
- 签名 v2 payload 明确包含从 `0.0.6` 出发的 3,875,276 字节差分，以及 NSIS/便携 EXE 完整回退资源。
- all-in-one 已独立匿名下载并确认哈希、包内公钥、便携标记及三个 Windows 启动入口；该结果可作为 `0.0.7` 手动恢复包的技术发布证据。
- Windows 10/11 的实际恢复、更新页公钥错误消失、`0.0.7 → 0.0.8` 升级、重启、异常恢复和数据保留仍待用户验收。

### 2026-08-29 正式版本 0.0.8 发布验收

- GitHub Actions #75 已完成 Web、Tauri、Rust、Windows 构建和签名，Gitee 七个正式附件全部通过匿名大小与 SHA-256 复验，稳定 manifest 已确认为 `0.0.8`。
- 签名 v2 payload 明确包含从 `0.0.7` 出发的 3,875,708 字节差分，以及 NSIS/便携 EXE 完整回退资源。
- all-in-one 已独立匿名下载并确认哈希、包内服务端/客户端版本、公钥及三个 Windows 启动入口；技术发布链路已闭环。
- Windows 10/11 的实际恢复、更新页检查、`0.0.7 → 0.0.8` 应用、重启、异常恢复和数据保留仍待用户验收，不标记为已完成。

### 2026-08-29 服务端升级下载反馈修复

- Windows 真机的客户端升级已经通过；服务端下载按钮此前直接打开 manifest 外部 URL，Tauri WebView 拦截时没有加载状态或错误提示。
- 更新中心现在校验下载地址、显示下载中状态并禁用重复点击；缺少地址、桌面端收到旧服务端外部地址、Web 弹窗被拦截、API 下载失败都会显示明确反馈，成功时提示保存文件名。
- 新服务端返回同源 `/api/v1/system/updates/server/download`，Go 使用已部署可信公钥流式完成服务端包 Minisign、1 字节至 512 MiB 大小、SHA-256 与 ZIP 结构校验；桌面请求为该大文件保留 12 分钟超时，比服务端默认下载时限多 2 分钟以接收具体错误，Web/Tauri 不承担外部服务端包的信任判断。
- Web 与 Client 生产构建已通过；Windows 新正式包中的实际大文件下载、断网/超时提示和后续批处理升级仍待复验。
