import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { MascotEmpty } from './MascotEmpty'

describe('MascotEmpty', () => {
  it('何が無いのかを文言で伝える', () => {
    render(<MascotEmpty>まだ履歴がありません。</MascotEmpty>)

    expect(screen.getByText('まだ履歴がありません。')).toBeVisible()
  })

  it('こんたてんは装飾で、読み上げ名に混ざらない', () => {
    render(<MascotEmpty>まだ履歴がありません。</MascotEmpty>)

    expect(screen.queryAllByRole('img')).toHaveLength(0)
  })

  it('空なだけで異常ではないので status にしない', () => {
    render(<MascotEmpty>まだ履歴がありません。</MascotEmpty>)

    // role=status にすると画面を開くたびに読み上げが割り込む。
    // 空は通常の状態であって、知らせるべき出来事ではない。
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
