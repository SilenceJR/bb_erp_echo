// 桌面端入口。
//
// 说明：
// - 直接复用 web/src/main.ts，避免 Web 和桌面端维护两套页面与 API 封装。
// - 后端地址仍由 VITE_API_BASE_URL 控制，默认连接 http://127.0.0.1:8080。
import '../../web/src/main'
