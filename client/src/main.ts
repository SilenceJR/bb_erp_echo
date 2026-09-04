// 桌面端入口。
//
// 说明：
// - 直接复用 web/src/main.ts，避免 Web 和桌面端维护两套页面与 API 封装。
// - 先注入 Tauri Rust HTTP 传输，再加载共用 Web 页面。
// - 后端地址可在客户端内保存，VITE_API_BASE_URL 仅作为首次启动默认值。
import './desktop-http'
import './native-file-drop'
import '../../web/src/main'
