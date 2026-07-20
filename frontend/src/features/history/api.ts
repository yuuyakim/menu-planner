import { apiDelete, apiGet } from '../../api/client'
import type { HistoryItem } from '../../api/types'

/** historiesQueryKey は履歴一覧のキャッシュキー。削除後の再取得にも使う。 */
export const historiesQueryKey = ['histories'] as const

/**
 * fetchHistories は検索履歴を取得する。
 * 並び順はサーバが決める（新しい順）。画面では並べ替えない。
 */
export async function fetchHistories(): Promise<HistoryItem[]> {
  const res = await apiGet<{ histories: HistoryItem[] }>('/histories')
  return res.histories
}

/** deleteHistory は履歴を1件削除する。 */
export function deleteHistory(id: string): Promise<void> {
  return apiDelete(`/histories/${id}`)
}

/** deleteAllHistories は履歴を全件削除する。0件でも成功する。 */
export function deleteAllHistories(): Promise<void> {
  return apiDelete('/histories')
}
