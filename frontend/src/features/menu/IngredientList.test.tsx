import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Ingredient } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { IngredientList } from './IngredientList'

const menuId = '018f0000-0000-7000-8000-000000000001'

function ingredient(
  name: string,
  category: Ingredient['category'],
): Ingredient {
  return { id: `${name}-id`, name, nameKana: name, category }
}

function respondIngredients(items: Ingredient[]) {
  server.use(
    http.get('/api/v1/menus/:id/ingredients', () =>
      HttpResponse.json({ ingredients: items }),
    ),
  )
}

describe('IngredientList', () => {
  it('食材をカテゴリの見出しごとに並べる', async () => {
    respondIngredients([
      ingredient('じゃがいも', 'vegetable'),
      ingredient('豚こま切れ肉', 'meat'),
    ])
    renderWithProviders(<IngredientList menuId={menuId} />)

    expect(await screen.findByText('じゃがいも')).toBeVisible()
    expect(screen.getByText('豚こま切れ肉')).toBeVisible()
    // 売り場が分かるよう、カテゴリを見出しにする。
    expect(screen.getByText('野菜')).toBeVisible()
    expect(screen.getByText('肉')).toBeVisible()
  })

  it('該当するカテゴリの見出しだけを出す', async () => {
    respondIngredients([ingredient('かぼちゃ', 'vegetable')])
    renderWithProviders(<IngredientList menuId={menuId} />)

    expect(await screen.findByText('かぼちゃ')).toBeVisible()
    expect(screen.queryByText('魚介')).not.toBeInTheDocument()
  })

  it('実際の材料はレシピ元で確認するよう伝える', async () => {
    // 代表的な食材の例であって正確な材料表ではない（spec.md 14.1）。
    // アレルギー対応と誤解されないよう、画面に必ず出す。
    respondIngredients([ingredient('かぼちゃ', 'vegetable')])
    renderWithProviders(<IngredientList menuId={menuId} />)

    expect(await screen.findByText(/レシピ/)).toBeVisible()
  })

  it('食材が無ければその旨を出す', async () => {
    respondIngredients([])
    renderWithProviders(<IngredientList menuId={menuId} />)

    expect(await screen.findByText(/登録されていません/)).toBeVisible()
  })

  it('取得に失敗したらエラーを出す', async () => {
    server.use(
      http.get('/api/v1/menus/:id/ingredients', () =>
        HttpResponse.json({ title: 'エラー' }, { status: 500 }),
      ),
    )
    renderWithProviders(<IngredientList menuId={menuId} />)

    expect(await screen.findByRole('alert')).toBeVisible()
  })
})
