package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func TestHistoryRepository_RecordMany_RecordsAll(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "bulk@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))

	// 週間献立の7件を用意する。
	menuIDs := make([]domain.MenuID, 7)
	for i := range menuIDs {
		m := insertMenu(t, pool, "献立"+string(rune('A'+i)), domain.GenreJapanese, domain.DifficultyEasy)
		menuIDs[i] = m.ID
	}

	repo := repository.NewHistoryRepository(pool)
	require.NoError(t, repo.RecordManyWithLimit(ctx, u.ID, menuIDs, domain.SearchModeWeekly, 15))

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM search_histories WHERE user_id=$1 AND search_mode='weekly'`,
		u.ID.String()).Scan(&count))
	require.Equal(t, 7, count, "7件すべて記録されるべき")
}

func TestHistoryRepository_RecordMany_PrunesOnceToLimit(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "bulk-prune@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	repo := repository.NewHistoryRepository(pool)

	// 既存10件（単発）。
	for i := 0; i < 10; i++ {
		require.NoError(t, repo.RecordWithLimit(ctx, u.ID, menu.ID, domain.SearchModeSingle, 15))
	}

	// 週間の7件を一括登録。10+7=17 → 一度の prune で15に収まる。
	weekly := make([]domain.MenuID, 7)
	for i := range weekly {
		weekly[i] = menu.ID
	}
	require.NoError(t, repo.RecordManyWithLimit(ctx, u.ID, weekly, domain.SearchModeWeekly, 15))

	var total, weeklyCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM search_histories WHERE user_id=$1`, u.ID.String()).Scan(&total))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM search_histories WHERE user_id=$1 AND search_mode='weekly'`, u.ID.String()).Scan(&weeklyCount))

	require.Equal(t, 15, total, "合計は15に収まる")
	// 週間の7件はいちばん新しいので、一度の prune では1件も消えない。
	// （もし prune が挿入ごとに走っていても最終件数は同じだが、7件全部が
	//  最新として残ることを確認しておく）
	require.Equal(t, 7, weeklyCount, "一括登録した7件はすべて残るべき")
}

func TestHistoryRepository_RecordMany_RollsBackOnFailure(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "bulk-rollback@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	good := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	// 3件目に存在しない献立IDを混ぜる。FK違反で全体がロールバックすべき。
	menuIDs := []domain.MenuID{good.ID, good.ID, domain.NewMenuID(), good.ID}

	repo := repository.NewHistoryRepository(pool)
	err := repo.RecordManyWithLimit(ctx, u.ID, menuIDs, domain.SearchModeWeekly, 15)
	require.Error(t, err)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM search_histories WHERE user_id=$1`, u.ID.String()).Scan(&count))
	require.Zero(t, count, "途中で失敗したら全件ロールバックされるべき")
}
