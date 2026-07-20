import { apiDelete, apiGet, apiPost } from '../../api/client'
import type { FavoriteItem } from '../../api/types'

/** favoritesQueryKey はお気に入り一覧のキャッシュキー。更新後の再取得にも使う。 */
export const favoritesQueryKey = ['favorites'] as const

/**
 * fetchFavorites はお気に入りを取得する。
 * 並び順はサーバが決める（新しい順）。件数の上限は無い（spec.md 2.6）。
 */
export async function fetchFavorites(): Promise<FavoriteItem[]> {
  const res = await apiGet<{ favorites: FavoriteItem[] }>('/favorites')
  return res.favorites
}

/** addFavorite は献立をお気に入りに追加する。重複は 409 になる。 */
export function addFavorite(menuId: string): Promise<{ menuId: string }> {
  return apiPost<{ menuId: string }>('/favorites', { menuId })
}

/** removeFavorite はお気に入りから献立を外す。 */
export function removeFavorite(menuId: string): Promise<void> {
  return apiDelete(`/favorites/${menuId}`)
}
