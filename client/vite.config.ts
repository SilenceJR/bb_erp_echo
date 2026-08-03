import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import {ElementPlusResolver} from 'unplugin-vue-components/resolvers'

// Vite 桌面端配置。
//
// 参数说明：
// - root 默认为 client 目录。
// - publicDir 指向 ../web/public，复用 Web 管理端静态资源。
// - server.fs.allow 允许 Vite 开发服务器读取 ../web/src，桌面端不复制页面代码。
// - API 实际由 Tauri Rust HTTP 插件发送，构建变量只作为首次启动默认地址。
export default defineConfig(({ command }) => {
  const desktopApiBase = process.env.VITE_API_BASE_URL || 'http://127.0.0.1:8080'

  return {
    plugins: [
      vue(),
      Components({
        resolvers: [ElementPlusResolver()],
        dts: 'src/components.d.ts',
      }),
    ],
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
