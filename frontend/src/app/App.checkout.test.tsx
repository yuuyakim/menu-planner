import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../test/render'
import { server } from '../test/server'
import { App } from './App'

const me = {
  id: '018f0000-0000-7000-8000-000000000001',
  email: 'user@example.com',
  displayName: 'ユーザー',
  plan: 'free',
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

describe('/checkout と /checkout/complete', () => {
  it('未認証だとログインへ誘導する', async () => {
    loggedOut()
    renderWithProviders(<App />, { route: '/checkout' })

    expect(
      await screen.findByRole('heading', { level: 1, name: 'ログイン' }),
    ).toBeVisible()
  })

  it('未認証だと /checkout/complete もログインへ誘導する', async () => {
    loggedOut()
    renderWithProviders(<App />, { route: '/checkout/complete' })

    expect(
      await screen.findByRole('heading', { level: 1, name: 'ログイン' }),
    ).toBeVisible()
  })

  it('認証済みなら申込確認画面を開ける', async () => {
    loggedIn()
    server.use(
      http.get('/api/v1/billing/preview', () =>
        HttpResponse.json({
          price: 300,
          currency: 'JPY',
          trialDays: 5,
          trialEligible: true,
          firstBillingAt: '2026-08-01T01:00:00Z',
          planManagementPath: 'アカウント設定 > プランの管理',
        }),
      ),
    )
    renderWithProviders(<App />, { route: '/checkout' })

    expect(
      await screen.findByRole('heading', { level: 1, name: 'お申込み内容の確認' }),
    ).toBeVisible()
  })

  it('認証済みなら復帰画面を開ける', async () => {
    loggedIn()
    renderWithProviders(<App />, { route: '/checkout/complete' })

    expect(
      await screen.findByRole('heading', { level: 1, name: 'お手続きを受け付けました' }),
    ).toBeVisible()
  })
})
