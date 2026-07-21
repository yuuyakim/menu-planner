package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func validIngredient() domain.Ingredient {
	return domain.Ingredient{
		ID:       domain.NewIngredientID(),
		Name:     "玉ねぎ",
		NameKana: "たまねぎ",
		Category: domain.CategoryVegetable,
	}
}

func TestParseIngredientCategory(t *testing.T) {
	for _, s := range []string{"vegetable", "meat", "seafood", "dairy_egg", "staple", "other"} {
		c, err := domain.ParseIngredientCategory(s)
		require.NoErrorf(t, err, "%q は有効なカテゴリ", s)
		assert.Equal(t, s, c.String())
	}
}

func TestParseIngredientCategory_Rejects(t *testing.T) {
	// 調味料は食材として持たない（spec.md 14.4）。
	// 誤って seasoning を使おうとしたら弾く。
	for _, s := range []string{"seasoning", "", "VEGETABLE", " vegetable", "野菜"} {
		_, err := domain.ParseIngredientCategory(s)
		assert.ErrorIsf(t, err, domain.ErrInvalidIngredientCategory, "%q は拒否する", s)
	}
}

func TestAllIngredientCategories_IsDisplayOrder(t *testing.T) {
	// 買い物リストの並び順（spec.md 5.5）。売り場を回る順に近い並びにする。
	assert.Equal(t, []domain.IngredientCategory{
		domain.CategoryVegetable,
		domain.CategoryMeat,
		domain.CategorySeafood,
		domain.CategoryDairyEgg,
		domain.CategoryStaple,
		domain.CategoryOther,
	}, domain.AllIngredientCategories())
}

func TestIngredient_Validate_AcceptsValid(t *testing.T) {
	require.NoError(t, validIngredient().Validate())
}

func TestIngredient_Validate_RejectsEmptyID(t *testing.T) {
	i := validIngredient()
	i.ID = domain.IngredientID{}
	assert.ErrorIs(t, i.Validate(), domain.ErrInvalidIngredient)
}

func TestIngredient_Validate_RejectsBlankName(t *testing.T) {
	// 空白だけの名前も空として扱う。DB側の制約と揃える。
	for _, name := range []string{"", "   ", "\t"} {
		i := validIngredient()
		i.Name = name
		assert.ErrorIsf(t, i.Validate(), domain.ErrInvalidIngredient, "name=%q", name)
	}
}

func TestIngredient_Validate_RejectsBlankNameKana(t *testing.T) {
	// カナは並び順に使うため、欠けていると一覧の順序が崩れる。
	i := validIngredient()
	i.NameKana = "  "
	assert.ErrorIs(t, i.Validate(), domain.ErrInvalidIngredient)
}

func TestIngredient_Validate_RejectsTooLongName(t *testing.T) {
	i := validIngredient()
	i.Name = strings.Repeat("あ", 101)
	assert.ErrorIs(t, i.Validate(), domain.ErrInvalidIngredient)
}

func TestIngredient_Validate_RejectsUnknownCategory(t *testing.T) {
	i := validIngredient()
	i.Category = domain.IngredientCategory("seasoning")
	assert.ErrorIs(t, i.Validate(), domain.ErrInvalidIngredient)
}

func TestParseIngredientID_RejectsZeroUUID(t *testing.T) {
	// ゼロ値は未設定と区別できないため受け付けない（MenuID と同じ方針）。
	_, err := domain.ParseIngredientID("00000000-0000-0000-0000-000000000000")
	assert.ErrorIs(t, err, domain.ErrInvalidIngredientID)

	_, err = domain.ParseIngredientID("not-a-uuid")
	assert.ErrorIs(t, err, domain.ErrInvalidIngredientID)
}

func TestNewIngredientID_IsNotZero(t *testing.T) {
	assert.False(t, domain.NewIngredientID().IsZero())
}
