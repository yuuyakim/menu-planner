import { Link } from 'react-router'

import { useCurrentUser, useLogout } from './useCurrentUser'

// AuthMenu はヘッダの右端に置くログイン状態の表示。
// 認証済みなら表示名とログアウト、未認証ならログインへの導線を出す。
export function AuthMenu() {
  const { user, isLoading } = useCurrentUser()
  const logout = useLogout()

  // 判定前に「ログイン」を出すと、認証済みの利用者に一瞬だけ
  // 未ログインの表示がちらつく。判定が付くまでは何も出さない。
  if (isLoading) return <span className="ml-auto" />

  if (!user) {
    return (
      <Link
        to="/login"
        className="ml-auto text-slate-600 hover:text-slate-900"
      >
        ログイン
      </Link>
    )
  }

  return (
    <span className="ml-auto flex items-center gap-3">
      <span className="text-sm text-slate-700">{user.displayName}</span>
      <button
        type="button"
        onClick={() => logout.mutate()}
        disabled={logout.isPending}
        className="text-sm text-slate-600 underline hover:text-slate-900 disabled:text-slate-400"
      >
        ログアウト
      </button>
    </span>
  )
}
