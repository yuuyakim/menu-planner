import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  test: {
    // ブラウザAPI(DOM/fetch)を使うため jsdom 上で動かす。
    environment: 'jsdom',
    // 単体テストは src だけ。e2e/ は Playwright が実ブラウザで動かすもので、
    // Vitest から実行すると @playwright/test の API が無くて落ちる。
    include: ['src/**/*.test.{ts,tsx}'],
    // describe / it / expect を import なしでも使えるようにする。
    // 各テストは明示的に import しているが、globals 前提のライブラリ
    // （@testing-library/jest-dom）が動くために必要。
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    // jsdom の既定のオリジン。'/api/...' のような相対URLをここに解決する。
    environmentOptions: {
      jsdom: { url: 'http://localhost:5173' },
    },
    css: false,
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.test.{ts,tsx}', 'src/test/**', 'src/main.tsx'],
    },
  },
  server: {
    // コンテナ外からアクセスできるよう 0.0.0.0 で待ち受ける
    host: true,
    port: 5173,
    fs: {
      // frontend の1つ上（repoルート）まで raw import を許可する。
      // 法務文書(docs/legal/*.md)を `?raw` で直接読み込むために必要。
      allow: ['..'],
    },
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
