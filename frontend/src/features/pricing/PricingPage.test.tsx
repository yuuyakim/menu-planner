import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { PricingPage } from './PricingPage'

function respondMe(plan: 'free' | 'premium') {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000002',
          email: 'user@example.com',
          displayName: 'ユーザー',
          plan,
        },
      }),
    ),
  )
}

describe('料金プラン画面', () => {
  it('未ログインでも料金と比較表が見える', async () => {
    renderWithProviders(<PricingPage />)

    expect(await screen.findByText(/月額300円/)).toBeInTheDocument()
    expect(screen.getByText('1週間の献立を組み立てる')).toBeInTheDocument()
  })

  it('未ログインには加入画面への CTA を出す', async () => {
    renderWithProviders(<PricingPage />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムを試す' }),
    ).toHaveAttribute('href', '/checkout')
  })

  // premium が /checkout を踏むと already-subscribed（409）で行き止まりになる。
  // AccountPage が同じ配慮をしているのに揃える。
  it('premium にはプラン管理への CTA を出し、加入画面へは送らない', async () => {
    respondMe('premium')
    renderWithProviders(<PricingPage />)

    expect(
      await screen.findByRole('link', { name: 'プランを管理する' }),
    ).toHaveAttribute('href', '/account')
    expect(
      screen.queryByRole('link', { name: 'プレミアムを試す' }),
    ).not.toBeInTheDocument()
  })

  it('free には加入画面への CTA を出す', async () => {
    respondMe('free')
    renderWithProviders(<PricingPage />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムを試す' }),
    ).toHaveAttribute('href', '/checkout')
  })

  // 先に free 向けの CTA を描いてから差し替えると、premium の利用者に
  // 一瞬「プレミアムを試す」が見える。比較表は状態に依らないので先に出す。
  it('加入状態の判定中は CTA を出さないが、比較表は出す', () => {
    renderWithProviders(<PricingPage />)

    expect(
      screen.queryByRole('link', { name: 'プレミアムを試す' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'プランを管理する' }),
    ).not.toBeInTheDocument()
    expect(screen.getByText('1週間の献立を組み立てる')).toBeInTheDocument()
  })

  // 上限の数値をフロントが持つと二重管理になる（spec.md「上限の数値を返さない理由」）。
  it('保存件数の数値を表示しない', async () => {
    respondMe('free')
    renderWithProviders(<PricingPage />)

    await screen.findByRole('link', { name: 'プレミアムを試す' })
    expect(screen.queryByText(/50/)).not.toBeInTheDocument()
  })

  it('特定商取引法に基づく表記へのリンクを添える', async () => {
    renderWithProviders(<PricingPage />)

    expect(
      await screen.findByRole('link', { name: /特定商取引法/ }),
    ).toHaveAttribute('href', '/legal/tokushoho')
  })
})
