import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router'

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
  const location = useLocation()
  const [promptingLogin, setPromptingLogin] = useState(false)

  // Esc で閉じられるようにする。開いている間だけ待ち受ける。
  useEffect(() => {
    if (!promptingLogin) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPromptingLogin(false)
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [promptingLogin])

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

  const label = isFavorite ? 'お気に入り済み' : 'お気に入りに追加'

  // 未ログインでも星は出す。隠すと機能の存在に気づけない。
  // 押されたときに、何をすれば使えるのかをその場で伝える。
  function onClick() {
    if (!user) {
      setPromptingLogin(true)
      return
    }
    toggle.mutate()
  }

  return (
    // 星はカードの右端に置く。エラーは絶対配置で重ねて出し、
    // 失敗したときだけカードの高さが変わる（＝並びがずれる）のを防ぐ。
    <div className="relative shrink-0">
      <button
        type="button"
        onClick={onClick}
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

      {promptingLogin && (
        <div
          role="dialog"
          aria-label="お気に入りにはログインが必要です"
          className="absolute right-0 top-full z-20 mt-1 w-64 rounded-lg border border-slate-200 bg-white p-4 shadow-lg"
        >
          <p className="text-sm text-slate-700">
            ログインすると、気に入った献立をお気に入りに登録できます。
          </p>
          <div className="mt-3 flex items-center gap-3">
            <Link
              // ログイン後は今いる画面に戻す。お気に入りにしたかった献立を
              // 探し直させない（戻り先は RequireAuth と同じ仕組み）。
              to="/login"
              state={{ from: location.pathname }}
              className="rounded-lg bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-emerald-700"
            >
              ログイン / 新規登録
            </Link>
            <button
              type="button"
              onClick={() => setPromptingLogin(false)}
              className="text-sm text-slate-600 underline"
            >
              閉じる
            </button>
          </div>
        </div>
      )}

      {toggle.error && (
        <div className="absolute right-0 top-full z-10 mt-1 w-max max-w-64">
          <ErrorMessage error={toggle.error} />
        </div>
      )}
    </div>
  )
}
