# 博邦 ERP API 文档

更新时间：2026-08-29

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

除 `/health`、`/ready`、`/api/v1/auth/login`、`/api/v1/auth/refresh`、`/api/v1/auth/logout`、`/swagger/*` 外，业务接口默认需要：

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
  "expires_at": "2026-08-28T14:00:00+08:00",
  "refresh_token": "...",
  "refresh_expires_at": "2026-09-27T12:00:00+08:00",
  "user": {
    "id": 1,
    "username": "admin",
    "account_type": "personal",
    "permissions": []
  }
}
```

默认 access token 有效期为 2 小时，refresh token 默认在 30 天无成功续期后失效；每次续期都会轮换 refresh token。服务端可通过 `BB_ERP_JWT_EXPIRES_IN` 和 `BB_ERP_JWT_REFRESH_EXPIRES_IN` 环境变量覆盖。

### POST /api/v1/auth/refresh

续期接口不需要 Bearer Token，只需提交最近一次登录或续期返回的 refresh token：

```json
{
  "refresh_token": "..."
}
```

成功返回与登录相同的令牌和当前用户结构；旧 refresh token 立即失效。令牌无效、已过期或已轮换返回 `401`，账号停用返回 `403`。

### POST /api/v1/auth/logout

撤销当前 refresh token，成功返回 `204 No Content`。客户端仍应立即清理本地 access token；已经签发的 access token 会在自身过期或密码版本变化前保持可验证。

### POST /api/v1/auth/change-password

修改当前登录账号密码。请求必须携带当前有效的 Bearer Token；成功后旧 JWT 立即失效，同时撤销该账号的全部 refresh token，需要重新登录。

请求：

```json
{
  "current_password": "admin123456",
  "new_password": "newAdmin123456"
}
```

成功返回 `204 No Content`。当前密码错误返回 `401`，新密码不符合长度要求或与旧密码相同返回 `400`。

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
PUT  /api/v1/system/departments/:id
PATCH /api/v1/system/departments/:id/status
GET  /api/v1/system/departments/:id/employees
PUT  /api/v1/system/departments/:id/employees
GET  /api/v1/system/employees
POST /api/v1/system/employees
PUT  /api/v1/system/employees/:id
PATCH /api/v1/system/employees/:id/status
DELETE /api/v1/system/employees/:id
GET  /api/v1/system/terminals
POST /api/v1/system/terminals
GET  /api/v1/system/users
POST /api/v1/system/users
PATCH /api/v1/system/users/:id/status
PATCH /api/v1/system/users/:id/affiliation
POST /api/v1/system/users/:id/reset-password
POST /api/v1/system/users/:id/roles
GET  /api/v1/system/roles
POST /api/v1/system/roles
POST /api/v1/system/roles/:id/permissions
GET  /api/v1/system/permissions
GET  /api/v1/system/audits
GET  /api/v1/system/updates/status
POST /api/v1/system/updates/check
GET  /api/v1/system/updates/server/download
GET  /api/v1/operator-employees
```

员工是独立业务档案，不等同于登录账号。员工可属于零个或多个部门；部门成员关系通过 `PUT /api/v1/system/departments/:id/employees` 以 `employee_ids` 数组原子替换。员工列表支持 `page`、`page_size`、`q`、`department_id` 和 `status`，返回按 `Asia/Shanghai` 当前日期计算的周岁和所属部门摘要。员工 `DELETE` 仅停用档案，可通过状态接口恢复；部门也只启停、不物理删除。停用部门不能新增成员、绑定账号或执行业务写入。

部门成员配置同时要求 `system:departments:write` 和 `system:employees:read`；员工档案使用 `system:employees:read/write`。管理员可通过用户归属接口修正账号的部门和终端。`GET /api/v1/operator-employees` 与完整员工档案权限分离，只返回当前账号部门及在职候选员工的 `id/name`。

