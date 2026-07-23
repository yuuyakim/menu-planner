import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { Menu, Role } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { MenuCard } from './MenuCard'

function menu(overrides: Partial<Menu> = {}): Menu {
  return {
    id: '018f0000-0000-7000-8000-000000000001',
    name: '親子丼',
    genre: 'japanese',
    difficulty: 'easy',
    role: 'main',
    description: '鶏肉と卵の定番。',
    ...overrides,
  }
}

describe('献立カード', () => {
  it('ジャンル・難易度・役割を並べる', () => {
    renderWithProviders(<MenuCard menu={menu()} />)

    expect(screen.getByText('和食')).toBeVisible()
    expect(screen.getByText('簡単')).toBeVisible()
    expect(screen.getByText('主菜')).toBeVisible()
  })

  // 「すべて」で引くと主菜以外も混ざる。何が出たのかカードだけで分かる必要がある。
  it.each<[Role, string]>([
    ['main', '主菜'],
    ['side', '副菜'],
    ['soup', '汁物'],
  ])('役割 %s を %s と表示する', (role, label) => {
    renderWithProviders(<MenuCard menu={menu({ role })} />)

    expect(screen.getByText(label)).toBeVisible()
  })
})
