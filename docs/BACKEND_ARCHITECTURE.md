# Go 后端产品架构与业务时序图

> 基准日期：2026-08-27
> 适用范围：Go 服务、业务模块、数据存储、更新服务和发布支撑

## 1. 给非技术用户的一句话

Go 后端就是 ERP 的“业务大脑”：它接收 Web 或桌面客户端的操作，检查用户是否有权限，处理库存、任务单、模具等业务，并把可靠的结果保存下来。

## 2. 阅读图例

- 绿色：代码功能已经实现。
- 黄色：代码已经实现，但还需要真实环境或发布环境验收。
- 蓝色虚线：后续规划功能或首版暂不支持的范围。
- 数据库和文件目录表示系统需要长期保存的数据，不代表用户需要直接操作它们。

## 3. Go 后端产品架构图

~~~mermaid
flowchart TB
    users["使用人员<br/>管理员 / 办公室人员 / 仓库人员 / 部门人员"]:::user
    clients["使用入口<br/>Web 浏览器或 Tauri 桌面客户端"]:::entry

    subgraph service["Go ERP 服务（统一业务入口）"]
        common["公共能力<br/>登录认证<br/>角色权限与组织/部门范围<br/>操作审计与请求日志<br/>健康检查与配置管理"]:::done
        domains["业务功能<br/>客户、联系人、供应商<br/>仓库、物料、产品<br/>库存、模具、任务单<br/>统计、图片、版本更新"]:::done
        validation["安全与业务检查<br/>权限、必填项、状态流转<br/>库存数量、金额、文件和签名校验"]:::done
    end

    subgraph storage["系统保存的数据"]
        db["SQLite 数据库<br/>账号、权限、资料、库存、任务、模具、审计"]:::done
        files["图片文件目录<br/>按业务对象受保护保存"]:::done
        stable["本机 stable 清单<br/>原子切换当前正式版本"]:::done
        cache["内容寻址客户端资源<br/>仅完整 NSIS 与 Portable"]:::done
    end

    users --> clients
    clients --> common
    common --> validation
    validation --> domains
    domains --> db
    domains --> files
    domains --> stable
    domains --> cache

    acceptance["待真实环境验收<br/>Server 2016 构建/服务回滚<br/>Windows 10 客户端更新"]:::pending
    planned["规划功能<br/>统计导出、任务提醒、自动备份<br/>多组织、多仓库和更细权限"]:::planned
    domains -.-> acceptance
    domains -.-> planned

    boundary["首版边界<br/>单组织、单仓库、内网优先"]:::boundary
    boundary -.-> service

    classDef user fill:#fff3e0,stroke:#d98c00,color:#4a2a00
    classDef entry fill:#e3f2fd,stroke:#1976d2,color:#0d2b45
    classDef done fill:#e8f5e9,stroke:#388e3c,color:#173d1c
    classDef pending fill:#fff8e1,stroke:#c58b00,color:#4b3800
    classDef planned fill:#eef2ff,stroke:#5c6bc0,color:#202a61
    classDef boundary fill:#f3e5f5,stroke:#8e44ad,color:#3d1f4d
~~~

### 3.1 后端各区域怎么理解

| 区域 | 用户能理解的含义 | 当前状态 |
| --- | --- | --- |
| 公共能力 | 谁可以登录、谁可以做什么、谁做过什么 | 已实现 |
| 基础资料 | 客户、联系人、供应商、仓库和物品信息 | 已实现 |
| 库存 | 入库、出库、调拨、库存余额和库存流水 | 已实现 |
| 模具 | 借出、归还、维修、保养和历史记录 | 已实现 |
| 任务单 | 办公室派发任务，部门处理并反馈结果 | 已实现 |
| 统计 | 查看库存、任务、模具、业务和审计汇总 | 已实现 |
| 文件 | 业务图片上传、预览、替换和删除 | 已实现 |
| 版本更新 | 检查、校验和缓存桌面客户端更新包 | 已实现，仍需发布环境验收 |
| 备份、导出、提醒 | 未来增强能力 | 规划中 |

## 4. 后端核心业务时序图

### 4.1 登录与权限判断

用户只需要记住：登录成功不等于所有功能都能使用，系统还会根据角色决定可以看到和操作的内容。

~~~mermaid
sequenceDiagram
    actor user as 用户
    participant page as 登录页面
    participant api as Go 服务（业务大脑）
    participant auth as 认证与权限
    participant db as 数据库

    user->>page: 输入账号和密码
    page->>api: 提交登录信息
    api->>auth: 检查账号和密码
    auth->>db: 查询账号、状态、角色和权限
    db-->>auth: 返回账号信息
    alt 账号有效
        auth->>db: 保存 refresh token 摘要
        auth-->>api: 生成 access/refresh 登录凭证
        api-->>page: 返回登录成功、令牌和可用权限
        page-->>user: 显示允许使用的菜单和操作
        loop access token 临近过期
            page->>api: 提交 refresh token
            api->>auth: 原子轮换 refresh token
            auth->>db: 撤销旧会话并保存新摘要
            api-->>page: 返回新的登录凭证
        end
    else 账号或密码错误
        auth-->>api: 拒绝登录
        api-->>page: 返回错误原因
        page-->>user: 提示重新输入
    end

    Note over api,db: 页面隐藏操作只是第一层保护，Go 服务每次执行操作时仍会再次检查权限。
