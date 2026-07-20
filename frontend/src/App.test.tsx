import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import App from './App'

// Testing Library が React を描画し、操作を反映できることの確認。
// 画面自体はこの後のPRで作り替えるため、ここでは基盤の疎通だけを見る。
describe('App', () => {
  it('描画できる', () => {
    render(<App />)
    expect(screen.getByRole('heading', { name: 'Get started' })).toBeVisible()
  })

  it('ユーザー操作が状態に反映される', async () => {
    const user = userEvent.setup()
    render(<App />)

    const button = screen.getByRole('button', { name: /Count is 0/ })
    await user.click(button)

    expect(screen.getByRole('button', { name: /Count is 1/ })).toBeVisible()
  })
})
