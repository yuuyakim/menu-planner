import { apiGet, apiPost } from '../../api/client'

export interface BillingPreview {
  price: number
  currency: string
  trialDays: number
  trialEligible: boolean
  firstBillingAt: string
  planManagementPath: string
}

/** getBillingPreview は申込確認画面（特商法12条の6の表示）の値を取得する。 */
export function getBillingPreview(): Promise<BillingPreview> {
  return apiGet<BillingPreview>('/billing/preview')
}

/** createCheckoutSession は Checkout セッションを作り、遷移先の Stripe URL を返す。 */
export function createCheckoutSession(): Promise<{ url: string }> {
  return apiPost<{ url: string }>('/billing/checkout-session')
}

export interface SubscriptionInfo {
  plan: 'free' | 'premium'
  status: 'none' | 'trialing' | 'active' | 'past_due' | 'canceled'
  currentPeriodEnd: string | null
  cancelAtPeriodEnd: boolean
  hasPortal: boolean
}

/** getSubscription は現在のプラン状態を取得する（表示用）。 */
export function getSubscription(): Promise<SubscriptionInfo> {
  return apiGet<SubscriptionInfo>('/billing/subscription')
}

/** createPortalSession は Stripe 顧客ポータルのセッションを作り、遷移先 URL を返す。 */
export function createPortalSession(): Promise<{ url: string }> {
  return apiPost<{ url: string }>('/billing/portal-session')
}

/** PlanInfo は誰にでも同じ、プランの公開情報。 */
export interface PlanInfo {
  price: number
  currency: string
  trialDays: number
}

/** getPlan はプランの公開情報を取得する（未ログインでも呼べる）。 */
export function getPlan(): Promise<PlanInfo> {
  return apiGet<PlanInfo>('/billing/plan')
}
