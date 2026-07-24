import { apiDelete, apiGet, apiPost } from '../../api/client'
import type {
  DayMenu,
  Ingredient,
  Menu,
  MenuMatch,
  Recipe,
  SavedShoppingItem,
  SavedWeeklyMenu,
  ShoppingItem,
} from '../../api/types'
import { withDefaultRole, type MenuFilter } from './filter'

// toQuery は絞り込み条件をクエリ文字列にする。
// undefined（＝すべて）は載せない。空文字を送っても同じ結果になるが、
// URLに出さない方が「指定していない」ことが読み取れる。
function toQuery(filter: MenuFilter): string {
  const params = new URLSearchParams()
  if (filter.genre) params.set('genre', filter.genre)
  if (filter.difficulty) params.set('difficulty', filter.difficulty)
  // **role は常に載せる。** 省略はサーバ側で主菜になるため（spec.md 2.10）、
  // 「すべて」を選んだのに載せないと主菜に絞られてしまう。
  params.set('role', withDefaultRole(filter).role)
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
    role: withDefaultRole(filter).role,
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
    role: withDefaultRole(filter).role,
  })
  return res.menu
}

/** savedWeeklyMenusQueryKey は保存した週間献立のキャッシュキー。 */
export const savedWeeklyMenusQueryKey = ['saved-weekly-menus'] as const

/**
 * saveWeeklyMenu は組み立てた1週間分をサーバに保存する（spec.md 2.8）。
 *
 * 保存できる件数はプランで決まる（free 10件 / premium 50件、spec.md 2.11）。
 * 超過すると 409 が返る。押し出さずに断る仕様なので、呼び出し側は
 * 「古いものを消してもらう」案内をする必要がある。件数はサーバが detail に入れて返す。
 */
export async function saveWeeklyMenu(week: DayMenu[]): Promise<string> {
  const res = await apiPost<{ id: string }>('/weekly-menus', {
    days: week.map((d) => ({ day: d.day, menuId: d.menu.id })),
  })
  return res.id
}

/** fetchSavedWeeklyMenus は保存した週間献立を新しい順に取得する。 */
export async function fetchSavedWeeklyMenus(): Promise<SavedWeeklyMenu[]> {
  const res = await apiGet<{ weeklyMenus: SavedWeeklyMenu[] }>('/weekly-menus')
  return res.weeklyMenus
}

/** deleteSavedWeeklyMenu は保存した週間献立を1件削除する。 */
export async function deleteSavedWeeklyMenu(id: string): Promise<void> {
  await apiDelete(`/weekly-menus/${id}`)
}

/** ingredientsQueryKey は食材マスタのキャッシュキー。 */
export const ingredientsQueryKey = ['ingredients'] as const

/**
 * fetchAllIngredients は食材マスタを全件取得する。手持ちの食材を選ぶ選択肢に使う。
 * カテゴリ順→カナ順で返る（spec.md 5.6）。
 */
export async function fetchAllIngredients(): Promise<Ingredient[]> {
  const res = await apiGet<{ ingredients: Ingredient[] }>('/ingredients')
  return res.ingredients
}

/**
 * searchByIngredients は手持ちの食材で作れる献立を探す。
 *
 * 完全一致には絞らず、各候補に不足食材が付く（spec.md 2.9）。
 * 並びは「不足の少ない順 → 一致の多い順」で、上位20件まで。
 */
export async function searchByIngredients(
  ingredientIds: string[],
): Promise<MenuMatch[]> {
  const res = await apiPost<{ matches: MenuMatch[] }>(
    '/menus/search-by-ingredients',
    { ingredientIds },
  )
  return res.matches
}

/** fetchMenu は献立を1件取得する。 */
export async function fetchMenu(menuId: string): Promise<Menu> {
  const res = await apiGet<{ menu: Menu }>(`/menus/${menuId}`)
  return res.menu
}

/** fetchIngredients は献立に必要な食材を取得する。調味料は含まない。 */
export async function fetchIngredients(menuId: string): Promise<Ingredient[]> {
  const res = await apiGet<{ ingredients: Ingredient[] }>(
    `/menus/${menuId}/ingredients`,
  )
  return res.ingredients
}

/**
 * fetchShoppingList は複数の献立から買い物リストを取得する。
 *
 * 週間献立はサーバに保存していないため、献立IDを送って組み立ててもらう。
 */
export async function fetchShoppingList(
  menuIds: string[],
): Promise<ShoppingItem[]> {
  const res = await apiPost<{ items: ShoppingItem[] }>('/shopping-list', {
    menuIds,
  })
  return res.items
}

/** savedShoppingListQueryKey は保存済み週の買い物リストのキャッシュキー。 */
export function savedShoppingListQueryKey(savedId: string) {
  return ['saved-shopping-list', savedId] as const
}

/**
 * fetchSavedShoppingList は保存済み週の買い物リストを取得する。
 *
 * 未保存の週と違いサーバ側に状態があるため、チェック済みや手動追加も含めて
 * 差分適用後の形で返る（spec.md 14.5）。
 */
export async function fetchSavedShoppingList(
  savedId: string,
): Promise<SavedShoppingItem[]> {
  const res = await apiGet<{ items: SavedShoppingItem[] }>(
    `/weekly-menus/${savedId}/shopping-list`,
  )
  return res.items
}
