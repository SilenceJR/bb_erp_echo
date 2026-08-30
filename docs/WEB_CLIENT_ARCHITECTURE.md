# Web 与 Tauri Client 产品架构与交互时序图

> 基准日期：2026-08-27
> 适用范围：web/ Vue 管理端、client/ Tauri 桌面端及两者共用的页面和业务逻辑

## 1. 给非技术用户的一句话

Web 和 Client 是两种使用 ERP 的方式：Web 在浏览器里打开，Client 是安装在电脑上的桌面窗口；两者看到的业务页面基本相同，区别主要在于“请求 Go 服务”和“桌面更新”由谁来完成。

## 2. 阅读图例

- 绿色：已经实现并接入当前代码。
- 黄色：已经有代码，但还需要真实浏览器、桌面或发布环境验收。
- 蓝色虚线：后续规划，不代表当前版本已经提供。
- “共享页面”表示 Web 和 Client 共用同一套页面、权限显示和业务操作。

## 3. Web 产品架构图

~~~mermaid
flowchart TB
    user["ERP 使用人员<br/>管理员 / 办公室 / 仓库 / 部门"]:::user
    browser["浏览器<br/>打开 ERP Web 地址"]:::entry

    subgraph web["Web 管理端（用户看到的页面）"]
        main["应用入口<br/>登录态、Element Plus、页面编排"]:::done
        shell["工作台壳层<br/>顶栏、侧边栏、移动端导航"]:::done
        pages["业务页面<br/>首页、资料、库存、模具、任务单、统计、系统设置"]:::done
        panels["交互组件<br/>表格、表单、详情抽屉、图片、确认弹窗"]:::done
        state["业务模块<br/>登录、列表、权限、库存、模具、任务单、更新状态"]:::done
        request["统一请求层<br/>页面通过同一种方式访问后端"]:::done
    end

    backend["Go 后端<br/>检查权限并处理业务"]:::done
    user --> browser
    browser --> main
    main --> shell
    shell --> pages
    pages --> panels
    pages --> state
    state --> request
    request --> backend

    mobile["移动宽度<br/>导航收进抽屉，列表变为卡片"]:::pending
    planned["规划方向<br/>继续拆分业务样式、真实浏览器全流程验收"]:::planned
    shell -.-> mobile
    panels -.-> planned

    classDef user fill:#fff3e0,stroke:#d98c00,color:#4a2a00
    classDef entry fill:#e3f2fd,stroke:#1976d2,color:#0d2b45
    classDef done fill:#e8f5e9,stroke:#388e3c,color:#173d1c
    classDef pending fill:#fff8e1,stroke:#c58b00,color:#4b3800
    classDef planned fill:#eef2ff,stroke:#5c6bc0,color:#202a61
~~~

### 3.1 Web 页面怎么理解

| 页面区域 | 用户看到的内容 | 当前状态 |
| --- | --- | --- |
| 登录页 | 登录、退出、服务器地址设置 | 已实现 |
| 工作台 | 顶栏、侧边栏、移动端导航 | 已实现 |
| 首页 | 业务概览、待关注事项和快捷入口 | 已实现 |
| 业务页面 | 资料、库存、模具、任务单和统计 | 已实现 |
| 交互组件 | 列表、搜索、分页、表单、详情抽屉 | 已实现 |
| 权限显示 | 不允许的菜单和操作不主动展示 | 已实现，后端仍是最终判断 |
| 统一弹窗 | 删除、放弃、继续和输入原因 | 已实现 |
| 全流程验收 | 使用真实 Go 服务和真实账号验证 | 待完成 |

## 4. Tauri Client 产品架构图

