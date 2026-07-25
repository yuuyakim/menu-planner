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
