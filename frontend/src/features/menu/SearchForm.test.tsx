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
      role: 'main',
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
      role: 'main',
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
      role: 'main',
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
      role: 'main',
    })
  })

  it('検索中はボタンを押せない', () => {
    renderWithProviders(<SearchForm onSubmit={vi.fn()} isPending />)

    expect(screen.getByRole('button', { name: '検索中…' })).toBeDisabled()
  })
})

// 役割の絞り込み（spec.md 2.10）。
// ジャンル・難易度と違い、既定が「すべて」ではなく「主菜」であることが要。
describe('検索フォーム: 役割', () => {
  it('役割3種と「すべて」を表示する', () => {
    renderWithProviders(<SearchForm onSubmit={vi.fn()} />)

    const role = group('種類')
    for (const label of ['主菜', '副菜', '汁物', 'すべて']) {
      expect(role.getByRole('radio', { name: label })).toBeVisible()
    }
    expect(role.getAllByRole('radio')).toHaveLength(4)
  })

  it('初期状態は「主菜」が選ばれている', () => {
    renderWithProviders(<SearchForm onSubmit={vi.fn()} />)

    // 未指定で副菜が単品提案されるのを避けるため、既定を主菜に倒している。
    expect(group('種類').getByRole('radio', { name: '主菜' })).toBeChecked()
    expect(group('種類').getByRole('radio', { name: 'すべて' })).not.toBeChecked()
  })

  it('何も触らずに送ると主菜で検索する', async () => {
    const onSubmit = vi.fn()
    renderWithProviders(<SearchForm onSubmit={onSubmit} />)

    await userEvent.click(screen.getByRole('button', { name: '献立を探す' }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ role: 'main' }),
    )
  })

  it('選んだ役割が検索条件に乗る', async () => {
    const onSubmit = vi.fn()
    renderWithProviders(<SearchForm onSubmit={onSubmit} />)

    await userEvent.click(group('種類').getByRole('radio', { name: '副菜' }))
    await userEvent.click(screen.getByRole('button', { name: '献立を探す' }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ role: 'side' }),
    )
  })

  it('「すべて」を選ぶと all を送る', async () => {
    const onSubmit = vi.fn()
    renderWithProviders(<SearchForm onSubmit={onSubmit} />)

    await userEvent.click(group('種類').getByRole('radio', { name: 'すべて' }))
    await userEvent.click(screen.getByRole('button', { name: '献立を探す' }))

    // undefined ではなく all を明示して送る。省略はサーバ側で主菜になるため。
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ role: 'all' }),
    )
  })
})
