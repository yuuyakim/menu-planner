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

// PlanView は状態表示の中身。ボタンは「アップグレード」「ポータル」「無し」の3択。
interface PlanView {
  label: string
  action: 'upgrade' | 'portal' | 'none'
}

// planView は SubscriptionInfo を画面表示（ラベル＋ボタン種別）に変換する。
// 表示・導線は spec の状態遷移表どおり status（と cancelAtPeriodEnd, hasPortal）
// で決まる。plan は「無料/有料の名称」でしかなく、ここでは使わない
// （plan だけで分岐すると、有料ステータスなのにポータルが無い場合や
// past_due の復帰導線が抜け落ちる）。
function planView(s: SubscriptionInfo): PlanView {
  const end = s.currentPeriodEnd ? formatJst(s.currentPeriodEnd) : ''

  switch (s.status) {
    case 'none':
      return { label: '無料プラン', action: 'upgrade' }
    case 'canceled':
      return { label: '解約済み（無料プラン）', action: 'upgrade' }
    case 'trialing':
    case 'active':
    case 'past_due': {
      if (!s.hasPortal) {
        // 手動付与などポータルが無い有料ステータス。課金されないので
        // 「次回請求」ではなく「まで有効」。ボタンは出さない。
        return { label: `プレミアム（${end}まで有効）`, action: 'none' }
      }
      if (s.status === 'trialing') {
        return {
          label: `プレミアム（無料お試し中）／初回課金 ${end}`,
          action: 'portal',
        }
      }
      if (s.status === 'past_due') {
        return {
          label: 'プレミアム（お支払いの確認中。カード情報の更新をお願いします）',
          action: 'portal',
        }
      }
      if (s.cancelAtPeriodEnd) {
        return {
          label: `プレミアム（${end}で解約予定。それまでご利用いただけます）`,
          action: 'portal',
        }
      }
      return { label: `プレミアム（次回請求 ${end}）`, action: 'portal' }
    }
  }
}

// AccountPage は「アカウント設定 > プランの管理」画面。
// 無料/解約済み（status が none/canceled）ならアップグレード導線、
// 有料ステータス（trialing/active/past_due）でポータルが使えるなら
// Stripe 顧客ポータルへの導線を出す。手動付与などポータルが無い場合は
// 状態表示のみにする（誤操作でポータルへ送れないため）。
// 有料ステータス中はアップグレード導線を出さない
// （既に加入済みなので ErrAlreadySubscribed で行き止まりになる）。
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

  const view = sub.data ? planView(sub.data) : null

  return (
    <section className="mx-auto max-w-md space-y-4">
      <h1 className="text-2xl font-bold text-kon-ink">アカウント設定</h1>
      <h2 className="font-medium text-kon-ink">プランの管理</h2>

      {sub.isPending && <MascotStatus>読み込み中…</MascotStatus>}

      {sub.error && <ErrorMessage error={sub.error} />}

      {view && (
        <>
          <p className="text-kon-ink">{view.label}</p>

          {portal.error && <ErrorMessage error={portal.error} />}

          {view.action === 'upgrade' && (
            <Link
              to="/checkout"
              className="inline-block rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95"
            >
              プレミアムにアップグレード
            </Link>
          )}

          {view.action === 'portal' && (
            <button
              type="button"
              disabled={portal.isPending}
              onClick={() => portal.mutate()}
              className="rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95 disabled:cursor-not-allowed disabled:bg-kon-leaf-soft disabled:text-kon-ink/70"
            >
              {portal.isPending ? '処理中…' : 'プランを管理する'}
            </button>
          )}
        </>
      )}
    </section>
  )
}
