// Package domain は献立提案のドメインモデルを定義する。
// このパッケージは他のどの層にも依存しない。
package domain

import "errors"

// ErrInvalidGenre は文字列が既知のジャンルに一致しないことを表す。
var ErrInvalidGenre = errors.New("不正なジャンルです")

// Genre は献立の料理ジャンル。
type Genre string

// 定義済みのジャンル。DBの genre カラムに格納される値と一致する。
const (
	GenreJapanese Genre = "japanese"
	GenreWestern  Genre = "western"
	GenreChinese  Genre = "chinese"
	GenreOther    Genre = "other"
)

// AllGenres は全ジャンルを定義順で返す。
func AllGenres() []Genre {
	return []Genre{GenreJapanese, GenreWestern, GenreChinese, GenreOther}
}

// ParseGenre は文字列を Genre に変換する。
// 表記ゆれを許容すると DB の値と乖離するため、完全一致のみを受け付ける。
func ParseGenre(s string) (Genre, error) {
	g := Genre(s)
	if !g.Valid() {
		return "", ErrInvalidGenre
	}
	return g, nil
}

// Valid は定義済みのジャンルかどうかを返す。
func (g Genre) Valid() bool {
	switch g {
	case GenreJapanese, GenreWestern, GenreChinese, GenreOther:
		return true
	default:
		return false
	}
}

// String は DB およびAPIで用いる文字列表現を返す。
func (g Genre) String() string {
	return string(g)
}
