# 博邦 ERP 首次使用帮助

更新时间：2026-08-29

本文面向第一次使用博邦 ERP 的管理员和员工，说明如何启动服务、连接服务器、登录系统、使用 Web/桌面客户端、调用 HTTP API，以及当前已实现模块和权限规则。

## 1. 一眼看懂

```text
管理员电脑 / 服务器
  1. 启动 Go 服务：go run ./cmd/server
  2. 确认地址：http://服务器IP:8080
  3. 首次用 admin / admin123456 登录
  4. 创建部门和员工、配置部门成员，再创建终端、账号和角色

员工电脑
  Web 方式：浏览器打开 http://服务器IP:8080
  桌面端：打开客户端，在登录页“服务器”填写 http://服务器IP:8080，测试连接后保存
```

默认服务端口是 `8080`。同一台电脑访问可用 `http://127.0.0.1:8080`；局域网其他电脑要使用运行 Go 服务电脑的内网 IP，例如 `http://192.168.1.20:8080`。

## 2. 启动服务器

### 开发或本机测试

在项目根目录执行：

```bash
go run ./cmd/server
```

也可以使用热重载：

```bash
air
```

启动后服务默认监听所有网卡的 `8080` 端口。

### 常用配置

默认配置可以直接本地运行，正式使用建议通过环境变量覆盖敏感配置：

```bash
BB_ERP_HTTP_HOST=0.0.0.0
BB_ERP_HTTP_PORT=8080
BB_ERP_DATABASE_PATH=data/erp.db
BB_ERP_JWT_EXPIRES_IN=2h
BB_ERP_JWT_REFRESH_EXPIRES_IN=720h
BB_ERP_ADMIN_USERNAME=admin
```

说明：

- `BB_ERP_HTTP_HOST=0.0.0.0` 表示允许局域网其他电脑访问。
- `BB_ERP_HTTP_PORT` 是端口，默认 `8080`。
- `BB_ERP_DATABASE_PATH` 是 SQLite 数据库文件，默认 `data/erp.db`。
- `BB_ERP_JWT_EXPIRES_IN` 是 access token 有效期，默认 `2h`。
- `BB_ERP_JWT_REFRESH_EXPIRES_IN` 是 refresh token 每次轮换后的滚动有效期，默认 `720h`（30 天）。
- JWT 密钥由系统内部使用，无需额外配置；首次登录后必须在“系统 / 用户”中修改默认管理员密码。

### 防火墙

如果员工电脑无法访问服务器，请先在服务器电脑放行 `8080/TCP` 入站访问。验证方式：

```bash
curl http://服务器IP:8080/health
curl http://服务器IP:8080/ready
```

能返回成功响应，说明服务和数据库基本可用。

## 3. 第一次登录

默认管理员账号：

```text
账号：admin
密码：admin123456
```

首次登录后建议立即完成：

1. 修改正式管理员密码。
2. 在“系统设置 / 部门”创建实际部门。
3. 在“系统设置 / 员工档案”创建员工，再回到部门的“管理员工”中选择一名或多名成员。
4. 在“系统设置 / 终端”创建车间公共电脑或部门终端。
5. 在“系统设置 / 用户账号”创建个人账号或部门终端账号，并绑定其当前部门。
6. 在“系统设置 / 角色”配置权限，再把角色分配给用户。

修改当前管理员密码：点击右上角账号头像，选择“修改密码”，输入当前密码、新密码和确认密码；修改成功后系统会退出当前会话，请使用新密码重新登录。

账号类型说明：

- 员工档案与登录账号相互独立，不会自动关联；一个员工可以同时属于多个部门，一个账号只使用一个当前部门。
- 个人账号：给个人登录使用；可以不绑定部门，但无部门时不能执行任务单或仓库写入。
- 部门终端账号：给车间公共电脑使用，审计日志仍记录登录账号、部门和终端。
- 每次任务或仓库写入都要从当前账号部门中选择“本次操作人”，系统同时保存登录身份和实际员工，形成双重责任记录。

## 4. Web 连接方式

