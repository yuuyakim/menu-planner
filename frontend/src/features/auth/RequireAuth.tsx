import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'

import { MascotStatus } from '../../components/MascotStatus'
import { useCurrentUser } from './useCurrentUser'

// RequireAuth は認証済みのときだけ子を表示する。
// 未認証ならログイン画面へ送り、元の行き先を state に残して
// ログイン後にそこへ戻せるようにする。
export function RequireAuth({ children }: { children: ReactNode }) {
  const { user, isLoading } = useCurrentUser()
  const location = useLocation()

  // 判定が付くまでは何も決めない。ここでログイン画面に飛ばすと、
  // 認証済みの利用者にも一瞬ログイン画面が見えてしまう。
  if (isLoading) {
    return <MascotStatus>読み込み中…</MascotStatus>
  }

  if (!user) {
    // replace にするのは、戻るボタンで守られた画面に戻ってこないようにするため。
    return <Navigate to="/login" state={{ from: location.pathname }} replace />
  }

  return <>{children}</>
}
