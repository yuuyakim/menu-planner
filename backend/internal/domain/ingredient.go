package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ingredientNameMaxLen は食材名の最大文字数（バイト数ではなく文字数）。
const ingredientNameMaxLen = 100

var (
	// ErrInvalidIngredientID は食材IDとして解釈できないことを表す。
	ErrInvalidIngredientID = errors.New("不正な食材IDです")
	// ErrInvalidIngredientCategory は文字列が既知のカテゴリに一致しないことを表す。
	ErrInvalidIngredientCategory = errors.New("不正な食材カテゴリです")
	// ErrInvalidIngredient は食材の必須項目が満たされていないことを表す。
	ErrInvalidIngredient = errors.New("不正な食材です")
)

// IngredientCategory は食材の分類。買い物リストの並び順に使う。
//
// **調味料の分類は持たない。** 醤油・塩・砂糖などの調味料は食材として
// 登録しないため（spec.md 14.4）。
type IngredientCategory string

// 定義済みのカテゴリ。DBの category カラムに格納される値と一致する。
const (
	CategoryVegetable IngredientCategory = "vegetable"
	CategoryMeat      IngredientCategory = "meat"
	CategorySeafood   IngredientCategory = "seafood"
	CategoryDairyEgg  IngredientCategory = "dairy_egg"
	CategoryStaple    IngredientCategory = "staple"
	CategoryOther     IngredientCategory = "other"
)

// AllIngredientCategories は全カテゴリを表示順で返す。
// 買い物リストはこの順に並べる（売り場を回る順に近づけるため）。
func AllIngredientCategories() []IngredientCategory {
	return []IngredientCategory{
		CategoryVegetable,
		CategoryMeat,
		CategorySeafood,
		CategoryDairyEgg,
		CategoryStaple,
		CategoryOther,
	}
}

// ParseIngredientCategory は文字列を IngredientCategory に変換する。
// 表記ゆれを許容すると DB の値と乖離するため、完全一致のみを受け付ける。
func ParseIngredientCategory(s string) (IngredientCategory, error) {
	c := IngredientCategory(s)
	if !c.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidIngredientCategory, s)
	}
	return c, nil
}

// Valid は定義済みのカテゴリかどうかを返す。
func (c IngredientCategory) Valid() bool {
	switch c {
	case CategoryVegetable, CategoryMeat, CategorySeafood,
		CategoryDairyEgg, CategoryStaple, CategoryOther:
		return true
	default:
		return false
	}
}

// Order は表示順における位置を返す。並び替えに使う。
func (c IngredientCategory) Order() int {
	for i, known := range AllIngredientCategories() {
		if c == known {
			return i
		}
	}
	// 未知のカテゴリは末尾に寄せる。
	return len(AllIngredientCategories())
}

// String は DB およびAPIで用いる文字列表現を返す。
func (c IngredientCategory) String() string {
	return string(c)
}

// IngredientID は食材の識別子。
type IngredientID struct {
	value uuid.UUID
}

// NewIngredientID は新しい食材IDを採番する。
func NewIngredientID() IngredientID {
	return IngredientID{value: uuid.New()}
}

// ParseIngredientID は文字列を IngredientID に変換する。
// ゼロ値のUUIDは未設定と区別できないため受け付けない。
func ParseIngredientID(s string) (IngredientID, error) {
	u, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return IngredientID{}, fmt.Errorf("%w: %q", ErrInvalidIngredientID, s)
	}
	if u == uuid.Nil {
		return IngredientID{}, fmt.Errorf("%w: ゼロ値のUUIDは使用できません", ErrInvalidIngredientID)
	}
	return IngredientID{value: u}, nil
}

// IsZero は未設定かどうかを返す。
func (id IngredientID) IsZero() bool {
	return id.value == uuid.Nil
}

// String はUUIDの文字列表現を返す。
func (id IngredientID) String() string {
	return id.value.String()
}

// Ingredient は献立に必要な食材。
//
// 分量は持たない（spec.md 14.2）。買い物リストは食材名のチェックリストとして
// 提供し、必要量は「どの献立で使うか」から利用者が判断する。
type Ingredient struct {
	ID       IngredientID
	Name     string
	NameKana string
	Category IngredientCategory
}

// Validate は必須項目が満たされているかを検証する。
func (i Ingredient) Validate() error {
	if i.ID.IsZero() {
		return fmt.Errorf("%w: IDが未設定です", ErrInvalidIngredient)
	}
	if strings.TrimSpace(i.Name) == "" {
		return fmt.Errorf("%w: 名前が空です", ErrInvalidIngredient)
	}
	if utf8.RuneCountInString(i.Name) > ingredientNameMaxLen {
		return fmt.Errorf("%w: 名前が長すぎます（最大%d文字）", ErrInvalidIngredient, ingredientNameMaxLen)
	}
	// カナは一覧の並び順に使うため、欠けていると順序が崩れる。
	if strings.TrimSpace(i.NameKana) == "" {
		return fmt.Errorf("%w: カナが空です", ErrInvalidIngredient)
	}
	if !i.Category.Valid() {
		return fmt.Errorf("%w: 不正なカテゴリです: %q", ErrInvalidIngredient, i.Category)
	}
	return nil
}
