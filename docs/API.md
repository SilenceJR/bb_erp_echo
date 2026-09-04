# 博邦 ERP API 文档

更新时间：2026-09-04

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

除 `/health`、`/ready`、`/api/v1/discovery/identity`、`/api/v1/auth/login`、`/api/v1/auth/refresh`、`/api/v1/auth/logout`、`/swagger/*` 外，业务接口默认需要：

```http
Authorization: Bearer <token>
```

## 公共接口

```text
GET  /health
GET  /ready
GET  /api/v1/discovery/identity
GET  /swagger/index.html
GET  /swagger/doc.json
```

## 局域网发现

Windows Tauri 客户端启动时可向 UDP `39080` 广播一次发现请求；服务端只
接受 loopback 或 RFC1918 IPv4 来源，并严格限制报文不超过 512 字节。服务端
会把响应单播回请求来源，不携带或信任任意 URL：

```json
{"kind":"discover","protocol":1,"nonce":"0123456789abcdef0123456789abcdef"}
```

服务端响应：

```json
{
  "kind": "announce",
  "protocol": 1,
  "nonce": "0123456789abcdef0123456789abcdef",
  "product": "bb-erp",
  "instance_id": "稳定 UUID",
  "server_name": "服务器名称",
  "http_port": 8080
}
```

客户端必须保持 nonce 原样匹配，并使用 UDP 响应来源 IP 与 `http_port` 构造
HTTP 地址；随后依次验证 `GET /ready` 返回 `200` 且 `status=ready`，以及
`GET /api/v1/discovery/identity` 返回的 `instance_id` 与 UDP 响应一致。候选
HTTP 请求不跟随重定向、不使用系统代理，响应体有大小上限；响应带有
`Cache-Control: no-store`，身份 DTO 拒绝未知字段，`server_name` 和
`server_version` 分别限制为最多 120/64 字节且不得包含控制字符。验证失败的
地址不可保存。多个已验证服务不得静默选择。

`GET /api/v1/discovery/identity` 是匿名接口，只返回客户端连接验证所需的
`product`、`discovery_protocol`、`instance_id`、`server_name` 和
`server_version`，不返回组织、账号、业务数据、更新地址或凭据。

服务端启动时会在数据库就绪后执行一次同协议预检；发现任何一个已通过
`/ready` 与身份验证的服务都会拒绝本实例启动，即使双方 `instance_id` 相同。
预检默认在 3 秒全局截止时间内完成，其中 UDP 收集窗口为 2.5 秒；最多收集
24 个去重候选，并以最多 4 个并发 HTTP 验证任务执行，慢候选不会阻塞其他候选。
预检没有收到响应时继续启动，UDP 响应器运行期的监听失败则触发服务整体关闭。

发现配置可通过环境变量覆盖：`BB_ERP_DISCOVERY_ENABLED`（默认 `true`）、
`BB_ERP_DISCOVERY_SERVER_NAME`（默认本机主机名）、`BB_ERP_DISCOVERY_BIND_HOST`
（默认 `0.0.0.0`）、`BB_ERP_DISCOVERY_PORT`（默认 `39080`），以及扫描、预检和
HTTP 验证超时。测试夹具应将 `BB_ERP_DISCOVERY_ENABLED` 设为 `false`，避免
多个进程争用发现端口。

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

当前已支持分页和模糊查询的主要接口包括系统用户、部门、终端、角色、权限、审计、客户资料、供应商、仓库物品、模具和任务单。

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

客户模块采用 `CustomerCode 1:N CustomerProfile`：客户编码是稳定业务锚点，客户资料保存简称、名称、地址、电话、联系人、联系人电话和业务员。除编码关联外其余字段均可空，联系人不再有独立接口或子表。新库存单和任务单的 `customer_id` 表示具体客户资料 ID；模具不再关联客户或产品主数据。

编码规范为 `BB-` 加至少三位正整数；`1`、`BB-1`、`bb-001` 均规范化为 `BB-001`。自动编号使用现有最大数字加一，不回填缺号。

### 客户编码

