import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { FavoriteItem } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { FavoritePage } from './FavoritePage'

function item(n: number): FavoriteItem {
  return {
    menu: {
      id: `018f0000-0000-7000-8000-00000000000${n}`,
      name: `献立${n}`,
      genre: 'japanese',
      difficulty: 'easy',
      role: 'main',
      description: `説明${n}`,
    },
    createdAt: `2026-07-2${n}T10:00:00Z`,
  }
}

function respondFavorites(...favorites: FavoriteItem[]) {
  server.use(
    http.get('/api/v1/favorites', () => HttpResponse.json({ favorites })),
  )
}

function row(name: string) {
  return within(screen.getByRole('listitem', { name: new RegExp(name) }))
}

describe('お気に入り画面', () => {
  it('サーバが返した順（新しい順）にそのまま並べる', async () => {
    respondFavorites(item(3), item(2), item(1))
    renderWithProviders(<FavoritePage />)

    const items = await screen.findAllByRole('listitem')
    expect(items).toHaveLength(3)
    expect(items[0]).toHaveTextContent('献立3')
    expect(items[2]).toHaveTextContent('献立1')
  })

  it('0件のときはその旨を伝える', async () => {
    respondFavorites()
    renderWithProviders(<FavoritePage />)

    expect(await screen.findByText(/まだお気に入りがありません/)).toBeVisible()
    expect(screen.queryByRole('listitem')).not.toBeInTheDocument()
  })

  it('15件を超えても全件表示する', async () => {
    // 履歴は15件でFIFO削除されるが、お気に入りに上限は無い（spec.md 2.6）。
    const many = Array.from({ length: 20 }, (_, i) => ({
      ...item(1),
      menu: { ...item(1).menu, id: `id-${i}`, name: `献立${i}` },
    }))
    respondFavorites(...many)
    renderWithProviders(<FavoritePage />)

    await waitFor(() =>
      expect(screen.getAllByRole('listitem')).toHaveLength(20),
    )
  })

  it('読み込み中を表示する', async () => {
    let release!: () => void
    const pending = new Promise<void>((resolve) => {
      release = resolve
    })
    server.use(
      http.get('/api/v1/favorites', async () => {
        await pending
        return HttpResponse.json({ favorites: [item(1)] })
      }),
    )
    renderWithProviders(<FavoritePage />)

    expect(await screen.findByRole('status')).toHaveTextContent('読み込み')
    release()
    expect(await screen.findByRole('listitem')).toBeVisible()
  })

  it('お気に入りから外せる', async () => {
    const user = userEvent.setup()
    let favorites = [item(1), item(2)]
    let deletedMenuId: string | undefined
    server.use(
      http.get('/api/v1/favorites', () => HttpResponse.json({ favorites })),
      http.delete('/api/v1/favorites/:menuId', ({ params }) => {
        deletedMenuId = params.menuId as string
        favorites = [item(2)]
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderWithProviders(<FavoritePage />)
    await screen.findAllByRole('listitem')

    await user.click(row('献立1').getByRole('button', { name: '外す' }))

    await waitFor(() =>
      expect(screen.queryByText('献立1')).not.toBeInTheDocument(),
    )
    expect(deletedMenuId).toBe(item(1).menu.id)
    expect(screen.getByText('献立2')).toBeVisible()
  })

  it('外すのに失敗したらメッセージを出し、一覧は残す', async () => {
    const user = userEvent.setup()
    respondFavorites(item(1))
    server.use(
      http.delete('/api/v1/favorites/:menuId', () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/x',
            title: 'お気に入りが見つかりません',
            status: 404,
          },
          { status: 404, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    renderWithProviders(<FavoritePage />)
    await screen.findAllByRole('listitem')

    await user.click(row('献立1').getByRole('button', { name: '外す' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'お気に入りが見つかりません',
    )
    expect(screen.getByText('献立1')).toBeVisible()
  })

  it('献立の詳細へ遷移できる', async () => {
    respondFavorites(item(1))
    renderWithProviders(<FavoritePage />)
    await screen.findAllByRole('listitem')

    expect(row('献立1').getByRole('link', { name: /レシピ/ })).toHaveAttribute(
      'href',
      `/menus/${item(1).menu.id}`,
    )
  })

  it('取得に失敗したらメッセージを出す', async () => {
    server.use(http.get('/api/v1/favorites', () => HttpResponse.error()))
    renderWithProviders(<FavoritePage />)

    expect(await screen.findByRole('alert')).toBeVisible()
  })
})

// 「すべて」で引けば副菜・汁物もお気に入りにできる（spec.md 2.10）。
// 一覧で役割が分からないと、詳細を開くまで何の一品か判別できない。
it('一覧に役割を出す', async () => {
  const side: FavoriteItem = {
    ...item(1),
    menu: { ...item(1).menu, name: 'ポテトサラダ', role: 'side' },
  }
  respondFavorites(side)
  renderWithProviders(<FavoritePage />)

  const row = await screen.findByRole('listitem', { name: 'ポテトサラダ' })
  expect(within(row).getByText('副菜')).toBeVisible()
})
