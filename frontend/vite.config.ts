import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // コンテナ外からアクセスできるよう 0.0.0.0 で待ち受ける
    host: true,
    port: 5173,
    proxy: {
      // ブラウザから見て同一オリジンにするため /api を backend に転送する。
      // これによりCookieがクロスオリジン扱いにならない。
      '/api': {
        target: process.env.BACKEND_ORIGIN ?? 'http://backend:8080',
        changeOrigin: true,
      },
    },
    watch: {
      // Windowsホストのバインドマウントではinotifyが効かないためポーリングする
      usePolling: true,
      interval: 300,
    },
  },
})
