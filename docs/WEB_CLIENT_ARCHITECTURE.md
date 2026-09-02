# Windows Tauri 客户端优先架构

> 基准日期：2026-09-01
> 产品与整改细则以 [CLIENT_FIRST_REMEDIATION.md](CLIENT_FIRST_REMEDIATION.md) 为准。

## 产品入口

- Windows 10/11 Tauri 是正式产品，1920×1080 为主验收分辨率，最小窗口 `1024×680`。
- Web 是桌面浏览器备用入口，复用同一套 Vue 页面、领域状态、权限、API 和错误语义。
- 当前按全新内网部署设计，不维护公网、代理、macOS、移动端原生客户端、旧 API 或旧客户端双轨；平板/手机仅提供低频 Web 访问适配。

## 分层

```text
Windows Tauri                         桌面 Web
  Rust 平台适配器                       浏览器适配器
        \                                 /
         Connection / Discovery / FileSave
         WindowLeave / ClientUpdate 端口
                       |
              Vue 共享业务与 UI
                       |
              Go API / 权限 / 状态机
                       |
                    SQLite
```

Go 是业务、权限、库存、任务、模具和审计的最终裁决者。Vue 不直接调用 Tauri API；Rust 不复制业务规则。

## 启动与连接

```text
Booting -> Discovering -> Validating
                          |-> 唯一实例 -> LoginReady
                          |-> 多实例   -> SelectServer
                          `-> 零实例   -> ManualSetup
```

Tauri 按私网 IPv4 网卡向 UDP `39080` 广播。响应只能提供候选来源 IP 与 HTTP 端口；只有 nonce、协议、产品、UUID、`/ready` 和匿名身份接口全部匹配才可连接。仅对来源 IP、端口和实例 ID 完全相同的重包去重；同一实例 ID 来自不同地址时按克隆冲突处理，禁止静默选择。

Web 不执行 UDP 发现，只验证当前同源 Go 服务。Tauri 的发现、保存、当前、手动和选择路径均以规范化 `{origin, instance_id}` 为服务身份；同 ID 不同 origin 保留为冲突候选，任一身份坐标变化都会在业务会话挂载前清除旧认证并卸载内存领域状态，防止令牌被发送到克隆服务。

## 平台能力

| 端口 | Tauri | Web |
| --- | --- | --- |
| Connection | Rust 验证内网 IPv4 服务 | 验证当前同源服务 |
| Discovery | UDP 广播 + HTTP 身份复核 | 不支持 |
| FileSave | 系统对话框、流式临时文件、原子替换 | 浏览器 Blob 下载 |
| WindowLeave | 标题栏关闭进入 DirtyGuardRegistry | `beforeunload` |
| ClientUpdate | Rust 验签、替换与恢复 | 不显示安装动作 |

## 共享前端

- 模块、菜单和权限由注册表集中声明；不引入 Vue Router。
- 导航采用“首页 → 业务办理/基础资料/数据与报表 → 系统设置”分组；`AppNavigation` 只按已授权模块渲染分组，不复制模块定义或权限判断。
- 页面状态按客户、部门、员工、仓库、任务、模具、统计和系统目录拆分。
- `useWorkspaceController` 只组合会话、导航和领域 facade；通用目录列表/分页/表单由 `useDirectoryOperations` 负责，任务产品、库存、日志和状态动作由 `useWorkorderOperations` 负责。
- 控制器在组合阶段直接由任务 state、operations 和明确跨域依赖构造四切片 `workorderContext`，根返回不再逐项暴露任务字段或命令。
- `WorkspaceSession` 直接 provide 该窄对象；任务列表、产品选择、详情、库存卡和动作 Dialog 不依赖全量 `WorkspaceContext`，仓库等非任务组件也不解构任务 facade。
- 设计令牌只在 `design-system.css`，消费点只使用 `--bb-*`；`styles.css` 仅作为分层样式入口，UI 原语统一页面头、筛选、表格、状态、Drawer 和表单动作。
- 动效采用 CSS 与 Motion 分层：hover/focus/颜色等高频反馈使用 CSS，状态驱动、布局和 enter/exit 使用 `motion-v`；全局尊重 reduced motion，平板/手机 Web 只做低频布局适配。
- 未保存状态统一登记到 `DirtyGuardRegistry`，覆盖导航、登出、切服、更新、刷新和标题栏关闭。
- Web 更新中心只提供服务端升级包的同源受保护下载；桌面客户端更新由 Tauri 面板负责检查、验签、应用和恢复，不提供无有效下载契约的 ZIP 卡片。
- 手机/平板 Web 不新增业务 API 或平台端口；窄屏使用可收起导航、可滚动表格和单列内容，不能影响 Tauri 的桌面窗口契约。

## 验收边界

自动化必须通过 Go 测试/静态检查、Web/Client 构建与 Rust 检查/测试。真实 Windows 10/11 仍需在 1080p 的 100%/125%/150% 缩放下验收零/一/多服务、UDP 阻断、登录权限、文件保存、关闭守卫与更新；静态构建不等于真机验收。
