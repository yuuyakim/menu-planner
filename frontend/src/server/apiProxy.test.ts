import { afterEach, describe, expect, it, vi } from 'vitest'

import { proxyApiRequest } from './apiProxy'

const backend = 'https://backend.example'

// stubFetch は fetch を差し替え、呼び出し引数を検査できるモックを返す。
// 引数の型を明示するのは、mock.calls[n][1] を型付きで取り出すため。
function stubFetch(response = new Response('ok', { status: 200 })) {
  const spy = vi.fn(
    async (_input: RequestInfo | URL, _init?: RequestInit) => response,
  )
  vi.stubGlobal('fetch', spy)
  return spy
}

describe('proxyApiRequest', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('パスとクエリを保ったまま backend に転送する', async () => {
    const fetchSpy = stubFetch()
    const req = new Request(
      'https://pages.example/api/v1/menus/suggest?genre=japanese',
    )

    await proxyApiRequest(req, backend, '203.0.113.7')

    const [url, init] = fetchSpy.mock.calls[0]
    expect(url).toBe('https://backend.example/api/v1/menus/suggest?genre=japanese')
    expect(init?.method).toBe('GET')
  })

  it('実クライアントIPを X-Forwarded-For に載せる', async () => {
    const fetchSpy = stubFetch()
    const req = new Request('https://pages.example/api/v1/menus/suggest')

    await proxyApiRequest(req, backend, '203.0.113.7')

    const init = fetchSpy.mock.calls[0][1]!
    const headers = new Headers(init.headers)
    expect(headers.get('x-forwarded-for')).toBe('203.0.113.7')
  })

  it('Host ヘッダは転送しない（Cloud Run のルーティングを壊さないため）', async () => {
    const fetchSpy = stubFetch()
    const req = new Request('https://pages.example/api/v1/menus/suggest', {
      headers: { host: 'pages.example' },
    })

    await proxyApiRequest(req, backend, '203.0.113.7')

    const headers = new Headers(fetchSpy.mock.calls[0][1]!.headers)
    expect(headers.get('host')).toBeNull()
  })

  it('Cookie ヘッダは透過する（認証を通すため）', async () => {
    const fetchSpy = stubFetch()
    const req = new Request('https://pages.example/api/v1/auth/me', {
      headers: { cookie: 'access_token=abc' },
    })

    await proxyApiRequest(req, backend, '203.0.113.7')

    const headers = new Headers(fetchSpy.mock.calls[0][1]!.headers)
    expect(headers.get('cookie')).toBe('access_token=abc')
  })

  it('POST の本文を転送する', async () => {
    const fetchSpy = stubFetch()
    const req = new Request('https://pages.example/api/v1/menus/suggest-weekly', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ genre: 'japanese' }),
    })

    await proxyApiRequest(req, backend, '203.0.113.7')

    const init = fetchSpy.mock.calls[0][1]!
    expect(init.method).toBe('POST')
    const forwarded = new Response(init.body as BodyInit)
    expect(await forwarded.text()).toBe(JSON.stringify({ genre: 'japanese' }))
  })

  it('リダイレクトは追わずそのまま返す（OAuth の 302 をブラウザへ）', async () => {
    const fetchSpy = stubFetch()
    const req = new Request('https://pages.example/api/v1/auth/google')

    await proxyApiRequest(req, backend, '203.0.113.7')

    expect(fetchSpy.mock.calls[0][1]?.redirect).toBe('manual')
  })

  it('backend の応答（Set-Cookie 含む）をそのまま返す', async () => {
    const backendRes = new Response('{"ok":true}', {
      status: 200,
      headers: { 'set-cookie': 'access_token=xyz; HttpOnly' },
    })
    stubFetch(backendRes)
    const req = new Request('https://pages.example/api/v1/auth/login', {
      method: 'POST',
    })

    const res = await proxyApiRequest(req, backend, '203.0.113.7')

    expect(res.status).toBe(200)
    expect(res.headers.get('set-cookie')).toBe('access_token=xyz; HttpOnly')
  })

  it('共有シークレットを X-Proxy-Secret として送る（転送IPを信頼させるため）', async () => {
    const fetchSpy = stubFetch()
    const req = new Request('https://pages.example/api/v1/menus/suggest')

    await proxyApiRequest(req, backend, '203.0.113.7', 'shhh')

    const headers = new Headers(fetchSpy.mock.calls[0][1]!.headers)
    expect(headers.get('x-proxy-secret')).toBe('shhh')
  })

  it('シークレット未設定なら X-Proxy-Secret を送らない', async () => {
    const fetchSpy = stubFetch()
    const req = new Request('https://pages.example/api/v1/menus/suggest')

    await proxyApiRequest(req, backend, '203.0.113.7')

    const headers = new Headers(fetchSpy.mock.calls[0][1]!.headers)
    expect(headers.get('x-proxy-secret')).toBeNull()
  })

  it('クライアントが送ってきた X-Proxy-Secret は握り潰す（詐称防止）', async () => {
    const fetchSpy = stubFetch()
    const req = new Request('https://pages.example/api/v1/menus/suggest', {
      headers: { 'x-proxy-secret': 'attacker-guess' },
    })

    await proxyApiRequest(req, backend, '203.0.113.7')

    const headers = new Headers(fetchSpy.mock.calls[0][1]!.headers)
    expect(headers.get('x-proxy-secret')).toBeNull()
  })

  it('backend のURL末尾スラッシュを二重にしない', async () => {
    const fetchSpy = stubFetch()
    const req = new Request('https://pages.example/api/v1/menus/suggest')

    await proxyApiRequest(req, 'https://backend.example/', null)

    expect(fetchSpy.mock.calls[0][0]).toBe(
      'https://backend.example/api/v1/menus/suggest',
    )
  })
})
