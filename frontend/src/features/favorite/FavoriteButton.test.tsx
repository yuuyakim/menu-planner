import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { FavoriteItem, Menu } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { FavoriteButton } from './FavoriteButton'

const menu: Menu = {
  id: '018f0000-0000-7000-8000-000000000001',
  name: '親子丼',
  genre: 'japanese',
  difficulty: 'easy',
  description: '鶏肉と卵の定番。',
}

function favorite(m: Menu): FavoriteItem {
  return { menu: m, createdAt: '2026-07-20T10:00:00Z' }
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

function respondFavorites(...favorites: FavoriteItem[]) {
  server.use(
    http.get('/api/v1/favorites', () => HttpResponse.json({ favorites })),
  )
}

describe('お気に入りボタン', () => {
  it('未ログインなら表示しない', async () => {
    // 既定のハンドラは未ログイン(401)。
    renderWithProviders(<FavoriteButton menu={menu} />)

    // ログイン判定が済んでもボタンは出ない。
    await waitFor(() =>
      expect(screen.queryByRole('button')).not.toBeInTheDocument(),
    )
  })

  it('未登録なら「お気に入りに追加」を出す', async () => {
    loggedIn()
    respondFavorites()
    renderWithProviders(<FavoriteButton menu={menu} />)

    expect(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    ).toBeVisible()
  })

  it('登録済みなら「お気に入り済み」を出す', async () => {
    loggedIn()
    respondFavorites(favorite(menu))
    renderWithProviders(<FavoriteButton menu={menu} />)

    expect(
      await screen.findByRole('button', { name: 'お気に入り済み' }),
    ).toBeVisible()
  })

  it('押すと追加され、表示が切り替わる', async () => {
    const user = userEvent.setup()
    loggedIn()
    let favorites: FavoriteItem[] = []
    let addedMenuId: string | undefined
    server.use(
      http.get('/api/v1/favorites', () => HttpResponse.json({ favorites })),
      http.post('/api/v1/favorites', async ({ request }) => {
        const body = (await request.json()) as { menuId: string }
        addedMenuId = body.menuId
        favorites = [favorite(menu)]
        return HttpResponse.json({ menuId: body.menuId }, { status: 201 })
      }),
    )
    renderWithProviders(<FavoriteButton menu={menu} />)

    await user.click(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    )

    expect(
      await screen.findByRole('button', { name: 'お気に入り済み' }),
    ).toBeVisible()
    expect(addedMenuId).toBe(menu.id)
  })

  it('もう一度押すと外れる', async () => {
    const user = userEvent.setup()
    loggedIn()
    let favorites: FavoriteItem[] = [favorite(menu)]
    let deletedMenuId: string | undefined
    server.use(
      http.get('/api/v1/favorites', () => HttpResponse.json({ favorites })),
      http.delete('/api/v1/favorites/:menuId', ({ params }) => {
        deletedMenuId = params.menuId as string
        favorites = []
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderWithProviders(<FavoriteButton menu={menu} />)

    await user.click(
      await screen.findByRole('button', { name: 'お気に入り済み' }),
    )

    expect(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    ).toBeVisible()
    expect(deletedMenuId).toBe(menu.id)
  })

  it('失敗したらメッセージを出し、状態は変えない', async () => {
    const user = userEvent.setup()
    loggedIn()
    respondFavorites()
    server.use(
      http.post('/api/v1/favorites', () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/x',
            title: 'この献立は既にお気に入りに登録されています',
            status: 409,
          },
          { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    renderWithProviders(<FavoriteButton menu={menu} />)

    await user.click(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    )

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'この献立は既にお気に入りに登録されています',
    )
    expect(
      screen.getByRole('button', { name: 'お気に入りに追加' }),
    ).toBeVisible()
  })
})