- `GET /api/v1/customer-codes?page=1&page_size=20&q=&filter=all|multiple|empty`：按编码分组返回资料、资料数和默认资料。
- `GET /api/v1/customer-codes/next`：返回下一个建议编码。
- `POST /api/v1/customer-codes`：创建编码；`{"code":"1"}` 创建 `BB-001`，留空时自动编号。
- `PATCH /api/v1/customer-codes/:id`：修改编码。
- `DELETE /api/v1/customer-codes/:id`：仅无关联资料时允许物理删除，否则返回 `409`。

### 客户资料

- `GET /api/v1/customers?page=1&page_size=20&q=`：资料分页查询。
- `GET /api/v1/customers/:id`：资料详情。
- `GET /api/v1/customers/options?q=`：统一业务选择项，返回资料 ID、编码、简称、名称和默认标记。
- `POST /api/v1/customers`：创建资料；`customer_code_id` 必填，其余业务字段可空。
- `PATCH /api/v1/customers/:id`：更新资料，不能更换所属编码。
- `PUT /api/v1/customers/:id/default`：把同编码资料切换为默认。
- `DELETE /api/v1/customers/:id?replacement_id=2`：物理删除资料。删除仍有同码资料的默认资料时必须传同编码替代资料；被库存单或任务单引用时返回 `409`。

只要编码下存在资料，就恰好一条默认资料：首条自动默认，追加资料不改变原默认；删除默认资料必须在同一事务指定替代项，删除后只剩一条时该资料保持默认。

### Excel 导入、导出与预览

- `GET /api/v1/customers/import-template`：下载 `.xlsx` 模板。
- `POST /api/v1/customers/import/preview`：multipart `file`，接受 `.xls/.xlsx`，限制 10 MiB、10,000 条、只读取首个工作表。
- `POST /api/v1/customers/import/commit`：multipart 重新上传同一 `file` 并传 `token`；令牌绑定用户、模块和文件 SHA-256，30 分钟过期且只能成功提交一次。任一错误整批回滚。
- `GET /api/v1/customers/export/preview?scope=current|all&q=&filter=&page=1&page_size=50`：返回最终工作表使用的九列元数据和分页标准化行；最大预览页大小 100。
- `GET /api/v1/customers/export?scope=current|all&q=&filter=`：按下载时最新数据生成样式化 `.xlsx`；空结果返回 `422`。

导出预览和下载共用同一列定义和数据转换，列序固定为：序号、客户编码、客户简称、客户名称、地址、电话、联系人、联系人电话、业务员。电话列按文本写入。权限为 `customers:read`（查看、预览、导出）、`customers:write`（编码与资料 CRUD）、`customers:import`（导入预览与确认）。本版本仅支持全新数据库，不提供旧客户、联系人表升级或迁移。

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

管理员通过 `POST /api/v1/system/users/:id/reset-password` 重置其他账号密码时，服务端在同一事务中递增目标账号的 `password_version` 并撤销该账号全部 refresh token；旧 access token 会因密码版本不匹配返回 `401`，旧 refresh token 也返回 `401`，目标账号必须使用新密码重新登录。

任务单及仓库/库存写请求统一要求 `operator_employee_id`。服务端在业务事务内重新读取当前账号并校验账号部门、部门状态、员工状态、组织和成员关系：缺字段返回 `400`，无部门或越权返回 `403`，员工/部门停用或成员关系失效返回 `409`。任务与库存状态流转使用带原状态及旧完成数量条件的更新，验证失败、并发状态冲突或关系刚失效均不产生部分业务写入。历史记录同时保留登录账号、终端、所选操作员工及请求时部门快照；新库不补造历史员工。操作员工是当前登录账号对现场责任人的申报，不等同于员工本人完成 PIN、刷卡或二次认证；追责时必须同时查看不可替代的登录账号和终端信息。带 `Idempotency-Key` 的库存请求只有在接口范围、账号/组织、规范化请求内容和操作员工都与首次请求一致时才返回原结果，任一差异均返回 `409`。新 SQLite schema 直接创建非空部分唯一幂等索引，不包含旧库索引升级或重复历史数据修复逻辑。

创建部门终端账号时，账号主记录和默认角色绑定在同一事务中写入，事务提交后立即刷新 Casbin 内存策略；默认角色配置缺失或绑定失败会返回服务端错误并回滚账号，不会返回 `201` 或留下无角色账号。数据库提交成功但策略刷新失败时返回 `503`，明确提示稍后重试，不会虚报账号已完全可用。角色权限编码必须全部存在，拼写错误或空结果会直接失败，不会被解释为绑定全部权限。

