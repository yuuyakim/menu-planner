import { http, HttpResponse } from 'msw'

// 既定のハンドラ。全テストで共通に使える最小限だけを置き、
// 個別の応答は各テストで server.use() を呼んで上書きする。
export const handlers = [
  http.get('/api/v1/health', () => HttpResponse.json({ status: 'ok' })),

  // 食材は献立を出す画面すべて（検索・詳細）が引く。既定を置かないと
  // 個々のテストが未処理リクエストで落ちる。中身を見たいテストは use で上書きする。
  http.get('/api/v1/menus/:id/ingredients', () =>
    HttpResponse.json({ ingredients: [] }),
  ),

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

  // 401 を受けたクライアントはセッションの再発行を試みる。
  // 既定は「未ログイン」に揃えて失敗させる。これが無いと再発行の試行が
  // 未定義リクエストとして通信エラー扱いになり、本番の挙動とズレる。
  http.post('/api/v1/auth/refresh', () =>
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