~~~mermaid
flowchart TB
    user["桌面端使用人员"]:::user
    window["Tauri 桌面窗口<br/>承载用户看到的 ERP 页面"]:::entry

    subgraph shared["与 Web 共用的页面和业务逻辑"]
        main["共用 Web 入口"]:::done
        workspace["共用工作台和业务页面"]:::done
        composables["共用业务模块<br/>登录、权限、库存、模具、任务单、更新状态"]:::done
        transport["统一请求抽象<br/>页面只说明要访问哪个业务接口"]:::done
    end

    subgraph desktop["桌面端专属能力"]
        bridge["desktop-http<br/>桌面端传话通道"]:::done
        rust["Rust/Tauri 核心<br/>发送请求、读取版本、管理文件和更新"]:::done
        local["本机设置<br/>保存 Go 服务地址和更新状态"]:::done
    end

    server["Go 后端<br/>业务接口和权限检查"]:::done
    packages["服务器局域网更新资源<br/>完整 NSIS 与便携版"]:::pending

    user --> window
    window --> main
    main --> workspace
    workspace --> composables
    composables --> transport
    transport --> bridge
    bridge --> rust
    rust --> server
    rust --> local
    rust --> packages

    boundary["复用约束<br/>页面不能直接判断浏览器或 Tauri<br/>平台差异由请求抽象和桌面能力处理"]:::boundary
    boundary -.-> shared

    classDef user fill:#fff3e0,stroke:#d98c00,color:#4a2a00
    classDef entry fill:#e3f2fd,stroke:#1976d2,color:#0d2b45
    classDef done fill:#e8f5e9,stroke:#388e3c,color:#173d1c
    classDef pending fill:#fff8e1,stroke:#c58b00,color:#4b3800
    classDef boundary fill:#f3e5f5,stroke:#8e44ad,color:#3d1f4d
~~~

### 4.1 Web 和 Client 的区别

| 对比项 | Web 浏览器 | Tauri Client |
| --- | --- | --- |
| 页面 | 使用共用 Vue 页面 | 使用同一套共用 Vue 页面 |
| 访问 Go 服务 | 浏览器发起同源或配置地址请求 | Rust HTTP 传话通道发起请求 |
| 服务地址 | 使用浏览器端配置 | 可保存到本机并切换 |
| 登录会话 | 共用 access/refresh token 自动轮换 | 共用 access/refresh token 自动轮换 |
| 桌面文件和安装 | 不负责安装桌面程序 | Rust/Tauri 负责下载、安装和回滚 |
| 更新显示 | 只能展示后端提供的正式版状态 | 可以检查、下载并执行正式版桌面更新；RC 使用独立便携 EXE 测试包 |
| 业务 API | 与 Client 使用同样的路径和字段 | 与 Web 使用同样的路径和字段 |

## 5. 前端核心交互时序图

### 5.1 Web 登录、加载工作台和显示权限

~~~mermaid
sequenceDiagram
    actor user as 用户
    participant browser as 浏览器
    participant login as 登录页面
    participant api as Go 服务
    participant workspace as 工作台

    user->>browser: 打开 ERP 地址
    browser->>login: 显示登录页面
    user->>login: 输入账号和密码
    login->>api: 提交登录
    api-->>login: 返回登录凭证和权限

    alt 登录成功
        login->>workspace: 进入工作台
        workspace->>api: 查询当前用户信息
        api-->>workspace: 返回角色、部门和权限
        workspace-->>user: 显示允许使用的菜单和操作
        loop access token 临近过期或接口返回 401
            workspace->>api: 提交 refresh token
            api-->>workspace: 返回轮换后的令牌
            workspace->>api: 使用新 access token 重试原请求
        end
    else 登录失败
        api-->>login: 返回错误
        login-->>user: 显示错误并保留登录页面
    end
~~~

### 5.2 Web 移动端导航、列表查询和详情抽屉

~~~mermaid
sequenceDiagram
    actor user as 用户
    participant page as Web 页面
    participant nav as 导航和页面状态
    participant api as Go 服务
    participant drawer as 详情抽屉

    user->>page: 在手机宽度打开页面
    page->>nav: 根据 activeKey 选择当前业务
    nav-->>user: 显示移动端抽屉导航和业务卡片
    user->>page: 输入关键字或切换分页
    page->>api: 查询列表
    api-->>page: 返回分页、搜索和筛选结果
    user->>page: 点击某条业务记录
    page->>api: 查询详情和相关流水/日志
    api-->>drawer: 返回详情数据
    drawer-->>user: 显示详情和可执行操作
    user->>nav: 切换到其他业务
    nav->>drawer: 关闭抽屉并清理上一条记录的状态
~~~

### 5.3 Web 表单、统一弹窗和错误提示

