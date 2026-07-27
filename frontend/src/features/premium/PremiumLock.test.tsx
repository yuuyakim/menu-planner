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

const props = {
  title: '1週間まとめて計画',
  description: '1週間分の献立をまとめて計画できます。',
}

describe('PremiumLock', () => {
  it('ログイン済み free は加入画面へのリンクを出す', async () => {
    respondMe('free')
    renderWithProviders(<PremiumLock {...props} />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムにアップグレード' }),
    ).toHaveAttribute('href', '/checkout')
  })

  // 未ログインにも同じ導線を出す。/checkout は RequireAuth で守られており、
  // 押すとログイン画面を経て /checkout へ戻る。
  it('未ログインも同じ加入導線と、ログインが要る旨を出す', async () => {
    renderWithProviders(<PremiumLock {...props} />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムにアップグレード' }),
    ).toHaveAttribute('href', '/checkout')
    expect(screen.getByText('ログインが必要です')).toBeInTheDocument()
  })

  it('ログイン済み free には「ログインが必要です」を出さない', async () => {
    respondMe('free')
    renderWithProviders(<PremiumLock {...props} />)

    await screen.findByRole('link', { name: 'プレミアムにアップグレード' })
    expect(screen.queryByText('ログインが必要です')).not.toBeInTheDocument()
  })

  it('料金と無料期間を出す', async () => {
    respondMe('free')
    renderWithProviders(<PremiumLock {...props} />)

    expect(await screen.findByText(/月額300円・5日間無料/)).toBeInTheDocument()
  })

  // 料金が引けなくても導線は残す。ここで丸ごと隠すと、この修正が直そうと
  // している「加入画面に行けない」状態に戻ってしまう。
  it('料金の取得に失敗しても加入導線は残る', async () => {
    respondMe('free')
    server.use(
      http.get('/api/v1/billing/plan', () => HttpResponse.error()),
    )
    renderWithProviders(<PremiumLock {...props} />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムにアップグレード' }),
    ).toHaveAttribute('href', '/checkout')
    expect(screen.queryByText(/月額/)).not.toBeInTheDocument()
  })

  it('プランの詳細へのリンクを出す', async () => {
    respondMe('free')
    renderWithProviders(<PremiumLock {...props} />)

    expect(
      await screen.findByRole('link', { name: 'プランの詳細を見る' }),
    ).toHaveAttribute('href', '/pricing')
  })

  it('判定が付くまではローディング表示を出す', () => {
    renderWithProviders(<PremiumLock {...props} />)

    expect(screen.getByRole('status')).toBeInTheDocument()
  })
})
