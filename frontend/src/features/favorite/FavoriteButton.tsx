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

// StarIcon は塗りの有無で ON/OFF を表す星。
// 文字（「お気に入りに追加」「お気に入り済み」）だと、読まないと状態が
// 分からず、似た文言の差を見分ける必要もある。星なら一目で分かる。
function StarIcon({ filled }: { filled: boolean }) {
  return (
    <svg
      width="24"
      height="24"
      viewBox="0 0 24 24"
      // 塗り分けが状態そのもの。テストもこの属性で確かめている。
      fill={filled ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinejoin="round"
      // 意味はボタンの aria-label が持つ。図形自体は読み上げ対象にしない。
      aria-hidden="true"
      focusable="false"
    >
      <path d="M12 2.6l2.9 5.9 6.5.95-4.7 4.58 1.11 6.47L12 17.45 6.19 20.5 7.3 14.03 2.6 9.45l6.5-.95L12 2.6z" />
    </svg>
  )
}

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

  const label = isFavorite ? 'お気に入り済み' : 'お気に入りに追加'

  return (
    <div className="space-y-2">
      <button
        type="button"
        onClick={() => toggle.mutate()}
        disabled={toggle.isPending}
        // 図形だけのボタンなので、名前は必ず属性で与える。
        aria-label={label}
        // マウス利用者にも意味が分かるようにする。
        title={label}
        // 押した結果ではなく、今どうなっているかを伝える。
        aria-pressed={isFavorite}
        className={
          isFavorite
            ? 'rounded-lg p-1.5 text-amber-400 hover:bg-amber-50 disabled:opacity-50'
            : 'rounded-lg p-1.5 text-slate-300 hover:bg-slate-100 hover:text-slate-400 disabled:opacity-50'
        }
      >
        <StarIcon filled={isFavorite} />
      </button>

      {toggle.error && <ErrorMessage error={toggle.error} />}
    </div>
  )
}