~~~mermaid
sequenceDiagram
    actor user as 用户
    participant form as 页面表单
    participant box as 统一确认/输入弹窗
    participant api as Go 服务
    participant state as 页面状态

    user->>form: 点击新增、编辑或危险操作
    alt 需要确认或填写原因
        form->>box: 打开确认或输入弹窗
        box-->>user: 显示居中卡片、遮罩和操作按钮
        user->>box: 选择确认/取消或填写原因
        box-->>form: 返回用户选择
    else 普通保存
        form->>state: 检查页面必填项
    end

    alt 用户确认并且输入有效
        form->>api: 提交业务请求
        api-->>form: 返回成功或业务错误
        form->>state: 刷新列表、详情和提示
        state-->>user: 显示最新结果
    else 用户取消或输入无效
        form->>state: 保留或清理当前表单
        state-->>user: 显示取消或错误提示
    end

    Note over box: 所有业务统一使用同一种确认和输入弹窗，避免不同页面样式不一致。
~~~

图片批量上传沿用同一请求抽象：Web 和 Tauri 都由共用页面一次提交重复 `file` 字段，服务端成功返回图片数组；替换仍是单图 PUT。

### 5.4 Client 启动、读取地址和测试连接

~~~mermaid
sequenceDiagram
    actor user as 用户
    participant window as Tauri 窗口
    participant shared as 共用 Web 页面
    participant bridge as desktop-http 传话通道
    participant rust as Rust/Tauri
    participant server as Go 服务

    user->>window: 打开桌面客户端
    window->>rust: 启动桌面能力
    rust->>rust: 读取已保存的 Go 服务地址
    rust-->>bridge: 提供当前服务地址
    bridge->>shared: 注入桌面请求能力
    shared-->>user: 显示登录页面

    user->>shared: 输入新服务地址并点击测试
    shared->>bridge: 请求测试 /health
    bridge->>rust: 发送 HTTP 请求
    rust->>server: 访问 Go 服务健康检查
    alt 服务可连接
        server-->>rust: 返回健康
        rust-->>bridge: 返回连接成功
        bridge-->>shared: 显示成功并允许保存
        shared-->>user: 保存地址
    else 无法连接
        server-->>rust: 超时或返回错误
        rust-->>bridge: 返回可读错误
        bridge-->>shared: 显示检查地址、端口和服务状态的提示
    end
~~~

### 5.5 Client 检查并执行桌面更新

~~~mermaid
sequenceDiagram
    actor user as 用户
    participant page as Client 更新页面
    participant bridge as desktop-http 传话通道
    participant rust as Rust/Tauri 更新能力
    participant server as Go 更新接口
    participant files as 本机安装文件

    user->>page: 点击检查更新
    page->>bridge: 请求客户端更新计划
    bridge->>rust: 调用桌面更新检查
    rust->>server: 查询当前版本、文件哈希和目标平台
    server-->>rust: 返回局域网完整包计划
    rust->>page: 显示目标版本并请求用户确认
    user->>page: 选择稍后处理或开始更新并重启
    page->>rust: 用户确认后开始更新
    rust->>server: 按 SHA-256 下载完整安装包
    rust->>rust: 校验类型、签名、哈希和文件大小

    alt 校验和安装成功
        rust->>files: 安排替换并保留回滚文件
        files-->>rust: 返回安装结果
        rust-->>bridge: 返回成功和进度
        bridge-->>page: 更新进度和重启提示
        page-->>user: 显示更新完成
    else 下载、校验或安装失败
        rust-->>bridge: 返回失败和当前版本保留状态
        bridge-->>page: 显示可理解的错误
        page-->>user: 建议检查局域网或重试更新
    end
~~~

## 6. 当前前端边界

已实现但需要真实环境验收的内容：

- Web 登录、退出、服务器设置和移动端导航。
- Web 列表、分页、搜索、筛选、表单和详情抽屉。
- 库存、模具、任务单、图片和统一弹窗的真实 API 流程。
- Tauri Client 的服务器地址保存、Rust HTTP 传输和更新状态展示。
- Windows 10/11 正式版安装、升级、回滚和异常网络场景；RC/手动便携 EXE 独立启动。

当前规划或尚未完成的内容：

- 业务样式继续按组件下沉。
- 浏览器全流程真实账号验收。
- Client macOS、Windows 10/11 真机运行态验收。
- 统计导出、任务实时提醒和更复杂的桌面能力。

