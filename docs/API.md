# 博邦 ERP API 文档

更新时间：2026-09-09

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

除 `/health`、`/ready`、`/api/v1/discovery/identity`、`/api/v1/version`、`/api/v1/client-updates/*`、`/api/v1/auth/login`、`/api/v1/auth/refresh`、`/api/v1/auth/logout`、`/swagger/*` 外，业务接口默认需要：

```http
Authorization: Bearer <token>
```

## 实时数据变更通知

```text
GET /api/v1/notifications/stream
Accept: text/event-stream
Authorization: Bearer <token>
```

该接口使用带 Bearer 头的 fetch-SSE 长连接，不接受 URL 中的令牌。
通知只保存在服务端与客户端当前进程内，不写数据库、不支持断线回放；
断线重连后客户端应重新加载当前可安全刷新的模块。

服务端在成功写请求返回后发送 `data_changed` 事件，按当前数据库中的
组织、账号状态和对应模块读权限过滤，并排除操作账号的所有在线会话。
事件为版本化安全摘要，包含 `v/id/kind/priority/occurred_at/display_for_ms`、
`module/title/summary/count/items/action/refresh/truncated`；不包含密码、令牌、
成本、本机路径或完整业务模型。

操作审计仍持久化到原有审计表，但不作为实时通知模块。`Action`
使用稳定的 `模块:动作` 编码；读取、OPTIONS 和导入预览不写审计。

## 公共接口

```text
GET  /health
GET  /ready
GET  /api/v1/discovery/identity
GET  /api/v1/version
GET  /api/v1/client-updates/check?current_version=<SemVer>
GET  /api/v1/client-updates/artifacts/<sha256>
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

- `GET /api/v1/customers/import-template`：下载 `.xlsx` 模板；与客户导入预览/确认一样需要 `customers:import`。
- `POST /api/v1/customers/import/preview`：multipart `file`，接受 `.xls/.xlsx`，限制 10 MiB、10,000 条、只读取首个工作表。
- `POST /api/v1/customers/import/commit`：multipart 重新上传同一 `file` 并传 `token`；令牌绑定用户、模块和文件 SHA-256，30 分钟过期且只能成功提交一次。任一错误整批回滚。
- `GET /api/v1/customers/export/preview?scope=current|all&q=&filter=&page=1&page_size=50`：返回最终工作表使用的九列元数据和分页标准化行；最大预览页大小 100。
- `GET /api/v1/customers/export?scope=current|all&q=&filter=`：按下载时最新数据生成样式化 `.xlsx`；空结果返回 `422`。

导出预览和下载共用同一列定义和数据转换，列序固定为：序号、客户编码、客户简称、客户名称、地址、电话、联系人、联系人电话、业务员。电话列按文本写入。权限为 `customers:read`（查看、导出预览与导出）、`customers:write`（编码与资料 CRUD）、`customers:import`（模板下载、导入预览与确认）。本版本仅支持全新数据库，不提供旧客户、联系人表升级或迁移。

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
GET  /api/v1/operator-employees
```

员工是独立业务档案，不等同于登录账号。员工可属于零个或多个部门；部门成员关系通过 `PUT /api/v1/system/departments/:id/employees` 以 `employee_ids` 数组原子替换。员工列表支持 `page`、`page_size`、`q`、`department_id` 和 `status`，返回按 `Asia/Shanghai` 当前日期计算的周岁和所属部门摘要。员工 `DELETE` 仅停用档案，可通过状态接口恢复；部门也只启停、不物理删除。停用部门不能新增成员、绑定账号或执行业务写入。

部门成员配置同时要求 `system:departments:write` 和 `system:employees:read`；员工档案使用 `system:employees:read/write`。管理员可通过用户归属接口修正账号的部门和终端。`GET /api/v1/operator-employees` 与完整员工档案权限分离，只返回当前账号部门及在职候选员工的 `id/name`。

管理员通过 `POST /api/v1/system/users/:id/reset-password` 重置其他账号密码时，服务端在同一事务中递增目标账号的 `password_version` 并撤销该账号全部 refresh token；旧 access token 会因密码版本不匹配返回 `401`，旧 refresh token 也返回 `401`，目标账号必须使用新密码重新登录。

