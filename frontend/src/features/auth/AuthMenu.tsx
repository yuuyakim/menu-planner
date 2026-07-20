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
            className="rounded-full bg-emerald-50 px-3 py-1 text-sm text-emerald-800"
          >
            ログアウトしました
          </span>
        )}
        <Link to="/login" className="text-slate-600 hover:text-slate-900">
          ログイン
        </Link>
      </span>
    )
  }

  return (
    <span className="ml-auto flex items-center gap-3">
      <span className="text-sm text-slate-700">{user.displayName}</span>
      <button
        type="button"
        onClick={() => logout.mutate(undefined, { onSettled: () => setNotice(true) })}
        disabled={logout.isPending}
        className="text-sm text-slate-600 underline hover:text-slate-900 disabled:text-slate-400"
      >
        ログアウト
      </button>
    </span>
  )
}
