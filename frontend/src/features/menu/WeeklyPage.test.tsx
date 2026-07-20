import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import type { Menu } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { WeeklyPage } from './WeeklyPage'

// 2026-07-20 は月曜。起点を固定して曜日表示を確かめられるようにする。
const monday = new Date(2026, 6, 20)

function menu(n: number): Menu {
  return {
    id: `018f0000-0000-7000-8000-00000000000${n}`,
    name: `献立${n}`,
    genre: 'japanese',
    difficulty: 'easy',
    description: `説明${n}`,
  }
}

function week() {
  return Array.from({ length: 7 }, (_, i) => ({ day: i + 1, menu: menu(i + 1) }))
}

// respondWeekly は週間献立の生成を仕込み、送られた本文を記録する。
function respondWeekly() {
  const bodies: unknown[] = []
  server.use(
    http.post('/api/v1/menus/suggest-weekly', async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ week: week() })
    }),
  )
  return bodies
}

// respondReroll は引き直しを仕込み、送られた本文を記録する。
function respondReroll(replacement: Menu) {
  const bodies: { day?: number; week?: string[] }[] = []
  server.use(
    http.post('/api/v1/menus/reroll-day', async ({ request }) => {
      bodies.push((await request.json()) as { day: number; week: string[] })
      return HttpResponse.json({ menu: replacement })
    }),
  )
  return bodies
}

function create() {
  return screen.getByRole('button', { name: '1週間分を作る' })
}

// dayItem は指定の日の領域を返す。日ごとにボタンが並ぶため、絞らないと取り違える。
function dayItem(day: number) {
  return within(screen.getByRole('listitem', { name: new RegExp(`^${day}日目`) }))
}

describe('週間献立', () => {
  it('7日分が表示される', async () => {
    const user = userEvent.setup()
    respondWeekly()
    renderWithProviders(<WeeklyPage today={monday} />)

    await user.click(create())

    const items = await screen.findAllByRole('listitem')
    expect(items).toHaveLength(7)
    expect(dayItem(1).getByText('献立1')).toBeVisible()
    expect(dayItem(7).getByText('献立7')).toBeVisible()
  })

  it('当日起点で日付と曜日を表示する', async () => {
    const user = userEvent.setup()
    respondWeekly()
    renderWithProviders(<WeeklyPage today={monday} />)

    await user.click(create())

    // 起点は「今日」。7日目は6日後の日曜。
    expect(await screen.findByText(/1日目.*7\/20\(月\).*今日/)).toBeVisible()
    expect(screen.getByText(/7日目.*7\/26\(日\)/)).toBeVisible()
  })

  it('各日から引き直せる', async () => {
    const user = userEvent.setup()
    respondWeekly()
    renderWithProviders(<WeeklyPage today={monday} />)
    await user.click(create())
    await screen.findAllByRole('listitem')

    const replaced: Menu = { ...menu(9), name: '差し替え後' }
    const bodies = respondReroll(replaced)

    await user.click(dayItem(3).getByRole('button', { name: '引き直す' }))

    await waitFor(() => expect(screen.getByText('差し替え後')).toBeVisible())
    // 引き直した日だけが変わり、他の日はそのまま。
    expect(dayItem(3).getByText('差し替え後')).toBeVisible()
    expect(dayItem(2).getByText('献立2')).toBeVisible()
    expect(dayItem(4).getByText('献立4')).toBeVisible()
    expect(screen.queryByText('献立3')).not.toBeInTheDocument()

    // サーバは週の状態を持たないため、現在の週を送る必要がある。
    expect(bodies[0].day).toBe(3)
    expect(bodies[0].week).toHaveLength(7)
    expect(bodies[0].week?.[0]).toBe(menu(1).id)
  })

  it('引き直しに失敗しても他の日は残る', async () => {
    const user = userEvent.setup()
    respondWeekly()
    renderWithProviders(<WeeklyPage today={monday} />)
    await user.click(create())
    await screen.findAllByRole('listitem')

    server.use(
      http.post('/api/v1/menus/reroll-day', () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/x',
            title: '条件に合う献立が見つかりません',
            status: 422,
          },
          {
            status: 422,
            headers: { 'Content-Type': 'application/problem+json' },
          },
        ),
      ),
    )

    await user.click(dayItem(3).getByRole('button', { name: '引き直す' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '条件に合う献立が見つかりません',
    )
    expect(screen.getAllByRole('listitem')).toHaveLength(7)
    expect(dayItem(3).getByText('献立3')).toBeVisible()
  })

  it('各日からレシピへ遷移できる', async () => {
    const user = userEvent.setup()
    respondWeekly()
    renderWithProviders(<WeeklyPage today={monday} />)

    await user.click(create())
    await screen.findAllByRole('listitem')

    const link = dayItem(2).getByRole('link', { name: /レシピ/ })
    expect(link).toHaveAttribute('href', `/menus/${menu(2).id}`)
  })

  it('作成前は献立を表示しない', () => {
    renderWithProviders(<WeeklyPage today={monday} />)
    expect(screen.queryByRole('listitem')).not.toBeInTheDocument()
  })

  it('絞り込み条件を本文で送る', async () => {
    const user = userEvent.setup()
    const bodies = respondWeekly()
    renderWithProviders(<WeeklyPage today={monday} />)

    const genre = within(screen.getByRole('group', { name: 'ジャンル' }))
    await user.click(genre.getByRole('radio', { name: '中華' }))
    await user.click(create())

    await screen.findAllByRole('listitem')
    expect(bodies[0]).toEqual({ genre: 'chinese', difficulty: undefined })
  })

  it('作成に失敗したらメッセージを出す', async () => {
    const user = userEvent.setup()
    server.use(
      http.post('/api/v1/menus/suggest-weekly', () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/x',
            title: '条件に合う献立が足りません',
            status: 422,
          },
          {
            status: 422,
            headers: { 'Content-Type': 'application/problem+json' },
          },
        ),
      ),
    )
    renderWithProviders(<WeeklyPage today={monday} />)

    await user.click(create())

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '条件に合う献立が足りません',
    )
  })
})