任务单及仓库/库存写请求统一要求 `operator_employee_id`。服务端在业务事务内重新读取当前账号并校验账号部门、部门状态、员工状态、组织和成员关系：缺字段返回 `400`，无部门或越权返回 `403`，员工/部门停用或成员关系失效返回 `409`。任务与库存状态流转使用带原状态及旧完成数量条件的更新，验证失败、并发状态冲突或关系刚失效均不产生部分业务写入。历史记录同时保留登录账号、终端、所选操作员工及请求时部门快照；旧数据不补造员工。操作员工是当前登录账号对现场责任人的申报，不等同于员工本人完成 PIN、刷卡或二次认证；追责时必须同时查看不可替代的登录账号和终端信息。带 `Idempotency-Key` 的库存请求只有在接口范围、账号/组织、规范化请求内容和操作员工都与首次请求一致时才返回原结果，任一差异均返回 `409`。旧数据库启动时会把同名普通索引显式升级为非空部分唯一索引；发现历史非空重复键时阻断启动并报告键和数量，不会静默删除数据。

## 版本与更新

更新源是普通 HTTPS JSON 地址，不绑定 Gitee 或 GitHub API。服务启动后异步检查，之后按配置周期检查；失败不会阻止业务服务，并保留上一次成功状态和已校验缓存。

```text
GET  /api/v1/version
GET  /api/v1/updates/client/status?current_version=1.2.2
GET  /api/v1/updates/client/download
GET  /api/v1/updates/client/plan?current_version=1.2.2&current_sha256=<sha256>&target=windows-x86_64&install_mode=portable
GET  /api/v1/updates/client/tauri/windows/x86_64/1.2.2
GET  /api/v1/updates/client/artifacts/<sha256>
GET  /api/v1/system/updates/status
POST /api/v1/system/updates/check
GET  /api/v1/system/updates/server/download
```

- `/api/v1/version`、`status` 和 `download` 保持既有兼容；新客户端发现 `/plan` 为 `404` 时退回旧版完整 ZIP 体验。
- Tauri 必须用 `current_version` 传真实安装版本；Web 不传桌面版本。
- 客户端下载接口只分发已通过大小、SHA-256 和 ZIP 校验的本地缓存包。
- `/plan` 仅支持 `windows-x86_64`，无更新返回 `204`；按精确版本、当前 EXE SHA、布局与缓存状态返回 `delta` 或 `full`，并始终带完整兜底资源。
- `/tauri/{target}/{arch}/{current_version}` 返回 Tauri updater 的 `version/url/signature`，无更新返回 `204`。
- `/artifacts/{sha256}` 不接受文件路径，只分发当前已验签 v2 manifest 声明并缓存的资源，支持 `ETag`、`Content-Length` 与 HTTP Range。
- `client_update_v2.payload` 是原始 JSON 的 Base64，`signature` 是 Tauri `.sig` 文件内容的 Base64；服务端必须配置对应 Minisign 公钥后才接受 v2 更新。
- `GET /api/v1/system/updates/status` 需要 `system:updates:read`。
- `POST /api/v1/system/updates/check` 需要 `system:updates:write`，立即执行完整检查并返回与 GET 相同的结构。检查失败也返回状态结构，错误在 `last_error` 中，便于管理页同时保留历史成功状态。
- `GET /api/v1/system/updates/server/download` 需要 `system:updates:read`。服务端按最近一次成功清单下载或复用缓存，并在返回附件前使用当前部署的可信公钥流式验证 Minisign 签名，同时校验文件大小（1 字节至 512 MiB）、SHA-256、ZIP 安全边界和必需文件；并发请求合并为一次下载。下载或校验失败返回 `502` 及具体错误，不会把损坏包写入正式缓存。
- 更新状态中的服务端 `download_url`/`download_path` 指向上述同源受保护接口，不再把外部下载地址直接交给 Tauri WebView；下载只提供升级包，不会自动替换当前进程。

更新状态示例：

