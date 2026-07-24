import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'

import { ApiError } from '../../api/client'
import type { DayMenu } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotStatus } from '../../components/MascotStatus'
import { useSessionState } from '../../hooks/useSessionState'
import { useCurrentUser } from '../auth/useCurrentUser'
import { historiesQueryKey } from '../history/api'
import { rerollDay, saveWeeklyMenu, savedWeeklyMenusQueryKey, suggestWeekly } from './api'
import { MenuCard } from './MenuCard'
import { SearchForm } from './SearchForm'
import { emptyMenuFilter, type MenuFilter } from './filter'

const weekdays = ['日', '月', '火', '水', '木', '金', '土'] as const

// 作った週は画面遷移をまたいで保つ（レシピを見て戻ると消えていた不具合）。
// サーバは週の状態を持たない（spec.md 5.1）ため、保持はクライアントの責務。
const weekKey = 'weekly.week'
const filterKey = 'weekly.filter'
const savedIdKey = 'weekly.savedId'

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

// saveErrorText は保存の失敗を利用者向けの文言にする。
//
// 409 だけ特別に扱う。上限に達したときサーバは押し出さずに断るため
// （spec.md 2.8）、利用者が次に取るべき行動は「古いものを消す」であって
// 「もう一度試す」ではない。それを伝えないと、押し直して失敗し続ける。
//
// **件数はここに書かない。** 上限はプランで変わる（free 10件 / premium 50件、
// spec.md 2.11）ため、こちらで固定すると premium の利用者に「10件まで」と
// 表示される。サーバが detail にプラン由来の件数を入れて返すので、それを出す。
function saveErrorText(error: Error): string {
  if (error instanceof ApiError && error.status === 409) {
    return error.detail ?? '保存できる件数の上限に達しました。古いものを削除してください。'
  }
  return error.message
}

type Props = {
  /** 週の起点。既定は今日。テストで固定できるように受け取る。 */
  today?: Date
}

