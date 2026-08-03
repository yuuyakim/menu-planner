import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Ingredient, Menu, MenuMatch } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { SearchByIngredientsPage } from './SearchByIngredientsPage'

function ing(id: string, name: string, category: Ingredient['category']): Ingredient {
  return { id, name, nameKana: name, category }
}

const onion = ing('i1', '玉ねぎ', 'vegetable')
const potato = ing('i2', 'じゃがいも', 'vegetable')
const beef = ing('i3', '牛肉', 'meat')

function menu(id: string, name: string): Menu {
  return {
    id,
    name,
    genre: 'japanese',
    difficulty: 'easy',
    role: 'main',
    description: `${name}の説明`,
  }
}

// respondIngredients は食材マスタを仕込む。
function respondIngredients(items: Ingredient[] = [onion, potato, beef]) {
  server.use(
    http.get('/api/v1/ingredients', () =>
      HttpResponse.json({ ingredients: items }),
    ),
  )
}

// respondSearch は検索結果を仕込み、送られた本文を記録する。
function respondSearch(matches: MenuMatch[]) {
  const bodies: { ingredientIds?: string[] }[] = []
  server.use(
    http.post('/api/v1/menus/search-by-ingredients', async ({ request }) => {
      bodies.push((await request.json()) as { ingredientIds: string[] })
      return HttpResponse.json({ matches })
    }),
  )
  return bodies
}

function searchButton() {
  return screen.getByRole('button', { name: 'この食材で探す' })
}

async function pick(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(await screen.findByLabelText(name))
}

describe('冷蔵庫から探す', () => {
  it('1つも選んでいなければ探せない', async () => {
    respondIngredients()
    renderWithProviders(<SearchByIngredientsPage />)

    // 押しても 400 になるだけなので押させない。
    await waitFor(() => expect(searchButton()).toBeDisabled())
  })

  it('選んだ食材を送り、候補と不足を並べる', async () => {
    const user = userEvent.setup()
    respondIngredients()
    const bodies = respondSearch([
      { menu: menu('m1', '肉じゃが'), matched: [onion, potato], missing: [beef] },
    ])
    renderWithProviders(<SearchByIngredientsPage />)

    await pick(user, '玉ねぎ')
    await pick(user, 'じゃがいも')
    await user.click(searchButton())

    const row = await screen.findByRole('listitem', { name: '肉じゃが' })
    expect(within(row).getByText(/使える食材: 玉ねぎ・じゃがいも/)).toBeVisible()
    // 「あと何を買えばよいか」がこの機能の要。
    expect(within(row).getByText('あと1品: 牛肉')).toBeVisible()

    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].ingredientIds).toEqual(['i1', 'i2'])
  })

  it('不足が無ければその旨を出す', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondSearch([
      { menu: menu('m2', 'オニオンスープ'), matched: [onion], missing: [] },
    ])
    renderWithProviders(<SearchByIngredientsPage />)

    await pick(user, '玉ねぎ')
    await user.click(searchButton())

    const row = await screen.findByRole('listitem', { name: 'オニオンスープ' })
    expect(within(row).getByText('足りない食材はありません')).toBeVisible()
  })

  it('不足0でも調味料などが要ることを伝える', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondSearch([
      { menu: menu('m2', 'オニオンスープ'), matched: [onion], missing: [] },
    ])
    renderWithProviders(<SearchByIngredientsPage />)

    await pick(user, '玉ねぎ')
    await user.click(searchButton())

    // 食材リストは代表例であって正確な材料表ではない（spec.md 14.1）。
    // 断らないと「これだけ買えば作れる」と受け取られる。
    expect(await screen.findByText(/調味料は含みません/)).toBeVisible()
    expect(screen.getByText(/実際の材料はレシピ元/)).toBeVisible()
  })

  it('見つからなければ、食材を増やすよう促す', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondSearch([])
    renderWithProviders(<SearchByIngredientsPage />)

    await pick(user, '玉ねぎ')
    await user.click(searchButton())

    expect(
      await screen.findByText(/その食材で作れる献立が見つかりませんでした/),
    ).toBeVisible()
  })

  it('候補からレシピへ進める', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondSearch([
      { menu: menu('m1', '肉じゃが'), matched: [onion], missing: [beef] },
    ])
    renderWithProviders(<SearchByIngredientsPage />)

    await pick(user, '玉ねぎ')
    await user.click(searchButton())

    const row = await screen.findByRole('listitem', { name: '肉じゃが' })
    expect(within(row).getByRole('link', { name: 'レシピを見る' })).toHaveAttribute(
      'href',
      '/menus/m1',
    )
  })

  it('食材を選び直したら前の結果を消す', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondSearch([
      { menu: menu('m1', '肉じゃが'), matched: [onion], missing: [beef] },
    ])
    renderWithProviders(<SearchByIngredientsPage />)

    await pick(user, '玉ねぎ')
    await user.click(searchButton())
    expect(await screen.findByRole('listitem', { name: '肉じゃが' })).toBeVisible()

    await pick(user, '牛肉')

    // 残したままだと、いま選んでいる食材の結果だと誤解させる。
    await waitFor(() =>
      expect(
        screen.queryByRole('listitem', { name: '肉じゃが' }),
      ).not.toBeInTheDocument(),
    )
  })

  it('検索に失敗したら知らせる', async () => {
    const user = userEvent.setup()
    respondIngredients()
    server.use(
      http.post('/api/v1/menus/search-by-ingredients', () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/ingredient-not-found',
            title: '指定された食材が見つかりません',
            status: 404,
          },
          { status: 404, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    renderWithProviders(<SearchByIngredientsPage />)

    await pick(user, '玉ねぎ')
    await user.click(searchButton())

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '指定された食材が見つかりません',
    )
  })
})

