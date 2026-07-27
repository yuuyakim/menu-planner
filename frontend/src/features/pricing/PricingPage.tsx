import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'

import { useCurrentUser } from '../auth/useCurrentUser'
import { getPlan } from '../billing/api'

// features は比較表の中身。spec.md 2.11 の線引きの表が正。
//
// **保存件数（50件）は書かない。** 上限の数値をフロントが持つと二重管理に
// なるため（spec.md「上限の数値を返さない理由」）、機能の有無だけを示す。
const features = [
  { label: '献立を1食提案・レシピ・履歴・お気に入り', free: true },
  { label: '冷蔵庫の食材から探す', free: true },
  { label: '買い物リストの作成', free: true },
  { label: '1週間の献立を組み立てる', free: false },
  { label: '1日だけ引き直す', free: false },
  { label: '週間献立の保存・呼び出し', free: false },
  { label: '買い物リストのチェックを残す', free: false },
] as const

// 2つの CTA（premium 向け・free 向け）で見た目を揃えるための共通クラス。
// 文言と遷移先だけが違うので、逐語重複を避けて1箇所にまとめる。
const ctaLinkClassName =
  'inline-block rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95'

// PricingPage は料金と機能の比較を出す公開ページ。
//
// 未ログインでも見える（RequireAuth で包まない）。加入を検討する前に見る
// 画面であり、料金を知るのにログインを要求するのは筋が通らないため。
export function PricingPage() {
  const { user, isLoading } = useCurrentUser()
  const plan = useQuery({ queryKey: ['billing', 'plan'], queryFn: getPlan })

  return (
    <section className="mx-auto max-w-xl space-y-6">
      <h1 className="text-2xl font-bold text-kon-ink">料金プラン</h1>

      <p className="text-kon-ink/75">
        献立を1食ずつ探すのは無料のまま使えます。1週間分をまとめて計画したいときに
        プレミアムをどうぞ。
      </p>

      <table className="w-full border-collapse text-sm">
        <caption className="sr-only">無料プランとプレミアムプランの比較</caption>
        <thead>
          <tr className="border-b border-kon-leaf-soft">
            <th scope="col" className="py-2 text-left font-medium text-kon-ink">
              できること
            </th>
            <th scope="col" className="w-20 py-2 text-center font-medium text-kon-ink">
              無料
            </th>
            <th scope="col" className="w-28 py-2 text-center font-medium text-kon-ink">
              プレミアム
              {/* 料金が引けないときはこの行だけ落とす。表そのものは出す。 */}
              {plan.data && (
                <span className="mt-0.5 block text-xs font-normal text-kon-ink/70">
                  月額{plan.data.price}円（税込）
                </span>
              )}
            </th>
          </tr>
        </thead>
        <tbody>
          {features.map((f) => (
            <tr key={f.label} className="border-b border-kon-leaf-soft/60">
              <td className="py-2 text-kon-ink/85">{f.label}</td>
              {/* 記号だけだと読み上げで意味が伝わらないため、文字を添えて隠す。 */}
              <td className="py-2 text-center text-kon-ink">
                {f.free ? '○' : '—'}
                <span className="sr-only">{f.free ? '使える' : '使えない'}</span>
              </td>
              <td className="py-2 text-center text-kon-ink">
                ○<span className="sr-only">使える</span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {plan.data && plan.data.trialDays > 0 && (
        <p className="text-sm text-kon-ink/75">
          はじめての方は{plan.data.trialDays}日間無料でお試しいただけます。
        </p>
      )}

      {/* 判定が付くまで CTA を出さない。先に free 向けを描いて差し替えると、
          premium の利用者に一瞬「プレミアムを試す」が見える
          （AuthMenu が同じ理由で判定前の描画を避けている）。 */}
      {!isLoading &&
        (user?.plan === 'premium' ? (
          <Link to="/account" className={ctaLinkClassName}>
            プランを管理する
          </Link>
        ) : (
          <Link to="/checkout" className={ctaLinkClassName}>
            プレミアムを試す
          </Link>
        ))}

      <p className="text-sm text-kon-ink/70">
        価格・支払方法・解約の条件は{' '}
        <Link
          to="/legal/tokushoho"
          className="underline decoration-kon-leaf underline-offset-2 hover:text-kon-ink"
        >
          特定商取引法に基づく表記
        </Link>{' '}
        をご確認ください。
      </p>
    </section>
  )
}
