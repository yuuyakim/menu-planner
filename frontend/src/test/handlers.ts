import { http, HttpResponse } from 'msw'

// 既定のハンドラ。全テストで共通に使える最小限だけを置き、
// 個別の応答は各テストで server.use() を呼んで上書きする。
export const handlers = [
  http.get('/api/v1/health', () => HttpResponse.json({ status: 'ok' })),

  // ログイン状態はヘッダが全画面で問い合わせる。既定は「未ログイン」。
  // 認証が要るテストは server.use() で 200 に差し替える。
  http.get('/api/v1/auth/me', () =>
    HttpResponse.json(
      {
        type: 'https://menu-planner.example.com/probs/token-invalid',
        title: '認証が必要です',
        status: 401,
      },
      { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
    ),
  ),
]
