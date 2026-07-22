import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import type { Ingredient } from '../../api/types'
import { IngredientPicker } from './IngredientPicker'

function ingredient(
  id: string,
  name: string,
  category: Ingredient['category'],
): Ingredient {
  return { id, name, nameKana: name, category }
}

const items: Ingredient[] = [
  ingredient('i1', 'じゃがいも', 'vegetable'),
  ingredient('i2', '玉ねぎ', 'vegetable'),
  ingredient('i3', '牛肉', 'meat'),
  ingredient('i4', '鮭', 'seafood'),
]

function renderPicker(selected: string[] = []) {
  const onToggle = vi.fn()
  const onClear = vi.fn()
  render(
    <IngredientPicker
      ingredients={items}
      selected={new Set(selected)}
      onToggle={onToggle}
      onClear={onClear}
    />,
  )
  return { onToggle, onClear }
}

// group は指定カテゴリの領域を返す。カテゴリをまたいだ取り違えを防ぐ。
function group(name: string) {
  return within(screen.getByRole('group', { name }))
}

describe('食材ピッカー', () => {
  it('カテゴリごとに分かれて並ぶ', () => {
    renderPicker()

    // 売り場を回る順（野菜→肉→魚介）。買い物リストと同じ並び。
    expect(group('野菜').getByLabelText('じゃがいも')).toBeInTheDocument()
    expect(group('野菜').getByLabelText('玉ねぎ')).toBeInTheDocument()
    expect(group('肉').getByLabelText('牛肉')).toBeInTheDocument()
    expect(group('魚介').getByLabelText('鮭')).toBeInTheDocument()
  })

  it('該当が無いカテゴリの見出しは出さない', () => {
    renderPicker()

    // 卵・乳製品／主食／その他は今回の選択肢に無い。空の見出しだけ並ぶと探しにくい。
    expect(screen.queryByRole('group', { name: '卵・乳製品' })).not.toBeInTheDocument()
    expect(screen.queryByRole('group', { name: '主食' })).not.toBeInTheDocument()
  })

  it('選ぶと onToggle に食材IDを渡す', async () => {
    const user = userEvent.setup()
    const { onToggle } = renderPicker()

    await user.click(group('野菜').getByLabelText('玉ねぎ'))

    expect(onToggle).toHaveBeenCalledExactlyOnceWith('i2')
  })

  it('選択済みを押すと解除として同じ経路を通る', async () => {
    const user = userEvent.setup()
    const { onToggle } = renderPicker(['i2'])

    expect(group('野菜').getByLabelText('玉ねぎ')).toBeChecked()
    await user.click(group('野菜').getByLabelText('玉ねぎ'))

    // 選択・解除は同じ onToggle。状態の持ち主は親。
    expect(onToggle).toHaveBeenCalledExactlyOnceWith('i2')
  })

  it('選択中の件数を伝える', () => {
    renderPicker(['i1', 'i3'])

    expect(screen.getByRole('status')).toHaveTextContent('2個を選択中')
  })

  it('1つも選んでいなければ何をすればよいか示す', () => {
    renderPicker()

    expect(screen.getByRole('status')).toHaveTextContent(
      '冷蔵庫にあるものを選んでください',
    )
    // 外すものが無いので、解除の導線は出さない。
    expect(
      screen.queryByRole('button', { name: '選択をすべて外す' }),
    ).not.toBeInTheDocument()
  })

  it('まとめて外せる', async () => {
    const user = userEvent.setup()
    const { onClear } = renderPicker(['i1', 'i2', 'i3'])

    await user.click(screen.getByRole('button', { name: '選択をすべて外す' }))

    // 1つずつ外させると、選び直しのたびに手間が増える。
    expect(onClear).toHaveBeenCalledOnce()
  })
})
