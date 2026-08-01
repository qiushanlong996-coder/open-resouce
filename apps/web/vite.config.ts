import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')

  // API 代理配置。dev 与 preview 共用一份：
  // 开发模式首次加载协作编辑器需要现场转换 milkdown 依赖图，很慢；
  // 用 `vite preview` 跑生产构建（chunk 已预打包）验证会稳定得多。
  const apiProxy = {
    '/api': {
      target: env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:18080',
      changeOrigin: true,
      secure: false,
      // 协作编辑走 WebSocket，必须开启转发。
      ws: true,
      rewriteWsOrigin: true,
    },
  }

  return {
    plugins: [react()],
    server: { proxy: apiProxy },
    preview: { proxy: apiProxy },
  }
})