### Go 服务直接托管 Web

后端启动后，浏览器打开：

```text
http://服务器IP:8080
```

这是推荐的局域网使用方式。Web 页面和 API 同源，不需要额外配置跨域。

### 单独启动 Web 开发服务器

开发调试时执行：

```bash
cd web
npm install
npm run dev
```

浏览器访问 Vite 输出的地址。生产使用优先让 Go 服务托管构建后的 Web 静态文件。

## 5. 桌面客户端选择服务器

桌面端复用同一套 Web 页面，但请求由 Tauri HTTP 插件从 Rust 层发出。

开发启动：

```bash
cd client
npm install
npm run desktop:dev
```

首次打开桌面端：

1. 在登录页找到“服务器”。
2. 填写运行 Go 服务的地址，例如 `http://192.168.1.20:8080`。
3. 点击“测试连接”。
4. 测试成功后点击“保存地址”。
5. 使用账号密码登录。

登录后也可以从顶部栏“服务器”入口切换服务地址。

地址填写规则：

- 只支持 `http://` 或 `https://`。
- 只能填写主机和端口，例如 `http://192.168.1.20:8080`。
- 不要填写 `/api/v1`、查询参数、账号密码或 Swagger 地址。
- 默认地址是 `http://127.0.0.1:8080`，只适合同一台电脑同时运行服务端和客户端。

桌面端当前允许访问：

- `http://127.0.0.1:*/*`
- `http://localhost:*/*`
- `http://10.*.*.*:*/*`
- `http://172.*.*.*:*/*`
- `http://192.168.*.*:*/*`
- `https://*/*`

## 6. HTTP / API 连接

公共接口：

```text
GET /health
GET /ready
GET /swagger/index.html
GET /swagger/doc.json
```

Swagger 文档地址：

```text
http://服务器IP:8080/swagger/index.html
```

