import type { components } from './schema'

// schema.d.ts は自動生成物で、参照が `components['schemas']['Menu']` と冗長になる。
// アプリ側はこの別名だけを使い、生成物への依存をこのファイルに閉じ込める。
// API仕様が変われば再生成された型がここを通って全体に伝播する。

type Schemas = components['schemas']

export type Genre = Schemas['Genre']
export type Difficulty = Schemas['Difficulty']
export type SearchMode = Schemas['SearchMode']

/** Role は献立が一食の中で担う役割（spec.md 2.10）。 */
export type Role = Schemas['Role']
/** RoleFilter は検索での絞り込み。`all` は「絞り込まない」。 */
export type RoleFilter = Schemas['RoleFilter']

export type Menu = Schemas['Menu']
export type DayMenu = Schemas['DayMenu']
export type SavedWeeklyMenu = Schemas['SavedWeeklyMenu']
export type Recipe = Schemas['Recipe']
export type User = Schemas['User']
export type HistoryItem = Schemas['HistoryItem']
export type FavoriteItem = Schemas['FavoriteItem']

export type Ingredient = Schemas['Ingredient']
export type IngredientCategory = Ingredient['category']
export type ShoppingItem = Schemas['ShoppingItem']
export type MenuMatch = Schemas['MenuMatch']

/** ResolvedWord は自由記述から対応づいた食材1件。word は利用者が書いた元の語。 */
export type ResolvedWord = Schemas['ResolvedWord']
/** ResolveResult は自由記述の解決結果（spec.md 2.9 / 設計 4.1）。 */
export type ResolveResult = Schemas['ResolveResult']

/** SavedShoppingItem は保存済み週の買い物リスト1件（差分適用後の形）。 */
export type SavedShoppingItem = Schemas['SavedShoppingItem']
/** Origin は差分行の由来。derived=献立由来 / manual=手動追加。 */
export type Origin = Schemas['Origin']
/** ShoppingListOverride は overlay 一括置換で送る1件（PUT のリクエスト形）。 */
export type ShoppingListOverride =
  Schemas['ShoppingListOverridesRequest']['items'][number]

/** RFC 7807 の problem+json。 */
export type Problem = Schemas['Problem']

// 画面で選択肢を並べるための値。型は生成物から取り、値はここで定義する
// （OpenAPI の enum は型としてしか生成されないため）。
// 網羅は Record の型注釈が保証する。増減すれば型エラーになる。
export const genreLabels: Record<Genre, string> = {
  japanese: '和食',
  western: '洋食',
  chinese: '中華',
  other: 'その他',
}

export const difficultyLabels: Record<Difficulty, string> = {
  easy: '簡単',
  normal: '普通',
  elaborate: '手が込んだ',
}

// roleLabels は献立が持つ役割の表示名。カードの表示に使う。
export const roleLabels: Record<Role, string> = {
  main: '主菜',
  side: '副菜',
  soup: '汁物',
}

// roleFilterLabels は検索の選択肢。**「すべて」を末尾に置く。**
// ジャンル・難易度は既定が「すべて」なので先頭に置いているが、
// 役割の既定は「主菜」（spec.md 2.10）で、先頭に置くと既定に見えてしまう。
export const roleFilterLabels: Record<RoleFilter, string> = {
  ...roleLabels,
  all: 'すべて',
}

/** defaultRoleFilter は検索の既定。未指定を主菜に倒すのはサーバと同じ判断。 */
export const defaultRoleFilter: RoleFilter = 'main'

export const genres = Object.keys(genreLabels) as Genre[]
export const difficulties = Object.keys(difficultyLabels) as Difficulty[]

// categoryLabels は食材カテゴリの表示名。買い物リストの見出しに使う。
// 調味料は食材として持たないため（spec.md 14.4）ここにも無い。
export const categoryLabels: Record<IngredientCategory, string> = {
  vegetable: '野菜',
  meat: '肉',
  seafood: '魚介',
  dairy_egg: '卵・乳製品',
  staple: '主食',
  other: 'その他',
}

// categoryOrder は売り場を回る順に近い並び。サーバもこの順で返すが、
// 見出しを出す側でも順序を持たないと、サーバの並びに暗黙に依存してしまう。
export const categoryOrder: IngredientCategory[] = [
  'vegetable',
  'meat',
  'seafood',
  'dairy_egg',
  'staple',
  'other',
]
