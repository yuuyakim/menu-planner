import { apiGet } from '../../api/client'
import type { Menu } from '../../api/types'
import type { MenuFilter } from './SearchForm'

// toQuery は絞り込み条件をクエリ文字列にする。
// undefined（＝すべて）は載せない。空文字を送っても同じ結果になるが、
// URLに出さない方が「指定していない」ことが読み取れる。
function toQuery(filter: MenuFilter): string {
  const params = new URLSearchParams()
  if (filter.genre) params.set('genre', filter.genre)
  if (filter.difficulty) params.set('difficulty', filter.difficulty)
  const query = params.toString()
  return query ? `?${query}` : ''
}

/** suggestMenu は条件に合う献立を1件取得する。 */
export async function suggestMenu(filter: MenuFilter): Promise<Menu> {
  const res = await apiGet<{ menu: Menu }>(`/menus/suggest${toQuery(filter)}`)
  return res.menu
}
