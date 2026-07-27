import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'

import { MascotStatus } from '../../components/MascotStatus'
import { useCurrentUser } from '../auth/useCurrentUser'
import { getPlan } from '../billing/api'

type Props = {
  /** ロック中の機能名。何ができるようになるかを一言で。 */
  title: string
  /** 補足説明。どう役立つかを具体的に伝える。 */
  description: string
}

// PremiumLock はプレミアム限定機能のプレビューカード。
//
// premium 向けの分岐は持たない。呼び出し側（WeeklyPage / SavedWeeklyPage）が
// premium でないときにだけ描画するため、ここに premium の枝を書いても到達しない。
//
// 未ログインにも同じ加入導線を出す。/checkout は RequireAuth で守られており、
// 押すとログイン画面へ送られ、ログイン後に /checkout へ戻る（RequireAuth が
// state.from を残し、LoginPage がそこへ戻す）。
export function PremiumLock({ title, description }: Props) {
  const { user, isLoading } = useCurrentUser()
  const plan = useQuery({ queryKey: ['billing', 'plan'], queryFn: getPlan })

  if (isLoading) {
    return <MascotStatus>読み込み中…</MascotStatus>
  }

  return (
    <div className="mx-auto max-w-md rounded-2xl border border-kon-leaf-soft bg-white p-6 text-center">
      <p className="text-lg font-bold text-kon-ink">{title}</p>
      <p className="mt-2 text-sm text-kon-ink/70">{description}</p>

      <Link
        to="/checkout"
        className="mt-4 inline-block rounded-full bg-kon-leaf px-5 py-2 font-medium text-white hover:bg-kon-leaf/90"
      >
        プレミアムにアップグレード
      </Link>

      {/* 料金が引けないときは、この行だけを落とす。カードごと隠すと
          加入導線まで消え、この画面が直そうとした不具合に戻る。
          無料期間は初回加入に限るため「はじめての方は」を必ず添える
          （解約して free に戻った人にはトライアルが付かない）。 */}
      {plan.data && (
        <p className="mt-2 text-sm text-kon-ink/70">
          月額{plan.data.price}円（税込）
          {plan.data.trialDays > 0 &&
            `・はじめての方は${plan.data.trialDays}日間無料`}
        </p>
      )}

      {/* 押してからログイン画面に飛ばされるより、先に断っておく方が親切
          （HomePage の requiresAuth 表示と同じ流儀）。 */}
      {!user && <p className="mt-1 text-xs text-kon-ink/60">ログインが必要です</p>}

      <Link
        to="/pricing"
        className="mt-3 block text-sm text-kon-ink/70 underline decoration-kon-leaf underline-offset-2 hover:text-kon-ink"
      >
        プランの詳細を見る
      </Link>
    </div>
  )
}
