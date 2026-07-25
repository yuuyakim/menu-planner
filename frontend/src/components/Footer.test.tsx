import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../test/render'
import { Footer } from './Footer'

// フッターは全ページ共通で、法務3ページへの導線を切らさないための要。
// 表示義務のあるページへ確実にたどり着けるよう、hrefを直接検証する。
describe('Footer', () => {
  it('特定商取引法に基づく表記へのリンクを持つ', () => {
    renderWithProviders(<Footer />)

    expect(
      screen.getByRole('link', { name: '特定商取引法に基づく表記' }),
    ).toHaveAttribute('href', '/legal/tokushoho')
  })

  it('利用規約へのリンクを持つ', () => {
    renderWithProviders(<Footer />)

    expect(screen.getByRole('link', { name: '利用規約' })).toHaveAttribute(
      'href',
      '/legal/terms',
    )
  })

  it('プライバシーポリシーへのリンクを持つ', () => {
    renderWithProviders(<Footer />)

    expect(
      screen.getByRole('link', { name: 'プライバシーポリシー' }),
    ).toHaveAttribute('href', '/legal/privacy')
  })
})
