import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Vite 桌面端配置。
//
// 参数说明：
// - root 默认为 client 目录。
// - publicDir 指向 ../web/public，复用 Web 管理端静态资源。
// - server.fs.allow 允许 Vite 开发服务器读取 ../web/src，桌面端不复制页面代码。
export default defineConfig({
  plugins: [vue()],
  publicDir: fileURLToPath(new URL('../web/public', import.meta.url)),
  server: {
    host: '127.0.0.1',
    port: 1420,
    strictPort: true,
    fs: {
      allow: [
        fileURLToPath(new URL('.', import.meta.url)),
        fileURLToPath(new URL('../web', import.meta.url)),
      ],
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  resolve: {
    alias: {
      '@web': fileURLToPath(new URL('../web/src', import.meta.url)),
    },
  },
})
