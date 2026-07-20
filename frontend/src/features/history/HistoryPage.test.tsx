import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { HistoryItem } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { HistoryPage } from './HistoryPage'

function entry(n: number, mode: 'single' | 'weekly' = 'single'): HistoryItem {
  return {
    id: `018f0000-0000-7000-8000-00000000010${n}`,
    menu: {
      id: `018f0000-0000-7000-8000-00000000020${n}`,
      name: `献立${n}`,
      genre: 'japanese',
      difficulty: 'easy',
      description: `説明${n}`,
    },
    searchMode: mode,
    // 表示順はサーバが決める。ここでは新しい順に並んだものを渡す。
    searchedAt: `2026-07-2${n}T10:00:00Z`,
  }
}

function respondHistories(...histories: HistoryItem[]) {
  server.use(
    http.get('/api/v1/histories', () => HttpResponse.json({ histories })),
  )
}

// item は指定の献立名を含む行を返す。行ごとに削除ボタンが並ぶため絞り込む。
function item(name: string) {
  const row = screen.getByRole('listitem', { name: new RegExp(name) })
  return within(row)
}

describe('履歴画面', () => {
  it('サーバが返した順（新しい順）にそのまま並べる', async () => {
    // 並び順はサーバの責務（searched_at DESC, seq DESC）。
    // 画面で並べ替えると、順序の決定が二重になって食い違う。
    respondHistories(entry(3), entry(2), entry(1))
    renderWithProviders(<HistoryPage />)

    const items = await screen.findAllByRole('listitem')
    expect(items).toHaveLength(3)
    expect(items[0]).toHaveTextContent('献立3')
    expect(items[2]).toHaveTextContent('献立1')
  })

  it('検索の種類が分かる', async () => {
    respondHistories(entry(1, 'single'), entry(2, 'weekly'))
    renderWithProviders(<HistoryPage />)

    await screen.findAllByRole('listitem')
    expect(item('献立1').getByText('1食分')).toBeVisible()
    expect(item('献立2').getByText('1週間')).toBeVisible()
  })

  it('0件のときはその旨を伝える', async () => {
    respondHistories()
    renderWithProviders(<HistoryPage />)

    expect(await screen.findByText(/まだ履歴がありません/)).toBeVisible()
    expect(screen.queryByRole('listitem')).not.toBeInTheDocument()
    // 消すものが無いので全件削除は出さない。
    expect(
      screen.queryByRole('button', { name: 'すべて削除' }),
    ).not.toBeInTheDocument()
  })

  it('読み込み中を表示する', async () => {
    let release!: () => void
    const pending = new Promise<void>((resolve) => {
      release = resolve
    })
    server.use(
      http.get('/api/v1/histories', async () => {
        await pending
        return HttpResponse.json({ histories: [entry(1)] })
      }),
    )
    renderWithProviders(<HistoryPage />)

    expect(await screen.findByRole('status')).toHaveTextContent('読み込み')
    release()
    expect(await screen.findByRole('listitem')).toBeVisible()
  })

  it('1件削除できる', async () => {
    const user = userEvent.setup()
    respondHistories(entry(1), entry(2))
    renderWithProviders(<HistoryPage />)
    await screen.findAllByRole('listitem')

    let deletedId: string | undefined
    server.use(
      http.delete('/api/v1/histories/:id', ({ params }) => {
        deletedId = params.id as string
        return new HttpResponse(null, { status: 204 })
      }),
      // 削除後の再取得では消えた状態を返す。
      http.get('/api/v1/histories', () =>
        HttpResponse.json({ histories: [entry(2)] }),
      ),
    )

    await user.click(item('献立1').getByRole('button', { name: '削除' }))

    await waitFor(() =>
      expect(screen.queryByText('献立1')).not.toBeInTheDocument(),
    )
    expect(deletedId).toBe(entry(1).id)
    expect(screen.getByText('献立2')).toBeVisible()
  })

  it('削除に失敗したらメッセージを出し、一覧は残す', async () => {
    const user = userEvent.setup()
    respondHistories(entry(1))
    renderWithProviders(<HistoryPage />)
    await screen.findAllByRole('listitem')

    server.use(
      http.delete('/api/v1/histories/:id', () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/x',
            title: '履歴が見つかりません',
            status: 404,
          },
          { status: 404, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )

    await user.click(item('献立1').getByRole('button', { name: '削除' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '履歴が見つかりません',
    )
    expect(screen.getByText('献立1')).toBeVisible()
  })

  it('全件削除は確認してから消す', async () => {
    const user = userEvent.setup()
    respondHistories(entry(1), entry(2))
    renderWithProviders(<HistoryPage />)
    await screen.findAllByRole('listitem')

    let called = false
    server.use(
      http.delete('/api/v1/histories', () => {
        called = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await user.click(screen.getByRole('button', { name: 'すべて削除' }))

    // 押しただけでは消えない。取り返しがつかない操作なので確認を挟む。
    expect(called).toBe(false)
    expect(screen.getByText('献立1')).toBeVisible()

    server.use(
      http.get('/api/v1/histories', () => HttpResponse.json({ histories: [] })),
    )
    await user.click(screen.getByRole('button', { name: '削除する' }))

    await waitFor(() => expect(called).toBe(true))
    expect(await screen.findByText(/まだ履歴がありません/)).toBeVisible()
  })

  it('全件削除の確認はやめられる', async () => {
    const user = userEvent.setup()
    respondHistories(entry(1))
    renderWithProviders(<HistoryPage />)
    await screen.findAllByRole('listitem')

    let called = false
    server.use(
      http.delete('/api/v1/histories', () => {
        called = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await user.click(screen.getByRole('button', { name: 'すべて削除' }))
    await user.click(screen.getByRole('button', { name: 'やめる' }))

    expect(called).toBe(false)
    expect(screen.getByText('献立1')).toBeVisible()
    expect(screen.getByRole('button', { name: 'すべて削除' })).toBeVisible()
  })

  it('献立の詳細へ遷移できる', async () => {
    respondHistories(entry(1))
    renderWithProviders(<HistoryPage />)
    await screen.findAllByRole('listitem')

    expect(item('献立1').getByRole('link', { name: /レシピ/ })).toHaveAttribute(
      'href',
      `/menus/${entry(1).menu.id}`,
    )
  })

  it('取得に失敗したらメッセージを出す', async () => {
    server.use(
      http.get('/api/v1/histories', () => HttpResponse.error()),
    )
    renderWithProviders(<HistoryPage />)

    expect(await screen.findByRole('alert')).toBeVisible()
  })
})
