import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { server } from './server'

// テスト基盤そのものの確認。これが緑なら jsdom / MSW / 後片付けが効いている。
describe('テスト基盤', () => {
  it('jsdom の DOM が使える', () => {
    document.body.innerHTML = '<p id="greeting">こんにちは</p>'
    expect(document.querySelector('#greeting')).toHaveTextContent('こんにちは')
  })

  it('MSW が fetch を横取りする', async () => {
    const res = await fetch('/api/v1/health')
    expect(res.status).toBe(200)
    await expect(res.json()).resolves.toEqual({ status: 'ok' })
  })

  it('テストごとにハンドラを上書きできる', async () => {
    server.use(
      http.get('/api/v1/health', () =>
        HttpResponse.json({ status: 'degraded' }, { status: 503 }),
      ),
    )

    const res = await fetch('/api/v1/health')
    expect(res.status).toBe(503)
  })

  it('上書きしたハンドラは次のテストに持ち越さない', async () => {
    // 直前のテストで 503 に差し替えたが、resetHandlers で戻っているはず。
    const res = await fetch('/api/v1/health')
    expect(res.status).toBe(200)
  })
})