```json
{
  "enabled": true,
  "manifest_url": "https://gitee.com/example/bb-erp-release/raw/main/update-manifest.json",
  "reachable": true,
  "checking": false,
  "check_interval": "6h0m0s",
  "interval_seconds": 21600,
  "last_attempt_at": "2026-08-26T02:00:00+08:00",
  "last_success_at": "2026-08-26T02:00:02+08:00",
  "next_check_at": "2026-08-26T08:00:02+08:00",
  "server": {
    "current_version": "1.2.2",
    "latest_version": "1.2.3",
    "available": true,
    "file_name": "bb-erp-server-windows.zip",
    "download_path": "/api/v1/system/updates/server/download",
    "download_url": "/api/v1/system/updates/server/download",
    "size": 12345678,
    "sha256": "..."
  },
  "client": {
    "current_version": "1.2.2",
    "latest_version": "1.2.3",
    "available": true,
    "cached": true,
    "file_name": "bb-erp-client-windows.zip",
    "download_path": "/api/v1/updates/client/download"
  },
  "client_protocol_version": 2,
  "client_full_cached": true,
  "client_delta_cached": true,
  "client_delta_from_version": "1.2.2",
  "client_cache_bytes": 34567890,
  "client_delta_degraded": ""
}
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

`GET /api/v1/warehouse/items` 和 `GET /api/v1/suppliers` 支持 `page`、`page_size`、`q`，返回统一分页结构。
`GET /api/v1/warehouse/items/:itemType/:itemID` 返回默认仓库内所有库位的库存合计；没有余额记录时数量为 `0`。无 `cost:view` 权限时不返回成本字段。

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

首版按单仓库使用，`/api/v1/warehouses` 只返回初始化阶段创建的默认仓库。系统编码固定为 `MAIN`，更新接口只允许修改名称；请求省略 `code` 或传 `MAIN`，传入其他编码返回 `400`，避免库存被拆到两个仓库。仓库内物品采用标签策略统一管理：

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
  "default_cost": 250,
  "operator_employee_id": 12
}
```

库存数量使用 4 位定点整数，例如 `10000` 表示 1 个单位。金额和单价使用分。无 `cost:view` 权限时，库存余额、流水和单据明细不返回 `avg_cost`、`unit_cost`、`amount`、`balance_amount` 等成本字段。

Web/Tauri 入库数量支持直接输入 `0–999999999` 范围内、最多 4 位小数的校准值；提交时仍转换为上述四位定点整数。数量为 0 或超过上限时不会创建库存流水。

新界面从具体物品办理出入库。接口自动使用默认仓库、生成单据编号并立即过账，支持 `Idempotency-Key`：

```json
{
  "business_type": "purchase_inbound",
  "supplier_id": 1,
  "quantity": 1000000,
  "unit_cost": 250,
  "reason": "采购到货",
  "operator_employee_id": 12
}
```

同一 `Idempotency-Key` 只有在接口范围、登录账号、组织、操作员工及规范化业务请求内容全部一致时才返回首次结果；跨接口复用，或物品、数量、单价、供应商/客户/部门、原因等任一字段不同，均返回 `409`，不会静默复用旧单据。

`business_type` 支持：

```text
purchase_inbound       采购入库，supplier_id 必填
return_rework_inbound  退货返工入库，customer_id 或 department_id 二选一
customer_outbound      客户出库，customer_id 必填
department_outbound    部门出库，department_id 必填
```

退货返工可选填 `original_document_id`；填写时原单必须为同一物品、同一客户或部门的已过账出库记录。旧库存单据接口继续保留，用于历史兼容和冲销。即时出入库的创建和立即过账使用同一操作员工；库存单据创建、过账、冲销可以分别选择员工，并分别保存员工与部门快照。

## 图片文件接口

图片接口均需要 `Authorization: Bearer <token>`。图片元数据保存在数据库，物理文件默认存放在 `static/uploads`，可通过 `BB_ERP_FILES_ROOT_DIR` 覆盖。系统不公开静态直链，读取图片内容必须使用受保护的 content 接口。

`owner_type` 只接受以下四种值：

```text
product          仓库产品，权限继承 product=warehouse
mold             模具，权限继承 mold
workorder        任务单，权限继承 workorder
department_task  部门子任务，权限继承 workorder，并兼容旧 tasks 权限
```

### GET /api/v1/files?owner_type=&owner_id=&category=