## 版本与更新

更新源是当前配置的 JSON 清单。服务启动后异步检查，之后按配置周期检查；失败不会阻止业务服务，并保留上一次成功状态和已校验缓存。Windows 客户端只连接已验证的 loopback/RFC1918 HTTP 服务，所有更新资源均由该内网服务同源代理。

```text
GET  /api/v1/version
GET  /api/v1/updates/client/plan?current_version=1.2.2&target=windows-x86_64&install_mode=portable
GET  /api/v1/updates/client/tauri/windows/x86_64/1.2.2
GET  /api/v1/updates/client/artifacts/<sha256>
GET  /api/v1/system/updates/status
POST /api/v1/system/updates/check
GET  /api/v1/system/updates/server/download
```

- Tauri 必须用 `current_version` 传真实安装版本；Web 不传桌面版本。客户端直接要求 `/plan` 当前契约，不执行协议降级，也不请求已删除的 `/updates/client/status` 或 `/updates/client/download`。
- 客户端资源接口只分发已通过大小、SHA-256、签名和安装布局校验的本地缓存包。
- `/plan` 仅支持 `windows-x86_64` 与 `nsis|portable`，无更新返回 `204`；`strategy` 固定为 `full`，资源与安装模式一一对应，不接受差分字段。
- `/tauri/{target}/{arch}/{current_version}` 返回 Tauri updater 的 `version/url/signature`，无更新返回 `204`。
- `/artifacts/{sha256}` 不接受文件路径，只分发当前已验签 manifest 声明并缓存的 NSIS/portable 完整资源，支持 `ETag`、`Content-Length` 与 HTTP Range。
- `client_update_v2.payload` 是原始 JSON 的 Base64，`signature` 是 Tauri `.sig` 文件内容的 Base64；payload 只允许 `protocol_version/version/target/layout_version/full`，其中 `full` 必须同时包含 NSIS 与 portable。服务端必须配置对应 Minisign 公钥，未知字段（包括 `deltas`）会被拒绝。
- 外层更新清单同样拒绝重复 JSON key、未知字段和尾随 JSON 内容；清单解析失败不会替换上一次成功状态或缓存。
- `GET /api/v1/system/updates/status` 需要 `system:updates:read`。
- `POST /api/v1/system/updates/check` 需要 `system:updates:write`，立即执行完整检查并返回与 GET 相同的结构。检查失败也返回状态结构，错误在 `last_error` 中，便于管理页同时保留历史成功状态。
- `GET /api/v1/system/updates/server/download` 需要 `system:updates:read`。服务端按最近一次成功清单下载或复用缓存，并在返回附件前使用当前部署的可信公钥流式验证 Minisign 签名，同时校验文件大小（1 字节至 512 MiB）、SHA-256、ZIP 安全边界和必需文件；并发请求合并为一次下载。下载或校验失败返回 `502` 及具体错误，不会把损坏包写入正式缓存。
- 更新状态中的服务端 `download_url`/`download_path` 指向上述同源受保护接口，不再把外部下载地址直接交给 Tauri WebView；下载只提供升级包，不会自动替换当前进程。

更新状态示例：

