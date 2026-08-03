# 博邦 ERP API 文档

更新时间：2026-08-03

## 文档入口

后端启动后可访问：

```text
GET /swagger/index.html
GET /swagger/doc.json
```

本文件是便于开发交接和代码审查的 Markdown 版接口说明。  
`docs/swagger.json` 和 `docs/swagger.yaml` 是由 swaggo 生成的机器可读 OpenAPI 规范。

重要约定：后续新增、删除、修改接口时，必须同步更新：

- handler 上的 swaggo 注释，并重新运行 `swag init`
- `docs/API.md`
- 如有调试流程变化，同步更新 `test.http`

## 认证

除 `/health`、`/ready`、`/api/v1/auth/login`、`/swagger/*` 外，业务接口默认需要：

```http
Authorization: Bearer <token>
```

## 公共接口

```text
GET  /health
GET  /ready
GET  /swagger/index.html
GET  /swagger/doc.json
```

## 列表分页与模糊查询

已接入数据表格的列表接口统一支持：

```text
page=1
page_size=20
q=关键字
```

`q` 也可写作 `keyword`。后端会在各模块常用字段中做模糊查询，例如名称、编码、电话、地址、状态、路径等。

分页响应：

```json
{
  "items": [],
  "total": 36,
  "page": 1,
  "page_size": 20,
  "keyword": "ABS"
}
```

当前已支持分页和模糊查询的主要接口包括系统用户、部门、终端、角色、权限、审计、客户、联系人、供应商、仓库物品、模具台账和任务单。

## 登录认证

### POST /api/v1/auth/login

请求：

```json
{
  "username": "admin",
  "password": "admin123456"
}
```

返回：

```json
{
  "access_token": "...",
  "expires_at": "2026-07-31T17:00:00+08:00",
  "user": {
    "id": 1,
    "username": "admin",
    "account_type": "personal",
    "permissions": []
  }
}
```

### GET /api/v1/auth/me

返回当前登录账号、账号类型、内部组织 ID、部门、终端、角色和权限。首版按单厂单组织使用，不提供组织管理菜单和多组织切换。

## 客户接口

### GET /api/v1/customers

返回客户分页列表，并预加载联系人和联系人电话明细。支持 `page`、`page_size`、`q`。

### POST /api/v1/customers

只创建客户本体，不创建联系人。

请求：

```json
{
  "name": "测试客户",
  "code": "CUST-001",
  "phone": "0755-88888888"
}
```

### PATCH /api/v1/customers/:id

通过客户 ID 更新客户档案。

请求：

```json
{
  "name": "测试客户-更新",
  "code": "CUST-001",
  "phone": "0755-66666666",
  "address": "深圳市宝安区"
}
```

### DELETE /api/v1/customers/:id

软删除客户本体。

业务规则：

- 删除 Customer 不代表删除联系人。
- 联系人和联系人电话明细保留，后续可单独转移、删除或做历史追溯。

## 联系人接口

### GET /api/v1/contacts

返回联系人分页列表，并预加载电话明细。支持 `page`、`page_size`、`q`。

### GET /api/v1/contacts/:id

通过联系人 ID 查询联系人详情。

### POST /api/v1/contacts

创建客户联系人，并通过 `customer_id` 建立客户关联。

请求：

```json
{
  "customer_id": 1,
  "name": "张三",
  "phones": [
    {
      "phone": "13800000000",
      "label": "手机",
      "primary": true
    }
  ]
}
```

### PATCH /api/v1/contacts/:id

更新联系人基础信息，并按请求内容整体替换电话明细。

请求：

```json
{
  "customer_id": 1,
  "name": "张三-更新",
  "phones": [
    {
      "phone": "13600000000",
      "label": "新手机",
      "primary": true
    }
  ]
}
```

### DELETE /api/v1/contacts/:id

软删除联系人，并同步软删除其电话明细。

## 系统接口概览