任务单及仓库/库存写请求统一要求 `operator_employee_id`。服务端在业务事务内重新读取当前账号并校验账号部门、部门状态、员工状态、组织和成员关系：缺字段返回 `400`，无部门或越权返回 `403`，员工/部门停用或成员关系失效返回 `409`。任务与库存状态流转使用带原状态及旧完成数量条件的更新，验证失败、并发状态冲突或关系刚失效均不产生部分业务写入。历史记录同时保留登录账号、终端、所选操作员工及请求时部门快照；新库不补造历史员工。操作员工是当前登录账号对现场责任人的申报，不等同于员工本人完成 PIN、刷卡或二次认证；追责时必须同时查看不可替代的登录账号和终端信息。带 `Idempotency-Key` 的库存请求只有在接口范围、账号/组织、规范化请求内容和操作员工都与首次请求一致时才返回原结果，任一差异均返回 `409`。新 SQLite schema 直接创建非空部分唯一幂等索引，不包含旧库索引升级或重复历史数据修复逻辑。

新建部门终端账号不再自动绑定角色，创建后必须由管理员按岗位显式授权；部门终端不能获得 `super_admin`。`super_admin` 是唯一锁定系统角色并拥有全部权限（含 `cost:view`），系统必须始终保留至少一个启用中的超级管理员。

管理写权限隐含读取配置所需列表：`system:users:write` 可读取用户和角色列表，`system:roles:write` 可读取角色和权限列表。写操作仍分别要求对应的 write 权限。管理员不能修改自己的角色、不能跨组织管理，也不能向他人授予自己未实际持有的角色或超出自身有效权限集合的权限；实际超级管理员可授予任意合法角色。普通管理员不能修改已被其他组织账号使用的共享角色；非法或重复 ID 整体失败，不产生部分写入。

全新数据库保留原有 `admin` 初始化规则。如果首次启动时显式配置 `BB_ERP_SILENCE_PASSWORD`，系统额外创建 `Silence` 超级管理员，初始密码以 bcrypt 哈希保存；未配置时跳过该额外账号且不阻断启动。已有数据库不会补建 Silence，也不会重置原有 admin。系统注入的 Silence 带内部托管标记，不进入 `GET /api/v1/system/users` 的 `items`、`total`、搜索或分页结果；账号管理的状态、归属、重置密码和角色接口对该隐藏账号返回不存在，但账号本人仍可通过认证模块修改密码。已有数据库中的普通同名账号没有托管标记，仍正常显示。

客户资料删除需要同时确认库存单据和任务单历史引用。任一引用表尚未初始化时返回 `503 module_not_initialized`，不会把缺表误判成“没有引用”后删除客户资料。

## 版本与内网客户端更新

`GET /api/v1/version` 是匿名服务端身份接口，只返回 `app_name` 与
`server_version`；服务端不检查、下载或替换自身程序。

Windows Tauri 客户端在已验证当前内网服务后，或由登录页和“设置 / 客户端更新”手动触发以下匿名接口。Web 备用入口不显示安装操作。

```text
GET /api/v1/client-updates/check?current_version=1.2.2
GET /api/v1/client-updates/artifacts/<sha256>
```

- `current_version` 必须是客户端真实 SemVer。无有效投放或已经是最新版本返回 `204`，并分别以 `X-Client-Update-Status: not_published` 或 `up_to_date` 区分；无效版本返回 `400`；发现不完整或不可信投放返回 `503`，不泄露服务器路径。
- 更新只支持 `windows-x86_64` 的完整 portable 单 EXE，拒绝降级、差分包、NSIS/MSI、外部 URL 和旧 `/updates/client/*` 协议。
- `200` 计划的精确字段为 `protocol_version`（固定 `1`）、`current_version`、`latest_version`、`target`、`strategy`（固定 `full`）、`download_size`、`signed_payload`、`signature` 与 `artifact`。其中 `artifact` 仅有 `kind`（`portable`）、`size`、`sha256`、`signature`、`download_path`；下载路径由服务端从已验证摘要生成。
- 服务器固定从可执行文件上一级的 `client/bb_erp_client.exe` 和 `client/client-update.json` 读取。后者是严格 JSON 签名信封 `{ "payload", "signature" }`；解码后的已签名 payload 只允许 `version`、`target` 与 `artifact`，artifact 只允许 `kind`、`size`、`sha256`、`signature`。未知/重复字段、尾随内容、无效签名、大小或哈希均被拒绝。
- 每次检查合并并发刷新；验证通过后先复制到服务端 `updates/client-cache/<sha256>`，再原子切换有效快照。投放复制中或刷新失败不会下发半成品，已有有效快照继续可用。
- 资源接口只分发当前快照允许的内容寻址 EXE，支持 `ETag`、`Content-Length` 和 HTTP Range。客户端在下载后再次验证清单签名、EXE 签名、大小和 SHA-256，再由临时助手原子替换；新版未成功启动时恢复旧 EXE。

