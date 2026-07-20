import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { SearchForm } from './SearchForm'

// group はラジオのまとまりを名前で取り出す。
// 「すべて」は両方の group にあるため、group で絞らないと取り違える。
function group(name: string) {
  return within(screen.getByRole('group', { name }))
}

describe('検索フォーム', () => {
  it('ジャンル4種と「すべて」を表示する', () => {
    renderWithProviders(<SearchForm onSubmit={vi.fn()} />)

    const genre = group('ジャンル')
    for (const label of ['すべて', '和食', '洋食', '中華', 'その他']) {
      expect(genre.getByRole('radio', { name: label })).toBeVisible()
    }
    expect(genre.getAllByRole('radio')).toHaveLength(5)
  })

  it('難易度3種と「すべて」を表示する', () => {
    renderWithProviders(<SearchForm onSubmit={vi.fn()} />)

    const difficulty = group('難易度')
    for (const label of ['すべて', '簡単', '普通', '手が込んだ']) {
      expect(difficulty.getByRole('radio', { name: label })).toBeVisible()
    }
    expect(difficulty.getAllByRole('radio')).toHaveLength(4)
  })

  it('初期状態はどちらも「すべて」が選ばれている', () => {
    renderWithProviders(<SearchForm onSubmit={vi.fn()} />)

    expect(group('ジャンル').getByRole('radio', { name: 'すべて' })).toBeChecked()
    expect(group('難易度').getByRole('radio', { name: 'すべて' })).toBeChecked()
  })

  it('未選択のまま検索すると絞り込みなしで送る', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderWithProviders(<SearchForm onSubmit={onSubmit} />)

    await user.click(screen.getByRole('button', { name: '献立を探す' }))

    // 「すべて」は絞り込まないことを表す。空文字ではなく undefined で送り、
    // クエリに genre= を出さない。
    expect(onSubmit).toHaveBeenCalledWith({
      genre: undefined,
      difficulty: undefined,
    })
  })

  it('選んだ条件を送る', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderWithProviders(<SearchForm onSubmit={onSubmit} />)

    await user.click(group('ジャンル').getByRole('radio', { name: '和食' }))
    await user.click(group('難易度').getByRole('radio', { name: '簡単' }))
    await user.click(screen.getByRole('button', { name: '献立を探す' }))

    expect(onSubmit).toHaveBeenCalledWith({
      genre: 'japanese',
      difficulty: 'easy',
    })
  })

  it('片方だけ選んでも送れる', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderWithProviders(<SearchForm onSubmit={onSubmit} />)

    await user.click(group('ジャンル').getByRole('radio', { name: '中華' }))
    await user.click(screen.getByRole('button', { name: '献立を探す' }))

    expect(onSubmit).toHaveBeenCalledWith({
      genre: 'chinese',
      difficulty: undefined,
    })
  })

  it('選び直して「すべて」に戻せる', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    renderWithProviders(<SearchForm onSubmit={onSubmit} />)

    await user.click(group('ジャンル').getByRole('radio', { name: '洋食' }))
    await user.click(group('ジャンル').getByRole('radio', { name: 'すべて' }))
    await user.click(screen.getByRole('button', { name: '献立を探す' }))

    expect(onSubmit).toHaveBeenCalledWith({
      genre: undefined,
      difficulty: undefined,
    })
  })

  it('検索中はボタンを押せない', () => {
    renderWithProviders(<SearchForm onSubmit={vi.fn()} isPending />)

    expect(screen.getByRole('button', { name: '検索中…' })).toBeDisabled()
  })
})
