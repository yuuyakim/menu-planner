import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ErrorBoundary } from './ErrorBoundary'

// Boom は描画時に必ず throw するコンポーネント。境界の捕捉を試すために使う。
function Boom(): never {
  throw new Error('描画に失敗しました')
}

describe('ErrorBoundary', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('エラーが無ければ子をそのまま描画する', () => {
    render(
      <ErrorBoundary>
        <p>正常な中身</p>
      </ErrorBoundary>,
    )
    expect(screen.getByText('正常な中身')).toBeVisible()
  })

  it('子が描画時にエラーを投げたらフォールバックを表示する', () => {
    // 境界が捕捉してもReactはエラーをコンソールに出す。テスト出力を汚さないよう黙らせる。
    vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    )

    // 復帰導線付きのフォールバック（role=alert）が出る。
    expect(screen.getByRole('alert')).toBeVisible()
    expect(
      screen.getByRole('heading', { name: '問題が発生しました' }),
    ).toBeVisible()
    // 投げた子の中身は出さない。
    expect(screen.queryByText('描画に失敗しました')).not.toBeInTheDocument()
  })

  it('フォールバックに再読み込みの導線がある', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    )
    expect(screen.getByRole('button', { name: '再読み込み' })).toBeVisible()
  })

  it('fallback を渡すとそれを表示する', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    render(
      <ErrorBoundary fallback={<p>専用のフォールバック</p>}>
        <Boom />
      </ErrorBoundary>,
    )
    expect(screen.getByText('専用のフォールバック')).toBeVisible()
  })
})
