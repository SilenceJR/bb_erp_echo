import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Vite 桌面端配置。
//
// 参数说明：
// - root 默认为 client 目录。
// - publicDir 指向 ../web/public，复用 Web 管理端静态资源。
// - server.fs.allow 允许 Vite 开发服务器读取 ../web/src，桌面端不复制页面代码。
// - dev 模式下 API 保持同源路径，由 Vite 代理到 Go 后端，方便 Air 看到真实请求。
// - build 模式下 Tauri 加载本地静态资源，默认把 API 地址写为 Go 后端 8080。
export default defineConfig(({ command }) => {
  const desktopApiBase = process.env.VITE_API_BASE_URL || 'http://127.0.0.1:8080'

  return {
    plugins: [vue()],
    publicDir: fileURLToPath(new URL('../web/public', import.meta.url)),
    define: {
      'import.meta.env.VITE_API_BASE_URL': JSON.stringify(command === 'build' ? desktopApiBase : ''),
    },
    server: {
      host: '127.0.0.1',
      port: 1420,
      strictPort: true,
      proxy: {
        '/api': {
          target: desktopApiBase,
          changeOrigin: true,
        },
        '/health': {
          target: desktopApiBase,
          changeOrigin: true,
        },
      },
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
  }
})
