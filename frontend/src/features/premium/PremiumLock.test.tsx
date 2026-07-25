import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { PremiumLock } from './PremiumLock'

// respondMe は現在のユーザーの応答を仕込む。プランだけを差し替える
// （AuthMenu.test.tsx / ShoppingListPage.test.tsx と同じ流儀）。
// 未ログインは test/handlers.ts の既定（401）に任せるため、ここでは呼ばない。
function respondMe(plan: 'free' | 'premium') {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000001',
          email: 'user@example.com',
          displayName: 'ユーザー',
          plan,
        },
      }),
    ),
  )
}

describe('PremiumLock', () => {
  it('未ログインはログイン導線を出す', async () => {
    renderWithProviders(
      <PremiumLock title="1週間まとめて計画" description="1週間分の献立をまとめて計画できます。" />,
    )

    expect(
      await screen.findByRole('link', { name: /ログイン/ }),
    ).toBeInTheDocument()
  })

  it('ログイン済み free はアップグレード導線を出す', async () => {
    respondMe('free')
    renderWithProviders(
      <PremiumLock title="1週間まとめて計画" description="1週間分の献立をまとめて計画できます。" />,
    )

    expect(await screen.findByText(/プレミアム/)).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /ログイン/ }),
    ).not.toBeInTheDocument()
  })

  it('判定が付くまではローディング表示を出す', () => {
    renderWithProviders(
      <PremiumLock title="1週間まとめて計画" description="1週間分の献立をまとめて計画できます。" />,
    )

    expect(screen.getByRole('status')).toBeInTheDocument()
  })
})