计划示例：

```json
{
  "protocol_version": 1,
  "current_version": "1.2.2",
  "latest_version": "1.2.3",
  "target": "windows-x86_64",
  "strategy": "full",
  "download_size": 18600000,
  "signed_payload": "eyJ2ZXJzaW9uIjoiMS4yLjMiLC4uLn0=",
  "signature": "VU5UUlVTVEVEX1NJR05BVFVSRS4uLg==",
  "artifact": {
    "kind": "portable",
    "size": 18600000,
    "sha256": "<64 位小写十六进制摘要>",
    "signature": "VU5UUlVTVEVEX1NJR05BVFVSRS4uLg==",
    "download_path": "/api/v1/client-updates/artifacts/<sha256>"
  }
}
```

## 产品资料与库存数量接口

产品资料字段固定为：

- `product_model`：产品型号，必填、唯一，产品与模具、任务单、库存统一使用该业务标识。
- `customer_model`：客户型号，可空。
- `material`：产品材料，可空，客户端提供可输入下拉建议。
- `ink_required`：是否刷墨。
- `status`：`active` 或 `disabled`。

```text
GET    /api/v1/products?page=&page_size=&q=&material=&ink_required=&status=
POST   /api/v1/products
GET    /api/v1/products/:id
PATCH  /api/v1/products/:id
DELETE /api/v1/products/:id
GET    /api/v1/products/:id/molds
GET    /api/v1/products/import-template
POST   /api/v1/products/import/preview
POST   /api/v1/products/import/commit
GET    /api/v1/products/export
```

产品删除前会检查模具、库存余额、库存流水和任务单引用；存在引用时返回 `409`。产品图片使用 `owner_type=product`，使用 `product:read/write` 权限。

产品 ZIP 包含 `products.xlsx` 和以产品型号命名的图片目录：

```text
products.xlsx          产品型号、客户型号、产品材料、是否刷墨、状态、图片数量
<产品型号>/             该产品的全部图片，文件名只用于展示和排序
```

产品导入按产品型号新增或更新，不删除文件外的产品；有图片目录时替换该产品图片，没有目录时保留原图片。字段或图片任一校验失败时整包不提交。

仓库只管理产品数量和库位，不再维护产品、物料和成本资料：

```text
GET  /api/v1/warehouses
POST /api/v1/warehouses
GET  /api/v1/warehouse/products?page=&page_size=&q=
GET  /api/v1/warehouse/products/:id
GET  /api/v1/warehouse/products/:id/movements
POST /api/v1/warehouse/products/:id/movements
GET  /api/v1/locations
POST /api/v1/locations
```

数量操作 `action` 只接受 `inbound`、`outbound`、`transfer`、`adjustment`。数量使用 4 位定点整数，例如 `1000000` 表示 100。入库、出库和库位调整提交 `quantity`；库位调整另需 `from_location_id`、`to_location_id`；盘点修正提交目标数量 `target_quantity`。所有操作在同一事务内更新余额并写入流水，库存不足返回 `409`。

供应商、成本、安全库存、物料、常规产品和生活物资不进入本版本仓库页面与接口；相关模型保留供后续开发。

## 图片文件接口

图片接口均需要 `Authorization: Bearer <token>`。图片元数据保存在数据库，物理文件默认存放在 `static/uploads`，可通过 `BB_ERP_FILES_ROOT_DIR` 覆盖。系统不公开静态直链，读取图片内容必须使用受保护的 content 接口。

`owner_type` 只接受以下四种值：

