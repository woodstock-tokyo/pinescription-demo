import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const backendHost = (env.VITE_BACKEND_HOST || 'localhost').trim()
  const backendPort = (env.VITE_BACKEND_PORT || '8080').trim()
  const wsTarget = (env.VITE_WS_TARGET || `ws://${backendHost}:${backendPort}`).trim()

  return {
    plugins: [react()],
    server: {
      port: 3000,
      proxy: {
        '/ws': {
          target: wsTarget,
          ws: true,
          changeOrigin: true,
        },
      },
    },
    preview: { port: 3000, host: true },
  }
})
