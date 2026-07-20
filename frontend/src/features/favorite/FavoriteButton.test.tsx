import { screen, waitFor, within } from '@testing-library/react'
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
  it('未ログインでも星は表示する', async () => {
    // 既定のハンドラは未ログイン(401)。
    // 隠すと機能の存在に気づけない。押したときに案内する。
    renderWithProviders(<FavoriteButton menu={menu} />)

    expect(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    ).toBeVisible()
  })

  it('未ログインで押すとログインを促し、登録はしない', async () => {
    const user = userEvent.setup()
    let called = false
    server.use(
      http.post('/api/v1/favorites', () => {
        called = true
        return HttpResponse.json({ menuId: menu.id }, { status: 201 })
      }),
    )
    renderWithProviders(<FavoriteButton menu={menu} />)

    await user.click(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    )

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('ログイン')
    // 401 を踏ませない。押した時点で分かっていることを問い合わせない。
    expect(called).toBe(false)
  })

  it('案内からログイン画面へ行ける', async () => {
    const user = userEvent.setup()
    renderWithProviders(<FavoriteButton menu={menu} />)

    await user.click(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    )

    const dialog = within(await screen.findByRole('dialog'))
    expect(dialog.getByRole('link', { name: /ログイン/ })).toHaveAttribute(
      'href',
      '/login',
    )
  })

  it('案内は閉じられる', async () => {
    const user = userEvent.setup()
    renderWithProviders(<FavoriteButton menu={menu} />)

    await user.click(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    )
    await screen.findByRole('dialog')

    await user.click(screen.getByRole('button', { name: '閉じる' }))

    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    )
  })

  it('案内は Esc でも閉じられる', async () => {
    const user = userEvent.setup()
    renderWithProviders(<FavoriteButton menu={menu} />)

    await user.click(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    )
    await screen.findByRole('dialog')

    await user.keyboard('{Escape}')

    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    )
  })

  it('ログイン中は案内を出さずに登録する', async () => {
    const user = userEvent.setup()
    loggedIn()
    let favorites: FavoriteItem[] = []
    server.use(
      http.get('/api/v1/favorites', () => HttpResponse.json({ favorites })),
      http.post('/api/v1/favorites', () => {
        favorites = [favorite(menu)]
        return HttpResponse.json({ menuId: menu.id }, { status: 201 })
      }),
    )
    renderWithProviders(<FavoriteButton menu={menu} />)

    await user.click(
      await screen.findByRole('button', { name: 'お気に入りに追加' }),
    )

    expect(
      await screen.findByRole('button', { name: 'お気に入り済み' }),
    ).toBeVisible()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
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

  it('文字ではなく星で表し、状態は aria-pressed で伝える', async () => {
    loggedIn()
    respondFavorites(favorite(menu))
    renderWithProviders(<FavoriteButton menu={menu} />)

    const button = await screen.findByRole('button', { name: 'お気に入り済み' })
    // 見た目は星だけ。読み上げと自動テストのために名前は必ず持たせる。
    expect(button).toHaveAccessibleName('お気に入り済み')
    expect(button).toHaveTextContent('')
    expect(button).toHaveAttribute('aria-pressed', 'true')
  })

  it('未登録の星は塗らない', async () => {
    loggedIn()
    respondFavorites()
    renderWithProviders(<FavoriteButton menu={menu} />)

    const button = await screen.findByRole('button', {
      name: 'お気に入りに追加',
    })
    expect(button).toHaveAttribute('aria-pressed', 'false')
    // 塗りの有無で ON/OFF を表す。
    expect(button.querySelector('svg')).toHaveAttribute('fill', 'none')
  })

  it('登録済みの星は塗る', async () => {
    loggedIn()
    respondFavorites(favorite(menu))
    renderWithProviders(<FavoriteButton menu={menu} />)

    const button = await screen.findByRole('button', { name: 'お気に入り済み' })
    expect(button.querySelector('svg')).toHaveAttribute('fill', 'currentColor')
  })
})