~~~

### 4.2 库存入库、出库、过账和冲销

用户只需要记住：库存变化不会直接改一个数字，而是先形成单据，检查无误后再留下可追溯的流水。

~~~mermaid
sequenceDiagram
    actor worker as 仓库人员
    participant page as 库存页面
    participant api as Go 服务
    participant check as 权限与库存检查
    participant db as 数据库

    worker->>page: 选择物品、数量和入库/出库类型
    page->>api: 提交库存单据
    api->>check: 检查权限、物品、数量和关联资料
    check->>db: 检查当前库存和重复请求
    db-->>check: 返回检查结果

    alt 检查通过
        check->>db: 在一次业务处理中保存单据
        check->>db: 更新库存余额
        check->>db: 写入库存流水
        db-->>api: 返回最新单据和库存
        api-->>page: 返回成功结果
        page-->>worker: 显示最新库存
    else 库存不足或数据不正确
        check-->>api: 拒绝本次操作
        api-->>page: 返回具体错误
        page-->>worker: 保留表单内容并提示修改
    end

    Note over check,db: 数量使用固定精度、金额使用分保存，避免小数计算误差。
    Note over api,db: 重复请求使用 Idempotency-Key 识别，避免重复入库或出库。
~~~

### 4.3 任务单派发、部门处理和办公室确认

用户只需要记住：办公室负责总任务，部门负责自己的子任务，所有过程都会留下记录。

~~~mermaid
sequenceDiagram
    actor office as 办公室人员
    actor department as 部门人员
    participant page as 任务单页面
    participant api as Go 服务
    participant db as 数据库
    participant log as 流转日志

    office->>page: 创建任务并选择目标部门
    page->>api: 保存任务草稿
    api->>db: 保存主任务和部门子任务
    api->>log: 记录“创建任务”
    api-->>page: 返回任务编号

    office->>page: 点击派发
    page->>api: 请求派发任务
    api->>db: 检查草稿状态并改为处理中
    api->>log: 记录“派发任务”
    api-->>page: 返回派发成功

    department->>page: 接收并开始处理部门子任务
    page->>api: 提交开始、部分完成或完成
    api->>db: 检查部门范围和当前状态
    api->>db: 保存部门子任务结果
    api->>log: 记录部门处理过程
    api-->>page: 返回最新子任务状态

    alt 所有部门子任务完成
        api->>db: 将主任务改为待办公室确认
        api->>log: 记录“等待办公室确认”
        api-->>page: 提示办公室确认
        office->>page: 选择正常完成或填写原因强制完成
        page->>api: 提交办公室确认
        api->>db: 保存最终状态
        api->>log: 记录最终确认
        api-->>page: 返回任务完成
    else 仍有部门未完成
        api-->>page: 返回当前进度
    end
~~~

### 4.4 模具借出、归还、维修和保养

用户只需要记住：模具不能随意跳状态，每次操作都会检查上一状态，并保存新的位置和历史。

~~~mermaid
sequenceDiagram
    actor worker as 模具使用人员
    participant page as 模具页面
    participant api as Go 服务
    participant check as 状态检查
    participant db as 数据库

    worker->>page: 查看模具当前状态
    page->>api: 请求模具详情和履历
    api->>db: 查询模具和操作历史
    db-->>api: 返回详情和履历
    api-->>page: 显示当前状态

    worker->>page: 发起借出、归还、维修或保养
    page->>api: 提交操作和位置/备注
    api->>check: 检查当前状态是否允许操作
    alt 状态允许
        check->>db: 更新模具状态、位置和保养日期
        check->>db: 写入模具履历
        db-->>api: 返回最新模具信息
        api-->>page: 显示操作成功
    else 状态不允许
        check-->>api: 拒绝不合法的状态转换
        api-->>page: 返回不能操作的原因
    end

    Note over check,db: 保存履历是为了以后能追溯模具去向、维修和保养过程。
~~~

### 4.5 业务图片单图/多图上传

用户只需要记住：一次可以选择多张图片，只有所有图片都校验并保存成功后，图库才会看到这批图片。

~~~mermaid
sequenceDiagram
    actor user as 用户
    participant page as 图片区域
    participant api as Go 文件服务
    participant check as 文件与业务权限检查
    participant files as 图片文件目录
    participant db as 数据库

    user->>page: 一次选择一张或多张图片
    page->>api: 以重复 file 字段提交同一个 multipart 请求
    api->>check: 检查 owner、权限以及每张图片格式和大小
    alt 全部检查通过
        check->>files: 写入本批次物理文件
        check->>db: 在一个事务中保存全部图片记录
        db-->>api: 返回图片元数据数组
        api-->>page: 返回成功结果
        page-->>user: 刷新图库并显示上传数量
    else 任一图片失败
        check->>files: 清理本批次已写入文件
        check-->>api: 回滚数据库记录并返回错误
        api-->>page: 返回失败原因
        page-->>user: 保留原图库并提示重新选择
    end
