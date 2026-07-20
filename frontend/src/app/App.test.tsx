import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { http, HttpResponse } from 'msw'

import { renderWithProviders } from '../test/render'
import { server } from '../test/server'
import { App } from './App'

// 履歴とお気に入りは認証必須になった（8-I）。
// ここで見たいのは遷移そのものなので、認証済みの状態にしておく。
function loggedIn() {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000001',
          email: 'user@example.com',
          displayName: 'ユーザー',
        },
      }),
    ),
  )
}

// ルーティングの骨組みの確認。各画面の中身は後続のPRで作る。
describe('App', () => {
  it('/ はホーム画面を表示する', async () => {
    renderWithProviders(<App />, { route: '/' })
    expect(
      await screen.findByRole('heading', { level: 1, name: /献立プランナー/ }),
    ).toBeVisible()
  })

  it('/search で検索画面を表示する', () => {
    renderWithProviders(<App />, { route: '/search' })
    expect(
      screen.getByRole('heading', { level: 1, name: '献立を探す' }),
    ).toBeVisible()
  })

  it('ホームから検索画面へ遷移できる', async () => {
    const user = userEvent.setup()
    renderWithProviders(<App />, { route: '/' })

    // ヘッダとホーム本文の両方に導線があるため、本文側を選ぶ。
    const main = within(screen.getByRole('main'))
    await user.click(await main.findByRole('link', { name: /献立を探す/ }))

    expect(
      await screen.findByRole('heading', { level: 1, name: '献立を探す' }),
    ).toBeVisible()
  })

  it('ヘッダのリンクで履歴へ遷移できる', async () => {
    const user = userEvent.setup()
    loggedIn()
    renderWithProviders(<App />, { route: '/' })

    await user.click(screen.getByRole('link', { name: '履歴' }))

    expect(
      await screen.findByRole('heading', { level: 1, name: '検索履歴' }),
    ).toBeVisible()
  })

  it('ヘッダのリンクでお気に入りへ遷移できる', async () => {
    const user = userEvent.setup()
    loggedIn()
    renderWithProviders(<App />, { route: '/' })

    await user.click(screen.getByRole('link', { name: 'お気に入り' }))

    expect(
      await screen.findByRole('heading', { level: 1, name: 'お気に入り' }),
    ).toBeVisible()
  })

  it('/login でログイン画面を表示する', () => {
    renderWithProviders(<App />, { route: '/login' })
    expect(
      screen.getByRole('heading', { level: 1, name: 'ログイン' }),
    ).toBeVisible()
  })

  it('未知のパスでは404を表示する', () => {
    renderWithProviders(<App />, { route: '/no-such-page' })
    expect(
      screen.getByRole('heading', { level: 1, name: 'ページが見つかりません' }),
    ).toBeVisible()
  })

  it('どの画面でもヘッダは残る', () => {
    renderWithProviders(<App />, { route: '/no-such-page' })
    expect(screen.getByRole('banner')).toBeVisible()
    expect(screen.getByRole('link', { name: '履歴' })).toBeVisible()
  })
})
