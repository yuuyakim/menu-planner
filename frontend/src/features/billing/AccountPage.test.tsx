import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import type { SubscriptionInfo } from './api'
import { AccountPage } from './AccountPage'

function subscription(overrides: Partial<SubscriptionInfo> = {}): SubscriptionInfo {
  return {
    plan: 'premium',
    status: 'active',
    currentPeriodEnd: '2026-08-01T01:00:00Z',
    cancelAtPeriodEnd: false,
    hasPortal: true,
    ...overrides,
  }
}

function respondSubscription(body: SubscriptionInfo) {
  server.use(
    http.get('/api/v1/billing/subscription', () => HttpResponse.json(body)),
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

describe('アカウント設定 > プランの管理', () => {
  it('有料プランなら次回請求日と「プランを管理する」ボタンを表示する', async () => {
    respondSubscription(subscription())
    renderWithProviders(<AccountPage />)

    expect(await screen.findByText(/次回請求/)).toBeVisible()
    expect(screen.getByText(/2026年8月1日/)).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'プランを管理する' }),
    ).toBeVisible()
  })

  it('「プランを管理する」を押すとポータルセッションを作り、返った url へ遷移する', async () => {
    const user = userEvent.setup()
    respondSubscription(subscription())
    server.use(
      http.post('/api/v1/billing/portal-session', () =>
        HttpResponse.json({ url: 'https://billing.stripe.example.com/session/y' }),
      ),
    )
    renderWithProviders(<AccountPage />)

    await user.click(
      await screen.findByRole('button', { name: 'プランを管理する' }),
    )

    await waitFor(() =>
      expect(window.location.href).toBe(
        'https://billing.stripe.example.com/session/y',
      ),
    )
  })

  it('無料プランなら「プレミアムにアップグレード」から/checkoutへのリンクを表示する', async () => {
    respondSubscription(
      subscription({
        plan: 'free',
        status: 'none',
        currentPeriodEnd: null,
        hasPortal: false,
      }),
    )
    renderWithProviders(<AccountPage />)

    const link = await screen.findByRole('link', {
      name: 'プレミアムにアップグレード',
    })
    expect(link).toHaveAttribute('href', '/checkout')
  })

  it('解約予約中なら期末日までの表示になる', async () => {
    respondSubscription(subscription({ cancelAtPeriodEnd: true }))
    renderWithProviders(<AccountPage />)

    expect(await screen.findByText(/解約予定/)).toBeVisible()
    expect(screen.getByText(/2026年8月1日/)).toBeVisible()
  })

  it('手動付与などポータルが無い有料ステータスはボタンを出さず「まで有効」を表示する（次回請求ではない）', async () => {
    respondSubscription(
      subscription({
        plan: 'premium',
        status: 'active',
        hasPortal: false,
        currentPeriodEnd: '2026-08-01T01:00:00Z',
      }),
    )
    renderWithProviders(<AccountPage />)

    expect(await screen.findByText(/まで有効/)).toBeVisible()
    expect(screen.getByText(/2026年8月1日/)).toBeVisible()
    expect(screen.queryByText(/次回請求/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'プランを管理する' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'プレミアムにアップグレード' }),
    ).not.toBeInTheDocument()
  })

  it('支払い確認中（past_due）でポータルがあれば、カード更新の案内と「プランを管理する」を出し、アップグレード導線は出さない', async () => {
    // plan は 'free' のままでも（例: Webhook 反映前など）status/hasPortal を
    // 優先して表示・導線を決める必要がある。plan 駆動だとここでアップグレード
    // リンクが出て /checkout → ErrAlreadySubscribed の行き止まりになる。
    respondSubscription(
      subscription({
        plan: 'free',
        status: 'past_due',
        hasPortal: true,
        currentPeriodEnd: '2026-08-01T01:00:00Z',
      }),
    )
    renderWithProviders(<AccountPage />)

    expect(
      await screen.findByText(/お支払いの確認中。カード情報の更新をお願いします/),
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'プランを管理する' }),
    ).toBeVisible()
    expect(
      screen.queryByRole('link', { name: 'プレミアムにアップグレード' }),
    ).not.toBeInTheDocument()
  })

  it('取得に失敗したらメッセージを出す', async () => {
    server.use(
      http.get('/api/v1/billing/subscription', () => HttpResponse.error()),
    )
    renderWithProviders(<AccountPage />)

    expect(await screen.findByRole('alert')).toBeVisible()
  })
})
