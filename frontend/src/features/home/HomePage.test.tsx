import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { HomePage } from './HomePage'

function loggedIn() {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000009',
          email: 'user@example.com',
          displayName: 'キムさん',
        },
      }),
    ),
  )
}

describe('ホーム画面', () => {
  it('各機能への入口が並ぶ', async () => {
    loggedIn()
    renderWithProviders(<HomePage />)

    expect(
      await screen.findByRole('link', { name: /献立を探す/ }),
    ).toHaveAttribute('href', '/search')
    expect(screen.getByRole('link', { name: /1週間の献立/ })).toHaveAttribute(
      'href',
      '/weekly',
    )
    expect(screen.getByRole('link', { name: /検索履歴/ })).toHaveAttribute(
      'href',
      '/histories',
    )
    expect(screen.getByRole('link', { name: /お気に入り/ })).toHaveAttribute(
      'href',
      '/favorites',
    )
  })

  it('ログイン中は誰として使っているかが分かる', async () => {
    loggedIn()
    renderWithProviders(<HomePage />)

    expect(await screen.findByText(/キムさん/)).toBeVisible()
    // ログイン中にログインを勧めない。
    expect(
      screen.queryByRole('link', { name: 'ログイン' }),
    ).not.toBeInTheDocument()
  })

  it('未ログインならログインの導線を出す', async () => {
    // 既定のハンドラは未ログイン(401)。
    renderWithProviders(<HomePage />)

    expect(await screen.findByRole('link', { name: 'ログイン' })).toHaveAttribute(
      'href',
      '/login',
    )
  })

  it('未ログインでも検索と週間献立は入口を出す', async () => {
    renderWithProviders(<HomePage />)

    // 未認証でも使える機能（spec.md 1.3）。
    expect(
      await screen.findByRole('link', { name: /献立を探す/ }),
    ).toBeVisible()
    expect(screen.getByRole('link', { name: /1週間の献立/ })).toBeVisible()
  })

  it('未ログインでは履歴とお気に入りに要ログインと添える', async () => {
    renderWithProviders(<HomePage />)

    const history = await screen.findByRole('link', { name: /検索履歴/ })
    // 入口自体はすぐ描画されるが、断りはログイン判定が付いてから出る。
    await waitFor(() =>
      expect(history).toHaveTextContent('ログインが必要'),
    )
    expect(screen.getByRole('link', { name: /お気に入り/ })).toHaveTextContent(
      'ログインが必要',
    )
  })

  it('ログイン中は要ログインの断りを出さない', async () => {
    loggedIn()
    renderWithProviders(<HomePage />)

    await screen.findByText(/キムさん/)
    await waitFor(() =>
      expect(screen.queryByText(/ログインが必要/)).not.toBeInTheDocument(),
    )
  })
})
