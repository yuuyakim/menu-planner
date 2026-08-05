import { Link } from 'react-router'

import type { DegradedReason } from '../../api/types'

type Props = {
  unresolved: string[]
  degraded: boolean
  reason?: DegradedReason
}

// degradedMessages は縮退の理由ごとの文言（設計 10章）。
//
// **値は5つ、文言は4つ。** counter_unavailable は llm_error と同じ文言を出す。
// 利用者から見ればどちらも「今うまく読めない」で、区別が要るのは運用の側だけ。
const partialMessage = '一部だけ読み取れました。残りは下から選んでください。'

const degradedMessages: Record<DegradedReason, string> = {
  llm_error: partialMessage,
  counter_unavailable: partialMessage,
  anon_daily_limit:
    '今日の読み取り上限に達しました。ログインすると回数が増えます。',
  user_daily_limit: '今日の読み取り上限に達しました。明日また使えます。',
  service_daily_limit:
    'ただいま読み取りが混み合っています。時間をおいてお試しください。',
}

// limitReasons は「上限に達した」系の理由。障害系（llm_error /
// counter_unavailable）と違い、チェックボックスの経路がまだ使えることを添える。
const limitReasons: DegradedReason[] = [
  'anon_daily_limit',
  'user_daily_limit',
  'service_daily_limit',
]

// ResolveResultPanel は読み取りの結果のうち、チェックに現れないものを伝える。
//
// **ピッカーの上に置く。** IngredientPicker は max-h-[55vh] のスクロール領域を
// 持つため、入ったチェックが画面外になりうる。変化に気付ける位置に出す（設計 6.2）。
export function ResolveResultPanel({ unresolved, degraded, reason }: Props) {
  if (unresolved.length === 0 && !degraded) return null

  // 理由が無い縮退は、上限ではなく LLM 側の失敗として扱う。
  // **未知の値にも partialMessage で落ちる。** reason は型上 DegradedReason
  // だが実際はAPIから届く値で、backend が先にデプロイされて5値目が増えると
  // 型では防げない。undefined のまま出すと空の灰色カードになるため、
  // マップに無ければ partialMessage を使う。
  const message = degradedMessages[reason ?? 'llm_error'] ?? partialMessage
  // ログインしても増えないケースで導線を出すと誤導になる。
  const showLogin = reason === 'anon_daily_limit'
  const isLimit = reason !== undefined && limitReasons.includes(reason)

  return (
    // aria-label を付けるのは、IngredientPicker の選択数表示も role="status" を
    // 持つため。名前が無いと支援技術（とテスト）がどちらの通知か区別できない。
    <div className="space-y-2" role="status" aria-label="読み取りの結果">
      {degraded && (
        <p className="rounded-2xl bg-kon-cream px-5 py-3 text-sm text-kon-ink/80">
          {message}
          {showLogin && (
            <Link
              to="/login"
              className="mt-2 inline-block rounded-full border border-kon-leaf-soft bg-white px-4 py-1.5 text-sm font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream"
            >
              ログイン
            </Link>
          )}
          {isLimit && (
            <span className="mt-1 block text-kon-ink/60">
              下のリストから選んで探すことはできます。
            </span>
          )}
        </p>
      )}
      {unresolved.length > 0 && (
        <p className="rounded-2xl bg-kon-cream px-5 py-3 text-sm text-kon-ink/80">
          登録がありませんでした: {unresolved.join('・')}
          <span className="mt-1 block text-kon-ink/60">
            この{unresolved.length}件は検索に使われません。
          </span>
        </p>
      )}
    </div>
  )
}
