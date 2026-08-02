import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { Route, Routes } from 'react-router'
import { beforeEach, describe, expect, it } from 'vitest'

import type { Menu, SavedWeeklyMenu } from '../../api/types'
import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { SavedWeeklyPage } from './SavedWeeklyPage'
import { WeeklyPage } from './WeeklyPage'

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

function days() {
  return Array.from({ length: 7 }, (_, i) => ({ day: i + 1, menu: menu(i + 1) }))
}

function savedWeek(id: string, createdAt: string): SavedWeeklyMenu {
  return { id, days: days(), createdAt }
}

// respondList は一覧を仕込む。
function respondList(weeks: SavedWeeklyMenu[]) {
  server.use(
    http.get('/api/v1/weekly-menus', () =>
      HttpResponse.json({ weeklyMenus: weeks }),
    ),
  )
}

// respondMe は現在のユーザーの応答を仕込む（WeeklyPage.test.tsx と同じ流儀）。
function respondMe(plan: 'free' | 'premium') {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000001',
          email: 'user@example.com',
          displayName: 'ユーザー',
          plan,
        },
      }),
    ),
  )
}

// 週間献立画面の代わりを置き、「開く」でそこへ移ったことと
// 開いた週が出ていることを確かめられるようにする。
function renderPage() {
  return renderWithProviders(
    <Routes>
      <Route path="/saved-weekly" element={<SavedWeeklyPage />} />
      <Route path="/weekly" element={<WeeklyStub />} />
    </Routes>,
    { route: '/saved-weekly' },
  )
}

// WeeklyStub は sessionStorage に置かれた週をそのまま描く。
// 本物の WeeklyPage と同じ読み取り方（useSessionState の初期値）をなぞる。
function WeeklyStub() {
  const raw = sessionStorage.getItem('menu-planner:weekly.week')
  const week = raw ? (JSON.parse(raw) as { day: number; menu: Menu }[]) : null
  return (
    <div>
      <h1>1週間の献立</h1>
      <ul>
        {week?.map((d) => (
          <li key={d.day}>{d.menu.name}</li>
        ))}
      </ul>
    </div>
  )
}

// 2026-07-22 12:34 に保存されたことにする。
const savedAt = new Date(2026, 6, 22, 12, 34).toISOString()

describe('プランによらず使える', () => {
  it('free でも保存した週間献立の一覧が出る', async () => {
    respondMe('free')
    renderWithProviders(<SavedWeeklyPage />)

    expect(
      await screen.findByRole('heading', { name: '保存した週間献立' }),
    ).toBeInTheDocument()
    // アップグレード誘導（撤廃済みの PremiumLock が出していた文言）が
    // 再発していないこと。
    expect(screen.queryByText('プレミアムにアップグレード')).not.toBeInTheDocument()
  })

  it('保存一覧を出す', async () => {
    respondMe('free')
    respondList([])
    renderPage()

    expect(await screen.findByText(/まだ保存した週間献立がありません/)).toBeVisible()
  })
})

