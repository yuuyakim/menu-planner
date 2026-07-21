import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { ApiError } from '../api/client'
import { ErrorMessage } from './ErrorMessage'

describe('ErrorMessage', () => {
  it('失敗を alert として伝える', () => {
    render(<ErrorMessage error={new Error('通信に失敗しました')} />)

    expect(screen.getByRole('alert')).toHaveTextContent('通信に失敗しました')
  })

  it('401 はログインし直すよう添える', () => {
    render(<ErrorMessage error={new ApiError(401, { title: '認証が必要です' })} />)

    expect(screen.getByRole('alert')).toHaveTextContent('ログインし直して')
  })

  it('想定外の例外は中身を出さない', () => {
    render(<ErrorMessage error={{ secret: 'internal' }} />)

    expect(screen.getByRole('alert')).toHaveTextContent(
      '予期しないエラーが発生しました。',
    )
  })

  it('失敗の表示にこんたてんは出さない', () => {
    render(<ErrorMessage error={new Error('通信に失敗しました')} />)

    // マスコットは「問題が起きていない場面」（待ち時間・空・404）にだけ出す。
    // 失敗に絵を添えると、うまくいかなかったことを軽く扱っているように見える。
    expect(screen.queryAllByRole('img')).toHaveLength(0)
    expect(document.querySelector('img')).toBeNull()
  })
})