```text
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

以上 GET 列表接口均支持 `page`、`page_size`、`q`。

## 库存前置闭环接口

```text
GET  /api/v1/warehouses
POST /api/v1/warehouses
GET  /api/v1/warehouse/tabs
GET  /api/v1/warehouse/items?tab=product
POST /api/v1/warehouse/items
GET  /api/v1/warehouse/items/:itemType/:itemID
GET  /api/v1/warehouse/items/:itemType/:itemID/movements
POST /api/v1/warehouse/items/:itemType/:itemID/movements
GET  /api/v1/suppliers
POST /api/v1/suppliers
PATCH /api/v1/suppliers/:id
GET  /api/v1/locations
POST /api/v1/locations
GET  /api/v1/materials
POST /api/v1/materials
GET  /api/v1/products
POST /api/v1/products
GET  /api/v1/inventory-documents
POST /api/v1/inventory-documents
POST /api/v1/inventory-documents/:id/post
POST /api/v1/inventory-documents/:id/reverse
GET  /api/v1/inventory-balances
GET  /api/v1/inventory-ledgers
```

`GET /api/v1/warehouse/items` 和 `GET /api/v1/suppliers` 支持 `page`、`page_size`、`q`，返回统一分页结构。

任务兼容旧路径：

```text
GET  /api/v1/warehouse
POST /api/v1/warehouse
GET  /api/v1/material
POST /api/v1/material
GET  /api/v1/product
POST /api/v1/product
GET  /api/v1/inventory
POST /api/v1/inventory
```

首版按单仓库使用，`/api/v1/warehouses` 只返回一个默认仓库。仓库内物品采用标签策略统一管理：

```text
product              产品，写入 products 表
production_material  生产物资，写入 materials 表且 category=生产物资
regular_product      常规产品，写入 materials 表且 category=常规产品
daily_supply         生活物资，写入 materials 表且 category=生活物资
```

仓库标签物品创建示例：

```json
{
  "tab": "production_material",
  "name": "ABS 原料",
  "code": "ABS-001",
  "unit": "kg",
  "spec": "通用",
  "safety_stock": 100000,
  "default_cost": 250
}
```

库存数量使用 4 位定点整数，例如 `10000` 表示 1 个单位。金额和单价使用分。无 `cost:view` 权限时，库存余额、流水和单据明细不返回 `avg_cost`、`unit_cost`、`amount`、`balance_amount` 等成本字段。

新界面从具体物品办理出入库。接口自动使用默认仓库、生成单据编号并立即过账，支持 `Idempotency-Key`：

```json
{
  "business_type": "purchase_inbound",
  "supplier_id": 1,
  "quantity": 1000000,
  "unit_cost": 250,
  "reason": "采购到货"
}
```

`business_type` 支持：

```text
purchase_inbound       采购入库，supplier_id 必填
return_rework_inbound  退货返工入库，customer_id 或 department_id 二选一
customer_outbound      客户出库，customer_id 必填
department_outbound    部门出库，department_id 必填
```

退货返工可选填 `original_document_id`；填写时原单必须为同一物品、同一客户或部门的已过账出库记录。旧库存单据接口继续保留，用于历史兼容和冲销。

## 任务单接口

任务单支持生产单和通用任务。主任务由办公室控制，部门只更新各自的子任务状态。

主任务状态：

```text
draft              草稿
processing         正在处理
paused             暂停
pending_close      待办公室确认
completed_normal   完成（正常完成）
completed_forced   完成（强制完成）
cancelled          取消
```

部门子任务状态：

```text
received           已收到
processing         正在处理
partial_completed  部分完成
completed          完成
```

接口清单：

```text
GET  /api/v1/workorder?page=&page_size=&q=&status=&type=&department_id=&priority=
POST /api/v1/workorder
POST /api/v1/workorder/:id/dispatch
POST /api/v1/workorder/:id/pause
POST /api/v1/workorder/:id/resume
POST /api/v1/workorder/:id/urgent
POST /api/v1/workorder/:id/complete
POST /api/v1/workorder/department-tasks/:id/start
POST /api/v1/workorder/department-tasks/:id/partial-complete
POST /api/v1/workorder/department-tasks/:id/complete
GET  /api/v1/workorder/:id/logs
```

创建生产单示例。数量使用 4 位定点整数，例如 `1000000` 表示 100 个：

```json
{
  "code": "WO-001",
  "type": "production",
  "product_name": "白色外壳",
  "planned_quantity": 1000000,
  "unit": "个",
  "due_at": "2026-08-10",
  "priority": "normal",
  "target_department_ids": [2, 3],
  "description": "注塑后流转到包装"
}
```

派发后系统自动把每个目标部门子任务置为 `received`，主任务置为 `processing`。部门可执行开始处理、部分完成和完成；全部部门完成后主任务自动进入 `pending_close`。办公室正常完成必须在 `pending_close` 执行；强制完成可在 `processing`、`paused`、`pending_close` 执行，但必须填写原因。

兼容旧路径：

```text
GET  /api/v1/tasks
POST /api/v1/tasks
```

## 统计报表接口

```text
GET  /api/v1/statistics
```

`GET /api/v1/statistics` 返回 Web/Tauri 统计首页聚合数据，包含：

- 顶部汇总：客户、供应商、联系人、仓库物品、库存总量、低库存、进行中任务、加急任务、待办公室确认任务、模具总数、需关注模具。
- 库存统计：按物品类型汇总、按物料分类汇总、低库存明细、近 14 天库存流水趋势。
- 任务统计：按主任务状态、任务类型、部门子任务处理情况、近 14 天任务创建趋势。
- 模具统计：按状态汇总、借出/维修/保养到期等需关注模具。
- 业务数据：客户、联系人、供应商、产品、物料、模具、任务单数量。
- 审计统计：按结果汇总和近 14 天趋势。

无 `cost:view` 权限时，响应中不会返回可用金额，`inventory_amount`、库存分类 `amount`、低库存 `amount` 和趋势 `amount` 均裁剪为 `0`。

响应片段：

```json
{
  "can_view_cost": false,
  "summary": {
    "customers": 10,
    "warehouse_items": 36,
    "inventory_quantity": 1280000,
    "low_stock_items": 2,
    "open_workorders": 5
  },
  "inventory": {
    "by_item_type": [{"name": "product", "value": 800000}],
    "low_stock": []
  },
  "workorders": {
    "by_status": [{"name": "processing", "value": 3}],
    "by_department": []
  }
}
```

## 模具接口

常用模具档案字段包括：模具编号、名称、客户 ID、产品 ID、穴数、成型材料、钢材、尺寸、重量、制造商、所有权、存放位置、当前位置、状态、保养周期、最近维修时间、最近/下次保养时间和备注。

状态约定：

```text
in_stock      在库
loaned        已借出
repairing     维修中
maintenance   保养中
scrapped      报废
```

```text
GET    /api/v1/molds
GET    /api/v1/molds/:id
POST   /api/v1/molds
PATCH  /api/v1/molds/:id
DELETE /api/v1/molds/:id
POST   /api/v1/molds/:id/loan
POST   /api/v1/molds/:id/return
POST   /api/v1/molds/:id/repair
POST   /api/v1/molds/:id/maintenance
```

`GET /api/v1/molds` 支持 `page`、`page_size`、`q`，并可继续使用 `status` 精确筛选状态。

兼容旧路径：

```text
GET  /api/v1/mold
POST /api/v1/mold
```

创建模具示例：

```json
{
  "code": "MOLD-001",
  "name": "白壳前模",
  "customer_id": 1,
  "product_id": 1,
  "cavity_count": 8,
  "mold_material": "ABS",
  "steel": "P20",
  "size": "450x350x280",
  "weight_gram": 180000,
  "manufacturer": "深圳模具厂",
  "owner": "客户A",
  "storage_location": "工模架 A1",
  "maintenance_cycle_days": 30,
  "remark": "白壳产品常用模具"
}
```

借出示例：

```json
{
  "location": "注塑车间 1 号机",
  "counterparty": "注塑部",
  "handler_name": "王工",
  "reason": "生产试模"
}
```

维修和保养都会写入 `mold_events` 履历；完成保养时会按保养周期自动计算 `next_maintenance_at`。

## 错误响应

统一结构：

```json
{
  "code": "BAD_REQUEST",
  "message": "请求参数校验失败",
  "request_id": "request-id"
}
```
