import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Menu, ShoppingItem } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { ShoppingListPage } from './ShoppingListPage'

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

const nikujaga = menu('018f0000-0000-7000-8000-0000000000a1', '肉じゃが')
const oyakodon = menu('018f0000-0000-7000-8000-0000000000a2', '親子丼')

function item(
  name: string,
  category: ShoppingItem['ingredient']['category'],
  usedIn: Menu[],
): ShoppingItem {
  return {
    ingredient: { id: `${name}-id`, name, nameKana: name, category },
    usedIn: usedIn.map((m) => ({ id: m.id, name: m.name })),
  }
}

function respondShoppingList(items: ShoppingItem[]) {
  const bodies: unknown[] = []
  server.use(
    http.post('/api/v1/shopping-list', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ items })
    }),
  )
  return bodies
}

// 週間献立を sessionStorage に置いた状態で開く。
// 買い物リストは週間献立から作るため、それが前提になる。
function withWeek(menus: Menu[]) {
  sessionStorage.setItem(
    'menu-planner:weekly.week',
    JSON.stringify(menus.map((m, i) => ({ day: i + 1, menu: m }))),
  )
}

// 保存済みの週を開いている状態にする。ShoppingListPage はこのIDが
// 有れば GET /weekly-menus/:id/shopping-list を使う。
function withSavedId(id: string) {
  sessionStorage.setItem('menu-planner:weekly.savedId', JSON.stringify(id))
}

describe('ShoppingListPage', () => {
  it('週間献立の食材を、どの献立で使うか付きで並べる', async () => {
    withWeek([nikujaga, oyakodon])
    const bodies = respondShoppingList([
      item('玉ねぎ', 'vegetable', [nikujaga, oyakodon]),
      item('鶏もも肉', 'meat', [oyakodon]),
    ])
    renderWithProviders(<ShoppingListPage />)

    expect(await screen.findByText('玉ねぎ')).toBeVisible()
    expect(screen.getByText('鶏もも肉')).toBeVisible()
    // 分量が無い分、何のために買うかを示す。
    expect(screen.getByText(/肉じゃが.*親子丼|肉じゃが、親子丼/)).toBeVisible()

    // 週間献立の献立IDが送られる。
    expect(bodies[0]).toEqual({ menuIds: [nikujaga.id, oyakodon.id] })
  })

  it('カテゴリを見出しにして売り場が分かるようにする', async () => {
    withWeek([nikujaga])
    respondShoppingList([item('玉ねぎ', 'vegetable', [nikujaga])])
    renderWithProviders(<ShoppingListPage />)

    expect(await screen.findByText('野菜')).toBeVisible()
  })

  it('週間献立が無ければ、先に作るよう案内して導線を出す', async () => {
    sessionStorage.clear()
    renderWithProviders(<ShoppingListPage />)

    // 案内文だけでなく、そこへ行けるリンクまで出す。
    expect(
      await screen.findByRole('link', { name: '1週間の献立を作る' }),
    ).toBeVisible()
    expect(screen.getByText(/まだ献立が決まっていません/)).toBeVisible()
  })

  it('実際の材料はレシピ元で確認するよう伝える', async () => {
    withWeek([nikujaga])
    respondShoppingList([item('玉ねぎ', 'vegetable', [nikujaga])])
    renderWithProviders(<ShoppingListPage />)

    expect(await screen.findByText(/レシピ元/)).toBeVisible()
  })

  it('取得に失敗したらエラーを出す', async () => {
    withWeek([nikujaga])
    server.use(
      http.post('/api/v1/shopping-list', () =>
        HttpResponse.json({ title: 'エラー' }, { status: 500 }),
      ),
    )
    renderWithProviders(<ShoppingListPage />)

    expect(await screen.findByRole('alert')).toBeVisible()
  })

  it('保存済みの週を開くと GET /weekly-menus/:id/shopping-list を使う', async () => {
    const savedId = '11111111-1111-1111-1111-111111111111'
    withWeek([nikujaga])
    withSavedId(savedId)

    let hit = false
    server.use(
      http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () => {
        hit = true
        return HttpResponse.json({
          items: [
            {
              name: 'にんじん',
              category: 'vegetable',
              origin: 'derived',
              checked: false,
              usedIn: [{ id: nikujaga.id, name: nikujaga.name }],
            },
          ],
        })
      }),
    )

    renderWithProviders(<ShoppingListPage />)
    expect(await screen.findByText('にんじん')).toBeInTheDocument()
    expect(hit).toBe(true)
  })

  it('未保存の週は従来どおり POST /shopping-list を使う', async () => {
    withWeek([nikujaga]) // savedId は設定しない
    const bodies = respondShoppingList([item('たまねぎ', 'vegetable', [nikujaga])])

    renderWithProviders(<ShoppingListPage />)
    expect(await screen.findByText('たまねぎ')).toBeInTheDocument()
    expect(bodies[0]).toEqual({ menuIds: [nikujaga.id] })
  })
})
