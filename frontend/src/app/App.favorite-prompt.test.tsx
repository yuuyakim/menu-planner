import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Menu } from '../api/types'
import { renderWithProviders } from '../test/render'
import { server } from '../test/server'
import { App } from './App'

const menu: Menu = {
  id: '018f0000-0000-7000-8000-000000000001',
  name: '親子丼',
  genre: 'japanese',
  difficulty: 'easy',
  description: '鶏肉と卵の定番。',
}

describe('未ログインでお気に入りを押したとき', () => {
  it('ログインしたら元の画面に戻る', async () => {
    const user = userEvent.setup()
    // 既定は未ログイン。週間献立は未認証でも使える画面。
    server.use(
      http.post('/api/v1/menus/suggest-weekly', () =>
        HttpResponse.json({
          week: Array.from({ length: 7 }, (_, i) => ({ day: i + 1, menu })),
        }),
      ),
      http.get('/api/v1/favorites', () => HttpResponse.json({ favorites: [] })),
    )
    renderWithProviders(<App />, { route: '/weekly' })

    await user.click(screen.getByRole('button', { name: '1週間分を作る' }))
    await screen.findAllByRole('listitem')

    // 星を押すと案内が出る。
    const stars = await screen.findAllByRole('button', {
      name: 'お気に入りに追加',
    })
    await user.click(stars[0])
    const dialog = within(await screen.findByRole('dialog'))

    // 案内からログインへ。
    await user.click(dialog.getByRole('link', { name: /ログイン/ }))
    expect(
      await screen.findByRole('heading', { level: 1, name: 'ログイン' }),
    ).toBeVisible()

    server.use(
      http.post('/api/v1/auth/login', () =>
        HttpResponse.json({
          user: {
            id: '018f0000-0000-7000-8000-000000000009',
            email: 'user@example.com',
            displayName: 'ユーザー',
          },
        }),
      ),
    )
    await user.type(screen.getByLabelText('メールアドレス'), 'user@example.com')
    await user.type(screen.getByLabelText('パスワード'), 'password123')
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    // ホームや検索画面ではなく、お気に入りにしたかった画面へ戻る。
    expect(
      await screen.findByRole('heading', { level: 1, name: '1週間の献立' }),
    ).toBeVisible()
  })
})
