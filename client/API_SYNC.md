# API 同步说明

桌面端 `client` 不复制 Web 管理端代码，而是通过 `client/src/main.ts` 直接导入 `../web/src/main.ts`。

## 同步规则

- 后端 API 地址仍由 `VITE_API_BASE_URL` 控制；桌面端默认值在 `client/vite.config.ts` 中维护。
- `npm run desktop:dev` 时 API 使用同源路径，并由 Vite 代理到 Go 后端，便于 Air 命令行看到请求。
- `npm run desktop:build` 时默认连接 `http://127.0.0.1:8080`，可通过 `VITE_API_BASE_URL` 覆盖。
- 模块导航和接口路径仍在 `web/src/data/modules.ts` 中维护。
- Go 后端新增或调整接口时，优先更新 `web/src/api/*` 和 `web/src/data/modules.ts`，桌面端会自动复用。
- 桌面端只负责 Rust/Tauri 壳、窗口配置、平台打包配置，不维护另一套业务 API。

## 调试说明

- 先启动 Go 后端：`go run ./cmd/server`
- 再启动桌面调试：`cd client && npm run desktop:dev`
- macOS 调试直接使用 Tauri dev。
- Windows 支持通过 `npm run desktop:build` 构建安装包，前提是本机已安装 Rust、Node.js 和 Tauri 对应平台依赖。
