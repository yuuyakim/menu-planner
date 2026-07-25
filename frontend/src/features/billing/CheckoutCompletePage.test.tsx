import { screen, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { CheckoutCompletePage } from './CheckoutCompletePage'

const user = {
  id: '018f0000-0000-7000-8000-000000000001',
  email: 'user@example.com',
  displayName: 'ユーザー',
}

// respondMe は /auth/me の応答を仕込む。プランだけを差し替える。
function respondMe(plan: 'free' | 'premium') {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({ user: { ...user, plan } }),
    ),
  )
}

beforeEach(() => {
  // refetchInterval（数秒おきの再取得）を偽タイマーで進めるため。
  // shouldAdvanceTime を付けないと、MSW の通信も偽タイマーで止まってしまい、
  // 応答が返らないままテストが時間切れになる。
  vi.useFakeTimers({ shouldAdvanceTime: true })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('決済復帰画面', () => {
  it('premium 反映後に成功表示になる', async () => {
    respondMe('free')
    renderWithProviders(<CheckoutCompletePage />)

    // 1. 最初は受付表示。
    expect(
      await screen.findByText('お手続きを受け付けました'),
    ).toBeVisible()
    expect(
      screen.queryByText('プレミアムが有効になりました'),
    ).not.toBeInTheDocument()

    // 2. サーバが premium を返すようになった状態で、再取得のタイマーを進める。
    respondMe('premium')
    await vi.advanceTimersByTimeAsync(2000)

    expect(
      await vi.waitFor(() => screen.getByText('プレミアムが有効になりました')),
    ).toBeVisible()
    expect(
      within(screen.getByRole('link', { name: '1週間の献立へ' })),
    ).toBeDefined()
    expect(screen.getByRole('link', { name: '1週間の献立へ' })).toHaveAttribute(
      'href',
      '/weekly',
    )
  })

  it('一定回数たっても premium にならなければ諦めた旨を表示する', async () => {
    respondMe('free')
    renderWithProviders(<CheckoutCompletePage />)

    await screen.findByText('お手続きを受け付けました')

    // 上限（10回）に達するまでポーリングを進める。
    for (let i = 0; i < 10; i += 1) {
      await vi.advanceTimersByTimeAsync(2000)
    }

    expect(
      await vi.waitFor(() =>
        screen.getByText(/反映まで少し時間がかかることがあります/),
      ),
    ).toBeVisible()
  })
})
