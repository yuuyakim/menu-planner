import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../test/render'
import { App } from './App'

// 料金ページは未ログインでも見えて欲しい（RequireAuth で包まない）。
// 加入を検討する前に見る画面であり、ログインを要求すると意味を成さない。
describe('料金ページ（未ログイン）', () => {
  it('/pricing で料金プランを表示し、ログイン画面へ送らない', async () => {
    renderWithProviders(<App />, { route: '/pricing' })

    expect(
      await screen.findByRole('heading', { level: 1, name: '料金プラン' }),
    ).toBeVisible()
    expect(
      screen.queryByRole('heading', { level: 1, name: 'ログイン' }),
    ).not.toBeInTheDocument()
  })
})
