import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Vite 配置：开发端口固定为 5173，和后端 CORS 默认白名单保持一致。
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    // 开发模式下把 API 和健康检查转发给 Echo 后端。
    //
    // 参数说明：
    // - target：后端默认监听地址。
    // - changeOrigin：改写 Origin，避免本地代理请求被跨域规则影响。
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      '/health': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
})
