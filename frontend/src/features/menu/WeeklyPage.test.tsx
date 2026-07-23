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
    role: 'main',
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
    expect(bodies[0]).toEqual({ genre: 'chinese', difficulty: undefined, role: 'main' })
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

  it('画面を離れて戻ってきても作った週が残る', async () => {
    const user = userEvent.setup()
    respondWeekly()
    const first = renderWithProviders(<WeeklyPage today={monday} />)

    await user.click(create())
    await screen.findAllByRole('listitem')

    // 詳細画面へ遷移して戻る＝この画面はアンマウントされ、作り直される。
    first.unmount()
    renderWithProviders(<WeeklyPage today={monday} />)

    // 「1週間分を作る」を押し直さなくても、作った週がそのまま出ている。
    expect(await screen.findByText('献立1')).toBeVisible()
    expect(screen.getAllByRole('listitem')).toHaveLength(7)
  })

  it('引き直した結果も画面を離れて戻ると残る', async () => {
    const user = userEvent.setup()
    respondWeekly()
    const first = renderWithProviders(<WeeklyPage today={monday} />)
    await user.click(create())
    await screen.findAllByRole('listitem')

    respondReroll({ ...menu(9), name: '差し替え後' })
    await user.click(dayItem(3).getByRole('button', { name: '引き直す' }))
    await waitFor(() => expect(screen.getByText('差し替え後')).toBeVisible())

    first.unmount()
    renderWithProviders(<WeeklyPage today={monday} />)

    expect(await screen.findByText('差し替え後')).toBeVisible()
    expect(screen.queryByText('献立3')).not.toBeInTheDocument()
  })

  it('作り直せば新しい週に置き換わる', async () => {
    const user = userEvent.setup()
    respondWeekly()
    const first = renderWithProviders(<WeeklyPage today={monday} />)
    await user.click(create())
    await screen.findAllByRole('listitem')
    first.unmount()

    server.use(
      http.post('/api/v1/menus/suggest-weekly', () =>
        HttpResponse.json({
          week: Array.from({ length: 7 }, (_, i) => ({
            day: i + 1,
            menu: { ...menu(i + 1), name: `新献立${i + 1}` },
          })),
        }),
      ),
    )
    renderWithProviders(<WeeklyPage today={monday} />)
    await screen.findByText('献立1')

    await user.click(create())

    expect(await screen.findByText('新献立1')).toBeVisible()
    expect(screen.queryByText('献立1')).not.toBeInTheDocument()
  })

  it('引き直しは復帰後の週に対しても正しい状態を送る', async () => {
    const user = userEvent.setup()
    respondWeekly()
    const first = renderWithProviders(<WeeklyPage today={monday} />)
    await user.click(create())
    await screen.findAllByRole('listitem')
    first.unmount()

    renderWithProviders(<WeeklyPage today={monday} />)
    await screen.findByText('献立1')

    const bodies = respondReroll({ ...menu(9), name: '復帰後の差し替え' })
    await user.click(dayItem(5).getByRole('button', { name: '引き直す' }))

    await waitFor(() =>
      expect(screen.getByText('復帰後の差し替え')).toBeVisible(),
    )
    // 復帰した週の献立IDが送られる（絞り込み条件も復帰している必要がある）。
    expect(bodies[0].day).toBe(5)
    expect(bodies[0].week).toHaveLength(7)
    expect(bodies[0].week?.[0]).toBe(menu(1).id)
  })
})

describe('買い物リストへの導線（11-G）', () => {
  it('週間献立を作ると買い物リストへのリンクが出る', async () => {
    const user = userEvent.setup()
    respondWeekly()
    renderWithProviders(<WeeklyPage today={monday} />)

    // 作る前は出さない。まだ買うものが決まっていないため。
    expect(
      screen.queryByRole('link', { name: '買い物リストを見る' }),
    ).not.toBeInTheDocument()

    await user.click(create())

    expect(
      await screen.findByRole('link', { name: '買い物リストを見る' }),
    ).toBeVisible()
  })
})

