import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { AuthMenu } from './AuthMenu'

// respondMe は現在のユーザーの応答を仕込む。プランだけを差し替える。
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

describe('AuthMenu', () => {
  it('premium ならバッジを出す', async () => {
    respondMe('premium')
    renderWithProviders(<AuthMenu />)

    expect(await screen.findByLabelText('プレミアム会員')).toBeInTheDocument()
  })

  it('free ならバッジを出さない', async () => {
    respondMe('free')
    renderWithProviders(<AuthMenu />)

    // 「無いこと」を検査する前に描画の完了を待つ。待たずに検査すると、
    // まだ何も描かれていない状態でも通ってしまい、常に緑の偽の合格になる。
    expect(await screen.findByText('ユーザー')).toBeInTheDocument()
    expect(screen.queryByLabelText('プレミアム会員')).not.toBeInTheDocument()
  })
})