```text
product          产品资料，权限单独使用 product
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

权限规则：产品图片使用产品资料权限，模具图片使用模具权限，任务单和部门子任务图片使用任务单权限。部门子任务的写入操作还限制为该子任务所属部门；读取不增加此部门限制。

模具导入需要额外的 `mold:import` 权限；导出、位置字典读取使用 `mold:read`，模具和位置写入、图片/DWG 删除使用 `mold:write`。

## 任务单接口

任务单支持生产单和通用任务。生产单必须从产品资料选择启用产品，服务端保存产品型号快照；通用任务不关联产品。

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

生产单创建请求示例：

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

成功响应中的 `product_id` 是产品资料内部关联 ID，`product_model` 是创建时保存的产品型号快照。实时库存通过 `GET /api/v1/warehouse/products/:id` 查询。临时产品建档接口和 `workorder:temporary-product:write` 已移除。

派发、暂停、恢复、加急、正常/强制完成，以及部门开始、部分完成、完成的 JSON 请求体都必须携带 `operator_employee_id`。强制完成必须填写原因；部门部分完成提交累计完成数量，必须大于当前累计值且小于计划数量。

## 统计报表接口

```text
GET  /api/v1/statistics
```

`GET /api/v1/statistics` 返回 Web/Tauri 统计首页聚合数据，包含：

- 顶部汇总：客户编码、供应商、产品资料、仓库总数量、进行中任务、加急任务、待办公室确认任务和模具总数。
- 库存统计：按产品型号汇总、按仓库库位汇总和近 14 天库存数量流水趋势。
- 任务统计：按主任务状态、任务类型、部门子任务处理情况、近 14 天任务创建趋势。
- 模具统计：按单模/共模和固定位置汇总。
- 业务数据：客户编码、供应商、产品、模具、任务单数量。
- 审计统计：按结果汇总和近 14 天趋势。

统计依赖的数据表尚未初始化时仍返回 `200`，并增加 `data_status`、`unavailable_sources` 和 `message`。客户端必须将不可用指标显示为“—”或待重构说明，不能展示成真实零值。

响应片段：

```json
{
  "data_status": "ready",
  "unavailable_sources": [],
  "summary": {
    "customers": 10,
    "warehouse_items": 36,
    "inventory_quantity": 1280000,
    "open_workorders": 5
  },
  "inventory": {
    "by_item_type": [{"name": "P-100", "value": 800000}],
    "by_location": [{"name": "A1-1", "value": 800000}],
    "low_stock": []
  },
  "workorders": {
    "by_status": [{"name": "processing", "value": 3}],
    "by_department": []
  }
}
```

## 模具接口

模具绑定产品资料，产品型号是唯一业务定位字段。

```text
GET    /api/v1/molds?page=&page_size=&q=&product_model=&mold_type=&location_id=&common_group_no=
POST   /api/v1/molds
GET    /api/v1/molds/:id
PATCH  /api/v1/molds/:id
DELETE /api/v1/molds/:id
POST   /api/v1/molds/bulk-location
GET    /api/v1/molds/:id/drawings
POST   /api/v1/molds/:id/drawings
GET    /api/v1/molds/:id/drawings/:drawing_id/content
DELETE /api/v1/molds/:id/drawings/:drawing_id
GET    /api/v1/molds/import-template
POST   /api/v1/molds/import/preview
POST   /api/v1/molds/import/commit
GET    /api/v1/molds/export
GET    /api/v1/mold-locations?include_disabled=
POST   /api/v1/mold-locations
POST   /api/v1/mold-locations/bulk
PATCH  /api/v1/mold-locations/:id
```

创建和更新请求字段为 `product_id`、`mold_type`、`cavity_count`、`location_id`、`common_group_no`、`remark`。响应同时返回 `product_model`。单模不能填写共模组号，共模必须填写共模组号。

模具 ZIP 包含 `molds.xlsx`、`locations.json`，资料目录按产品型号和三位序号组织：

```text
molds.xlsx       序号、产品型号、模具类型、模穴数、模具位置、共模组号、图片数量、备注
locations.json   模具位置字典
<产品型号>/001/   第一条模具的图片和 DWG/FDWG
<产品型号>/002/   第二条模具的图片和 DWG/FDWG
```

模具导入全量替换模具、位置、模具图片和图纸；缺少产品型号时自动建立仅含产品型号的启用产品占位记录。任一字段、图片或图纸校验失败时整包回滚。模具图片和图纸分别使用 `owner_type=mold`，权限为 `mold:read/write`；导入需要 `mold:import`。

## 错误响应

统一结构：

```json
{
  "code": "BAD_REQUEST",
  "message": "请求参数校验失败",
  "request_id": "request-id"
}
```