describe('週間献立の保存', () => {
  // 既定のハンドラは /auth/me に 401 を返す（＝未ログイン）。
  // 保存は認証が要るため、ログイン済みの場合だけ差し替える。
  function signedIn() {
    server.use(
      http.get('/api/v1/auth/me', () =>
        HttpResponse.json({
          user: {
            id: '018f0000-0000-7000-8000-0000000000ff',
            email: 'user@example.com',
            displayName: 'ユーザー',
          },
        }),
      ),
    )
  }

  // respondSave は保存を仕込み、送られた本文を記録する。
  function respondSave() {
    const bodies: { days?: { day: number; menuId: string }[] }[] = []
    server.use(
      http.post('/api/v1/weekly-menus', async ({ request }) => {
        bodies.push((await request.json()) as { days: { day: number; menuId: string }[] })
        return HttpResponse.json(
          { id: '018f0000-0000-7000-8000-0000000000aa' },
          { status: 201 },
        )
      }),
    )
    return bodies
  }

  function saveButton() {
    return screen.getByRole('button', { name: 'この週を保存する' })
  }

  it('週を作る前は保存の導線を出さない', () => {
    signedIn()
    renderWithProviders(<WeeklyPage today={monday} />)

    // 保存するものがまだ無い。買い物リストと同じ扱い。
    expect(
      screen.queryByRole('button', { name: 'この週を保存する' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'ログインして保存する' }),
    ).not.toBeInTheDocument()
  })

  it('未ログインなら保存ボタンではなくログイン導線を出す', async () => {
    const user = userEvent.setup()
    respondWeekly()
    renderWithProviders(<WeeklyPage today={monday} />)

    await user.click(create())
    await screen.findAllByRole('listitem')

    // 押せないボタンを見せるより、何をすれば保存できるかを示す。
    const link = await screen.findByRole('link', { name: 'ログインして保存する' })
    expect(link).toHaveAttribute('href', '/login')
    expect(
      screen.queryByRole('button', { name: 'この週を保存する' }),
    ).not.toBeInTheDocument()
  })

  it('ログイン済みなら7日分を送って保存し、知らせを出す', async () => {
    const user = userEvent.setup()
    signedIn()
    respondWeekly()
    const bodies = respondSave()
    renderWithProviders(<WeeklyPage today={monday} />)

    await user.click(create())
    await screen.findAllByRole('listitem')

    await user.click(await screen.findByRole('button', { name: 'この週を保存する' }))

    expect(await screen.findByRole('status')).toHaveTextContent('保存しました')
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].days).toHaveLength(7)
    expect(bodies[0].days?.[0]).toEqual({ day: 1, menuId: menu(1).id })
    expect(bodies[0].days?.[6]).toEqual({ day: 7, menuId: menu(7).id })
  })

  // 上限はプランで変わる（free 10件 / premium 50件）。件数をこちらで持つと
  // premium の利用者に「10件まで」と出るため、サーバの detail をそのまま出す。
  it('上限に達していたら、サーバが返した件数で古いものを消すよう伝える', async () => {
    const user = userEvent.setup()
    signedIn()
    respondWeekly()
    server.use(
      http.post('/api/v1/weekly-menus', () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/saved-weekly-menu-limit-reached',
            title: '保存できる週間献立の上限に達しました',
            status: 409,
            detail:
              '保存できる週間献立の上限に達しました: 保存できるのは50件までです。「保存した週間献立」から古いものを削除してください',
          },
          { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    renderWithProviders(<WeeklyPage today={monday} />)

    await user.click(create())
    await screen.findAllByRole('listitem')
    await user.click(await screen.findByRole('button', { name: 'この週を保存する' }))

    // 押し直しても直らない。次に取るべき行動は「古いものを消す」。
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('50件まで')
    expect(alert).not.toHaveTextContent('10件まで')
    expect(alert).toHaveTextContent('削除してください')
  })

  it('引き直すと保存済みの知らせを消す', async () => {
    const user = userEvent.setup()
    signedIn()
    respondWeekly()
    respondSave()
    renderWithProviders(<WeeklyPage today={monday} />)

    await user.click(create())
    await screen.findAllByRole('listitem')
    await user.click(await screen.findByRole('button', { name: 'この週を保存する' }))
    expect(await screen.findByRole('status')).toHaveTextContent('保存しました')

    respondReroll({ ...menu(9), name: '差し替え後' })
    await user.click(dayItem(3).getByRole('button', { name: '引き直す' }))

    // 出したままだと「今の画面の状態が保存されている」と誤解させる。
    await waitFor(() =>
      expect(screen.queryByRole('status')).not.toBeInTheDocument(),
    )
    expect(saveButton()).toBeVisible()
  })
})