登录接口：

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123456"
}
```

业务接口需要携带登录返回的 token：

```http
Authorization: Bearer <token>
```

客户端会在 access token 到期前自动使用 refresh token 续期。连续 30 天未成功续期后需要重新登录；修改密码后原登录会话会全部失效。

列表接口通常支持：

```text
page=1
page_size=20
q=关键字
```

`q` 也可以写作 `keyword`。

## 7. 当前已实现功能

### 首页

- 显示服务状态。
- 按权限显示常用功能和业务入口。
- 桌面端可显示客户端更新提示。

### 系统设置

- 部门：新增、编辑、启停部门，并通过“管理员工”搜索、多选和一次保存成员关系。
- 员工档案：维护姓名、电话、入职日期、籍贯、居住地址和出生日期；年龄自动按当前日期计算，可办理离职停用和恢复。
- 用户账号：创建个人账号和部门终端账号，启用/停用，修改当前密码、重置其他账号密码，分配角色，并通过“账号归属”修正当前部门和终端。
- 个人账号可以不绑定部门，但未绑定时不能执行任务、仓库或库存写入；部门终端账号必须同时绑定同一部门下的终端。员工调部门、终端更换或账号建错归属后，由管理员在“系统设置 → 用户账号 → 账号归属”修正。
- 修正账号归属需要 `system:users:write`，Web 页面还需要 `system:departments:read` 与 `system:terminals:read` 加载可选部门和终端；缺少这些查看权限时入口会禁用并提示原因。
- 终端：维护公共电脑和部门终端。
- 角色：创建角色，并给角色配置权限。
- 权限：查看系统权限清单。
- 操作审计：查看最近组织内操作记录。

### 客户与联系人

- 客户：分页、模糊搜索、新增、更新、删除客户档案。
- 联系人：维护客户联系人和多条电话明细。
- 删除客户不会自动删除联系人，联系人可保留用于转移或历史追溯。

### 供应商

- 维护采购入库使用的供应商档案。
- 仓库办理采购入库时可选择供应商；有供应商维护权限时可快速新增供应商。

### 仓库与库存

当前按单仓库使用，仓库物品按标签分类：

```text
product              产品
production_material  生产物资
regular_product      常规产品
daily_supply         生活物资
```

可用能力：

- 查看仓库物品列表。
- 按名称、编码等关键字搜索。
- 查看物品详情、当前库存、库存流水。
- 生产单从仓库“产品”中搜索选择；产品详情库存显示默认仓库全部库位合计。
- 生产单可在有独立建档权限时临时添加尚未入库产品；临时添加会建立正式启用产品档案，初始安全库存和当前库存为 0，不产生入库流水。
- 办理采购入库、退货返工、客户出库、部门出库。
- 新建仓库物品、修改仓库资料和办理出入库时，必须选择当前账号部门下的在职员工为本次操作人。
- 移动加权平均成本、禁止负库存、库存流水和单据过账已实现。
- 数量使用 4 位定点整数：接口中的 `10000` 表示 1 个单位；界面会按普通小数显示。
- 入库数量可以直接输入 `0–999999999` 范围内的实际到货或盘点校准值，支持最多 4 位小数；提交后会按四位定点精度保存，数量为 0 时不会创建流水。
- 金额和单价使用分；没有 `cost:view` 权限时不显示成本、金额字段。

出入库依赖权限：

- 采购入库：需要 `inventory:documents:write` 和 `suppliers:read`。
- 退货返工：需要 `inventory:documents:write`，并至少具备 `customers:read` 或 `system:departments:read`。
- 客户出库：需要 `inventory:documents:write` 和 `customers:read`。
- 部门出库：需要 `inventory:documents:write` 和 `system:departments:read`。
- 查看库存流水：需要 `inventory:documents:read`。

### 模具台账

可维护：

- 模具编号、名称、客户、产品、穴数、成型材料、钢材、尺寸、重量、制造商、所有权、存放位置、当前位置、保养周期和备注。
- 模具状态：在库、已借出、维修中、保养中、报废。
- 借出、归还、维修、保养履历。
- 完成保养后会按保养周期计算下次保养日期。

### 任务单

任务单分为：

- `production`：生产单。
- `general`：通用任务。

主任务状态：

```text
draft              草稿
processing         正在处理
paused             暂停
pending_close      待办公室确认
completed_normal   正常完成
completed_forced   强制完成
cancelled          取消
```

部门子任务状态：

```text
received           已收到
processing         正在处理
partial_completed  部分完成
completed          完成
```

常用流程：

1. 办公室创建任务单，选择目标部门和本次操作人。
2. 办公室派发时再次选择本次操作人，系统为每个目标部门生成子任务。
3. 部门开始处理、部分完成和完成时，各自选择当前部门的实际操作员工。
4. 所有部门完成后，主任务进入“待办公室确认”。
5. 办公室确认正常完成；特殊情况在同一确认框选择操作人并填写原因后强制完成。

创建、临时产品建档、派发、暂停、恢复、加急和完成均不自动沿用上一次操作人。账号无部门、部门已停用、部门无在职员工或员工关系刚被移除时，系统会阻止提交并提示管理员处理。

### 统计报表

统计报表聚合展示：

- 库存总量、库存金额、低库存。
- 任务单状态、类型、部门处理情况和趋势。
- 模具状态和需关注模具。
- 客户、联系人、供应商、产品、物料、模具、任务单数量。
- 审计结果和趋势。

没有 `cost:view` 权限时，库存金额和相关成本统计会隐藏或置为 `0`。

### 更新能力

服务端已提供版本和更新相关接口：

- `GET /api/v1/version`
- `GET /api/v1/updates/client/status`
- `GET /api/v1/system/updates/client/download`（需更新管理权限，仅供管理员人工恢复）
- `POST /api/v1/system/updates/check`

正式局域网方案不配置外部更新地址。Windows 计划任务完成构建、签名和服务端健康检查后，原子激活安装目录内的 `updates/stable/update-manifest.json`；Go 服务从该本机文件读取更新状态和分发资源，不访问 Gitee、GitHub 或自身 HTTP 地址。管理员恢复 ZIP 仅在登录后按更新管理权限人工下载，客户端自动更新仍须由用户确认。

## 8. 权限说明

系统按角色分配权限。菜单是否显示、按钮是否可用、字段是否展示，都与当前用户权限有关。

内置角色：

- `super_admin`：超级管理员，拥有大多数权限，但默认不包含 `cost:view`。
- `boss`：老板，默认拥有 `cost:view`。
- `department_terminal_operator`：部门终端操作员，默认拥有仓库查看、任务读写、旧任务兼容读写和库存查看；无仓库查看权限时不应授予任务单权限。

常用权限：

| 功能 | 查看权限 | 维护权限 |
| --- | --- | --- |
| 用户账号 | `system:users:read` | `system:users:write` |
| 部门 | `system:departments:read` | `system:departments:write` |
| 员工档案 | `system:employees:read` | `system:employees:write` |
| 终端 | `system:terminals:read` | `system:terminals:write` |
| 角色 | `system:roles:read` | `system:roles:write` |
| 权限清单 | `system:permissions:read` | - |
| 操作审计 | `system:audits:read` | - |
| 客户 | `customers:read` | `customers:write` |
| 联系人 | `contacts:read` | `contacts:write` |
| 供应商 | `suppliers:read` | `suppliers:write` |
| 仓库物品 | `warehouse:read` | `warehouse:write` |
| 库存单据 | `inventory:documents:read` | `inventory:documents:write` |
| 库存余额 | `inventory:balances:read` | - |
| 库存流水 | `inventory:ledgers:read` | - |
| 模具 | `mold:read` | `mold:write` |
| 任务单 | `workorder:read` | `workorder:write` |
| 生产单临时产品建档 | - | `workorder:temporary-product:write`（同时需要 `workorder:write`） |
| 统计报表 | `statistics:read` | `statistics:write` |
| 成本金额 | `cost:view` | - |

建议：

- 普通员工只给与岗位相关的查看和维护权限。
- `workorder:temporary-product:write` 默认只分配给 `super_admin`；确需在任务单内建档时，由管理员显式分配给对应角色。
- 车间公共电脑优先使用部门终端账号，不使用老板或超级管理员账号。
- 成本金额单独用 `cost:view` 控制，不要默认给所有管理账号。

## 9. 常见问题

### 桌面端测试连接失败

检查：

1. Go 服务是否正在运行。
2. 地址是否只填写到主机和端口，例如 `http://192.168.1.20:8080`。
3. 服务器电脑防火墙是否放行 `8080/TCP`。
4. 员工电脑和服务器是否在同一局域网。
5. 不要填写 `http://服务器IP:8080/swagger/index.html` 或 `/api/v1`。

