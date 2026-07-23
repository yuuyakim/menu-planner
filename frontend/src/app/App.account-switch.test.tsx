import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Menu } from '../api/types'
import { renderWithProviders } from '../test/render'
import { server } from '../test/server'
import { App } from './App'

// アカウントAとBで、それぞれ別の献立をお気に入り／履歴に持たせる。
const menuA: Menu = {
  id: '018f0000-0000-7000-8000-0000000000a1',
  name: 'Aの親子丼',
  genre: 'japanese',
  difficulty: 'easy',
  role: 'main',
  description: 'アカウントAのデータ。',
}
const menuB: Menu = {
  id: '018f0000-0000-7000-8000-0000000000b1',
  name: 'Bのカレー',
  genre: 'western',
  difficulty: 'easy',
  role: 'main',
  description: 'アカウントBのデータ。',
}

function userResponse(id: string, name: string) {
  return { user: { id, email: `${name}@example.com`, displayName: name } }
}

/** asUser は「そのユーザーとしてログイン済み」のサーバ応答に切り替える。 */
function asUser(id: string, name: string, menu: Menu) {
  server.use(
    http.get('/api/v1/auth/me', () => HttpResponse.json(userResponse(id, name))),
    http.get('/api/v1/favorites', () =>
      HttpResponse.json({
        favorites: [{ menu, createdAt: '2026-07-20T10:00:00Z' }],
      }),
    ),
    http.get('/api/v1/histories', () =>
      HttpResponse.json({
        histories: [
          {
            id: `hist-${menu.id}`,
            menu,
            mode: 'single',
            searchedAt: '2026-07-20T10:00:00Z',
          },
        ],
      }),
    ),
  )
}

/** logoutThenLoginAs はログアウトし、別アカウントでログインし直す。 */
async function logoutThenLoginAs(
  user: ReturnType<typeof userEvent.setup>,
  id: string,
  name: string,
  menu: Menu,
) {
  // ログアウト直後に /auth/me の再取得が走るため、押す前に未認証の応答にしておく。
  server.use(
    http.post(
      '/api/v1/auth/logout',
      () => new HttpResponse(null, { status: 204 }),
    ),
    http.get('/api/v1/auth/me', () => new HttpResponse(null, { status: 401 })),
  )
  await user.click(
    within(screen.getByRole('banner')).getByRole('button', {
      name: 'ログアウト',
    }),
  )
  await screen.findByRole('link', { name: 'ログイン' })

  // ここから別アカウントとしてログインする。
  server.use(
    http.post('/api/v1/auth/login', () =>
      HttpResponse.json(userResponse(id, name)),
    ),
  )
  asUser(id, name, menu)

  await user.click(screen.getByRole('link', { name: 'ログイン' }))
  await user.type(await screen.findByLabelText('メールアドレス'), `${name}@example.com`)
  await user.type(screen.getByLabelText('パスワード'), 'password123')
  await user.click(screen.getByRole('button', { name: 'ログイン' }))

  // ヘッダに新しいユーザーの表示名が出たらログイン完了。
  await waitFor(() =>
    expect(within(screen.getByRole('banner')).getByText(name)).toBeVisible(),
  )
}

describe('アカウントを切り替えたとき', () => {
  it('前のユーザーのお気に入りが残らない', async () => {
    const user = userEvent.setup()
    asUser('user-a', 'エー', menuA)
    renderWithProviders(<App />, { route: '/favorites' })

    // Aのお気に入りをキャッシュに載せる。
    expect(await screen.findByText(menuA.name)).toBeVisible()

    await logoutThenLoginAs(user, 'user-b', 'ビー', menuB)

    await user.click(
      within(screen.getByRole('banner')).getByRole('link', {
        name: 'お気に入り',
      }),
    )

    // Bのお気に入りが出て、Aのものは消えていること。
    expect(await screen.findByText(menuB.name)).toBeVisible()
    expect(screen.queryByText(menuA.name)).not.toBeInTheDocument()
  })

  it('前のユーザーの履歴が残らない', async () => {
    const user = userEvent.setup()
    asUser('user-a', 'エー', menuA)
    renderWithProviders(<App />, { route: '/histories' })

    expect(await screen.findByText(menuA.name)).toBeVisible()

    await logoutThenLoginAs(user, 'user-b', 'ビー', menuB)

    await user.click(
      within(screen.getByRole('banner')).getByRole('link', { name: '履歴' }),
    )

    expect(await screen.findByText(menuB.name)).toBeVisible()
    expect(screen.queryByText(menuA.name)).not.toBeInTheDocument()
  })
})

describe('週間献立の持ち越し（Issue #83）', () => {
  // 週間献立は sessionStorage に保存している（レシピを見て戻っても消えないように）。
  // ログアウトしたら、その保存も表示中の値も捨てなければならない。
  it('ログアウトすると週間献立が残らない', async () => {
    const user = userEvent.setup()
    asUser('user-a', 'エー', menuA)
    server.use(
      http.post('/api/v1/menus/suggest-weekly', () =>
        HttpResponse.json({
          week: [{ day: 1, menu: menuA }],
        }),
      ),
    )
    renderWithProviders(<App />, { route: '/weekly' })

    await user.click(
      await screen.findByRole('button', { name: '1週間分を作る' }),
    )
    expect(await screen.findByText(menuA.name)).toBeVisible()

    // 画面を開いたままログアウトする。
    server.use(
      http.post(
        '/api/v1/auth/logout',
        () => new HttpResponse(null, { status: 204 }),
      ),
      http.get('/api/v1/auth/me', () => new HttpResponse(null, { status: 401 })),
    )
    await user.click(
      within(screen.getByRole('banner')).getByRole('button', {
        name: 'ログアウト',
      }),
    )
    await screen.findByRole('link', { name: 'ログイン' })

    // 別画面へ移るまでもなく、その場で消えていること。
    expect(screen.queryByText(menuA.name)).not.toBeInTheDocument()
    expect(sessionStorage.getItem('menu-planner:weekly.week')).toBeNull()
  })
})
