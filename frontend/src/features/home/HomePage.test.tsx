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

  it('こんたてんが声をかけてくる', async () => {
    renderWithProviders(<HomePage />)

    // 吹き出しの文言は画像に焼き込まず本文として置く。
    // 翻訳・読み上げ・拡大のどれもテキストの方が素直に効く。
    //
    // 文言は折り返し位置を制御するため文節ごとの span に分かれている。
    // 既定の照合は要素の直下テキストしか見ないので、段落全体の textContent で照合する。
    expect(
      await screen.findByText(
        (_, el) =>
          el?.tagName === 'P' &&
          el.textContent === '今日のごはん、一緒に考えよっ！',
      ),
    ).toBeVisible()
  })

  it('マスコットは装飾として扱い、読み上げの邪魔をしない', async () => {
    renderWithProviders(<HomePage />)

    await screen.findByRole('link', { name: /献立を探す/ })

    // 入口リンクの読み上げ名は「献立を探す」＋説明であってほしい。
    // 画像に alt を付けるとそこへキャラクターの説明が割り込む。
    expect(screen.queryAllByRole('img')).toHaveLength(0)
  })

  it('ログイン中は要ログインの断りを出さない', async () => {
    loggedIn()
    renderWithProviders(<HomePage />)

    await screen.findByText(/キムさん/)
    await waitFor(() =>
      expect(screen.queryByText(/ログインが必要/)).not.toBeInTheDocument(),
    )
  })

  it('free には料金プランへの案内を出す', async () => {
    server.use(
      http.get('/api/v1/auth/me', () =>
        HttpResponse.json({
          user: {
            id: '018f0000-0000-7000-8000-000000000009',
            email: 'user@example.com',
            displayName: 'キムさん',
            plan: 'free',
          },
        }),
      ),
    )
    renderWithProviders(<HomePage />)

    expect(
      await screen.findByRole('link', { name: 'プランを見る' }),
    ).toHaveAttribute('href', '/pricing')
  })

  it('未ログインにも料金プランへの案内を出す', async () => {
    renderWithProviders(<HomePage />)

    expect(
      await screen.findByRole('link', { name: 'プランを見る' }),
    ).toHaveAttribute('href', '/pricing')
  })

  // 加入済みの利用者に勧誘を出さない。
  it('premium には料金プランへの案内を出さない', async () => {
    server.use(
      http.get('/api/v1/auth/me', () =>
        HttpResponse.json({
          user: {
            id: '018f0000-0000-7000-8000-000000000009',
            email: 'user@example.com',
            displayName: 'キムさん',
            plan: 'premium',
          },
        }),
      ),
    )
    renderWithProviders(<HomePage />)

    // 先に判定が付いたことを確かめてから「無いこと」を検査する。
    // 部分一致にするのは既存テスト（:49）と同じ理由で、文言の細部に縛られないため。
    expect(await screen.findByText(/キムさん/)).toBeVisible()
    expect(
      screen.queryByRole('link', { name: 'プランを見る' }),
    ).not.toBeInTheDocument()
  })

  // 判定前に出すと、premium の利用者に一瞬勧誘が見える。
  it('判定が付くまでは案内を出さない', () => {
    renderWithProviders(<HomePage />)

    expect(
      screen.queryByRole('link', { name: 'プランを見る' }),
    ).not.toBeInTheDocument()
  })
})
