import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Menu } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { SearchPage } from './SearchPage'

const oyakodon: Menu = {
  id: '018f0000-0000-7000-8000-000000000001',
  name: '親子丼',
  genre: 'japanese',
  difficulty: 'easy',
  description: '鶏肉と卵の定番。',
}

const curry: Menu = {
  id: '018f0000-0000-7000-8000-000000000002',
  name: 'カレーライス',
  genre: 'western',
  difficulty: 'easy',
  description: 'みんな好きなやつ。',
}

// respondWith は /menus/suggest が順に返す献立を仕込む。
// 呼ばれたURLも記録し、条件がクエリに乗るかを確かめられるようにする。
function respondWith(...menus: Menu[]) {
  const urls: string[] = []
  let call = 0
  server.use(
    http.get('/api/v1/menus/suggest', ({ request }) => {
      urls.push(request.url)
      const menu = menus[Math.min(call, menus.length - 1)]
      call += 1
      return HttpResponse.json({ menu })
    }),
  )
  return urls
}

// problem は RFC 7807 の失敗応答を返す。
function respondProblem(status: number, title: string) {
  server.use(
    http.get('/api/v1/menus/suggest', () =>
      HttpResponse.json(
        { type: 'https://example.com/probs/x', title, status },
        { status, headers: { 'Content-Type': 'application/problem+json' } },
      ),
    ),
  )
}

function search() {
  return screen.getByRole('button', { name: '献立を探す' })
}

