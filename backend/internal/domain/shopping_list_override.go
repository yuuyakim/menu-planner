package domain

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidOrigin は文字列が既知の origin に一致しないことを表す。
	ErrInvalidOrigin = errors.New("不正な差分の由来です")
	// ErrInvalidOverride は差分の必須項目が満たされていないことを表す。
	ErrInvalidOverride = errors.New("不正な買い物リストの差分です")
)

// Origin は買い物リストの差分行の由来。
//
// derived は献立から導出された品目に対する差分（チェック・非表示）。
// manual は利用者が自分で足した品目。DBの origin カラムの値と一致する。
type Origin string

const (
	// OriginDerived は献立由来の品目への差分。
	OriginDerived Origin = "derived"
	// OriginManual は利用者が手で足した品目。
	OriginManual Origin = "manual"
)

// ParseOrigin は文字列を Origin に変換する。完全一致のみ受け付ける。
func ParseOrigin(s string) (Origin, error) {
	o := Origin(s)
	if !o.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidOrigin, s)
	}
	return o, nil
}

// Valid は定義済みの origin かどうかを返す。
func (o Origin) Valid() bool {
	switch o {
	case OriginDerived, OriginManual:
		return true
	default:
		return false
	}
}

// String は DB およびAPIで用いる文字列表現を返す。
func (o Origin) String() string { return string(o) }

// ShoppingListOverride は保存済みの週の買い物リストに重ねる差分1行（設計 5.1）。
//
// **これはリストの実体ではなく、献立から導出したリストからの「ズレ」だけを持つ。**
// 行が無いことは「献立由来のまま・未チェック」を意味する。
// 主キーは (SavedWeeklyMenuID, Name)。同じリストに同名の品目は作れない。
type ShoppingListOverride struct {
	SavedWeeklyMenuID SavedWeeklyMenuID
	Name              string
	Category          IngredientCategory
	Origin            Origin
	// Checked はチェック済みか。
	Checked bool
	// Hidden は「家にあるから消した」など、表示から外すか。
	Hidden bool
}

// Validate は必須項目が満たされているかを検証する。
func (o ShoppingListOverride) Validate() error {
	if o.SavedWeeklyMenuID.IsZero() {
		return fmt.Errorf("%w: 週間献立IDが未設定です", ErrInvalidOverride)
	}
	if strings.TrimSpace(o.Name) == "" {
		return fmt.Errorf("%w: 名前が空です", ErrInvalidOverride)
	}
	if !o.Category.Valid() {
		return fmt.Errorf("%w: 不正なカテゴリです: %q", ErrInvalidOverride, o.Category)
	}
	if !o.Origin.Valid() {
		return fmt.Errorf("%w: 不正な由来です: %q", ErrInvalidOverride, o.Origin)
	}
	return nil
}
