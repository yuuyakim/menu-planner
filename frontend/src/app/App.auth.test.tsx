import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'

import { renderWithProviders } from '../test/render'
import { server } from '../test/server'
import { App } from './App'

const me = {
  id: '018f0000-0000-7000-8000-000000000001',
  email: 'user@example.com',
  displayName: 'キムさん',
}

// loggedIn は認証済みの状態にする。
function loggedIn() {
  server.use(http.get('/api/v1/auth/me', () => HttpResponse.json({ user: me })))
}

// loggedOut は未認証の状態にする（サーバは 401 を返す）。
function loggedOut() {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json(
        { type: 'https://example.com/probs/x', title: '認証が必要です', status: 401 },
        { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
      ),
    ),
  )
}

describe('未認証のとき', () => {
  it('検索画面は使える', async () => {
    loggedOut()
    renderWithProviders(<App />, { route: '/search' })

    expect(
      await screen.findByRole('heading', { level: 1, name: '献立を探す' }),
    ).toBeVisible()
    expect(screen.getByRole('button', { name: '献立を探す' })).toBeEnabled()
  })

  // 週間献立は premium 限定（Task 7）。未ログインは生成画面ではなくロックへ誘導する。
  //
  // 名前は完全一致で 'ログインする'（PremiumLock 自身のCTA）に絞る。正規表現
  // /ログイン/ だとヘッダの「ログイン」導線にもマッチし、両方が同時に存在する
  // ため「複数要素にマッチした」エラーになる（PremiumLock のローディング委譲化
  // でヘッダとほぼ同時に描画されるようになったため、以前は稀にしか露呈しなかった）。
  it('週間献立はロックへ誘導する', async () => {
    loggedOut()
    renderWithProviders(<App />, { route: '/weekly' })

    expect(
      await screen.findByRole('link', { name: 'ログインする' }),
    ).toBeVisible()
    expect(
      screen.queryByRole('heading', { level: 1, name: '1週間の献立' }),
    ).not.toBeInTheDocument()
  })

  it('履歴画面はログインへ誘導する', async () => {
    loggedOut()
    renderWithProviders(<App />, { route: '/histories' })

    expect(
      await screen.findByRole('heading', { level: 1, name: 'ログイン' }),
    ).toBeVisible()
    expect(
      screen.queryByRole('heading', { name: '検索履歴' }),
    ).not.toBeInTheDocument()
  })

  it('お気に入り画面もログインへ誘導する', async () => {
    loggedOut()
    renderWithProviders(<App />, { route: '/favorites' })

    expect(
      await screen.findByRole('heading', { level: 1, name: 'ログイン' }),
    ).toBeVisible()
  })

  it('ログイン後は行こうとしていた画面に着く', async () => {
    loggedOut()
    renderWithProviders(<App />, { route: '/histories' })
    await screen.findByRole('heading', { level: 1, name: 'ログイン' })

    // ログインが成功する状況にする。
    server.use(
      http.post('/api/v1/auth/login', () => HttpResponse.json({ user: me })),
    )
    const u = userEvent.setup()
    await u.type(screen.getByLabelText('メールアドレス'), 'user@example.com')
    await u.type(screen.getByLabelText('パスワード'), 'password123')
    await u.click(screen.getByRole('button', { name: 'ログイン' }))

    // 検索画面ではなく、もともと開こうとした履歴画面へ。
    expect(
      await screen.findByRole('heading', { level: 1, name: '検索履歴' }),
    ).toBeVisible()
  })

  it('ヘッダにログインへの導線を出す', async () => {
    loggedOut()
    renderWithProviders(<App />, { route: '/' })

    const header = within(screen.getByRole('banner'))
    expect(await header.findByRole('link', { name: 'ログイン' })).toBeVisible()
    expect(
      header.queryByRole('button', { name: 'ログアウト' }),
    ).not.toBeInTheDocument()
  })
})

