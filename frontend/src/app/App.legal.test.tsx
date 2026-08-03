import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../test/render'
import { App } from './App'

// 法務2ページは未ログインでも見えて欲しい（RequireAuth で包まない）。
// テストは既定のハンドラ（未ログイン）のまま実行し、リダイレクトされず
// 各ページの見出しが出ることを確認する。
// アサートは安定部分文字列のみ。「（草案）」などは Task 4 で整形されるため使わない。
describe('法務ページ（未ログイン）', () => {
  it('/legal/terms で利用規約を表示する', () => {
    renderWithProviders(<App />, { route: '/legal/terms' })

    expect(screen.getByRole('heading', { name: /利用規約/ })).toBeVisible()
    expect(
      screen.queryByRole('heading', { level: 1, name: 'ログイン' }),
    ).not.toBeInTheDocument()
  })

  it('/legal/privacy でプライバシーポリシーを表示する', () => {
    renderWithProviders(<App />, { route: '/legal/privacy' })

    expect(
      screen.getByRole('heading', { name: /プライバシーポリシー/ }),
    ).toBeVisible()
    expect(
      screen.queryByRole('heading', { level: 1, name: 'ログイン' }),
    ).not.toBeInTheDocument()
  })
})
