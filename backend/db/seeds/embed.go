// Package seeds は献立マスタの初期データをバイナリに埋め込む。
package seeds

import _ "embed"

// MenusSQL は献立マスタ120件の INSERT 文。
// name の UNIQUE 制約と ON CONFLICT DO NOTHING により再実行しても重複しない。
//
//go:embed menus.sql
var MenusSQL string

// IngredientsSQL は食材マスタの INSERT 文（spec.md 14章）。
// 調味料は含まない。
//
//go:embed ingredients.sql
var IngredientsSQL string

// MenuIngredientsSQL は献立と食材の紐付けの INSERT 文。
// menus / ingredients の後に流す必要がある（名前で結合するため）。
//
//go:embed menu_ingredients.sql
var MenuIngredientsSQL string
