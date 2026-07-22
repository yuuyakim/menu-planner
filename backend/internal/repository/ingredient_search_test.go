package repository_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

// link は献立と食材を紐づける。
func link(t *testing.T, pool *pgxpool.Pool, menuID, ingredientID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO menu_ingredients (menu_id, ingredient_id) VALUES ($1,$2)`,
		menuID, ingredientID)
	require.NoError(t, err)
}

func TestIngredientRepository_FindMenuIDsByIngredientIDs(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewIngredientRepository(pool)
	clearIngredients(t, pool)

	nikujaga := insertMenu(t, pool, "肉じゃが", domain.GenreJapanese, domain.DifficultyEasy)
	mapo := insertMenu(t, pool, "麻婆豆腐", domain.GenreChinese, domain.DifficultyNormal)

	onion := insertIngredient(t, pool, "玉ねぎ", "たまねぎ", "vegetable")
	tofu := insertIngredient(t, pool, "豆腐", "とうふ", "other")
	link(t, pool, nikujaga.ID.String(), onion)
	link(t, pool, mapo.ID.String(), tofu)

	onionID, err := domain.ParseIngredientID(onion)
	require.NoError(t, err)

	got, err := repo.FindMenuIDsByIngredientIDs(ctx, []domain.IngredientID{onionID})
	require.NoError(t, err)

	// 玉ねぎを使う肉じゃがだけが返る。豆腐しか使わない麻婆豆腐は落ちる。
	require.Len(t, got, 1)
	assert.Equal(t, nikujaga.ID, got[0])
}

func TestIngredientRepository_FindMenuIDsByIngredientIDs_重複せず1件になる(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewIngredientRepository(pool)
	clearIngredients(t, pool)

	m := insertMenu(t, pool, "肉じゃが", domain.GenreJapanese, domain.DifficultyEasy)
	onion := insertIngredient(t, pool, "玉ねぎ", "たまねぎ", "vegetable")
	potato := insertIngredient(t, pool, "じゃがいも", "じゃがいも", "vegetable")
	link(t, pool, m.ID.String(), onion)
	link(t, pool, m.ID.String(), potato)

	ids := make([]domain.IngredientID, 0, 2)
	for _, raw := range []string{onion, potato} {
		id, err := domain.ParseIngredientID(raw)
		require.NoError(t, err)
		ids = append(ids, id)
	}

	got, err := repo.FindMenuIDsByIngredientIDs(context.Background(), ids)
	require.NoError(t, err)
	// 2つの食材がどちらも同じ献立に紐づいていても、献立は1件（DISTINCT）。
	assert.Len(t, got, 1)
}

func TestIngredientRepository_FindMenuIDsByIngredientIDs_0件指定(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewIngredientRepository(pool)

	got, err := repo.FindMenuIDsByIngredientIDs(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestIngredientRepository_FindByIDs_存在するものだけ返る(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewIngredientRepository(pool)
	clearIngredients(t, pool)

	onion := insertIngredient(t, pool, "玉ねぎ", "たまねぎ", "vegetable")
	onionID, err := domain.ParseIngredientID(onion)
	require.NoError(t, err)

	// 存在しないIDは黙って落ちる（件数の判断は service の仕事）。
	got, err := repo.FindByIDs(context.Background(),
		[]domain.IngredientID{onionID, domain.NewIngredientID()})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "玉ねぎ", got[0].Name)
}
