import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'

import type { FavoriteItem } from '../../api/types'
import { difficultyLabels, genreLabels } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotEmpty } from '../../components/MascotEmpty'
import { MascotStatus } from '../../components/MascotStatus'
import { favoritesQueryKey, fetchFavorites, removeFavorite } from './api'

// FavoritePage はお気に入りの一覧と削除。
// 履歴と違い件数の上限が無く、自動削除もされない（spec.md 2.6）。
export function FavoritePage() {
  const queryClient = useQueryClient()

  const {
    data: favorites,
    isPending,
    error,
  } = useQuery({
    queryKey: favoritesQueryKey,
    queryFn: fetchFavorites,
  })

  const remove = useMutation({
    mutationFn: removeFavorite,
    // 手元の配列から抜かず取り直す。サーバの状態を正とする。
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: favoritesQueryKey }),
  })

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold text-kon-ink">お気に入り</h1>

      {isPending && <MascotStatus>読み込み中…</MascotStatus>}

      {error && <ErrorMessage error={error} />}
      {/* 削除の失敗で一覧を消さない。他の献立はそのまま操作できる。 */}
      {remove.error && <ErrorMessage error={remove.error} />}

      {favorites?.length === 0 && (
        <MascotEmpty image="/mascot/face-happy.png">
          まだお気に入りがありません。気に入った献立を登録しておくと、ここから探せます。
        </MascotEmpty>
      )}

      {favorites && favorites.length > 0 && (
        <ul className="space-y-3">
          {favorites.map((f) => (
            <FavoriteRow
              key={f.menu.id}
              favorite={f}
              onRemove={() => remove.mutate(f.menu.id)}
              isRemoving={remove.isPending}
            />
          ))}
        </ul>
      )}
    </section>
  )
}

type RowProps = {
  favorite: FavoriteItem
  onRemove: () => void
  isRemoving: boolean
}

function FavoriteRow({ favorite, onRemove, isRemoving }: RowProps) {
  const { menu } = favorite

  return (
    // aria-label に献立名を入れ、行を名前で特定できるようにする。
    <li
      aria-label={menu.name}
      className="flex items-center gap-4 rounded-2xl border border-kon-leaf-soft bg-white p-4"
    >
      <div className="min-w-0 flex-1">
        <p className="font-medium text-kon-ink">{menu.name}</p>
        <p className="mt-1 flex gap-2 text-sm text-kon-ink/60">
          <span className="rounded-full bg-kon-cream px-2 py-0.5">
            {genreLabels[menu.genre]}
          </span>
          <span className="rounded-full bg-kon-cream px-2 py-0.5">
            {difficultyLabels[menu.difficulty]}
          </span>
        </p>
      </div>
      <Link
        to={`/menus/${menu.id}`}
        className="shrink-0 text-sm font-medium text-kon-ink underline decoration-kon-leaf decoration-2 underline-offset-2"
      >
        レシピを見る
      </Link>
      <button
        type="button"
        onClick={onRemove}
        disabled={isRemoving}
        className="shrink-0 text-sm text-kon-ink/60 hover:text-kon-ink disabled:text-kon-ink/30"
      >
        外す
      </button>
    </li>
  )
}
