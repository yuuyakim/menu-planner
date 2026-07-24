import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it } from 'vitest'

import type { Menu, ShoppingItem } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { ShoppingListPage } from './ShoppingListPage'

// respondMe は現在のユーザーの応答を仕込む。プランだけを差し替える
// （AuthMenu.test.tsx と同じ流儀）。
function respondMe(plan: 'free' | 'premium') {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000001',
          email: 'user@example.com',
          displayName: 'ユーザー',
          plan,
        },
      }),
    ),
  )
}

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
  // 「一度きり」フラグは localStorage に残るため、他のテストへ漏れないよう消す。
  // 共通の setup.ts は sessionStorage しか消していない。
  afterEach(() => localStorage.clear())

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
              hidden: false,
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

  it('premium が保存済みの週でチェックすると PUT で永続化する', async () => {
    const savedId = '11111111-1111-1111-1111-111111111111'
    withWeek([nikujaga])
    withSavedId(savedId)
    respondMe('premium')
    server.use(
      http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
        HttpResponse.json({
          items: [
            {
              name: 'にんじん',
              category: 'vegetable',
              origin: 'derived',
              checked: false,
              hidden: false,
              usedIn: [],
            },
          ],
        }),
      ),
    )

    const puts: unknown[] = []
    server.use(
      http.put(
        `/api/v1/weekly-menus/${savedId}/shopping-list`,
        async ({ request }) => {
          puts.push(await request.json())
          return new HttpResponse(null, { status: 204 })
        },
      ),
    )

    renderWithProviders(<ShoppingListPage />)
    const box = await screen.findByRole('checkbox', { name: /にんじん/ })
    await userEvent.click(box)

    await waitFor(() => expect(puts.length).toBe(1))
    expect(puts[0]).toEqual({
      items: [
        {
          name: 'にんじん',
          category: 'vegetable',
          origin: 'derived',
          checked: true,
          hidden: false,
        },
      ],
    })
  })

  it('free はチェックしても PUT を投げない（その場限り）', async () => {
    const savedId = '11111111-1111-1111-1111-111111111111'
    withWeek([nikujaga])
    withSavedId(savedId)
    respondMe('free')
    server.use(
      http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
        HttpResponse.json({
          items: [
            {
              name: 'にんじん',
              category: 'vegetable',
              origin: 'derived',
              checked: false,
              hidden: false,
              usedIn: [],
            },
          ],
        }),
      ),
    )
    let put = false
    server.use(
      http.put(`/api/v1/weekly-menus/${savedId}/shopping-list`, () => {
        put = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    renderWithProviders(<ShoppingListPage />)
    await userEvent.click(
      await screen.findByRole('checkbox', { name: /にんじん/ }),
    )
    expect(
      await screen.findByRole('checkbox', { name: /にんじん/ }),
    ).toBeChecked()
    expect(put).toBe(false)
  })

  it('free が初めてチェックすると案内が1回だけ出る', async () => {
    const savedId = '11111111-1111-1111-1111-111111111111'
    withWeek([nikujaga])
    withSavedId(savedId)
    respondMe('free')
    server.use(
      http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
        HttpResponse.json({
          items: [
            {
              name: 'にんじん',
              category: 'vegetable',
              origin: 'derived',
              checked: false,
              hidden: false,
              usedIn: [],
            },
            {
              name: 'たまねぎ',
              category: 'vegetable',
              origin: 'derived',
              checked: false,
              hidden: false,
              usedIn: [],
            },
          ],
        }),
      ),
    )

    renderWithProviders(<ShoppingListPage />)
    await userEvent.click(
      await screen.findByRole('checkbox', { name: /にんじん/ }),
    )
    expect(await screen.findByText(/プレミアム/)).toBeInTheDocument()

    // 閉じる
    await userEvent.click(screen.getByRole('button', { name: /閉じる/ }))
    expect(screen.queryByText(/プレミアム/)).not.toBeInTheDocument()

    // 別の（まだ未チェックの）品目を初めてチェックする、正真正銘の2度目の
    // 「追加」操作。ここで案内が再度出ないことこそが「1回だけ」の本質。
    // 同じ品目を再クリックすると外す操作（adding === false）になり、
    // adding のガードだけで通ってしまって guidanceDone 側の検証にならない。
    await userEvent.click(
      screen.getByRole('checkbox', { name: /たまねぎ/ }),
    )
    expect(
      screen.getByRole('checkbox', { name: /たまねぎ/ }),
    ).toBeChecked()
    expect(screen.queryByText(/プレミアム/)).not.toBeInTheDocument()
  })

  it('premium にはチェックしても案内を出さない', async () => {
    const savedId = '11111111-1111-1111-1111-111111111111'
    withWeek([nikujaga])
    withSavedId(savedId)
    respondMe('premium')
    server.use(
      http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
        HttpResponse.json({
          items: [
            {
              name: 'にんじん',
              category: 'vegetable',
              origin: 'derived',
              checked: false,
              hidden: false,
              usedIn: [],
            },
          ],
        }),
      ),
      http.put(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
        new HttpResponse(null, { status: 204 }),
      ),
    )
    renderWithProviders(<ShoppingListPage />)
    await userEvent.click(
      await screen.findByRole('checkbox', { name: /にんじん/ }),
    )
    expect(screen.queryByText(/プレミアム/)).not.toBeInTheDocument()
  })

  it('premium が品目を手で足すと manual として PUT に載る', async () => {
    const savedId = '11111111-1111-1111-1111-111111111111'
    withWeek([nikujaga])
    withSavedId(savedId)
    respondMe('premium')
    server.use(
      http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
        HttpResponse.json({ items: [] }),
      ),
    )
    const puts: { items: unknown[] }[] = []
    server.use(
      http.put(
        `/api/v1/weekly-menus/${savedId}/shopping-list`,
        async ({ request }) => {
          puts.push((await request.json()) as { items: unknown[] })
          return new HttpResponse(null, { status: 204 })
        },
      ),
    )

    renderWithProviders(<ShoppingListPage />)
    await userEvent.type(await screen.findByLabelText(/品目を追加/), '牛乳')
    await userEvent.selectOptions(screen.getByLabelText(/カテゴリ/), 'dairy_egg')
    await userEvent.click(screen.getByRole('button', { name: /追加/ }))

    await waitFor(() => expect(puts.length).toBe(1))
    expect(puts[0].items).toContainEqual({
      name: '牛乳',
      category: 'dairy_egg',
      origin: 'manual',
      checked: false,
      hidden: false,
    })
  })

  it('premium が導出品目を消すと hidden として PUT に載る', async () => {
    const savedId = '11111111-1111-1111-1111-111111111111'
    withWeek([nikujaga])
    withSavedId(savedId)
    respondMe('premium')
    server.use(
      http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
        HttpResponse.json({
          items: [
            {
              name: 'にんじん',
              category: 'vegetable',
              origin: 'derived',
              checked: false,
              hidden: false,
              usedIn: [],
            },
          ],
        }),
      ),
    )
    const puts: { items: unknown[] }[] = []
    server.use(
      http.put(
        `/api/v1/weekly-menus/${savedId}/shopping-list`,
        async ({ request }) => {
          puts.push((await request.json()) as { items: unknown[] })
          return new HttpResponse(null, { status: 204 })
        },
      ),
    )

    renderWithProviders(<ShoppingListPage />)
    await userEvent.click(
      await screen.findByRole('button', { name: /にんじんを消す/ }),
    )
    await waitFor(() => expect(puts.length).toBe(1))
    expect(puts[0].items).toContainEqual({
      name: 'にんじん',
      category: 'vegetable',
      origin: 'derived',
      checked: false,
      hidden: true,
    })
  })

  it('保存済みの週で hidden の品目は表示されないが overlay には残る', async () => {
    const savedId = '11111111-1111-1111-1111-111111111111'
    withWeek([nikujaga])
    withSavedId(savedId)
    respondMe('premium')
    server.use(
      http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
        HttpResponse.json({
          items: [
            {
              name: 'たまねぎ',
              category: 'vegetable',
              origin: 'derived',
              checked: false,
              hidden: true,
              usedIn: [],
            },
            {
              name: 'にんじん',
              category: 'vegetable',
              origin: 'derived',
              checked: false,
              hidden: false,
              usedIn: [],
            },
          ],
        }),
      ),
    )
    const puts: { items: unknown[] }[] = []
    server.use(
      http.put(
        `/api/v1/weekly-menus/${savedId}/shopping-list`,
        async ({ request }) => {
          puts.push((await request.json()) as { items: unknown[] })
          return new HttpResponse(null, { status: 204 })
        },
      ),
    )

    renderWithProviders(<ShoppingListPage />)
    // hidden な品目（たまねぎ）は画面に出ない。
    expect(await screen.findByText('にんじん')).toBeInTheDocument()
    expect(screen.queryByText('たまねぎ')).not.toBeInTheDocument()

    // 別の（表示されている）品目をチェックしても、
    // 送る overlay には hidden な品目が消えずに残っている
    // （そうでないと、次に開いたときに hidden が復活してしまう）。
    await userEvent.click(
      await screen.findByRole('checkbox', { name: /にんじん/ }),
    )
    await waitFor(() => expect(puts.length).toBe(1))
    expect(puts[0].items).toContainEqual({
      name: 'たまねぎ',
      category: 'vegetable',
      origin: 'derived',
      checked: false,
      hidden: true,
    })
    expect(puts[0].items).toContainEqual({
      name: 'にんじん',
      category: 'vegetable',
      origin: 'derived',
      checked: true,
      hidden: false,
    })
  })
})
