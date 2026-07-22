package repository_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

// clearIngredients は食材マスタを空にする。
//
// newTestPool の後始末は `TRUNCATE menus, users CASCADE` で、**ingredients は消えない**
// （ingredients は menus を参照していないため CASCADE が届かない）。
// 全件を数えるテストはそのままだと直前のテストが残したデータに引きずられるので、
// ここで明示的に空にする。このパッケージは t.Parallel() を使わないため安全。
func clearIngredients(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), "TRUNCATE ingredients CASCADE")
	require.NoError(t, err)
}

func TestIngredientRepository_FindAll_カナ順で全件返る(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewIngredientRepository(pool)
	clearIngredients(t, pool)

	// カナ順が崩れる並びで入れ、SQL 側が並べ直すことを確かめる。
	insertIngredient(t, pool, "玉ねぎ", "たまねぎ", "vegetable")
	insertIngredient(t, pool, "じゃがいも", "じゃがいも", "vegetable")
	insertIngredient(t, pool, "鶏もも肉", "とりももにく", "meat")

	got, err := repo.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)

	names := make([]string, 0, len(got))
	for _, i := range got {
		names = append(names, i.Name)
	}
	// repository が保証するのはカナ順まで。カテゴリ順は service の仕事。
	assert.Equal(t, []string{"じゃがいも", "たまねぎ", "とりももにく"},
		[]string{got[0].NameKana, got[1].NameKana, got[2].NameKana})
	assert.Equal(t, []string{"じゃがいも", "玉ねぎ", "鶏もも肉"}, names)
}

func TestIngredientRepository_FindAll_カテゴリが解釈されて返る(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewIngredientRepository(pool)
	clearIngredients(t, pool)

	insertIngredient(t, pool, "鮭", "さけ", "seafood")

	got, err := repo.FindAll(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "seafood", got[0].Category.String())
	assert.False(t, got[0].ID.IsZero())
}

func TestIngredientRepository_FindAll_0件でもnilを返さない(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewIngredientRepository(pool)
	clearIngredients(t, pool)

	got, err := repo.FindAll(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}
