import { Link } from 'react-router'

import { MascotStatus } from '../../components/MascotStatus'
import { useCurrentUser } from '../auth/useCurrentUser'

type Props = {
  /** ロック中の機能名。何ができるようになるかを一言で。 */
  title: string
  /** 補足説明。どう役立つかを具体的に伝える。 */
  description: string
}

// PremiumLock はプレミアム限定機能のプレビューカード。
//
// 決済フローは未実装（設計スコープ外）。ログイン済み free の
// 「アップグレード」導線は当面は案内文言に留め、決済導入時に差し替える。
export function PremiumLock({ title, description }: Props) {
  const { user, isLoading } = useCurrentUser()

  if (isLoading) {
    return <MascotStatus>読み込み中…</MascotStatus>
  }

  return (
    <div className="mx-auto max-w-md rounded-2xl border border-kon-leaf-soft bg-white p-6 text-center">
      <p className="text-lg font-bold text-kon-ink">{title}</p>
      <p className="mt-2 text-sm text-kon-ink/70">{description}</p>
      {user ? (
        // ログイン済み free: アップグレード導線。決済導線は未実装のため当面は案内文言。
        <p className="mt-4 rounded-lg bg-kon-leaf/10 p-3 text-sm text-kon-ink">
          プレミアムプランでご利用いただけます。
        </p>
      ) : (
        <Link
          to="/login"
          className="mt-4 inline-block rounded-full bg-kon-leaf px-5 py-2 font-medium text-white hover:bg-kon-leaf/90"
        >
          ログインする
        </Link>
      )}
    </div>
  )
}
