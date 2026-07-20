import { apiGet, apiPost } from '../../api/client'
import type { DayMenu, Menu, Recipe } from '../../api/types'
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

/** fetchRecipes は献立のレシピリンクを取得する。0件でも成功として空配列を返す。 */
export async function fetchRecipes(menuId: string): Promise<Recipe[]> {
  const res = await apiGet<{ recipes: Recipe[] }>(`/menus/${menuId}/recipes`)
  return res.recipes
}

/** suggestWeekly は1週間分（7日）の献立を生成する。 */
export async function suggestWeekly(filter: MenuFilter): Promise<DayMenu[]> {
  const res = await apiPost<{ week: DayMenu[] }>('/menus/suggest-weekly', {
    genre: filter.genre,
    difficulty: filter.difficulty,
  })
  return res.week
}

/**
 * rerollDay は週間献立の指定日だけを引き直す。
 * サーバは週の状態を持たないため、現在の週の献立IDを送る必要がある。
 */
export async function rerollDay(
  day: number,
  week: DayMenu[],
  filter: MenuFilter,
): Promise<Menu> {
  const res = await apiPost<{ menu: Menu }>('/menus/reroll-day', {
    day,
    week: week.map((d) => d.menu.id),
    genre: filter.genre,
    difficulty: filter.difficulty,
  })
  return res.menu
}

/** fetchMenu は献立を1件取得する。 */
export async function fetchMenu(menuId: string): Promise<Menu> {
  const res = await apiGet<{ menu: Menu }>(`/menus/${menuId}`)
  return res.menu
}
