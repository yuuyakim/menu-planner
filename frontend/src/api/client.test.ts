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

  // アクセストークンは15分で切れるが、リフレッシュトークンは30日有効。
  // 401 を受けたら黙って再発行を試み、利用者がログインし直さずに済むようにする。
  describe('401 を受けたときのセッション再発行', () => {
    it('再発行に成功したら元のリクエストをやり直して結果を返す', async () => {
      let expired = true
      server.use(
        http.get('/api/v1/auth/me', () => {
          if (expired) return problem(401, '認証が必要です')
          return HttpResponse.json({ user: { id: 'u1' } })
        }),
        http.post('/api/v1/auth/refresh', () => {
          expired = false
          return new HttpResponse(null, { status: 204 })
        }),
      )

      const data = await apiGet<{ user: { id: string } }>('/auth/me')

      expect(data.user.id).toBe('u1')
    })

    it('再発行にも失敗したら 401 のままにする', async () => {
      server.use(
        http.get('/api/v1/auth/me', () => problem(401, '認証が必要です')),
        http.post('/api/v1/auth/refresh', () => problem(401, '認証が必要です')),
      )

      const err = await rejectedWith(apiGet('/auth/me'), ApiError)

      expect(err.status).toBe(401)
    })

    it('やり直しは一度だけ（再発行後も 401 なら諦める）', async () => {
      let calls = 0
      server.use(
        http.get('/api/v1/auth/me', () => {
          calls += 1
          return problem(401, '認証が必要です')
        }),
        http.post(
          '/api/v1/auth/refresh',
          () => new HttpResponse(null, { status: 204 }),
        ),
      )

      await rejectedWith(apiGet('/auth/me'), ApiError)

      // 初回とやり直しの2回で打ち切る。無限に往復させない。
      expect(calls).toBe(2)
    })

    it('同時に 401 になっても再発行は1回にまとめる', async () => {
      let refreshes = 0
      let expired = true
      server.use(
        http.get('/api/v1/histories', () => {
          if (expired) return problem(401, '認証が必要です')
          return HttpResponse.json({ histories: [] })
        }),
        http.get('/api/v1/favorites', () => {
          if (expired) return problem(401, '認証が必要です')
          return HttpResponse.json({ favorites: [] })
        }),
        http.post('/api/v1/auth/refresh', () => {
          refreshes += 1
          expired = false
          return new HttpResponse(null, { status: 204 })
        }),
      )

      await Promise.all([apiGet('/histories'), apiGet('/favorites')])

      // 画面が複数のAPIを並べて呼ぶのは普通なので、まとめないと
      // 同じ再発行が同時に何本も飛ぶ。
      expect(refreshes).toBe(1)
    })

    it('ログイン自体の 401 では再発行しない', async () => {
      let refreshes = 0
      server.use(
        http.post('/api/v1/auth/login', () =>
          problem(401, 'メールアドレスまたはパスワードが違います'),
        ),
        http.post('/api/v1/auth/refresh', () => {
          refreshes += 1
          return new HttpResponse(null, { status: 204 })
        }),
      )

      const err = await rejectedWith(
        apiPost('/auth/login', { email: 'a@example.com', password: 'x' }),
        ApiError,
      )

      // 資格情報の誤りに再発行を試みても無意味で、
      // 再発行が成功する状況では往復が止まらなくなる。
      expect(err.status).toBe(401)
      expect(refreshes).toBe(0)
    })
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
