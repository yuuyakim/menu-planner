package domain

import "errors"

// ErrInvalidSearchMode は文字列が既知の検索種別に一致しないことを表す。
var ErrInvalidSearchMode = errors.New("不正な検索種別です")

// SearchMode は履歴に残す検索の種類。1食分か週間かを区別する。
type SearchMode string

// 定義済みの検索種別。DBの search_mode カラムに格納される値と一致する。
const (
	// SearchModeSingle は1食分の献立検索。
	SearchModeSingle SearchMode = "single"
	// SearchModeWeekly は1週間分の献立検索。
	SearchModeWeekly SearchMode = "weekly"
)

// ParseSearchMode は文字列を SearchMode に変換する。
// 表記ゆれを許容すると DB の値と乖離するため、完全一致のみを受け付ける。
func ParseSearchMode(s string) (SearchMode, error) {
	m := SearchMode(s)
	if !m.Valid() {
		return "", ErrInvalidSearchMode
	}
	return m, nil
}

// Valid は定義済みの検索種別かどうかを返す。
func (m SearchMode) Valid() bool {
	switch m {
	case SearchModeSingle, SearchModeWeekly:
		return true
	default:
		return false
	}
}

// String は DB およびAPIで用いる文字列表現を返す。
func (m SearchMode) String() string {
	return string(m)
}
