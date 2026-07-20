import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../test/render'
import { App } from './App'

// ルーティングの骨組みの確認。各画面の中身は後続のPRで作る。
describe('App', () => {
  it('/ で検索画面を表示する', () => {
    renderWithProviders(<App />, { route: '/' })
    expect(
      screen.getByRole('heading', { level: 1, name: '献立を探す' }),
    ).toBeVisible()
  })

  it('ヘッダのリンクで履歴へ遷移できる', async () => {
    const user = userEvent.setup()
    renderWithProviders(<App />, { route: '/' })

    await user.click(screen.getByRole('link', { name: '履歴' }))

    expect(
      screen.getByRole('heading', { level: 1, name: '検索履歴' }),
    ).toBeVisible()
  })

  it('ヘッダのリンクでお気に入りへ遷移できる', async () => {
    const user = userEvent.setup()
    renderWithProviders(<App />, { route: '/' })

    await user.click(screen.getByRole('link', { name: 'お気に入り' }))

    expect(
      screen.getByRole('heading', { level: 1, name: 'お気に入り' }),
    ).toBeVisible()
  })

  it('/login でログイン画面を表示する', () => {
    renderWithProviders(<App />, { route: '/login' })
    expect(
      screen.getByRole('heading', { level: 1, name: 'ログイン' }),
    ).toBeVisible()
  })

  it('未知のパスでは404を表示する', () => {
    renderWithProviders(<App />, { route: '/no-such-page' })
    expect(
      screen.getByRole('heading', { level: 1, name: 'ページが見つかりません' }),
    ).toBeVisible()
  })

  it('どの画面でもヘッダは残る', () => {
    renderWithProviders(<App />, { route: '/no-such-page' })
    expect(screen.getByRole('banner')).toBeVisible()
    expect(screen.getByRole('link', { name: '履歴' })).toBeVisible()
  })
})
