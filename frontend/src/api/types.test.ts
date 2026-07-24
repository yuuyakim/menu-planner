import { describe, expect, it } from 'vitest'

import {
  difficulties,
  difficultyLabels,
  genreLabels,
  genres,
  type Menu,
} from './types'

// 生成された型が期待どおり使えることの確認。
// 型そのものは tsc が見るので、ここでは値の網羅性を確かめる。
describe('API型', () => {
  it('ジャンル4種のラベルが揃っている', () => {
    expect(genres).toEqual(['japanese', 'western', 'chinese', 'other'])
    expect(genreLabels.japanese).toBe('和食')
  })

  it('難易度3種のラベルが揃っている', () => {
    expect(difficulties).toEqual(['easy', 'normal', 'elaborate'])
    expect(difficultyLabels.elaborate).toBe('手が込んだ')
  })

  it('生成された Menu 型でオブジェクトを組み立てられる', () => {
    // 生成物の形が変われば、この代入が型エラーになる（tsc が検知する）。
    const menu: Menu = {
      id: '018f0000-0000-7000-8000-000000000001',
      name: '親子丼',
      genre: 'japanese',
      difficulty: 'easy',
      role: 'main',
      description: '鶏肉と卵の定番。',
    }
    expect(menu.genre).toBe('japanese')
  })
})
