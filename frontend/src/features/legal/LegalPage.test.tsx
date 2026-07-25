import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { LegalPage } from './LegalPage'

describe('LegalPage', () => {
  it('md の見出し・本文・GFMテーブルを描画する', () => {
    const markdown =
      '# 見出し\n\n本文\n\n| a | b |\n|---|---|\n| 1 | 2 |'

    renderWithProviders(<LegalPage markdown={markdown} />)

    expect(
      screen.getByRole('heading', { name: '見出し' }),
    ).toBeVisible()
    expect(screen.getByText('本文')).toBeVisible()

    // remark-gfm が効いてテーブルとして描画されることを確認する。
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeVisible()
  })
})
