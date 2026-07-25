import { useMutation, useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'

import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotStatus } from '../../components/MascotStatus'
import { createPortalSession, getSubscription, type SubscriptionInfo } from './api'

// formatJst は日時をJSTの日本語表記（年月日のみ）にする。
function formatJst(iso: string): string {
  return new Intl.DateTimeFormat('ja-JP', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    timeZone: 'Asia/Tokyo',
  }).format(new Date(iso))
}

// planLabel は購読状態を利用者向けの一文にする。
function planLabel(s: SubscriptionInfo): string {
  if (s.plan !== 'premium') {
    return s.status === 'canceled' ? '解約済み（無料プラン）' : '無料プラン'
  }
  const end = s.currentPeriodEnd ? formatJst(s.currentPeriodEnd) : ''
  if (s.status === 'trialing') return `プレミアム（無料お試し中）／初回課金 ${end}`
  if (s.status === 'past_due') {
    return 'プレミアム（お支払いの確認中。カード情報の更新をお願いします）'
  }
  if (s.cancelAtPeriodEnd) return `プレミアム（${end}で解約予定。それまでご利用いただけます）`
  return `プレミアム（次回請求 ${end}）`
}

// AccountPage は「アカウント設定 > プランの管理」画面。
// 無料/解約済みならアップグレード導線、有料でポータルが使えるなら
// Stripe 顧客ポータルへの導線を出す。手動付与などポータルが無い場合は
// 状態表示のみにする（誤操作でポータルへ送れないため）。
export function AccountPage() {
  const sub = useQuery({
    queryKey: ['billing', 'subscription'],
    queryFn: getSubscription,
  })

  const portal = useMutation({
    mutationFn: createPortalSession,
    onSuccess: ({ url }) => {
      // Stripe のホスト側画面へ遷移する。SPA内の遷移ではないので
      // react-router の navigate ではなく window.location を使う。
      window.location.href = url
    },
  })

  return (
    <section className="mx-auto max-w-md space-y-4">
      <h1 className="text-2xl font-bold text-kon-ink">アカウント設定</h1>
      <h2 className="font-medium text-kon-ink">プランの管理</h2>

      {sub.isPending && <MascotStatus>読み込み中…</MascotStatus>}

      {sub.error && <ErrorMessage error={sub.error} />}

      {sub.data && (
        <>
          <p className="text-kon-ink">{planLabel(sub.data)}</p>

          {portal.error && <ErrorMessage error={portal.error} />}

          {sub.data.plan !== 'premium' ? (
            <Link
              to="/checkout"
              className="inline-block rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95"
            >
              プレミアムにアップグレード
            </Link>
          ) : (
            sub.data.hasPortal && (
              <button
                type="button"
                disabled={portal.isPending}
                onClick={() => portal.mutate()}
                className="rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95 disabled:cursor-not-allowed disabled:bg-kon-leaf-soft disabled:text-kon-ink/70"
              >
                {portal.isPending ? '処理中…' : 'プランを管理する'}
              </button>
            )
          )}
        </>
      )}
    </section>
  )
}
