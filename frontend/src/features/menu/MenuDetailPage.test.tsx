import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { Route, Routes } from 'react-router'

import type { Menu } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { MenuDetailPage } from './MenuDetailPage'

const menuId = '018f0000-0000-7000-8000-000000000001'

const oyakodon: Menu = {
  id: menuId,
  name: '親子丼',
  genre: 'japanese',
  difficulty: 'easy',
  role: 'main',
  description: '鶏肉と卵の定番。',
}

// パスパラメータから献立IDを読むため、ルート付きで描画する。
function renderAt(id: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/menus/:id" element={<MenuDetailPage />} />
    </Routes>,
    { route: `/menus/${id}` },
  )
}

describe('献立の詳細', () => {
  it('献立とレシピを表示する', async () => {
    server.use(
      http.get(`/api/v1/menus/${menuId}`, () =>
        HttpResponse.json({ menu: oyakodon }),
      ),
      http.get(`/api/v1/menus/${menuId}/recipes`, () =>
        HttpResponse.json({
          recipes: [
            {
              title: '親子丼のレシピ',
              url: 'https://example.com/r/1',
              domain: 'example.com',
              snippet: '作り方',
            },
          ],
        }),
      ),
    )

    renderAt(menuId)

    // 何のページかが見出しから分かるよう、献立名を h1 にする。
    expect(
      await screen.findByRole('heading', { level: 1, name: '親子丼' }),
    ).toBeVisible()
    expect(await screen.findByRole('link', { name: /親子丼のレシピ/ })).toBeVisible()
  })

  it('存在しない献立はメッセージを出す', async () => {
    server.use(
      http.get(`/api/v1/menus/${menuId}`, () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/x',
            title: '献立が見つかりません',
            status: 404,
          },
          { status: 404, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )

    renderAt(menuId)

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '献立が見つかりません',
    )
  })

  it('戻る導線は来た画面に戻る', async () => {
    const user = userEvent.setup()
    server.use(
      http.get(`/api/v1/menus/${menuId}`, () =>
        HttpResponse.json({ menu: oyakodon }),
      ),
      http.get(`/api/v1/menus/${menuId}/recipes`, () =>
        HttpResponse.json({ recipes: [] }),
      ),
    )

    renderWithProviders(
      <Routes>
        <Route path="/weekly" element={<h1>1週間の献立</h1>} />
        <Route path="/menus/:id" element={<MenuDetailPage />} />
      </Routes>,
      { history: ['/weekly', `/menus/${menuId}`] },
    )

    await screen.findByText('親子丼')
    await user.click(screen.getByRole('button', { name: /戻る/ }))

    // 週間献立から来たら週間献立へ。検索画面へ固定で飛ばさない。
    expect(
      await screen.findByRole('heading', { name: '1週間の献立' }),
    ).toBeVisible()
  })

  it('直接開いたときは検索画面への導線を出す', async () => {
    server.use(
      http.get(`/api/v1/menus/${menuId}`, () =>
        HttpResponse.json({ menu: oyakodon }),
      ),
      http.get(`/api/v1/menus/${menuId}/recipes`, () =>
        HttpResponse.json({ recipes: [] }),
      ),
    )

    renderAt(menuId)

    await screen.findByText('親子丼')
    // 戻る先の履歴が無いので、戻るボタンではなくリンクを出す。
    expect(
      screen.queryByRole('button', { name: /戻る/ }),
    ).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /献立を探す/ })).toHaveAttribute(
      'href',
      '/search',
    )
  })
})
