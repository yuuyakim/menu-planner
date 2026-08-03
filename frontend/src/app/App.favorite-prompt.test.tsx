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
  role: 'main',
  description: '鶏肉と卵の定番。',
}

describe('未ログインでお気に入りを押したとき', () => {
  it('ログインしたら元の画面に戻る', async () => {
    const user = userEvent.setup()
    // 既定は未ログイン。検索画面は未認証でも使える画面。
    // （週間献立は RequireAuth で守られており未認証では使えないため、
    // ここでの確認先には使えない。）
    server.use(
      http.get('/api/v1/menus/suggest', () => HttpResponse.json({ menu })),
      http.get('/api/v1/favorites', () => HttpResponse.json({ favorites: [] })),
    )
    renderWithProviders(<App />, { route: '/search' })

    await user.click(screen.getByRole('button', { name: '献立を探す' }))
    await screen.findByRole('article')

    // 星を押すと案内が出る。
    const star = await screen.findByRole('button', {
      name: 'お気に入りに追加',
    })
    await user.click(star)
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

    // ホームではなく、お気に入りにしたかった画面へ戻る。
    expect(
      await screen.findByRole('heading', { level: 1, name: '献立を探す' }),
    ).toBeVisible()
  })
})
