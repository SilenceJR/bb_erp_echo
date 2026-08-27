# Web 与 Tauri Client 端进度与维护规范

> 基准日期：2026-08-27
> 适用范围：`web/` Vue Web 应用、`client/` Tauri 桌面壳及两者共用的前端源码

## 1. 状态结论

Web 模块化代码改造、统一弹窗样式改造以及 Tauri Client 对共用源码的构建接入已经完成，Web 与 Tauri Client 的生产构建均已通过。

完整浏览器运行态验收尚未完成，尤其是需要真实 Go API、登录账号和业务数据的流程。业务专属 CSS 仍有一部分保留在 `web/src/styles.css`，用于维持跨页面、跨子组件和响应式样式的一致性；后续可以继续按组件拆分，但不应直接删除全局规则。

## 2. 进度清单

| 范围 | 状态 | 说明 |
| --- | --- | --- |
| 应用入口收敛 | 已完成 | `App.vue` 只负责 Element Plus 配置、登录态分支、工作台挂载和页面插槽编排。 |
| 登录与服务器设置 | 已完成 | 登录、登出、健康检查、服务器地址设置和客户端更新状态由 `useAuth` 及登录/设置组件承载。 |
| 应用壳层与导航 | 已完成 | 顶栏、侧边栏、移动端抽屉导航独立于 `AppWorkspace.vue`，继续使用 `activeKey`。 |
| 首页仪表盘 | 已完成 | 首页指标、快捷入口、业务分组和更新提示拆至 `DashboardPage.vue`。 |
| 通用资料模块 | 已完成 | 列表、分页、搜索、筛选、新增、编辑和空状态由 `ModulePage.vue` 编排。 |
| 统计报表 | 已完成 | 统计数据加载和展示已纳入模块页与工作台控制器。 |
| 库存模块 | 已完成 | 库存详情、流水、出入库、快速新增供应商和关闭未提交表单确认已拆至库存抽屉及 `useWarehouse` 状态组合式函数。 |
| 模具模块 | 已完成 | 详情、借出、归还、维修、保养、履历和请求过期保护已拆至模具抽屉及 `useMold` 状态组合式函数。 |
| 任务单模块 | 已完成 | 任务详情、办公室操作、部门子任务和流转日志已拆至任务单抽屉及 `useWorkorder` 状态组合式函数。 |
| 角色与账号权限 | 已完成 | 权限配置、角色分配、选项缓存、权限禁用规则由 `useAssignment` 管理。 |
| 图片管理 | 已完成 | `ImageGallery.vue` 的删除确认已改用统一弹窗封装。 |
| Web/Tauri 复用 | 已完成 | `client/src/main.ts` 继续直接复用 `web/src/main.ts` 和同一套业务组件。 |
| Client HTTP 传输 | 已完成 | Tauri 通过 `desktop-http.ts` 注入 Rust HTTP 传输，业务请求继续使用共用的 `HttpTransport` 抽象。 |
| Client 服务器地址 | 已完成 | 首次默认地址、登录页设置、顶栏设置和保存后的地址优先级遵循 `client/API_SYNC.md`。 |
| Client 更新入口 | 已完成 | Vue 仅展示更新状态并触发桌面更新流程；版本、EXE 哈希、签名和本机路径由 Tauri/Rust 负责。 |
| Client Tauri 构建配置 | 已完成 | `tauri.conf.json` 保持 Web 产物作为 `frontendDist`，窗口、CSP、更新器和安装模式继续由 Client 管理。 |
| MessageBox 样式 | 已完成 | 显式引入 Element Plus MessageBox 样式，补充遮罩、居中、不透明卡片、层级、按钮和移动端宽度规则。 |
| 业务 CSS 全量下沉 | 待完成 | 当前仍有较多跨组件基础和页面样式位于 `web/src/styles.css`，后续拆分需要配合 `:deep`、子组件样式和响应式规则验证。 |
| 浏览器全流程验收 | 待完成 | 需要启动 Go API 并使用测试账号完成登录、权限、库存、模具、任务单和图片流程。 |
| 功能分支与提交 | 待完成 | 当前工作区仍在 `main`；创建 `codex/*` 分支受到本地 `.git` 权限和引用目录冲突限制。 |
| Client 桌面运行态 | 待完成 | 需要在 macOS、Windows 10/11 真机完成登录、窗口、服务器切换、弹窗和更新流程验收。 |
| Client 安装包验证 | 待完成 | 需要执行 `npm run desktop:build`，并完成 Windows 安装、升级、回滚和断网场景验证。 |

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
- Windows 安装包、NSIS/便携版升级、断网、损坏资源、不可写目录和回滚验收。

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
