import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'

import { NotFoundPage } from './NotFoundPage'

function renderPage() {
  return render(
    <MemoryRouter>
      <NotFoundPage />
    </MemoryRouter>,
  )
}

describe('NotFoundPage', () => {
  it('見つからなかったことを見出しで伝える', () => {
    renderPage()

    expect(
      screen.getByRole('heading', { level: 1, name: 'ページが見つかりません' }),
    ).toBeVisible()
  })

  it('行き止まりにせず、ホームへ戻せる', () => {
    renderPage()

    expect(screen.getByRole('link', { name: 'ホームへ' })).toHaveAttribute(
      'href',
      '/',
    )
  })

  it('こんたてんを出す。URLの誤りは失敗ではなく行き先が無いだけなので', () => {
    const { container } = renderPage()

    // 通信の失敗や不具合とは違い、ここは「何も壊れていない」場面。
    // 迷ったときに迎えてくれる相手が居た方が、行き止まり感が和らぐ。
    expect(container.querySelector('img')).not.toBeNull()
    // ただし絵は装飾。伝えるのは見出しと本文の役目。
    expect(screen.queryAllByRole('img')).toHaveLength(0)
  })
})
