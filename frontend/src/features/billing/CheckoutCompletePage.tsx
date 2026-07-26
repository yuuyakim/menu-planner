import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Link } from 'react-router'

import { fetchMe } from '../auth/api'
import { meQueryKey } from '../auth/useCurrentUser'

// maxPolls は premium への反映を待つ最大の再取得回数。
// Webhook の到達には多少の遅延がありうるが、無限に待たせず諦めどころを設ける。
const maxPolls = 10
// pollIntervalMs はポーリングの間隔。
const pollIntervalMs = 2000

// CheckoutCompletePage は Stripe Checkout から戻ってきたときの画面。
//
// Webhook によるプラン反映は非同期なので、戻ってきた直後はまだ free のことがある。
// /auth/me を短い間隔で取り直し、premium になった時点で成功表示に切り替える。
// 上限回数まで反映が確認できなければ、諦めて後で確認してもらう案内に切り替える。
export function CheckoutCompletePage() {
  // 取得の試行回数を数える。成功・失敗どちらでも 1 回の試行として数えないと、
  // /auth/me が持続的にエラーを返す場合（5xx・タイムアウト等）に
  // 上限判定が働かず無限にポーリングしてしまう。
  const [attempts, setAttempts] = useState(0)

  const me = useQuery({
    queryKey: meQueryKey,
    queryFn: fetchMe,
    // 401 は取り直しても変わらない。ポーリングは premium 反映待ちの手段であって、
    // 通信エラーの再試行はここでの目的ではない。
    retry: false,
    // premium になったら、または（成功+失敗の）総試行回数が上限に達したら止める。
    // この callback が受け取る query は内部状態そのものなので dataUpdateCount /
    // errorUpdateCount を持つ。成功のみを数えると、持続的なエラー時に
    // dataUpdateCount が増えず無限にポーリングし続けてしまう。
    refetchInterval: (query) => {
      if (query.state.data?.plan === 'premium') {
        return false
      }
      if (query.state.dataUpdateCount + query.state.errorUpdateCount >= maxPolls) {
        return false
      }
      return pollIntervalMs
    },
  })

  useEffect(() => {
    if (me.dataUpdatedAt || me.errorUpdatedAt) {
      setAttempts((n) => n + 1)
    }
    // dataUpdatedAt と errorUpdatedAt のどちらが変化しても 1 試行として数える。
  }, [me.dataUpdatedAt, me.errorUpdatedAt])

  const active = me.data?.plan === 'premium'
  const exhausted = attempts >= maxPolls

  if (active) {
    return (
      <section className="mx-auto max-w-md space-y-2 text-center">
        <h1 className="text-xl font-bold text-kon-ink">プレミアムが有効になりました</h1>
        <p className="text-kon-ink">
          ありがとうございます。1週間の献立をまとめて計画できます。
        </p>
        <Link
          to="/weekly"
          className="mt-4 inline-block rounded-full bg-kon-leaf px-4 py-2 font-medium text-white"
        >
          1週間の献立へ
        </Link>
      </section>
    )
  }

  return (
    <section className="mx-auto max-w-md space-y-2 text-center">
      <h1 className="text-xl font-bold text-kon-ink">お手続きを受け付けました</h1>
      <p className="text-kon-ink">
        {exhausted
          ? '反映まで少し時間がかかることがあります。しばらくしてから再読み込みしてください。'
          : 'プレミアムの有効化を確認しています…'}
      </p>
    </section>
  )
}
