import { useEffect, useState } from 'react'
import { Link } from 'react-router'

import { useCurrentUser, useLogout } from './useCurrentUser'

// noticeDuration はログアウトの知らせを出しておく時間。
// 出しっぱなしにすると、次の操作のときに古い知らせが残って紛らわしい。
const noticeDuration = 5000

// AuthMenu はヘッダの右端に置くログイン状態の表示。
// 認証済みなら表示名とログアウト、未認証ならログインへの導線を出す。
export function AuthMenu() {
  const { user, isLoading } = useCurrentUser()
  const logout = useLogout()
  const [notice, setNotice] = useState(false)

  useEffect(() => {
    if (!notice) return
    const timer = setTimeout(() => setNotice(false), noticeDuration)
    return () => clearTimeout(timer)
  }, [notice])

  // 判定前に「ログイン」を出すと、認証済みの利用者に一瞬だけ
  // 未ログインの表示がちらつく。判定が付くまでは何も出さない。
  if (isLoading) return <span className="ml-auto" />

  if (!user) {
    return (
      <span className="ml-auto flex items-center gap-3">
        {/*
          検索画面はログイン有無で見た目が変わらないため、ヘッダの表示が
          切り替わるだけでは「ログアウトできたのか」が分かりにくい。
          何が起きたかを言葉で伝える。
        */}
        {notice && (
          <span
            role="status"
            className="rounded-full bg-kon-leaf/20 px-3 py-1 text-sm text-kon-ink"
          >
            ログアウトしました
          </span>
        )}
        <Link
          to="/login"
          className="whitespace-nowrap text-sm text-kon-ink/70 hover:text-kon-ink"
        >
          ログイン
        </Link>
      </span>
    )
  }

  return (
    <span className="ml-auto flex items-center gap-3">
      {/*
        プレミアムであることの表示。free には何も出さない。
        アップグレード導線は買い物リストのバナー等、文脈に応じた場所（→/checkout）
        で既に案内しているため、ここでは重複した勧誘をせず状態の表示に留める。
      */}
      {user.plan === 'premium' && (
        <span
          aria-label="プレミアム会員"
          className="rounded-full bg-kon-leaf/20 px-2 py-0.5 text-xs text-kon-ink"
        >
          プレミアム
        </span>
      )}
      <span className="text-sm text-kon-ink/80">{user.displayName}</span>
      <button
        type="button"
        onClick={() => logout.mutate(undefined, { onSettled: () => setNotice(true) })}
        disabled={logout.isPending}
        className="whitespace-nowrap text-sm text-kon-ink/70 underline decoration-kon-leaf underline-offset-2 hover:text-kon-ink disabled:text-kon-ink/40"
      >
        ログアウト
      </button>
    </span>
  )
}