describe('検索結果', () => {
  it('検索ボタンで結果が表示される', async () => {
    const user = userEvent.setup()
    respondWith(oyakodon)
    renderWithProviders(<SearchPage />)

    // 検索前は結果を出さない。
    expect(screen.queryByRole('article')).not.toBeInTheDocument()

    await user.click(search())

    const result = await screen.findByRole('article')
    expect(within(result).getByText('親子丼')).toBeVisible()
    expect(within(result).getByText('鶏肉と卵の定番。')).toBeVisible()
    expect(within(result).getByText('和食')).toBeVisible()
    expect(within(result).getByText('簡単')).toBeVisible()
  })

  it('選んだ条件をクエリに載せる', async () => {
    const user = userEvent.setup()
    const urls = respondWith(oyakodon)
    renderWithProviders(<SearchPage />)

    const genre = within(screen.getByRole('group', { name: 'ジャンル' }))
    await user.click(genre.getByRole('radio', { name: '和食' }))
    await user.click(search())

    await screen.findByRole('article')
    expect(urls[0]).toContain('genre=japanese')
    // 「すべて」の難易度はクエリに出さない。
    expect(urls[0]).not.toContain('difficulty=')
  })

  it('検索中はローディングを表示する', async () => {
    const user = userEvent.setup()
    // 応答を保留し、読み込み中の状態を観測できるようにする。
    let release!: () => void
    const pending = new Promise<void>((resolve) => {
      release = resolve
    })
    server.use(
      http.get('/api/v1/menus/suggest', async () => {
        await pending
        return HttpResponse.json({ menu: oyakodon })
      }),
    )
    renderWithProviders(<SearchPage />)

    await user.click(search())

    expect(await screen.findByRole('status')).toHaveTextContent('検索中')
    expect(screen.getByRole('button', { name: '検索中…' })).toBeDisabled()

    release()
    await screen.findByRole('article')
    // レシピ欄も読み込み中に role=status を出すため、検索中の表示だけを見る。
    expect(screen.queryByText('検索中…')).not.toBeInTheDocument()
  })

  it('検索中のこんたてんは装飾で、待っていることは文言が伝える', async () => {
    const user = userEvent.setup()
    let release!: () => void
    const pending = new Promise<void>((resolve) => {
      release = resolve
    })
    server.use(
      http.get('/api/v1/menus/suggest', async () => {
        await pending
        return HttpResponse.json({ menu: oyakodon })
      }),
    )
    renderWithProviders(<SearchPage />)

    await user.click(search())

    // 絵に alt を付けると、読み上げが「検索中」の前にキャラの説明を挟む。
    // 状態を伝えるのは文言の役目に留める。
    const status = await screen.findByRole('status')
    expect(status).toHaveTextContent('検索中')
    expect(screen.queryAllByRole('img')).toHaveLength(0)

    release()
    await screen.findByRole('article')
  })

  it('422 は条件に合う献立が無いことを伝える', async () => {
    const user = userEvent.setup()
    respondProblem(422, '条件に合う献立が見つかりません')
    renderWithProviders(<SearchPage />)

    await user.click(search())

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('条件に合う献立が見つかりません')
    // 失敗しても検索はやり直せる。
    expect(search()).toBeEnabled()
  })

  it('通信エラーでもメッセージを表示する', async () => {
    const user = userEvent.setup()
    server.use(http.get('/api/v1/menus/suggest', () => HttpResponse.error()))
    renderWithProviders(<SearchPage />)

    await user.click(search())

    const alert = await screen.findByRole('alert')
    expect(alert.textContent).not.toBe('')
  })

  it('「別の献立を見る」で引き直せる', async () => {
    const user = userEvent.setup()
    const urls = respondWith(oyakodon, curry)
    renderWithProviders(<SearchPage />)

    const genre = within(screen.getByRole('group', { name: 'ジャンル' }))
    await user.click(genre.getByRole('radio', { name: '和食' }))
    await user.click(search())
    expect(await screen.findByText('親子丼')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '別の献立を見る' }))

    await waitFor(() => expect(screen.getByText('カレーライス')).toBeVisible())
    expect(screen.queryByText('親子丼')).not.toBeInTheDocument()
    // 引き直しは同じ条件で行う。フォームを操作し直す必要はない。
    expect(urls).toHaveLength(2)
    expect(urls[1]).toContain('genre=japanese')
  })

  it('引き直しに失敗したらメッセージを出す', async () => {
    const user = userEvent.setup()
    respondWith(oyakodon)
    renderWithProviders(<SearchPage />)

    await user.click(search())
    await screen.findByText('親子丼')

    respondProblem(422, '条件に合う献立が見つかりません')
    await user.click(screen.getByRole('button', { name: '別の献立を見る' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '条件に合う献立が見つかりません',
    )
  })

  it('レシピ取得が502でも献立の表示は消えない', async () => {
    const user = userEvent.setup()
    respondWith(oyakodon)
    server.use(
      http.get(`/api/v1/menus/${oyakodon.id}/recipes`, () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/x',
            title: 'レシピの取得に失敗しました',
            status: 502,
          },
          { status: 502, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    renderWithProviders(<SearchPage />)

    await user.click(search())
    expect(await screen.findByText('親子丼')).toBeVisible()

    // レシピ欄だけが失敗し、献立とその引き直し導線は残る。
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'レシピの取得に失敗しました',
    )
    expect(screen.getByText('親子丼')).toBeVisible()
    expect(screen.getByRole('button', { name: '別の献立を見る' })).toBeVisible()
    expect(screen.getByRole('button', { name: '再試行' })).toBeVisible()
  })

  it('お気に入りの星は献立カードの中に置く', async () => {
    const user = userEvent.setup()
    respondWith(oyakodon)
    server.use(
      http.get('/api/v1/auth/me', () =>
        HttpResponse.json({
          user: {
            id: '018f0000-0000-7000-8000-000000000009',
            email: 'user@example.com',
            displayName: 'ユーザー',
          },
        }),
      ),
      http.get('/api/v1/favorites', () => HttpResponse.json({ favorites: [] })),
    )
    renderWithProviders(<SearchPage />)

    await user.click(search())
    const card = await screen.findByRole('article')

    // カードの外（下）ではなく中に置く。縦に積むとカードが高くなる。
    expect(
      within(card).getByRole('button', { name: 'お気に入りに追加' }),
    ).toBeVisible()
  })
})
