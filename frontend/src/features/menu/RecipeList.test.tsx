import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Recipe } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { RecipeList } from './RecipeList'

const menuId = '018f0000-0000-7000-8000-000000000001'

function recipe(n: number): Recipe {
  return {
    title: `親子丼のレシピ${n}`,
    url: `https://example.com/recipe/${n}`,
    domain: `example${n}.com`,
    snippet: `作り方${n}`,
  }
}

function respondRecipes(...recipes: Recipe[]) {
  server.use(
    http.get(`/api/v1/menus/${menuId}/recipes`, () =>
      HttpResponse.json({ recipes }),
    ),
  )
}

function respondFailure(status: number, title: string) {
  server.use(
    http.get(`/api/v1/menus/${menuId}/recipes`, () =>
      HttpResponse.json(
        { type: 'https://example.com/probs/x', title, status },
        { status, headers: { 'Content-Type': 'application/problem+json' } },
      ),
    ),
  )
}

describe('レシピ一覧', () => {
  it('3件表示される', async () => {
    respondRecipes(recipe(1), recipe(2), recipe(3))
    renderWithProviders(<RecipeList menuId={menuId} />)

    const links = await screen.findAllByRole('link')
    expect(links).toHaveLength(3)
    expect(links[0]).toHaveTextContent('親子丼のレシピ1')
    expect(screen.getByText('example1.com')).toBeVisible()
  })

  it('新しいタブで開き、開いた先から元のページを操作できない', async () => {
    respondRecipes(recipe(1), recipe(2), recipe(3))
    renderWithProviders(<RecipeList menuId={menuId} />)

    const links = await screen.findAllByRole('link')
    for (const link of links) {
      expect(link).toHaveAttribute('target', '_blank')
      // noopener が無いと、開いた先から window.opener 経由でこの画面を操作できる。
      expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    }
  })

  it('3件未満でも表示できる', async () => {
    respondRecipes(recipe(1))
    renderWithProviders(<RecipeList menuId={menuId} />)

    expect(await screen.findAllByRole('link')).toHaveLength(1)
  })

  it('0件でも欄自体は壊れない', async () => {
    respondRecipes()
    renderWithProviders(<RecipeList menuId={menuId} />)

    expect(await screen.findByText(/見つかりませんでした/)).toBeVisible()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('取得中はローディングを表示する', async () => {
    let release!: () => void
    const pending = new Promise<void>((resolve) => {
      release = resolve
    })
    server.use(
      http.get(`/api/v1/menus/${menuId}/recipes`, async () => {
        await pending
        return HttpResponse.json({ recipes: [recipe(1)] })
      }),
    )
    renderWithProviders(<RecipeList menuId={menuId} />)

    expect(await screen.findByRole('status')).toHaveTextContent('読み込み')

    release()
    expect(await screen.findByRole('link')).toBeVisible()
  })

  it('502 はエラーと再試行の導線を出す', async () => {
    respondFailure(502, 'レシピの取得に失敗しました')
    renderWithProviders(<RecipeList menuId={menuId} />)

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'レシピの取得に失敗しました',
    )
    expect(screen.getByRole('button', { name: '再試行' })).toBeVisible()
  })

  it('再試行で取り直せる', async () => {
    const user = userEvent.setup()
    respondFailure(502, 'レシピの取得に失敗しました')
    renderWithProviders(<RecipeList menuId={menuId} />)

    await screen.findByRole('alert')

    // 外部の検索APIが復帰した状況を作る。
    respondRecipes(recipe(1), recipe(2))
    await user.click(screen.getByRole('button', { name: '再試行' }))

    await waitFor(() =>
      expect(screen.getAllByRole('link')).toHaveLength(2),
    )
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('献立が変われば取り直す', async () => {
    const otherId = '018f0000-0000-7000-8000-000000000002'
    respondRecipes(recipe(1))
    server.use(
      http.get(`/api/v1/menus/${otherId}/recipes`, () =>
        HttpResponse.json({
          recipes: [{ ...recipe(9), title: 'カレーのレシピ' }],
        }),
      ),
    )

    const { rerender } = renderWithProviders(<RecipeList menuId={menuId} />)
    expect(await screen.findByText('親子丼のレシピ1')).toBeVisible()

    rerender(<RecipeList menuId={otherId} />)

    expect(await screen.findByText('カレーのレシピ')).toBeVisible()
  })

  it('リンクのタイトルが空でもURLを手掛かりに開ける', async () => {
    respondRecipes({ ...recipe(1), title: '' })
    renderWithProviders(<RecipeList menuId={menuId} />)

    const link = await screen.findByRole('link')
    expect(link).toHaveAccessibleName()
    expect(link).toHaveAttribute('href', 'https://example.com/recipe/1')
  })
})
