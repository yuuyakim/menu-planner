import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import type { Menu } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { useCurrentUser } from '../auth/useCurrentUser'
import {
  addFavorite,
  favoritesQueryKey,
  fetchFavorites,
  removeFavorite,
} from './api'

// FavoriteButton は献立をお気に入りに出し入れするトグル。
// 献立が表示される場所（検索結果・週間献立・詳細）に置く。
export function FavoriteButton({ menu }: { menu: Menu }) {
  const { user } = useCurrentUser()
  const queryClient = useQueryClient()

  const { data: favorites } = useQuery({
    queryKey: favoritesQueryKey,
    queryFn: fetchFavorites,
    // 未ログインでは 401 になるだけなので問い合わせない。
    enabled: Boolean(user),
  })

  // 一覧を共有しているので、献立ごとに問い合わせる必要はない。
  const isFavorite = favorites?.some((f) => f.menu.id === menu.id) ?? false

  const toggle = useMutation({
    mutationFn: () =>
      isFavorite ? removeFavorite(menu.id) : addFavorite(menu.id).then(() => {}),
    onSuccess: () => {
      // 一覧のキャッシュを無効化する。これを忘れると、お気に入り画面に
      // 遷移しても古いままになる（8-J で履歴に起きた不具合と同じ）。
      void queryClient.invalidateQueries({ queryKey: favoritesQueryKey })
    },
  })

  // お気に入りは本人のものだけを扱うため、未ログインでは出さない。
  if (!user) return null

  return (
    <div className="space-y-2">
      <button
        type="button"
        onClick={() => toggle.mutate()}
        disabled={toggle.isPending}
        // 押した結果どうなるかではなく、今どうなっているかを名前にする。
        aria-pressed={isFavorite}
        className={
          isFavorite
            ? 'rounded-lg border border-amber-400 bg-amber-50 px-4 py-1.5 text-sm font-medium text-amber-800 disabled:opacity-50'
            : 'rounded-lg border border-slate-300 bg-white px-4 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50'
        }
      >
        {isFavorite ? 'お気に入り済み' : 'お気に入りに追加'}
      </button>

      {toggle.error && <ErrorMessage error={toggle.error} />}
    </div>
  )
}
