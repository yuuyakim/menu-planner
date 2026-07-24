import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router'

import type { DayMenu, SavedWeeklyMenu } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotEmpty } from '../../components/MascotEmpty'
import { MascotStatus } from '../../components/MascotStatus'
import { useSessionState } from '../../hooks/useSessionState'
import {
  deleteSavedWeeklyMenu,
  fetchSavedWeeklyMenus,
  savedWeeklyMenusQueryKey,
} from './api'
import { emptyMenuFilter, type MenuFilter } from './filter'

// WeeklyPage が作業中の週を置いている場所と同じキー。
// 「開く」はここへ書き戻すだけで済む（spec.md 5.3）。
const weekKey = 'weekly.week'
const filterKey = 'weekly.filter'

// formatSavedAt は保存日時を見出しにする。名前を付けない仕様（spec.md 2.8）のため、
// これと中身の献立名が識別の手がかりになる。
function formatSavedAt(iso: string): string {
  const d = new Date(iso)
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(
    d.getMinutes(),
  ).padStart(2, '0')} に保存`
}

// summarize は中身をひと目で見分けられるようにする。
// 7件すべて並べると一覧が縦に伸びて選びにくくなるため、先頭3件だけ出す。
function summarize(days: DayMenu[]): string {
  const names = days.map((d) => d.menu.name)
  const head = names.slice(0, 3).join('・')
  return names.length > 3 ? `${head} ほか${names.length - 3}件` : head
}

// SavedWeeklyPage は保存した週間献立の一覧・削除・呼び出し。
export function SavedWeeklyPage() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  // 「開く」は作業中の週を差し替えるだけ。サーバへの問い合わせは要らない
  // （一覧の応答に7日分が入っている）。
  const [, setWeek] = useSessionState<DayMenu[] | null>(weekKey, null)
  const [, setFilter] = useSessionState<MenuFilter>(filterKey, emptyMenuFilter)

  const {
    data: saved,
    isPending,
    error,
  } = useQuery({
    queryKey: savedWeeklyMenusQueryKey,
    queryFn: fetchSavedWeeklyMenus,
  })

  const remove = useMutation({
    mutationFn: deleteSavedWeeklyMenu,
    // 手元の配列から抜かず取り直す。サーバの状態を正とする。
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: savedWeeklyMenusQueryKey }),
  })

  function open(week: SavedWeeklyMenu) {
    setWeek(week.days)
    // 絞り込み条件は保存していない。前回の条件を引き継ぐと、
    // この週とは無関係な条件で引き直すことになるため空に戻す。
    setFilter(emptyMenuFilter)
    void navigate('/weekly')
  }

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold text-kon-ink">保存した週間献立</h1>

      {isPending && <MascotStatus>読み込み中…</MascotStatus>}

      {error && <ErrorMessage error={error} />}
      {/* 削除の失敗で一覧を消さない。他の週はそのまま操作できる。 */}
      {remove.error && <ErrorMessage error={remove.error} />}

      {saved?.length === 0 && (
        <MascotEmpty image="/mascot/pose-dish.png">
          まだ保存した週間献立がありません。1週間の献立を作って保存すると、買い物のときにここから開けます。
        </MascotEmpty>
      )}

      {saved && saved.length > 0 && (
        <ul className="space-y-3">
          {saved.map((w) => (
            <li
              key={w.id}
              // 行を保存日時で特定できるようにする。名前が無いぶん、
              // ここが唯一の安定した見分け方になる。
              aria-label={formatSavedAt(w.createdAt)}
              className="rounded-2xl border border-kon-leaf-soft bg-white p-4"
            >
              <p className="font-medium text-kon-ink">
                {formatSavedAt(w.createdAt)}
              </p>
              <p className="mt-1 text-sm text-kon-ink/60">{summarize(w.days)}</p>
              <div className="mt-3 flex gap-3">
                <button
                  type="button"
                  onClick={() => open(w)}
                  className="rounded-full bg-kon-leaf px-5 py-2 text-sm font-medium text-white transition-colors hover:brightness-95"
                >
                  開く
                </button>
                <button
                  type="button"
                  onClick={() => remove.mutate(w.id)}
                  disabled={remove.isPending}
                  className="rounded-full border border-kon-leaf-soft bg-white px-5 py-2 text-sm font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream disabled:cursor-not-allowed disabled:text-kon-ink/40"
                >
                  削除
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