describe('認証済みのとき', () => {
  it('ヘッダに表示名とログアウトを出す', async () => {
    loggedIn()
    renderWithProviders(<App />, { route: '/' })

    const header = within(screen.getByRole('banner'))
    expect(await header.findByText('キムさん')).toBeVisible()
    expect(header.getByRole('button', { name: 'ログアウト' })).toBeVisible()
    expect(header.queryByRole('link', { name: 'ログイン' })).not.toBeInTheDocument()
  })

  it('履歴画面を開ける', async () => {
    loggedIn()
    renderWithProviders(<App />, { route: '/histories' })

    expect(
      await screen.findByRole('heading', { level: 1, name: '検索履歴' }),
    ).toBeVisible()
  })

  it('ログアウトすると未認証の表示に戻る', async () => {
    loggedIn()
    let loggedOutCalled = false
    server.use(
      http.post('/api/v1/auth/logout', () => {
        loggedOutCalled = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderWithProviders(<App />, { route: '/' })

    const header = within(screen.getByRole('banner'))
    await header.findByText('キムさん')

    // ログアウト後は 401 が返る状態になる。
    loggedOut()
    const u = userEvent.setup()
    await u.click(header.getByRole('button', { name: 'ログアウト' }))

    await waitFor(() => expect(loggedOutCalled).toBe(true))
    expect(await header.findByRole('link', { name: 'ログイン' })).toBeVisible()
    expect(header.queryByText('キムさん')).not.toBeInTheDocument()
  })

  it('ログアウト後は履歴画面がログインへ誘導される', async () => {
    loggedIn()
    server.use(
      http.post(
        '/api/v1/auth/logout',
        () => new HttpResponse(null, { status: 204 }),
      ),
      // ログアウトでキャッシュを丸ごと初期化するため（Issue #78）、
      // 誘導される前に履歴が一度だけ取り直される。
      http.get('/api/v1/histories', () => HttpResponse.json({ histories: [] })),
    )
    renderWithProviders(<App />, { route: '/histories' })
    await screen.findByRole('heading', { level: 1, name: '検索履歴' })

    loggedOut()
    const u = userEvent.setup()
    await u.click(
      within(screen.getByRole('banner')).getByRole('button', {
        name: 'ログアウト',
      }),
    )

    // 見ていた画面が守られた画面なら、その場でログインへ誘導する。
    expect(
      await screen.findByRole('heading', { level: 1, name: 'ログイン' }),
    ).toBeVisible()
  })

  it('ログアウトしたことを言葉で伝える', async () => {
    loggedIn()
    server.use(
      http.post(
        '/api/v1/auth/logout',
        () => new HttpResponse(null, { status: 204 }),
      ),
    )
    // 検索画面はログイン有無で見た目が変わらない。
    // ヘッダの表示だけでは「ログアウトできたのか」が分かりにくい。
    renderWithProviders(<App />, { route: '/' })

    const header = within(screen.getByRole('banner'))
    await header.findByText('キムさん')

    loggedOut()
    const u = userEvent.setup()
    await u.click(header.getByRole('button', { name: 'ログアウト' }))

    expect(await screen.findByRole('status')).toHaveTextContent(
      'ログアウトしました',
    )
  })

  it('ログアウトの知らせはしばらくして消える', async () => {
    // shouldAdvanceTime を付けないと MSW の通信が偽タイマーで止まり、
    // 応答が返らないままテストが時間切れになる。
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      loggedIn()
      server.use(
        http.post(
          '/api/v1/auth/logout',
          () => new HttpResponse(null, { status: 204 }),
        ),
      )
      const u = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
      renderWithProviders(<App />, { route: '/' })

      const header = within(screen.getByRole('banner'))
      await vi.waitFor(() => header.getByText('キムさん'))

      loggedOut()
      await u.click(header.getByRole('button', { name: 'ログアウト' }))
      await vi.waitFor(() =>
        expect(screen.getByRole('status')).toHaveTextContent('ログアウトしました'),
      )

      // 出しっぱなしにすると、次の操作のときに古い知らせが残って紛らわしい。
      // タイマーで起きる状態更新なので act で包んで反映させる。
      await act(async () => {
        await vi.advanceTimersByTimeAsync(6000)
      })
      expect(screen.queryByRole('status')).not.toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })
})
