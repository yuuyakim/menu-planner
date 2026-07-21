import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router'

import type { HistoryItem, SearchMode } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotEmpty } from '../../components/MascotEmpty'
import { MascotStatus } from '../../components/MascotStatus'
import {
  deleteAllHistories,
  deleteHistory,
  fetchHistories,
  historiesQueryKey,
} from './api'

// searchModeLabels は検索の種類の表示。型が網羅を保証する。
const searchModeLabels: Record<SearchMode, string> = {
  single: '1食分',
  weekly: '1週間',
}

// formatSearchedAt は検索日時を読みやすくする。
// 日時はサーバがUTCで返すため、表示は利用者の時間帯に合わせる。
function formatSearchedAt(iso: string): string {
  const d = new Date(iso)
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(
    d.getMinutes(),
  ).padStart(2, '0')}`
}

// HistoryPage は検索履歴の一覧と削除。
// 保持は最新15件までで、超過分はサーバが自動で消す（spec.md 2.5）。
export function HistoryPage() {
  const queryClient = useQueryClient()
  const [confirmingClear, setConfirmingClear] = useState(false)

  const {
    data: histories,
    isPending,
    error,
  } = useQuery({
    queryKey: historiesQueryKey,
    queryFn: fetchHistories,
  })

  // 削除後は再取得する。手元で配列から抜くより、サーバの状態を正とする方が
  // 「消えたつもりで消えていない」食い違いが起きない。
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: historiesQueryKey })

  const removeOne = useMutation({
    mutationFn: deleteHistory,
    onSuccess: invalidate,
  })

  const removeAll = useMutation({
    mutationFn: deleteAllHistories,
    onSuccess: () => {
      setConfirmingClear(false)
      return invalidate()
    },
  })

  const deleteError = removeOne.error ?? removeAll.error

  return (
    <section className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-kon-ink">検索履歴</h1>
        {histories && histories.length > 0 && !confirmingClear && (
          <button
            type="button"
            onClick={() => setConfirmingClear(true)}
            className="text-sm text-kon-ink/70 underline decoration-kon-leaf underline-offset-2 hover:text-kon-ink"
          >
            すべて削除
          </button>
        )}
      </div>

      {/* 取り返しがつかない操作なので、押した直後には消さない。 */}
      {confirmingClear && (
        <div className="flex flex-wrap items-center gap-3 rounded-2xl border border-kon-gold bg-kon-gold/20 px-4 py-3">
          <span className="text-kon-ink">履歴をすべて削除しますか？</span>
          <button
            type="button"
            onClick={() => removeAll.mutate()}
            disabled={removeAll.isPending}
            className="rounded-full bg-kon-wood px-4 py-1.5 text-sm font-medium text-white transition-colors hover:brightness-95 disabled:bg-kon-leaf-soft disabled:text-kon-ink/70"
          >
            削除する
          </button>
          <button
            type="button"
            onClick={() => setConfirmingClear(false)}
            className="text-sm text-kon-ink/70 underline"
          >
            やめる
          </button>
        </div>
      )}

      {isPending && <MascotStatus>読み込み中…</MascotStatus>}

      {error && <ErrorMessage error={error} />}
      {/* 削除の失敗で一覧を消さない。他の履歴はそのまま操作できる。 */}
      {deleteError && <ErrorMessage error={deleteError} />}

      {histories?.length === 0 && (
        <MascotEmpty>まだ履歴がありません。献立を探すと、ここに残ります。</MascotEmpty>
      )}

      {histories && histories.length > 0 && (
        <ul className="space-y-3">
          {histories.map((h) => (
            <HistoryRow
              key={h.id}
              history={h}
              onDelete={() => removeOne.mutate(h.id)}
              isDeleting={removeOne.isPending}
            />
          ))}
        </ul>
      )}
    </section>
  )
}

type RowProps = {
  history: HistoryItem
  onDelete: () => void
  isDeleting: boolean
}

function HistoryRow({ history, onDelete, isDeleting }: RowProps) {
  return (
    // aria-label に献立名を入れ、行を名前で特定できるようにする。
    <li
      aria-label={history.menu.name}
      className="flex items-center gap-4 rounded-2xl border border-kon-leaf-soft bg-white p-4"
    >
      <div className="min-w-0 flex-1">
        <p className="font-medium text-kon-ink">{history.menu.name}</p>
        <p className="mt-1 flex gap-2 text-sm text-kon-ink/60">
          <span className="rounded-full bg-kon-cream px-2 py-0.5">
            {searchModeLabels[history.searchMode]}
          </span>
          <span>{formatSearchedAt(history.searchedAt)}</span>
        </p>
      </div>
      <Link
        to={`/menus/${history.menu.id}`}
        className="shrink-0 text-sm font-medium text-kon-ink underline decoration-kon-leaf decoration-2 underline-offset-2"
      >
        レシピを見る
      </Link>
      <button
        type="button"
        onClick={onDelete}
        disabled={isDeleting}
        className="shrink-0 text-sm text-kon-ink/60 hover:text-kon-ink disabled:text-kon-ink/30"
      >
        削除
      </button>
    </li>
  )
}