// respondResolve は自由記述の解決結果を仕込み、送られた本文を記録する。
function respondResolve(result: {
  resolved: { word: string; ingredient: Ingredient }[]
  unresolved: string[]
  degraded: boolean
}) {
  const bodies: { text?: string }[] = []
  server.use(
    http.post('/api/v1/ingredients/resolve', async ({ request }) => {
      bodies.push((await request.json()) as { text: string })
      return HttpResponse.json(result)
    }),
  )
  return bodies
}

// readAloud はテキストを入力して「読み取る」を押す。
async function readAloud(
  user: ReturnType<typeof userEvent.setup>,
  text: string,
) {
  await user.type(await screen.findByLabelText('冷蔵庫にあるものを書く'), text)
  await user.click(screen.getByRole('button', { name: '読み取る' }))
}

describe('冷蔵庫の中身を自由記述で入力する', () => {
  it('読み取るとピッカーにチェックが入る', async () => {
    const user = userEvent.setup()
    respondIngredients()
    const bodies = respondResolve({
      resolved: [{ word: 'たまねぎ', ingredient: onion }],
      unresolved: [],
      degraded: false,
    })
    renderWithProviders(<SearchByIngredientsPage />)

    await readAloud(user, 'たまねぎ')

    expect(await screen.findByText('1個を選択中')).toBeInTheDocument()
    expect(bodies).toEqual([{ text: 'たまねぎ' }])
    // チェックが入れば、そのまま検索に進める。
    await waitFor(() => expect(searchButton()).toBeEnabled())
  })

  it('手で入れたチェックを消さずに足す', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondResolve({
      resolved: [{ word: 'たまねぎ', ingredient: onion }],
      unresolved: [],
      degraded: false,
    })
    renderWithProviders(<SearchByIngredientsPage />)

    await pick(user, '牛肉')
    await readAloud(user, 'たまねぎ')

    // 置き換えると、先に手で入れた選択が黙って消える。
    expect(await screen.findByText('2個を選択中')).toBeInTheDocument()
  })

  it('未解決の語を明示する', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondResolve({
      resolved: [],
      unresolved: ['オクラ', 'ゴーヤ'],
      degraded: false,
    })
    renderWithProviders(<SearchByIngredientsPage />)

    await readAloud(user, 'オクラ、ゴーヤ')

    const status = await screen.findByRole('status', { name: '読み取りの結果' })
    expect(status).toHaveTextContent('登録がありませんでした')
    expect(status).toHaveTextContent('オクラ')
    expect(status).toHaveTextContent('ゴーヤ')
  })

  it('縮退したことを伝える', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondResolve({ resolved: [], unresolved: ['豚こま'], degraded: true })
    renderWithProviders(<SearchByIngredientsPage />)

    await readAloud(user, '豚こま')

    expect(
      await screen.findByRole('status', { name: '読み取りの結果' }),
    ).toHaveTextContent('一部だけ読み取れました')
  })

  it('空のままでは読み取れない', async () => {
    respondIngredients()
    renderWithProviders(<SearchByIngredientsPage />)

    // 押しても 400 になるだけなので押させない。
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '読み取る' })).toBeDisabled(),
    )
  })

  it('選択をすべて外すと読み取り結果の表示も消える', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondResolve({
      resolved: [{ word: 'たまねぎ', ingredient: onion }],
      unresolved: ['オクラ'],
      degraded: false,
    })
    renderWithProviders(<SearchByIngredientsPage />)

    await readAloud(user, 'たまねぎ、オクラ')
    expect(
      await screen.findByRole('status', { name: '読み取りの結果' }),
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '選択をすべて外す' }))

    // 選択を捨てたのに「登録がありませんでした」だけ残ると、
    // いつの読み取りの話なのか分からなくなる。
    await waitFor(() =>
      expect(screen.queryByRole('status', { name: '読み取りの結果' })).toBeNull(),
    )
  })
})
