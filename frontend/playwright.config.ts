import { defineConfig, devices } from '@playwright/test'

// E2E は docker compose で起動したアプリ一式に対して実行する。
// 単体テスト（Vitest + MSW）はモックの上で動くため、実際のDB・API・
// ブラウザのレイアウトが絡む問題はここでしか見つからない。
export default defineConfig({
  testDir: './e2e',
  // 1テストあたりの上限。実DBと実ブラウザなので単体テストより余裕を持たせる。
  timeout: 30_000,
  expect: { timeout: 10_000 },
  // テスト内に .only が残っていたらCIでは失敗させる（他が実行されなくなるため）。
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  // 献立はランダムに選ばれ、履歴・お気に入りはユーザーごとに独立している。
  // テストごとに別ユーザーを作るので並列で問題ない。
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:5173',
    // 失敗した再試行だけ記録する。常に録ると実行時間と容量を食う。
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    locale: 'ja-JP',
    timezoneId: 'Asia/Tokyo',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