### 登录后看不到菜单

当前账号没有对应模块的查看权限。请让管理员进入“系统设置 / 角色”，给角色配置权限，再把角色分配给用户。

### 看不到成本或金额

当前账号没有 `cost:view` 权限。需要查看库存金额、平均成本、采购单价或成本统计时，请管理员单独分配该权限。

### 采购入库不能选择供应商

需要 `suppliers:read` 权限。若还要在入库时快速新增供应商，需要 `suppliers:write` 权限。

### Web 能打开但 API 报未登录

业务接口需要登录 token。浏览器正常登录即可；调试 HTTP API 时，需要把登录返回的 `access_token` 放到 `Authorization: Bearer <token>`。

## 10. 给管理员的最短部署清单

1. 在服务器电脑启动 Go 服务。
2. 浏览器访问 `http://127.0.0.1:8080/health` 确认服务正常。
3. 在同一局域网另一台电脑访问 `http://服务器IP:8080/health`。
4. 用默认管理员登录并修改正式密码。
5. 创建部门、终端、用户、角色。
6. 给角色配置权限，尤其确认 `cost:view` 是否需要分配。
7. 员工 Web 端访问 `http://服务器IP:8080`。
8. 员工桌面端在登录页保存 `http://服务器IP:8080`。
9. 使用 `http://服务器IP:8080/swagger/index.html` 给开发或接口调试人员查看 API。