```json
{
  "enabled": true,
  "manifest_url": "http://192.168.1.10/releases/update-manifest.json",
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
    "size": 18600000,
    "sha256": "..."
  },
  "client_protocol_version": 2,
  "client_full_cached": true,
  "client_cache_bytes": 24000000
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

退货返工可选填 `original_document_id`；填写时原单必须为同一物品、同一客户或部门的已过账出库记录。即时出入库的创建和立即过账使用同一操作员工；库存单据创建、过账、冲销可以分别选择员工，并分别保存员工与部门快照。

## 图片文件接口

图片接口均需要 `Authorization: Bearer <token>`。图片元数据保存在数据库，物理文件默认存放在 `static/uploads`，可通过 `BB_ERP_FILES_ROOT_DIR` 覆盖。系统不公开静态直链，读取图片内容必须使用受保护的 content 接口。

`owner_type` 只接受以下四种值：

```text
product          仓库产品，权限继承 product=warehouse
mold             模具，权限继承 mold
workorder        任务单，权限继承 workorder
department_task  部门子任务，权限继承 workorder
```

### GET /api/v1/files?owner_type=&owner_id=&category=

按业务对象查询图片元数据。`owner_type` 和 `owner_id` 必填，`category` 可选且精确匹配。返回数组，每项包含：`id`、`owner_type`、`owner_id`、`uploaded_by`、`category`、`original_name`、`size`、`mime_type`、`extension`、可选的 `replaces_id`、原图 `content_url`、可选静态预览 `preview_url` 和 `created_at`。旧记录没有预览文件时不返回 `preview_url`。

### POST /api/v1/files/images

使用 `multipart/form-data` 上传一张或多张图片。多图时重复使用同名 `file` 字段，字段如下：

```text
file        必填，图片文件；至少一个，可重复传入多个文件
owner_type  必填，product、mold、workorder 或 department_task
owner_id    必填，关联业务对象 ID
category    可选，图片分类
```

格式支持 JPG/JPEG/JFIF、PNG、GIF、WebP、HEIC/HEIF、AVIF、BMP、TIF/TIFF 和 SVG。服务端按扩展名选择解码器并验证真实内容，保留原图，同时生成最长边不超过 2560 像素的 JPEG 静态预览；动画和多帧图片只取首帧/封面，JPEG 预览会应用手机照片的 EXIF 方向。图片接口取消原 20 MiB 单文件业务限制，但仍受服务端全局请求上限和转换安全预算约束：单批最多 100 张且本批静态预览合计不超过 256 MiB，原图像素总量不得超过 3200 万，HEIC/HEIF/AVIF 当前安全解码输入上限为 128 MiB，SVG 上限为 8 MiB且只生成栅格预览，同时最多执行两个图片转换任务。失败响应包含文件名和可理解原因，请求编号由统一错误响应提供。成功返回 HTTP 201 和图片元数据数组，单图时数组长度为 1。批量上传采用全有或全无处理，任一文件校验、预览生成或保存失败时整批不入库并清理已写入的原图和预览文件。

### GET /api/v1/files/:id/content

使用 Bearer 权限读取原图二进制内容，返回原始图片 MIME 类型；不是公开静态资源。SVG 原图使用 `Content-Disposition: attachment`，避免把不可信矢量内容直接嵌入同源页面，其他格式使用 `inline`。

### GET /api/v1/files/:id/preview

使用 Bearer 权限读取服务端生成的 JPEG 静态预览。图库优先使用该地址，避免一次性下载 HEIC/TIFF 等原图或高清大图；旧记录没有 `preview_url` 时使用原 `content_url`。新记录的预览读取失败时客户端不自动下载高清原图，而是保留文件名并显示失败原因、重试入口和可用的请求编号。

### PUT /api/v1/files/:id/content

使用 `multipart/form-data` 替换图片，字段为必填 `file` 和可选 `category`。未提供 `category` 时沿用旧分类。替换在同一数据库事务内条件删除旧记录并创建新记录；旧记录已被其他操作变更时返回明确冲突提示。事务提交后清理旧原图及预览文件，瞬时清理失败会写入待清理任务并在服务下次启动时重试。

### DELETE /api/v1/files/:id

软删除图片元数据并在提交后清理对应物理文件，成功返回 HTTP 204；瞬时清理失败不会恢复出指向残缺文件的可见记录，而是写入待清理任务并在服务下次启动时重试。

权限规则：产品图片使用仓库权限，模具图片使用模具权限，任务单和部门子任务图片使用任务单权限。部门子任务的写入操作还限制为该子任务所属部门；读取不增加此部门限制。

模具导入需要额外的 `mold:import` 权限；导出、位置字典读取使用 `mold:read`，模具和位置写入、图片/DWG 删除使用 `mold:write`。

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

## 统计报表接口

```text
GET  /api/v1/statistics
```

`GET /api/v1/statistics` 返回 Web/Tauri 统计首页聚合数据，包含：

- 顶部汇总：客户编码、供应商、仓库物品、库存总量、低库存、进行中任务、加急任务、待办公室确认任务和模具总数。
- 库存统计：按物品类型汇总、按物料分类汇总、低库存明细、近 14 天库存流水趋势。
- 任务统计：按主任务状态、任务类型、部门子任务处理情况、近 14 天任务创建趋势。
- 模具统计：按单模/共模和固定位置汇总。
- 业务数据：客户编码、供应商、产品、物料、模具、任务单数量。
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

模具是一条产品/型号档案，字段为：`id`、`mold_number`（手工填写且全局唯一）、`model`、`mold_type`（`single` 单模或 `common` 共模）、`location_id`、`common_group_no`、`remark`。共模必须填写共模组号，单模不得填写。

固定位置由位置字典维护，初始包含 `A1-1`、`B1-1`。正在使用的位置不能停用；停用位置不能分配给新模具。

```text
GET    /api/v1/molds
GET    /api/v1/molds/:id
POST   /api/v1/molds
PATCH  /api/v1/molds/:id
DELETE /api/v1/molds/:id
POST   /api/v1/molds/bulk-location
GET    /api/v1/mold-locations
POST   /api/v1/mold-locations
PATCH  /api/v1/mold-locations/:id
GET    /api/v1/molds/:id/drawings
POST   /api/v1/molds/:id/drawings
GET    /api/v1/molds/:id/drawings/:drawing_id/content
DELETE /api/v1/molds/:id/drawings/:drawing_id
GET    /api/v1/molds/export
GET    /api/v1/molds/import-template
POST   /api/v1/molds/import/preview
POST   /api/v1/molds/import/commit
```

`GET /api/v1/molds` 支持 `page`、`page_size`、`q`、`mold_type`、`location_id`、`common_group_no`，返回 `image_count` 和 `drawing_count`。图片分为 `product_material`、`supplement` 两组，补充图数量不限且总数自动统计。图纸只允许 `.dwg`、`.fdwg`，本期提供上传、下载、删除，暂不预览。

创建示例：

```json
{
  "mold_number": "CYF1809-2-1",
  "model": "CYF1809-2",
  "mold_type": "common",
  "location_id": 1,
  "common_group_no": "G-001",
  "remark": "前后模一组"
}
```

### 模具资料包

导出与导入使用 ZIP 全量资料包，模板如下：

```text
博邦模具资料包.zip
├── molds.xlsx
├── locations.json
├── images/
│   └── <模具编号>/
│       ├── product_material/
│       └── supplement/
└── drawings/
    └── <模具编号>/
