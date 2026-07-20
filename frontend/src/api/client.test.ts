import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { ApiError, NetworkError, apiDelete, apiGet, apiPost } from './client'
import { server } from '../test/server'

// rejectedWith は「指定した型で失敗すること」を確かめ、その値を返す。
// .catch((e) => e) だと unknown のままで status などを読めないため、
// ここで型を絞ってから各テストが中身を検証する。
async function rejectedWith<T>(
  promise: Promise<unknown>,
  ctor: new (...args: never[]) => T,
): Promise<T> {
  try {
    await promise
  } catch (e) {
    if (e instanceof ctor) return e
    throw e
  }
  throw new Error('失敗するはずが成功した')
}

// problem は RFC 7807 の応答を組み立てる。
function problem(status: number, title: string, detail?: string) {
  return HttpResponse.json(
    {
      type: `https://menu-planner.example.com/probs/x`,
      title,
      status,
      ...(detail ? { detail } : {}),
    },
    { status, headers: { 'Content-Type': 'application/problem+json' } },
  )
}

describe('APIクライアント', () => {
  it('GET で本文を取得できる', async () => {
    server.use(
      http.get('/api/v1/menus/suggest', () =>
        HttpResponse.json({ menu: { name: '親子丼' } }),
      ),
    )

    const data = await apiGet<{ menu: { name: string } }>('/menus/suggest')
    expect(data.menu.name).toBe('親子丼')
  })

  it('Cookie を送る設定でリクエストする', async () => {
    let credentials: RequestCredentials | undefined
    server.use(
      http.get('/api/v1/auth/me', ({ request }) => {
        credentials = request.credentials
        return HttpResponse.json({ user: { id: 'u1' } })
      }),
    )

    await apiGet('/auth/me')

    // 認証は HttpOnly Cookie なので、include でないと一切認証が通らない。
    expect(credentials).toBe('include')
  })

  it('POST は JSON として本文を送る', async () => {
    let body: unknown
    let contentType: string | null = null
    server.use(
      http.post('/api/v1/favorites', async ({ request }) => {
        contentType = request.headers.get('Content-Type')
        body = await request.json()
        return HttpResponse.json({ menuId: 'm1' }, { status: 201 })
      }),
    )

    await apiPost('/favorites', { menuId: 'm1' })

    expect(contentType).toBe('application/json')
    expect(body).toEqual({ menuId: 'm1' })
  })

  it('204 は本文なしで解決する', async () => {
    server.use(
      http.delete(
        '/api/v1/favorites/m1',
        () => new HttpResponse(null, { status: 204 }),
      ),
    )

    await expect(apiDelete('/favorites/m1')).resolves.toBeUndefined()
  })

  it('problem+json のエラーを解釈する', async () => {
    server.use(
      http.post('/api/v1/favorites', () =>
        problem(409, 'この献立は既にお気に入りに登録されています', '重複'),
      ),
    )

    const err = await rejectedWith(
      apiPost('/favorites', { menuId: 'm1' }),
      ApiError,
    )

    expect(err.status).toBe(409)
    expect(err.title).toBe('この献立は既にお気に入りに登録されています')
    expect(err.detail).toBe('重複')
    // メッセージには利用者に見せられる文言を入れる。
    expect(err.message).toContain('この献立は既にお気に入りに登録されています')
  })

  it('problem+json でない失敗もステータスは保つ', async () => {
    server.use(
      http.get(
        '/api/v1/menus/suggest',
        () => new HttpResponse('<html>502</html>', { status: 502 }),
      ),
    )

    const err = await rejectedWith(apiGet('/menus/suggest'), ApiError)

    expect(err.status).toBe(502)
    // 本文を解釈できなくても、画面が出せる文言は必ず持たせる。
    expect(err.message).not.toBe('')
  })

  it('401 かどうかを判定できる', async () => {
    server.use(
      http.get('/api/v1/auth/me', () => problem(401, '認証が必要です')),
    )

    const err = await rejectedWith(apiGet('/auth/me'), ApiError)

    expect(err.isUnauthorized).toBe(true)
  })

  it('ネットワークエラーは NetworkError になる', async () => {
    server.use(http.get('/api/v1/menus/suggest', () => HttpResponse.error()))

    // 通信自体が届かなかった場合はステータスが無い。
    // ApiError と混ぜると「401だからログインへ」のような分岐が壊れる。
    const err = await rejectedWith(apiGet('/menus/suggest'), NetworkError)

    expect(err).not.toBeInstanceOf(ApiError)
    expect(err.message).not.toBe('')
  })

  it('本文が壊れた 200 もエラーになる', async () => {
    server.use(
      http.get('/api/v1/menus/suggest', () =>
        HttpResponse.text('not json', {
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )

    await expect(apiGet('/menus/suggest')).rejects.toBeInstanceOf(NetworkError)
  })
})