按业务对象查询图片元数据。`owner_type` 和 `owner_id` 必填，`category` 可选且精确匹配。返回数组，每项包含：`id`、`owner_type`、`owner_id`、`uploaded_by`、`category`、`original_name`、`size`、`mime_type`、`extension`、可选的 `replaces_id`、`content_url` 和 `created_at`。

### POST /api/v1/files/images

使用 `multipart/form-data` 上传一张或多张图片。多图时重复使用同名 `file` 字段，字段如下：

```text
file        必填，图片文件；至少一个，可重复传入多个文件
owner_type  必填，product、mold、workorder 或 department_task
owner_id    必填，关联业务对象 ID
category    可选，图片分类
```

每张文件大小不得超过 20 MiB；格式白名单为 JPEG、PNG、WebP、GIF，文件扩展名和检测到的 MIME 必须匹配。成功返回 HTTP 201 和图片元数据数组，单图时数组长度为 1。批量上传采用全有或全无处理，任一文件校验或保存失败时整批不入库并清理已写入的文件。

### GET /api/v1/files/:id/content

使用 Bearer 权限读取图片二进制内容，返回原始图片 MIME 类型；响应带 `Content-Disposition: inline`，不是公开静态资源。

### PUT /api/v1/files/:id/content

使用 `multipart/form-data` 替换图片，字段为必填 `file` 和可选 `category`。未提供 `category` 时沿用旧分类。替换成功返回新的图片元数据；旧元数据软删除并清理旧物理文件。

### DELETE /api/v1/files/:id

软删除图片元数据并清理对应物理文件，成功返回 HTTP 204。

权限规则：产品图片使用仓库权限，模具图片使用模具权限，任务单和部门子任务图片使用任务单权限，同时兼容 `/api/v1/tasks` 权限。部门子任务的写入操作还限制为该子任务所属部门；读取不增加此部门限制。

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
POST /api/v1/workorder/products
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

创建生产单时必须从仓库产品列表（`GET /api/v1/warehouse/items?tab=product&q=关键字`）中选择启用产品并提交 `product_id`。服务端根据产品主数据保存 `product_name` 和 `unit` 快照；请求中的名称和单位不作为自由文本接受。数量使用 4 位定点整数，例如 `1000000` 表示 100 个：

```json
{
  "code": "WO-001",
  "type": "production",
  "product_id": 1,
  "planned_quantity": 1000000,
  "due_at": "2026-08-10",
  "priority": "normal",
  "target_department_ids": [2, 3],
  "description": "注塑后流转到包装",
  "operator_employee_id": 12
}
```

成功响应中的 `product_id`、`product_name` 和 `unit` 分别是关联产品 ID 及创建时的名称、单位快照；产品后续改名不会改写历史任务单。生产单详情需要实时库存时，按 `product_id` 调用 `GET /api/v1/warehouse/items/product/:product_id`，响应中的 `quantity` 是默认仓库全部库位合计。

### POST /api/v1/workorder/products

在生产单内临时建立尚未入库产品的正式仓库产品档案。接口同时需要 `workorder:write` 和 `workorder:temporary-product:write`，默认只有超级管理员拥有后者；管理员可在角色权限中显式分配。新产品立即启用，安全库存和当前库存为 `0`，不会创建库存余额或库存流水。

请求：

```json
{
  "name": "白色外壳",
  "code": "P-001",
  "spec": "标准",
  "unit": "个",
  "operator_employee_id": 12
}
```

`name`、`code` 必填，`spec` 可选，`unit` 缺省为 `个`；编码重复返回 `409`。创建成功返回标准 `model.Product`，前端可立即用返回的 ID 选中产品并查询库存。

派发、暂停、恢复、加急、正常/强制完成，以及部门开始、部分完成、完成的 JSON 请求体都必须携带 `operator_employee_id`；强制完成还必须填写原因。部门部分完成提交的是累计完成数量，必须严格大于当前累计值且小于计划数量，重复提交相同累计值返回 `400`。派发后系统自动把每个目标部门子任务置为 `received`，主任务置为 `processing`。部门可执行开始处理、部分完成和完成；全部部门完成后主任务自动进入 `pending_close`。流转日志保存操作员工、当前账号部门、登录账号和终端快照，员工改名、调部门或停用不会改写历史。

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
