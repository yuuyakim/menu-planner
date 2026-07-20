import { http, HttpResponse } from 'msw'

// 既定のハンドラ。全テストで共通に使える最小限だけを置き、
// 個別の応答は各テストで server.use() を呼んで上書きする。
export const handlers = [
  http.get('/api/v1/health', () => HttpResponse.json({ status: 'ok' })),
]
