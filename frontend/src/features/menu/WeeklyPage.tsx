import { useMutation } from '@tanstack/react-query'
import { Link } from 'react-router'

import type { DayMenu } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { useSessionState } from '../../hooks/useSessionState'
import { rerollDay, suggestWeekly } from './api'
import { MenuCard } from './MenuCard'
import { SearchForm, type MenuFilter } from './SearchForm'

const weekdays = ['日', '月', '火', '水', '木', '金', '土'] as const

// 作った週は画面遷移をまたいで保つ（レシピを見て戻ると消えていた不具合）。
// サーバは週の状態を持たない（spec.md 5.1）ため、保持はクライアントの責務。
const weekKey = 'weekly.week'
const filterKey = 'weekly.filter'

// dayLabel は「何日目か」を日付つきの見出しにする。
//
// 週間献立は当日起点（spec.md 13.3）。サーバは day を 1..7 の連番で返すだけで
// 曜日を知らないため、起点（今日）から曜日を決めるのは呼び出し側の仕事。
function dayLabel(day: number, today: Date): string {
  const date = new Date(today)
  date.setDate(date.getDate() + day - 1)
  const weekday = weekdays[date.getDay()]
  const base = `${day}日目 ${date.getMonth() + 1}/${date.getDate()}(${weekday})`
  return day === 1 ? `${base} 今日` : base
}

type Props = {
  /** 週の起点。既定は今日。テストで固定できるように受け取る。 */
  today?: Date
}

// WeeklyPage は1週間分の献立を作り、日ごとに引き直せる画面。
export function WeeklyPage({ today = new Date() }: Props) {
  // 引き直しには週と絞り込み条件の両方が要るので、どちらも保持する。
  const [filter, setFilter] = useSessionState<MenuFilter>(filterKey, {})
  const [week, setWeek] = useSessionState<DayMenu[] | null>(weekKey, null)

  const create = useMutation({
    mutationFn: suggestWeekly,
    onSuccess: setWeek,
  })

  const reroll = useMutation({
    mutationFn: (day: number) => rerollDay(day, week ?? [], filter),
    onSuccess: (menu, day) => {
      // 引き直した日だけを差し替える。他の日には触れない。
      setWeek(
        (current) =>
          current?.map((d) => (d.day === day ? { ...d, menu } : d)) ?? null,
      )
    },
  })

  const error = create.error ?? reroll.error

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold">1週間の献立</h1>

      <SearchForm
        onSubmit={(next) => {
          setFilter(next)
          create.mutate(next)
        }}
        isPending={create.isPending}
        submitLabel="1週間分を作る"
      />

      {create.isPending && (
        <p role="status" className="text-slate-600">
          作成中…
        </p>
      )}

      {/* 引き直しの失敗で週全体を消さない。失敗した日以外はそのまま使える。 */}
      {error && <ErrorMessage error={error} />}

      {week && (
        <ul className="space-y-4">
          {week.map((d) => (
            <li key={d.day} aria-label={dayLabel(d.day, today)}>
              <h2 className="mb-2 font-medium text-slate-700">
                {dayLabel(d.day, today)}
              </h2>
              <MenuCard menu={d.menu} />
              <div className="mt-2 flex gap-3">
                <button
                  type="button"
                  onClick={() => reroll.mutate(d.day)}
                  disabled={reroll.isPending}
                  className="rounded-lg border border-slate-300 bg-white px-4 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:text-slate-400"
                >
                  引き直す
                </button>
                <Link
                  to={`/menus/${d.menu.id}`}
                  className="rounded-lg border border-slate-300 bg-white px-4 py-1.5 text-sm font-medium text-emerald-700 hover:bg-slate-50"
                >
                  レシピを見る
                </Link>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
