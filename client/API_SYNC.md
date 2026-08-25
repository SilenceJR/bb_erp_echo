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
- 桌面壳通过 `@tauri-apps/api/app` 的 `getVersion()` 读取真实安装版本，请求 `/api/v1/updates/client/status` 时以 `current_version` 传给 Go；Web 端不发送桌面版本，也不显示客户端安装包提示。

## 更新请求

- `/api/v1/version` 保持启动兼容信息。
- Tauri 调用 `/api/v1/updates/client/status?current_version=<安装版本>`，只有服务端已校验并缓存更高版本客户端包时才展示下载提示。
- 管理页读取 `/api/v1/system/updates/status`；拥有 `system:updates:write` 时可调用 `POST /api/v1/system/updates/check`。
- 客户端包仍从 `/api/v1/updates/client/download` 下载；manifest 和 Release 附件来自公开 HTTPS 地址，传输层不依赖 Gitee/GitHub 专属 API。

## 调试说明

- 先启动 Go 后端：`go run ./cmd/server`
- 再启动桌面调试：`cd client && npm run desktop:dev`
- macOS 调试直接使用 Tauri dev。
- Windows 支持通过 `npm run desktop:build` 构建安装包，前提是本机已安装 Rust、Node.js 和 Tauri 对应平台依赖。
