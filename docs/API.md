# 博邦 ERP API 文档

更新时间：2026-07-30

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

返回客户列表，并预加载联系人和联系人电话明细。

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

返回联系人列表，并预加载电话明细。

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

## 业务骨架接口

```text
GET  /api/v1/workorder
POST /api/v1/workorder
GET  /api/v1/statistics
POST /api/v1/statistics
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

兼容旧路径：

```text
GET  /api/v1/tasks
POST /api/v1/tasks
```

## 错误响应

统一结构：

```json
{
  "code": "BAD_REQUEST",
  "message": "请求参数校验失败",
  "request_id": "request-id"
}
```
