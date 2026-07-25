import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import type { BillingPreview } from './api'
import { CheckoutPage } from './CheckoutPage'

function preview(overrides: Partial<BillingPreview> = {}): BillingPreview {
  return {
    price: 300,
    currency: 'JPY',
    trialDays: 5,
    trialEligible: true,
    firstBillingAt: '2026-08-01T01:00:00Z',
    planManagementPath: 'アカウント設定 > プランの管理',
    ...overrides,
  }
}

function respondPreview(body: BillingPreview) {
  server.use(
    http.get('/api/v1/billing/preview', () => HttpResponse.json(body)),
  )
}

// window.location.href への代入は jsdom がナビゲーションとして実装していない
// （console にエラーが出るだけで例外にはならない）。差し替えて、遷移先として
// 書き込まれた値を検証できるようにする。href の初期値は元のまま保つ必要が
// ある（相対パスの fetch は window.location を基準に解決されるため、
// 空文字にすると `/api/v1/...` へのリクエスト自体が壊れる）。
const originalLocation = window.location

beforeEach(() => {
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { ...originalLocation },
  })
})

afterEach(() => {
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: originalLocation,
  })
})

describe('申込確認画面', () => {
  it('料金・トライアル・初回課金日時・解約方法・返金・お支払い方法の6項目を表示する', async () => {
    respondPreview(preview())
    renderWithProviders(<CheckoutPage />)

    expect(await screen.findByText(/300円/)).toBeVisible()
    expect(screen.getByText(/5日間無料/)).toBeVisible()
    expect(screen.getByText(/2026年8月1日/)).toBeVisible()
    expect(
      screen.getByText(/アカウント設定 > プランの管理/),
    ).toBeVisible()
    expect(screen.getByText(/返金はできません/)).toBeVisible()
    expect(screen.getByText(/Stripe/)).toBeVisible()

    expect(screen.getByRole('link', { name: '利用規約' })).toHaveAttribute(
      'href',
      '/legal/terms',
    )
    expect(
      screen.getByRole('link', { name: 'プライバシーポリシー' }),
    ).toHaveAttribute('href', '/legal/privacy')
  })

  it('同意するまでボタンは押せない', async () => {
    const user = userEvent.setup()
    respondPreview(preview())
    renderWithProviders(<CheckoutPage />)

    const button = await screen.findByRole('button', {
      name: '無料お試しを開始する',
    })
    expect(button).toBeDisabled()

    await user.click(screen.getByRole('checkbox'))

    expect(button).toBeEnabled()
  })

  it('同意して押すと Checkout の url へ遷移する', async () => {
    const user = userEvent.setup()
    respondPreview(preview())
    server.use(
      http.post('/api/v1/billing/checkout-session', () =>
        HttpResponse.json({ url: 'https://stripe.example.com/session/x' }),
      ),
    )
    renderWithProviders(<CheckoutPage />)

    await user.click(await screen.findByRole('checkbox'))
    await user.click(
      screen.getByRole('button', { name: '無料お試しを開始する' }),
    )

    await waitFor(() =>
      expect(window.location.href).toBe('https://stripe.example.com/session/x'),
    )
  })

  it('Checkout セッションの作成に失敗したらメッセージを出す', async () => {
    const user = userEvent.setup()
    respondPreview(preview())
    server.use(
      http.post('/api/v1/billing/checkout-session', () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/already-subscribed',
            title: 'すでに購読中です',
            status: 409,
          },
          { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    renderWithProviders(<CheckoutPage />)

    await user.click(await screen.findByRole('checkbox'))
    await user.click(
      screen.getByRole('button', { name: '無料お試しを開始する' }),
    )

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'すでに購読中です',
    )
  })

  it('トライアル対象外なら申込時に課金される旨を表示する', async () => {
    respondPreview(preview({ trialEligible: false }))
    renderWithProviders(<CheckoutPage />)

    expect(await screen.findByText(/お申込み時/)).toBeVisible()
    expect(screen.queryByText(/日間無料/)).not.toBeInTheDocument()
  })

  it('取得に失敗したらメッセージを出す', async () => {
    server.use(
      http.get('/api/v1/billing/preview', () => HttpResponse.error()),
    )
    renderWithProviders(<CheckoutPage />)

    expect(await screen.findByRole('alert')).toBeVisible()
  })
})
