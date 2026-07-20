package domain

import "time"

// Favorite はお気に入り1件の読み取りモデル。献立の情報を含む。
//
// お気に入り自体の識別子は持たない。1ユーザーが同じ献立を二重に登録できない
// （favorites の UNIQUE 制約）ため、(ユーザー, 献立) の組で一意に定まり、
// APIも献立IDで削除する（DELETE /favorites/:menuId）。
type Favorite struct {
	// Menu はお気に入りに入っている献立。
	Menu Menu
	// CreatedAt は登録した時刻。一覧の並び順に使う。
	CreatedAt time.Time
}
