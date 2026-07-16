package domain_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseMenuID_有効なUUID(t *testing.T) {
	t.Parallel()

	raw := uuid.NewString()
	got, err := domain.ParseMenuID(raw)
	require.NoError(t, err)
	assert.Equal(t, raw, got.String())
}

func TestParseMenuID_無効な値(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"空文字":        "",
		"UUIDでない":    "not-a-uuid",
		"桁が足りない":     "018f5c1e-1234",
		"連番のような値":    "1",
		"空白のみ":       "   ",
		"ゼロUUIDは不許可": "00000000-0000-0000-0000-000000000000",
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := domain.ParseMenuID(input)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidMenuID)
		})
	}
}

func TestNewMenuID_毎回異なる値を返す(t *testing.T) {
	t.Parallel()

	a := domain.NewMenuID()
	b := domain.NewMenuID()
	assert.NotEqual(t, a, b)
	assert.False(t, a.IsZero())
}

func TestMenuID_IsZero(t *testing.T) {
	t.Parallel()

	assert.True(t, domain.MenuID{}.IsZero())
	assert.False(t, domain.NewMenuID().IsZero())
}

func validMenu() domain.Menu {
	return domain.Menu{
		ID:          domain.NewMenuID(),
		Name:        "親子丼",
		NameKana:    "おやこどん",
		Genre:       domain.GenreJapanese,
		Difficulty:  domain.DifficultyEasy,
		Description: "鶏肉と卵を甘辛い出汁でとじた定番の丼もの",
	}
}

func TestMenu_Validate_正常(t *testing.T) {
	t.Parallel()

	require.NoError(t, validMenu().Validate())
}

func TestMenu_Validate_必須項目の欠落(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*domain.Menu){
		"IDが未設定":         func(m *domain.Menu) { m.ID = domain.MenuID{} },
		"名前が空":           func(m *domain.Menu) { m.Name = "" },
		"名前が空白のみ":        func(m *domain.Menu) { m.Name = "   " },
		"かなが空":           func(m *domain.Menu) { m.NameKana = "" },
		"ジャンルが不正":        func(m *domain.Menu) { m.Genre = domain.Genre("italian") },
		"ジャンルが空":         func(m *domain.Menu) { m.Genre = "" },
		"難易度が不正":         func(m *domain.Menu) { m.Difficulty = domain.Difficulty("hard") },
		"難易度が空":          func(m *domain.Menu) { m.Difficulty = "" },
		"説明が空":           func(m *domain.Menu) { m.Description = "" },
		"名前が長すぎる(101文字)": func(m *domain.Menu) { m.Name = strings.Repeat("あ", 101) },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := validMenu()
			mutate(&m)
			err := m.Validate()
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidMenu)
		})
	}
}

func TestMenu_Validate_名前がちょうど100文字なら通る(t *testing.T) {
	t.Parallel()

	m := validMenu()
	m.Name = strings.Repeat("あ", 100)
	require.NoError(t, m.Validate())
}

func TestMenuFilter_Empty_絞り込みなし(t *testing.T) {
	t.Parallel()

	var f domain.MenuFilter
	assert.True(t, f.IsEmpty())
	assert.Nil(t, f.Genre)
	assert.Nil(t, f.Difficulty)
}

func TestMenuFilter_条件を指定するとIsEmptyがfalse(t *testing.T) {
	t.Parallel()

	g := domain.GenreJapanese
	assert.False(t, domain.MenuFilter{Genre: &g}.IsEmpty())

	d := domain.DifficultyEasy
	assert.False(t, domain.MenuFilter{Difficulty: &d}.IsEmpty())
}

func TestMenuFilter_Validate(t *testing.T) {
	t.Parallel()

	t.Run("nilは有効", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, domain.MenuFilter{}.Validate())
	})

	t.Run("不正なジャンルはエラー", func(t *testing.T) {
		t.Parallel()
		g := domain.Genre("italian")
		require.ErrorIs(t, domain.MenuFilter{Genre: &g}.Validate(), domain.ErrInvalidGenre)
	})

	t.Run("不正な難易度はエラー", func(t *testing.T) {
		t.Parallel()
		d := domain.Difficulty("hard")
		require.ErrorIs(t, domain.MenuFilter{Difficulty: &d}.Validate(), domain.ErrInvalidDifficulty)
	})
}

func TestMenuFilter_ExcludeIDs(t *testing.T) {
	t.Parallel()

	id := domain.NewMenuID()
	f := domain.MenuFilter{ExcludeIDs: []domain.MenuID{id}}

	// 除外指定があっても「絞り込み条件」としては空とみなす
	// (ジャンル/難易度の指定有無とは別の軸のため)
	assert.True(t, f.IsEmpty())
	assert.Len(t, f.ExcludeIDs, 1)
}
