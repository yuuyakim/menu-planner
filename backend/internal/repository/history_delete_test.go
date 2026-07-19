package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// addHistoryReturningID は履歴を1件入れ、その HistoryID を返す。
func addHistoryReturningID(t *testing.T, pool *pgxpool.Pool, userID domain.UserID, menuID domain.MenuID) domain.HistoryID {
	t.Helper()
	raw := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO search_histories (id, user_id, menu_id, search_mode)
		 VALUES ($1,$2,$3,'single')`,
		raw, userID.String(), menuID.String())
	require.NoError(t, err)
	id, err := domain.ParseHistoryID(raw)
	require.NoError(t, err)
	return id
}

func TestHistoryRepository_Delete_Own(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "del@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	histID := addHistoryReturningID(t, pool, u.ID, menu.ID)

	repo := repository.NewHistoryRepository(pool)
	require.NoError(t, repo.Delete(ctx, u.ID, histID))

	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM search_histories WHERE id=$1`, histID.String()).Scan(&count))
	require.Zero(t, count, "自分の履歴は削除される")
}

func TestHistoryRepository_Delete_NotFound(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "del-nf@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))

	repo := repository.NewHistoryRepository(pool)
	// 存在しない履歴ID。
	missing, err := domain.ParseHistoryID(uuid.NewString())
	require.NoError(t, err)
	err = repo.Delete(ctx, u.ID, missing)
	require.ErrorIs(t, err, service.ErrHistoryNotFound)
}

func TestHistoryRepository_Delete_OtherUserForbidden(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	owner := newUser(t, "owner@example.com")
	other := newUser(t, "other@example.com")
	require.NoError(t, userRepo.CreateWithPassword(ctx, owner, "hash"))
	require.NoError(t, userRepo.CreateWithPassword(ctx, other, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	histID := addHistoryReturningID(t, pool, owner.ID, menu.ID)

	repo := repository.NewHistoryRepository(pool)
	// 別ユーザーが owner の履歴を消そうとする。
	err := repo.Delete(ctx, other.ID, histID)
	require.ErrorIs(t, err, service.ErrHistoryForbidden)

	// 実際には消えていない。
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM search_histories WHERE id=$1`, histID.String()).Scan(&count))
	require.Equal(t, 1, count, "他人の履歴は消えない")
}

func TestHistoryRepository_DeleteAll(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	u := newUser(t, "del-all@example.com")
	other := newUser(t, "keep@example.com")
	require.NoError(t, userRepo.CreateWithPassword(ctx, u, "hash"))
	require.NoError(t, userRepo.CreateWithPassword(ctx, other, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	repo := repository.NewHistoryRepository(pool)
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.Add(ctx, u.ID, menu.ID, domain.SearchModeSingle))
	}
	require.NoError(t, repo.Add(ctx, other.ID, menu.ID, domain.SearchModeSingle))

	require.NoError(t, repo.DeleteAll(ctx, u.ID))

	var mine, theirs int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM search_histories WHERE user_id=$1`, u.ID.String()).Scan(&mine))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM search_histories WHERE user_id=$1`, other.ID.String()).Scan(&theirs))
	require.Zero(t, mine, "自分の履歴は全件消える")
	require.Equal(t, 1, theirs, "他ユーザーの履歴は残る")
}