describe('保存した週間献立', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('保存日時と中身の一部を並べる', async () => {
    respondMe('free')
    respondList([savedWeek('w-1', savedAt)])
    renderPage()

    // 名前を付けない仕様なので、保存日時が識別の手がかりになる。
    const row = await screen.findByRole('listitem', { name: /7\/22 12:34 に保存/ })
    // 7件すべて並べると選びにくい。先頭3件と残りの件数だけ出す。
    expect(within(row).getByText('献立1・献立2・献立3 ほか4件')).toBeVisible()
  })

  it('1件も無ければ、作って保存するよう促す', async () => {
    respondMe('free')
    respondList([])
    renderPage()

    expect(await screen.findByText(/まだ保存した週間献立がありません/)).toBeVisible()
    expect(screen.queryByRole('listitem')).not.toBeInTheDocument()
  })

  it('開くと週間献立画面に移り、その週が出ている', async () => {
    const user = userEvent.setup()
    respondMe('free')
    respondList([savedWeek('w-1', savedAt)])
    renderPage()

    const row = await screen.findByRole('listitem', { name: /7\/22 12:34 に保存/ })
    await user.click(within(row).getByRole('button', { name: '開く' }))

    expect(await screen.findByRole('heading', { name: '1週間の献立' })).toBeVisible()
    // 一覧の応答に7日分が入っているため、開くのに再取得は要らない。
    expect(screen.getByText('献立1')).toBeVisible()
    expect(screen.getByText('献立7')).toBeVisible()
  })

  it('開いたとき、前回の絞り込み条件は引き継がない', async () => {
    const user = userEvent.setup()
    // 別の週を作ったときの条件が残っている状態を作る。
    sessionStorage.setItem(
      'menu-planner:weekly.filter',
      JSON.stringify({ genre: 'chinese' }),
    )
    respondMe('free')
    respondList([savedWeek('w-1', savedAt)])
    renderPage()

    const row = await screen.findByRole('listitem', { name: /7\/22 12:34 に保存/ })
    await user.click(within(row).getByRole('button', { name: '開く' }))

    await screen.findByRole('heading', { name: '1週間の献立' })
    // 保存に条件は含まれない。無関係な条件で引き直させない。
    // 役割だけは既定の主菜が入る（未指定にすると省略時の解釈がサーバ任せになる）。
    expect(sessionStorage.getItem('menu-planner:weekly.filter')).toBe(
      JSON.stringify({ role: 'main' }),
    )
  })

  it('開いた週は本物の週間献立画面でそのまま続けられる', async () => {
    const user = userEvent.setup()
    respondMe('free')
    respondList([savedWeek('w-1', savedAt)])
    // スタブではなく本物の WeeklyPage に載せ替える。
    // 「開く」が sessionStorage に書き戻すだけで済む、という設計（spec.md 5.3）が
    // 実際に成立しているかは、本物に対してでないと確かめられない。
    renderWithProviders(
      <Routes>
        <Route path="/saved-weekly" element={<SavedWeeklyPage />} />
        <Route path="/weekly" element={<WeeklyPage today={new Date(2026, 6, 22)} />} />
      </Routes>,
      { route: '/saved-weekly' },
    )

    const row = await screen.findByRole('listitem', { name: /7\/22 12:34 に保存/ })
    await user.click(within(row).getByRole('button', { name: '開く' }))

    // 7日分が復元され、日付つきの見出しも付く。
    const items = await screen.findAllByRole('listitem')
    expect(items).toHaveLength(7)
    expect(screen.getByText(/1日目.*7\/22\(水\).*今日/)).toBeVisible()
    expect(screen.getByText('献立7')).toBeVisible()
    // 引き直しの導線も出る＝作った直後と同じ状態になっている。
    expect(screen.getAllByRole('button', { name: '引き直す' })).toHaveLength(7)
    // 買い物リストへも行ける。ShoppingListPage は同じ sessionStorage を読むため
    // 無改修で動く、という 12-A の見込みがここで裏取りできる。
    expect(screen.getByRole('link', { name: '買い物リストを見る' })).toBeVisible()
  })

  it('削除すると一覧から消える', async () => {
    const user = userEvent.setup()
    let deleted: string | undefined
    respondMe('free')
    respondList([savedWeek('w-1', savedAt)])
    server.use(
      http.delete('/api/v1/weekly-menus/:id', ({ params }) => {
        deleted = params.id as string
        // 消えた後の一覧に差し替える。手元から抜かず取り直す作りのため。
        respondList([])
        return new HttpResponse(null, { status: 204 })
      }),
    )
    renderPage()

    const row = await screen.findByRole('listitem', { name: /7\/22 12:34 に保存/ })
    await user.click(within(row).getByRole('button', { name: '削除' }))

    await waitFor(() => expect(screen.queryByRole('listitem')).not.toBeInTheDocument())
    expect(deleted).toBe('w-1')
    expect(screen.getByText(/まだ保存した週間献立がありません/)).toBeVisible()
  })

  it('削除に失敗しても一覧は残る', async () => {
    const user = userEvent.setup()
    respondMe('free')
    respondList([savedWeek('w-1', savedAt)])
    server.use(
      http.delete('/api/v1/weekly-menus/:id', () =>
        HttpResponse.json(
          {
            type: 'https://example.com/probs/saved-weekly-menu-not-found',
            title: '保存した週間献立が見つかりません',
            status: 404,
          },
          { status: 404, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    renderPage()

    const row = await screen.findByRole('listitem', { name: /7\/22 12:34 に保存/ })
    await user.click(within(row).getByRole('button', { name: '削除' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '保存した週間献立が見つかりません',
    )
    // 他の週はそのまま操作できる。
    expect(screen.getByRole('listitem', { name: /7\/22 12:34 に保存/ })).toBeVisible()
  })
})
