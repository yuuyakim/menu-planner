import { screen } from '@testing-library/react'
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

    expect(await screen.findByText('親子丼')).toBeVisible()
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
})
