import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { HistoryItem, Menu } from '../api/types'
import { renderWithProviders } from '../test/render'
import { server } from '../test/server'
import { App } from './App'

const menu: Menu = {
  id: '018f0000-0000-7000-8000-000000000001',
  name: '親子丼',
  genre: 'japanese',
  difficulty: 'easy',
  role: 'main',
  description: '鶏肉と卵の定番。',
}

const recorded: HistoryItem = {
  id: '018f0000-0000-7000-8000-000000000101',
  menu,
  searchMode: 'single',
  searchedAt: '2026-07-20T10:00:00Z',
}

// plan は premium にしておく。値自体はもう機能に影響しない（週間献立は
// サブスク撤廃によりログインのみで使える）が、/auth/me は plan を必須で
// 返すため何か設定する。
function loggedIn() {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000009',
          email: 'user@example.com',
          displayName: 'ユーザー',
          plan: 'premium',
        },
      }),
    ),
  )
}

// 検索すると履歴が1件増えるサーバを用意する。
// サーバ側で記録されるので、フロントは取り直さないと古いままになる。
function serverThatRecords() {
  let histories: HistoryItem[] = []
  server.use(
    http.get('/api/v1/histories', () => HttpResponse.json({ histories })),
    http.get('/api/v1/menus/suggest', () => {
      histories = [recorded]
      return HttpResponse.json({ menu })
    }),
    http.get('/api/v1/menus/:id/recipes', () =>
      HttpResponse.json({ recipes: [] }),
    ),
  )
}

describe('検索と履歴のつながり', () => {
  it('検索した献立が、遷移するだけで履歴に出る', async () => {
    const user = userEvent.setup()
    loggedIn()
    serverThatRecords()
    renderWithProviders(<App />, { route: '/histories' })

    // まず履歴を開く。この時点では空で、その結果がキャッシュに載る。
    expect(await screen.findByText(/まだ履歴がありません/)).toBeVisible()

    // 検索する（サーバ側で履歴に記録される）。
    await user.click(screen.getByRole('link', { name: '献立を探す' }))
    await user.click(screen.getByRole('button', { name: '献立を探す' }))
    expect(await screen.findByRole('article')).toBeVisible()

    // 履歴へ戻る。リロードせずに反映されていること。
    await user.click(screen.getByRole('link', { name: '履歴' }))

    expect(await screen.findByRole('listitem')).toHaveTextContent('親子丼')
    expect(screen.queryByText(/まだ履歴がありません/)).not.toBeInTheDocument()
  })

  it('週間献立を作っても履歴に出る', async () => {
    const user = userEvent.setup()
    loggedIn()
    let histories: HistoryItem[] = []
    server.use(
      http.get('/api/v1/histories', () => HttpResponse.json({ histories })),
      http.post('/api/v1/menus/suggest-weekly', () => {
        histories = [recorded]
        return HttpResponse.json({
          week: Array.from({ length: 7 }, (_, i) => ({ day: i + 1, menu })),
        })
      }),
    )
    renderWithProviders(<App />, { route: '/histories' })

    expect(await screen.findByText(/まだ履歴がありません/)).toBeVisible()

    await user.click(screen.getByRole('link', { name: '1週間の献立' }))
    // /auth/me の応答が解決するまでは生成ボタンがまだ無い。
    await user.click(await screen.findByRole('button', { name: '1週間分を作る' }))
    await screen.findAllByRole('listitem')

    await user.click(screen.getByRole('link', { name: '履歴' }))

    expect(await screen.findByRole('listitem')).toHaveTextContent('親子丼')
  })
})
