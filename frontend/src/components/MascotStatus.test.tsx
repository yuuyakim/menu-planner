import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { MascotStatus } from './MascotStatus'

describe('MascotStatus', () => {
  it('待っていることを status として伝える', () => {
    render(<MascotStatus>検索中…</MascotStatus>)

    expect(screen.getByRole('status')).toHaveTextContent('検索中…')
  })

  it('こんたてんは装飾で、読み上げ名に混ざらない', () => {
    render(<MascotStatus>作成中…</MascotStatus>)

    // 絵に alt を付けると、読み上げが文言の前にキャラの説明を挟む。
    // 状態を伝えるのは文言の役目に留める。
    expect(screen.queryAllByRole('img')).toHaveLength(0)
  })

  it('場面に合わせて絵を差し替えられる', () => {
    const { container } = render(
      <MascotStatus image="/mascot/pose-dish.png">作成中…</MascotStatus>,
    )

    expect(container.querySelector('img')).toHaveAttribute(
      'src',
      '/mascot/pose-dish.png',
    )
  })
})
