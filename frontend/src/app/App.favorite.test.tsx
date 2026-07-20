import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { FavoriteItem, Menu } from '../api/types'
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

function loggedIn() {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000009',
          email: 'user@example.com',
          displayName: 'ユーザー',
        },
      }),
    ),
  )
}

// 検索結果からお気に入りに入れると、サーバ側の一覧が増える状態を作る。
// キャッシュを無効化しないと、お気に入り画面に遷移しても古いままになる
// （8-J で履歴に起きたのと同じ種類の不具合）。
function serverThatStoresFavorite() {
  let favorites: FavoriteItem[] = []
  server.use(
    http.get('/api/v1/favorites', () => HttpResponse.json({ favorites })),
    http.post('/api/v1/favorites', async ({ request }) => {
      const body = (await request.json()) as { menuId: string }
      favorites = [{ menu, createdAt: '2026-07-20T10:00:00Z' }]
      return HttpResponse.json({ menuId: body.menuId }, { status: 201 })
    }),
    http.get('/api/v1/menus/suggest', () => HttpResponse.json({ menu })),
    http.get('/api/v1/menus/:id/recipes', () =>
      HttpResponse.json({ recipes: [] }),
    ),
  )
}

describe('お気に入りの追加と一覧のつながり', () => {
  it('追加した献立が、遷移するだけでお気に入り一覧に出る', async () => {
    const user = userEvent.setup()
    loggedIn()
    serverThatStoresFavorite()
    renderWithProviders(<App />, { route: '/favorites' })

    // 先にお気に入り画面を開き、空の結果をキャッシュに載せる。
    expect(await screen.findByText(/まだお気に入りがありません/)).toBeVisible()

    await user.click(screen.getByRole('link', { name: '献立を探す' }))
    await user.click(screen.getByRole('button', { name: '献立を探す' }))
    await screen.findByRole('article')

    await user.click(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    )
    await screen.findByRole('button', { name: 'お気に入り済み' })

    await user.click(screen.getByRole('link', { name: 'お気に入り' }))

    expect(await screen.findByRole('listitem')).toHaveTextContent('親子丼')
    expect(
      screen.queryByText(/まだお気に入りがありません/),
    ).not.toBeInTheDocument()
  })
})
