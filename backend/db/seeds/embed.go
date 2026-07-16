// Package seeds は献立マスタの初期データをバイナリに埋め込む。
package seeds

import _ "embed"

// MenusSQL は献立マスタ120件の INSERT 文。
// name の UNIQUE 制約と ON CONFLICT DO NOTHING により再実行しても重複しない。
//
//go:embed menus.sql
var MenusSQL string
