package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func TestFavoriteRepository_Add(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	u := createUser(t, pool, "fav-add@example.com")
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	require.NoError(t, repo.Add(ctx, u.ID, menu.ID))

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM favorites WHERE user_id=$1 AND menu_id=$2`,
		u.ID.String(), menu.ID.String()).Scan(&count))
	require.Equal(t, 1, count)
}

func TestFavoriteRepository_Add_Duplicate(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	u := createUser(t, pool, "fav-dup@example.com")
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	require.NoError(t, repo.Add(ctx, u.ID, menu.ID))

	// 2回目は一意制約に当たり、409 に変換できるエラーになる。
	err := repo.Add(ctx, u.ID, menu.ID)
	require.ErrorIs(t, err, service.ErrFavoriteExists)
}

func TestFavoriteRepository_Add_UnknownMenu(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	u := createUser(t, pool, "fav-nomenu@example.com")

	// 存在しない献立は外部キー違反になり、404 に変換できるエラーになる。
	err := repo.Add(ctx, u.ID, domain.NewMenuID())
	require.ErrorIs(t, err, repository.ErrMenuNotFound)
}

func TestFavoriteRepository_Add_DifferentUsersSameMenu(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	a := createUser(t, pool, "fav-x@example.com")
	b := createUser(t, pool, "fav-y@example.com")
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	require.NoError(t, repo.Add(ctx, a.ID, menu.ID))
	require.NoError(t, repo.Add(ctx, b.ID, menu.ID), "別ユーザーなら同じ献立を登録できる")
}
