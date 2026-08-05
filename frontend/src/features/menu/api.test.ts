import { afterEach, describe, expect, it, vi } from 'vitest'

import { resolveIngredients } from './api'

afterEach(() => {
  vi.restoreAllMocks()
})

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('resolveIngredients', () => {
  it('テキストを送って解決結果を返す', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse({
        resolved: [
          {
            word: '豚こま',
            ingredient: {
              id: 'i1',
              name: '豚こま切れ肉',
              nameKana: 'ぶたこまぎれにく',
              category: 'meat',
            },
          },
        ],
        unresolved: ['オクラ'],
        degraded: false,
      }),
    )

    const got = await resolveIngredients('豚こま、オクラ')

    expect(got.resolved).toHaveLength(1)
    expect(got.resolved[0].ingredient.name).toBe('豚こま切れ肉')
    expect(got.resolved[0].word).toBe('豚こま')
    expect(got.unresolved).toEqual(['オクラ'])
    expect(got.degraded).toBe(false)
    expect(fetchSpy).toHaveBeenCalledOnce()

    // 送信先とボディを確かめる。ここがずれると画面は動くのに解決だけ効かない。
    const [url, init] = fetchSpy.mock.calls[0]
    expect(String(url)).toContain('/ingredients/resolve')
    expect(JSON.parse(String(init?.body))).toEqual({ text: '豚こま、オクラ' })
  })

  it('縮退していても解決できた分は返す', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse({
        resolved: [
          {
            word: '玉ねぎ',
            ingredient: {
              id: 'i2',
              name: '玉ねぎ',
              nameKana: 'たまねぎ',
              category: 'vegetable',
            },
          },
        ],
        unresolved: ['豚こま'],
        degraded: true,
      }),
    )

    const got = await resolveIngredients('玉ねぎ、豚こま')

    expect(got.degraded).toBe(true)
    expect(got.resolved).toHaveLength(1)
    expect(got.unresolved).toEqual(['豚こま'])
  })
})