~~~

### 4.6 服务端读取本地清单并分发客户端完整包

用户只需要记住：计划任务先在服务器本机完成构建、签名和验证，服务端只读取已经原子激活的 stable 清单，并按哈希提供完整客户端包；不访问公网发布源，也不开放更新目录。

~~~mermaid
sequenceDiagram
    participant task as Windows 计划任务
    participant api as Go 更新服务
    participant stable as 本机 stable 清单
    participant verify as 文件与签名检查
    participant artifacts as 内容寻址资源
    participant client as 桌面客户端

    task->>verify: 构建、签名并校验完整包
    verify->>artifacts: 按 SHA-256 保存 NSIS/Portable
    verify->>stable: 原子切换已验证清单
    api->>stable: 读取当前正式版本
    api->>artifacts: 确认清单资源存在且哈希一致

    client->>api: 查询是否有可用更新
    api-->>client: 返回签名完整包计划
    client->>api: 按 sha256 下载受控资源
    api-->>client: 流式返回完整包
    Note over api,client: 私钥只在发布端签名；服务端和客户端使用公钥验证。
~~~

## 5. 当前后端边界

已实现但需要真实环境验收的内容：

- JWT 使用系统内部密钥；access token 默认 2 小时，refresh token 按活跃会话滚动 30 天；管理员首次登录后在系统内修改默认密码。
- 真实账号下的权限和组织/部门数据范围。
- Server 2016 构建、服务升级/回滚和 Windows 10 客户端安装、断网、重启。
- SQLite、图片目录和更新缓存的备份恢复。
- 长时间运行、并发访问和异常网络环境。

当前没有实现、不能当作已有功能宣传的内容：

- 统计报表导出。
- 任务实时提醒。
- 自动备份和恢复演练工具。
- 多组织切换和多仓库业务。

## 6. 相关文档

- [Go 后端状态、进度与维护台账](BACKEND_STATUS.md)
- [API 文档](API.md)
- [Windows 本机打包与局域网发布](WINDOWS_LAN_RELEASE.md)
- [Web、Client 前端产品架构与时序图](WEB_CLIENT_ARCHITECTURE.md)

## 7. 维护规则

后端 API、权限、配置、数据库、更新流程或业务状态发生变化时，必须同步更新本文档对应架构图/时序图，并同步维护 API 文档、后端状态文档和相关 Web/Client 文档。

## 8. 后续后端修改时的实施关注点

本文档是 Go 后端的架构基线。以后实施后端改动时，先判断改动属于哪一类，再按对应关注点检查；不要只修改代码而留下过期的架构图或时序图。

| 改动类型 | 需要检查或更新的内容 | 实施关注点 |
| --- | --- | --- |
| 新增业务模块或接口 | 架构图、对应业务时序图、API 文档 | 应用装配、路由、权限、请求校验、数据模型、测试和 OpenAPI 是否完整接入。 |
| 修改库存、任务单或模具流程 | 对应时序图和业务边界 | 状态转换、事务、幂等、并发冲突、审计记录和失败后的数据一致性。 |
| 修改数据库模型或字段 | 数据层说明和受影响时序图 | 自动迁移、旧数据兼容、索引、软删除、默认组织/仓库和恢复方案。 |
| 修改权限或数据范围 | 公共能力层和相关业务图 | 页面隐藏不是最终授权；后端必须继续校验角色、组织、部门和 cost:view。 |
| 修改金额、库存数量或成本 | 库存时序图和业务说明 | 数量固定精度、金额单位、加权平均成本和无成本权限时的字段裁剪。 |
| 修改图片文件流程 | 文件能力和相关业务时序图 | owner 权限继承、路径安全、文件与数据库的一致性、替换/删除失败回滚。 |
| 修改更新检查或发布流程 | 更新时序图和发布文档 | 公钥/私钥边界、签名和哈希校验、本地 stable 原子切换、完整包与回滚。 |
| 修改配置、部署或启动流程 | 架构边界和 README | JWT 和管理员初始密码按当前内部默认策略运行；更新验签配置、目录权限和备份要求必须明确。 |

### 8.1 后端实施完成前检查

1. 找到改动涉及的业务模块、公共中间件、数据模型和路由注册位置。
2. 明确 API、权限、数据库字段、状态流转、审计和错误行为是否发生变化。
3. 修改实现后同步更新本文档对应图示、docs/API.md、OpenAPI 产物和 BACKEND_STATUS.md。
4. 为正常路径、权限拒绝、输入错误、重复请求、并发冲突和外部资源失败补充测试。
5. 分别记录代码、文档、验证和提交结果；未验证的内容不得标记为已完成。

小范围注释修正、单个测试修复或不改变架构的数据处理修正，直接记录在 BACKEND_STATUS.md 的待修改/待修复和验证记录中，不需要重复绘制架构图。
