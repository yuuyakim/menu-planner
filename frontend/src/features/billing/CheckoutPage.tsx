import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router'

import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotStatus } from '../../components/MascotStatus'
import { createCheckoutSession, getBillingPreview } from './api'

// formatJst は日時をJSTの日本語表記にする（特商法上の表示に使う）。
function formatJst(iso: string): string {
  return new Intl.DateTimeFormat('ja-JP', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Asia/Tokyo',
  }).format(new Date(iso))
}

// CheckoutPage は Stripe Checkout に進む前の申込確認画面。
// 特商法12条の6が求める6項目（価格・支払方法・支払時期・提供時期・
// 解約方法・返金）をここでまとめて示し、同意を得てから決済に進ませる。
export function CheckoutPage() {
  const [agreed, setAgreed] = useState(false)

  const preview = useQuery({
    queryKey: ['billing', 'preview'],
    queryFn: getBillingPreview,
  })

  const start = useMutation({
    mutationFn: createCheckoutSession,
    onSuccess: ({ url }) => {
      // Stripe のホスト側画面へ遷移する。SPA内の遷移ではないので
      // react-router の navigate ではなく window.location を使う。
      window.location.href = url
    },
  })

  return (
    <section className="mx-auto max-w-md space-y-6">
      <h1 className="text-2xl font-bold text-kon-ink">お申込み内容の確認</h1>

      {preview.isPending && <MascotStatus>読み込み中…</MascotStatus>}

      {preview.error && <ErrorMessage error={preview.error} />}

      {preview.data && (
        <>
          <dl className="space-y-3 text-kon-ink">
            <div>
              <dt className="font-medium">プラン</dt>
              <dd>プレミアム（月額 {preview.data.price}円・税込）</dd>
            </div>

            {preview.data.trialEligible ? (
              <div>
                <dt className="font-medium">無料お試し</dt>
                <dd>
                  {preview.data.trialDays}
                  日間無料。初回課金は{' '}
                  <strong>{formatJst(preview.data.firstBillingAt)}</strong>
                  です。この日時より前に解約すれば課金されません。
                </dd>
              </div>
            ) : (
              <div>
                <dt className="font-medium">初回課金</dt>
                <dd>お申込み時（{formatJst(preview.data.firstBillingAt)}）</dd>
              </div>
            )}

            <div>
              <dt className="font-medium">解約方法</dt>
              <dd>
                「{preview.data.planManagementPath}」からいつでも解約できます（解約後も期末まで利用できます）。
              </dd>
            </div>

            <div>
              <dt className="font-medium">返金</dt>
              <dd>原則として返金はできません（当方の責による場合等を除く）。</dd>
            </div>

            <div>
              <dt className="font-medium">お支払い</dt>
              <dd>
                クレジットカード（決済代行：Stripe）。次の画面でカード情報を入力します。
              </dd>
            </div>
          </dl>

          <label className="flex items-start gap-2 text-sm text-kon-ink">
            <input
              type="checkbox"
              checked={agreed}
              onChange={(e) => setAgreed(e.target.checked)}
              className="mt-1"
            />
            <span>
              <Link
                to="/legal/terms"
                className="font-medium text-kon-ink underline decoration-kon-leaf decoration-2 underline-offset-2"
              >
                利用規約
              </Link>
              と
              <Link
                to="/legal/privacy"
                className="font-medium text-kon-ink underline decoration-kon-leaf decoration-2 underline-offset-2"
              >
                プライバシーポリシー
              </Link>
              に同意します
            </span>
          </label>

          {start.error && <ErrorMessage error={start.error} />}

          <button
            type="button"
            disabled={!agreed || start.isPending}
            onClick={() => start.mutate()}
            className="w-full rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95 disabled:cursor-not-allowed disabled:bg-kon-leaf-soft disabled:text-kon-ink/70"
          >
            {start.isPending ? '処理中…' : '無料お試しを開始する'}
          </button>
        </>
      )}
    </section>
  )
}
