import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/ws': {
        target:    'ws://backend:8080',
        ws:        true,
        changeOrigin: true,
      },
    },
  },
  preview: { port: 3000, host: true },
})
