# API 同步说明

桌面端 `client` 不复制 Web 管理端代码，而是通过 `client/src/main.ts` 直接导入 `../web/src/main.ts`。

## 同步规则

- Web 生产环境使用相对 API 路径，推荐由 Go 同时托管 Web 静态文件，浏览器始终访问同源服务。
- 桌面端通过 `@tauri-apps/plugin-http` 从 Rust 层发送 HTTP 请求，不依赖 macOS/Windows WebView 的跨域行为。
- 共用请求层通过 `HttpTransport` 接口选择浏览器或桌面传输，业务 API 不感知运行平台。
- `npm run desktop:dev` 和生产安装包都通过同一套 Rust HTTP 传输访问 Go 后端。
- 首次启动默认连接 `http://127.0.0.1:8080`，可在登录页或登录后的顶栏设置运行 Go 服务的内网地址，例如 `http://192.168.1.20:8080`。
- `VITE_API_BASE_URL` 只用于指定安装包首次启动的默认地址；用户保存后的地址优先，并可在未来直接切换为公网 HTTPS 地址。
- 服务地址只接受不带路径、参数和凭据的 `http://` 或 `https://` 源地址；每次实际请求仍只能使用站内 API 路径。
- 模块导航和接口路径仍在 `web/src/data/modules.ts` 中维护。
- Go 后端新增或调整接口时，优先更新 `web/src/api/*` 和 `web/src/data/modules.ts`，桌面端会自动复用。
- 桌面端只负责 Rust/Tauri 壳、窗口配置、平台打包配置，不维护另一套业务 API。
- 桌面壳通过 `@tauri-apps/api/app` 的 `getVersion()` 读取真实安装版本；Rust 更新引擎计算当前 EXE SHA 并向 Go 请求更新计划。Vue 只触发检查/安装并展示状态，不接触本机路径、任意下载 URL或签名决策；Web 端不显示桌面自动安装按钮。

## 更新请求

- `/api/v1/version` 保持启动兼容信息。
- 旧客户端继续调用 `/api/v1/updates/client/status?current_version=<安装版本>`。
- 新客户端由 Rust 调用 `/api/v1/updates/client/plan`，传入真实版本、当前 EXE SHA、`windows-x86_64` 和安装模式。返回 `204` 表示已是最新；返回 `404` 表示旧服务端，界面退回完整 ZIP 提示。
- 差分包仅适用于签名 payload 声明的精确上一版本和来源 EXE 哈希；任何不匹配或失败均自动切换完整资源。NSIS 安装版由 Tauri updater 安装，便携版由本地 helper 原子替换并在 90 秒 Ready 超时后回滚。
- `/api/v1/updates/client/tauri/windows/x86_64/<当前版本>` 提供官方 updater JSON；`/api/v1/updates/client/artifacts/<sha256>` 只分发当前已验签 manifest 中的内容寻址资源，支持 ETag 和 Range。
- 管理页读取 `/api/v1/system/updates/status`；拥有 `system:updates:write` 时可调用 `POST /api/v1/system/updates/check`。
- 兼容客户端 ZIP 仍从 `/api/v1/updates/client/download` 下载；manifest 和 Release 附件来自公开 HTTPS 地址，传输层不依赖 Gitee/GitHub 专属 API。局域网服务可使用 HTTP，但 Rust 只接受 loopback/私网地址，所有自动更新资源仍须通过端到端签名与哈希验证；公网服务必须 HTTPS。

## 调试说明

- 先启动 Go 后端：`go run ./cmd/server`
- 再启动桌面调试：`cd client && npm run desktop:dev`
- macOS 调试直接使用 Tauri dev。
- Windows 支持通过 `npm run desktop:build` 构建安装包，前提是本机已安装 Rust、Node.js 和 Tauri 对应平台依赖。