// WeeklyPage は1週間分の献立を作り、日ごとに引き直せる画面。
export function WeeklyPage({ today = new Date() }: Props) {
  // 引き直しには週と絞り込み条件の両方が要るので、どちらも保持する。
  const [filter, setFilter] = useSessionState<MenuFilter>(filterKey, emptyMenuFilter)
  const [week, setWeek] = useSessionState<DayMenu[] | null>(weekKey, null)
  // 買い物リストの取得経路（GET/POST）の切り替えに使う保存ID。
  const [, setSavedId] = useSessionState<string | null>(savedIdKey, null)

  const queryClient = useQueryClient()

  const create = useMutation({
    mutationFn: suggestWeekly,
    onSuccess: (next) => {
      setWeek(next)
      // 新しく作った週は保存済みの週とは無関係。古い保存IDを持ち越すと、
      // 買い物リストが別の週の差分を誤って重ねてしまう（spec.md 14.5）。
      setSavedId(null)
      // 週の確定はサーバ側で7件まとめて履歴に記録される（6-A）。
      void queryClient.invalidateQueries({ queryKey: historiesQueryKey })
    },
  })

  // 引き直しは履歴に記録されない（確定した献立ではないため。4-F）。
  // 記録されないので履歴の無効化もしない。
  const reroll = useMutation({
    mutationFn: (day: number) => rerollDay(day, week ?? [], filter),
    onSuccess: (menu, day) => {
      // 引き直した日だけを差し替える。他の日には触れない。
      setWeek(
        (current) =>
          current?.map((d) => (d.day === day ? { ...d, menu } : d)) ?? null,
      )
      // 中身が保存時点からずれるため、保存済みの差分経路からは外す。
      setSavedId(null)
    },
  })

  // 保存は認証が要る（spec.md 2.8）。未ログインなら押させずログインへ誘う。
  const { user } = useCurrentUser()

  const save = useMutation({
    mutationFn: () => saveWeeklyMenu(week ?? []),
    onSuccess: (newId) => {
      // 保存直後からこの週を永続化経路（GET）にする。
      setSavedId(newId)
      // 保存一覧を開いたときに、今保存したものが載っている状態にする。
      void queryClient.invalidateQueries({ queryKey: savedWeeklyMenusQueryKey })
    },
  })

  // 引き直すと保存済みの内容とずれるため、知らせを消す。
  // 出したままだと「今の画面の状態が保存されている」と誤解させる。
  const clearSaved = () => {
    if (save.isSuccess || save.isError) save.reset()
  }

  const error = create.error ?? reroll.error

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold text-kon-ink">1週間の献立</h1>

      <SearchForm
        onSubmit={(next) => {
          setFilter(next)
          clearSaved()
          create.mutate(next)
        }}
        isPending={create.isPending}
        submitLabel="1週間分を作る"
      />

      {/* 週の組み立ては1食分より待つ。出来上がりの絵を添えて何を待って
          いるかを分かりやすくする。 */}
      {create.isPending && (
        <MascotStatus image="/mascot/pose-dish.png">作成中…</MascotStatus>
      )}

      {/* 引き直しの失敗で週全体を消さない。失敗した日以外はそのまま使える。 */}
      {error && <ErrorMessage error={error} />}

      {week && (
        <>
          {/* 献立が決まってはじめて買うものが決まる。作る前は出さない。
              保存も同じで、週が無ければ保存するものが無い。 */}
          <div className="flex flex-wrap items-center gap-3">
            <Link
              to="/shopping-list"
              className="inline-block rounded-full bg-kon-leaf px-5 py-2 font-medium text-white hover:bg-kon-leaf/90"
            >
              買い物リストを見る
            </Link>

            {user ? (
              <button
                type="button"
                onClick={() => save.mutate()}
                disabled={save.isPending}
                className="rounded-full border border-kon-leaf-soft bg-white px-5 py-2 font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream disabled:cursor-not-allowed disabled:text-kon-ink/40"
              >
                {save.isPending ? '保存中…' : 'この週を保存する'}
              </button>
            ) : (
              // 押せないボタンを出すより、何をすれば保存できるかを示す。
              <Link
                to="/login"
                className="rounded-full border border-kon-leaf-soft bg-white px-5 py-2 font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream"
              >
                ログインして保存する
              </Link>
            )}
          </div>

          {save.isSuccess && (
            <p role="status" className="rounded-2xl bg-kon-cream px-5 py-3 text-kon-ink">
              保存しました。「保存した週間献立」からいつでも開けます
            </p>
          )}

          {save.error && (
            <p
              role="alert"
              className="rounded-2xl border border-kon-peach bg-kon-peach/25 px-5 py-3 text-kon-ink"
            >
              {saveErrorText(save.error)}
            </p>
          )}
          <ul className="space-y-4">
          {week.map((d) => (
            <li key={d.day} aria-label={dayLabel(d.day, today)}>
              {/* 見出し(h2)にはしない。この中の MenuCard が献立名を h2 で
                  持つため、同じ項目に h2 が2つ並んで階層が読めなくなる。
                  日付は li の aria-label が読み上げる。 */}
              <p className="mb-2 font-medium text-kon-ink/80">
                {dayLabel(d.day, today)}
              </p>
              <MenuCard menu={d.menu} />
              <div className="mt-2 flex gap-3">
                <button
                  type="button"
                  onClick={() => {
                    clearSaved()
                    reroll.mutate(d.day)
                  }}
                  disabled={reroll.isPending}
                  className="rounded-full border border-kon-leaf-soft bg-white px-4 py-1.5 text-sm font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream disabled:cursor-not-allowed disabled:text-kon-ink/40"
                >
                  引き直す
                </button>
                <Link
                  to={`/menus/${d.menu.id}`}
                  className="rounded-full border border-kon-leaf-soft bg-white px-4 py-1.5 text-sm font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream"
                >
                  レシピを見る
                </Link>
              </div>
            </li>
          ))}
        </ul>
        </>
      )}
    </section>
  )
}
