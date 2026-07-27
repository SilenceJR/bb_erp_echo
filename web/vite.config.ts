import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Vite 配置：开发端口固定为 5173，和后端 CORS 默认白名单保持一致。
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
  },
})
