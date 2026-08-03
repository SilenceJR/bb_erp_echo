# BB ERP Echo 项目状态

最后更新：2026-08-03 14:48:35 CST

## 用途

这是给 AI Agent 和开发者阅读的项目交接/状态文档。后续在本仓库继续开发前，建议先阅读本文件。

## 当前方向

- 首版按单工厂、单组织使用，不做多组织 UI 或多组织业务流程。
- 后端：Go Echo v5、GORM、SQLite WAL、Casbin 权限。
- Web：Vue 3 管理端，element-plus 组件。
- 桌面端：Tauri 客户端复用 Web UI 和同一套 API。
- 当前可用范围覆盖塑胶工厂内部 ERP 的客户、联系人、仓库库存、模具、任务单、统计报表、权限、审计和桌面端连接。

## 已实现模块

### 登录、组织、权限、审计

- 支持 JWT 登录和 `/api/v1/auth/me`。
- 支持个人账号和部门终端账号。
- 默认按单组织使用。
- 使用 Casbin 按 API object/action 做权限校验。
- Web 中用户分配角色弹窗、角色配置权限弹窗已修复。
- 审计日志会区分个人操作人和部门终端账号。

### 客户与联系人

- 客户支持分页和模糊查询。
- 客户包含 `phone` 字段，可用于客户座机/基础联系电话。
- 联系人支持多条电话明细，包括号码标签和主号码标记。

### 仓库与库存

- 仓库按单仓库方向实现。
- Web 仓库页使用分类标签：
    - `product`：产品
    - `production_material`：生产物资
    - `regular_product`：常规产品
    - `daily_supply`：生活物资
- 仓库物品支持分页和模糊查询。
- 已实现库存单据、库存余额、库存流水、物品出入库、移动加权平均成本、禁止负库存、幂等键和成本字段裁剪。
- 成本和金额字段需要 `cost:view` 权限。

### 模具

- 已实现模具台账，包含属性、借出、归还、维修、保养、状态、保养周期和履历。
- 模具列表支持分页和模糊查询。

### 任务单

- `/api/v1/workorder` 已实现，`/api/v1/tasks` 保留为兼容路径。
- 任务单类型：
    - `production`：生产单
    - `general`：通用任务
- 主任务状态：
    - `draft`：草稿
    - `processing`：正在处理
    - `paused`：暂停
    - `pending_close`：待办公室确认
    - `completed_normal`：正常完成
    - `completed_forced`：强制完成
    - `cancelled`：取消
- 优先级：
    - `normal`：普通
    - `urgent`：加急
- 部门子任务状态：
    - `draft`：内部预派发状态
    - `received`：已收到
    - `processing`：正在处理
    - `partial_completed`：部分完成
    - `completed`：完成
- 办公室可创建生产单/通用任务，选择多个流转部门，派发、暂停、恢复、加急、正常完成或填写原因强制完成。
- 部门可开始处理、填写部分完成数量、完成本部门任务。
- 所有部门子任务完成后，主任务自动进入 `pending_close`。
- `WorkOrderFlowLog` 记录任务流转日志。
- Web/Tauri UI 已包含任务列表筛选、新建表单、任务详情抽屉、部门子任务卡片、办公室操作、部门操作和流转日志展示。

### 统计报表

- `/api/v1/statistics` 已实现为统计报表聚合接口。
- 统计响应包含：
    - 顶部汇总卡片
    - 库存按物品类型和物料分类统计
    - 低库存列表
    - 库存流水趋势
    - 任务单状态/类型/部门处理统计和趋势
    - 模具状态和需关注模具
    - 基础资料数量
    - 审计结果和趋势
    - 最近任务单
- 没有 `cost:view` 权限时会裁剪成本金额。
- Web/Tauri UI 已有统计报表专用页面，使用指标卡和报表面板展示。

### Tauri 桌面端连接

- 已修复桌面端服务器地址配置和测试连接。
- Tauri HTTP scope 已允许本地服务的 health/API 请求。
- Tauri 客户端复用 Web UI 行为。

## API 变更同步要求

修改 API 时必须同步以下内容：

- 后端 handler 和测试
- `docs/API.md`
- `docs/docs.go`
- `docs/swagger.json`
- `docs/swagger.yaml`
- `test.http`
- Web 模块入口和 UI
- 如果 Web UI 被 Tauri 复用，同时验证 Tauri client 构建

重要接口：

- `GET /api/v1/statistics`
- `GET /api/v1/workorder`
- `POST /api/v1/workorder`
- `POST /api/v1/workorder/:id/dispatch`
- `POST /api/v1/workorder/:id/pause`
- `POST /api/v1/workorder/:id/resume`
- `POST /api/v1/workorder/:id/urgent`
- `POST /api/v1/workorder/:id/complete`
- `POST /api/v1/workorder/department-tasks/:id/start`
- `POST /api/v1/workorder/department-tasks/:id/partial-complete`
- `POST /api/v1/workorder/department-tasks/:id/complete`
- `GET /api/v1/workorder/:id/logs`

## 验证状态

最后确认通过：

```bash
go test ./...
npm run build   # 在 web/ 目录执行
npm run build   # 在 client/ 目录执行
```

已知但不影响结果的构建提示：

- Web 构建会提示 chunk 较大。
- Client 构建会提示 `@vueuse/core` 的 Rollup 注释和 chunk 较大。

## Git 工作区注意事项

- 当前有较多未提交改动，覆盖后端、Web、client、文档和测试。
- `.idea/*`、`tmp/*`、`web/.gitkeep` 是本地未跟踪文件。不要删除，除非用户明确要求。
- `README.md` 有既有改动。不要回退无关内容。

## 下一步可做事项

- 统计报表可继续增加图表组件；当前是卡片和报表面板。
- 增加统计 CSV/XLSX 导出。
- 如果需要实时提醒，可为任务单增加 WebSocket 通知。
- 增加备份/恢复 UI 和计划备份管理。
- 如果角色边界要更严格，可继续细化办公室和部门操作权限。