## 7. 相关文档

- [Go 后端产品架构与业务时序图](BACKEND_ARCHITECTURE.md)
- [Go 后端状态、进度与维护台账](BACKEND_STATUS.md)
- [Web 与 Tauri Client 端进度与维护规范](WEB_STATUS.md)
- [Web/Tauri API、传输和更新同步规则](../client/API_SYNC.md)

## 8. 维护规则

Web 和 Client 共用的页面、请求路径、权限显示或更新交互发生变化时，必须同步更新本文档、BACKEND_STATUS.md、WEB_STATUS.md 及受影响的 API/客户端同步文档。页面不得直接实现平台专属网络逻辑，平台差异必须留在请求抽象和 Tauri 专属能力中。

## 9. 后续 Web/Client 修改时的实施关注点

本文档是 Web 和 Tauri Client 的架构基线。以后实施前端改动时，先判断是共用页面变化、Web 专属变化还是 Client 专属变化，再检查对应边界；不要把平台差异重新写进业务页面。

| 改动类型 | 需要检查或更新的内容 | 实施关注点 |
| --- | --- | --- |
| 新增页面或导航 | Web 架构图、工作台说明和 WEB_STATUS.md | 页面放入对应目录，继续使用 activeKey，不在 App.vue 堆积业务状态，不新增路由系统。 |
| 修改共用业务页面 | Web 架构图、相关交互时序图和 WEB_STATUS.md | Web 与 Client 都要能使用同一套 props、事件、组合式函数和 API 字段。 |
| 修改请求或传输方式 | Web/Client 关系图、API_SYNC.md 和后端文档 | 页面只依赖统一请求抽象；Web 使用浏览器请求，Client 使用 desktop-http 和 Rust HTTP。 |
| 修改 Client 专属能力 | Client 架构图、更新时序图和 API_SYNC.md | 服务器地址、版本、文件路径、安装、回滚和 Tauri IPC 只放在桌面能力层。 |
| 修改登录、权限或服务器切换 | 登录时序图和状态文档 | 登录请求需绑定当前服务地址和认证代次；切服、退出或失效响应不能被旧请求覆盖。 |
| 修改列表、详情或抽屉 | 移动端时序图和 WEB_STATUS.md | 分页、搜索、筛选、AbortController、请求序号、过期响应和关闭抽屉时的状态清理。 |
| 修改表单或危险操作 | 表单/弹窗时序图 | 统一使用 appMessageBox，保留确认/取消、输入原因、键盘关闭、遮罩和移动端布局。 |
| 修改响应式布局 | Web 架构说明和 WEB_STATUS.md | 桌面表格与移动卡片都要验证，不能只在宽屏下确认视觉结果。 |
| 修改更新页面 | Client 架构图、更新时序图和发布文档 | Vue 只展示和触发，Rust 负责签名校验、下载、安装、回滚和进度。 |

### 9.1 前端实施完成前检查

1. 先确认改动影响 Web、Client 还是两者共用部分，并列出受影响的组件、组合式函数、请求接口和状态。
2. 检查权限隐藏、后端最终授权、加载/错误/空状态、重复提交和旧请求覆盖问题。
3. 修改共用源码后同步更新本文档、WEB_STATUS.md、BACKEND_STATUS.md 及 API_SYNC.md（如适用）。
4. 在桌面宽度和移动宽度检查页面、抽屉、表单、弹窗和导航；Client 专属功能再检查 Tauri 运行边界。
5. 分别记录代码、文档、Web/Client 构建、运行态验证和提交结果。

小范围文案、样式或单页面交互修正，直接记录在 WEB_STATUS.md 的进度和验证记录中；只有影响页面层次、数据流、平台边界或核心交互时才需要调整架构图。

## 10. 文档分工

- 本文档：记录 Web/Client 稳定架构、共享边界、核心交互和实施关注点。
- BACKEND_ARCHITECTURE.md：记录 Go 后端稳定架构、业务数据流和后端实施关注点。
- WEB_STATUS.md：记录前端已完成、待完成、待修复和真实运行态验收进度。
- BACKEND_STATUS.md：记录后端已完成、待完成、待修改、待修复和验证进度。
