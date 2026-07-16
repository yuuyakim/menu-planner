package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ドメインの検証エラー。
var (
	// ErrInvalidMenuID は献立IDとして解釈できない値を表す。
	ErrInvalidMenuID = errors.New("不正な献立IDです")
	// ErrInvalidMenu は献立の必須項目が満たされていないことを表す。
	ErrInvalidMenu = errors.New("不正な献立です")
)

// menuNameMaxLen は献立名の最大文字数（バイト数ではなく文字数）。
const menuNameMaxLen = 100

// MenuID は献立の識別子。
type MenuID struct {
	value uuid.UUID
}

// NewMenuID は新しい献立IDを採番する。
func NewMenuID() MenuID {
	return MenuID{value: uuid.New()}
}

// ParseMenuID は文字列を MenuID に変換する。
// ゼロ値のUUIDは未設定と区別できないため受け付けない。
func ParseMenuID(s string) (MenuID, error) {
	u, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return MenuID{}, fmt.Errorf("%w: %q", ErrInvalidMenuID, s)
	}
	if u == uuid.Nil {
		return MenuID{}, fmt.Errorf("%w: ゼロ値のUUIDは使用できません", ErrInvalidMenuID)
	}
	return MenuID{value: u}, nil
}

// IsZero は未設定かどうかを返す。
func (id MenuID) IsZero() bool {
	return id.value == uuid.Nil
}

// String はUUIDの文字列表現を返す。
func (id MenuID) String() string {
	return id.value.String()
}

// Menu は献立マスタの1件を表す。
type Menu struct {
	ID          MenuID
	Name        string
	NameKana    string
	Genre       Genre
	Difficulty  Difficulty
	Description string
}

// Validate は必須項目が満たされているかを検証する。
func (m Menu) Validate() error {
	if m.ID.IsZero() {
		return fmt.Errorf("%w: IDが未設定です", ErrInvalidMenu)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("%w: 名前が空です", ErrInvalidMenu)
	}
	if utf8.RuneCountInString(m.Name) > menuNameMaxLen {
		return fmt.Errorf("%w: 名前は%d文字以内にしてください", ErrInvalidMenu, menuNameMaxLen)
	}
	if strings.TrimSpace(m.NameKana) == "" {
		return fmt.Errorf("%w: かなが空です", ErrInvalidMenu)
	}
	if !m.Genre.Valid() {
		return fmt.Errorf("%w: ジャンルが不正です(%q)", ErrInvalidMenu, m.Genre)
	}
	if !m.Difficulty.Valid() {
		return fmt.Errorf("%w: 難易度が不正です(%q)", ErrInvalidMenu, m.Difficulty)
	}
	if strings.TrimSpace(m.Description) == "" {
		return fmt.Errorf("%w: 説明が空です", ErrInvalidMenu)
	}
	return nil
}

// MenuFilter は献立検索の絞り込み条件。
// Genre / Difficulty が nil の場合はその軸で絞り込まない。
type MenuFilter struct {
	Genre      *Genre
	Difficulty *Difficulty
	// ExcludeIDs は候補から除外する献立ID（直近の履歴などに使う）。
	ExcludeIDs []MenuID
}

// IsEmpty はジャンル・難易度いずれの絞り込みも指定されていないことを返す。
// ExcludeIDs は絞り込みの軸ではないため判定に含めない。
func (f MenuFilter) IsEmpty() bool {
	return f.Genre == nil && f.Difficulty == nil
}

// Validate は指定された条件が既知の値かを検証する。
func (f MenuFilter) Validate() error {
	if f.Genre != nil && !f.Genre.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidGenre, *f.Genre)
	}
	if f.Difficulty != nil && !f.Difficulty.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidDifficulty, *f.Difficulty)
	}
	return nil
}