```

`molds.xlsx` 列为：序号、模具编号、模具型号、模具类型、模具位置、共模组号、图片总数、备注。导入忽略序号和图片总数，重新生成 ID 并计算图片数量；标准目录优先，扁平目录按文件名兜底。文件名中的 `+` 表示共模图片需要复制到多个编号，例如 `A+B+C-1.jpg`；`产品材料`、`产品图`、`材质`归入产品材料，`前模`、`后模`、`开模`、`尺寸`、`局部`归入补充图。无法识别的图片会在预览阶段列出，可人工指定图片组和一个或多个模具编号；未修正、未匹配、重复或扩展名不受支持的文件不能提交。资料包提交时复用图库的真实内容解码和静态预览流程，因此系统导出的 JPG/JFIF、PNG、GIF、WebP、HEIC/HEIF、AVIF、BMP、TIFF、SVG 图片可以按相同安全边界回导。

导入采用“预览—修正—确认—提交”，预览阶段即按图库规则解码真实图片内容。资料包最多 2000 个源条目，共模复制后最多落盘 5000 个图片/图纸；声明解压总量和提交时实际写入总量均不得超过 4 GiB，单个 ZIP 请求仍不得超过 2 GiB；`molds.xlsx`、`locations.json` 和人工修正参数另有 64/4/4 MiB 小对象边界。提交会全量替换模具、模具图片、DWG 和位置字典，不影响客户、用户、库存和工单；模具图片、图纸写入/删除与全量导入使用同一资产互斥边界，旧物理文件在事务提交后清理，失败时进入启动重试任务。Excel 单独导出不包含图片；`.xls` 内嵌图片不在本期自动提取范围内。

本期未单独开放“图片文件夹只追加”接口；如需增量图片导入，当前应将图片放入上述 ZIP 的 `images/` 目录后预览导入。

## 错误响应

统一结构：

```json
{
  "code": "BAD_REQUEST",
  "message": "请求参数校验失败",
  "request_id": "request-id"
}
```
