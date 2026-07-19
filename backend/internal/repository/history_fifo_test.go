package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func TestHistoryRepository_FIFO_KeepsUpTo15(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "fifo15@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	repo := repository.NewHistoryRepository(pool)
	for i := 0; i < 15; i++ {
		require.NoError(t, repo.RecordWithLimit(ctx, u.ID, menu.ID, domain.SearchModeSingle, 15))
	}

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM search_histories WHERE user_id=$1`, u.ID.String()).Scan(&count))
	require.Equal(t, 15, count, "15件までは削除されない")
}

func TestHistoryRepository_FIFO_16thEvictsOldest(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "fifo16@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	repo := repository.NewHistoryRepository(pool)

	// 最初の1件を過去の時刻で直接入れ、確実に最古にする。id を覚えておく。
	oldestID := uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO search_histories (id, user_id, menu_id, search_mode, searched_at)
		 VALUES ($1, $2, $3, 'single', $4)`,
		oldestID, u.ID.String(), menu.ID.String(), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	// 残り15件を通常経路で入れる（now()）。合計16件になり、prune で最古が消える。
	for i := 0; i < 15; i++ {
		require.NoError(t, repo.RecordWithLimit(ctx, u.ID, menu.ID, domain.SearchModeSingle, 15))
	}

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM search_histories WHERE user_id=$1`, u.ID.String()).Scan(&count))
	require.Equal(t, 15, count, "16件目でちょうど15件に収まる")

	// 最古の1件が消えている。
	var exists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM search_histories WHERE id=$1)`, oldestID).Scan(&exists))
	require.False(t, exists, "最古の履歴が削除されるべき")
}

func TestHistoryRepository_FIFO_PerUser(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	a := newUser(t, "fifo-a@example.com")
	b := newUser(t, "fifo-b@example.com")
	require.NoError(t, userRepo.CreateWithPassword(ctx, a, "hash"))
	require.NoError(t, userRepo.CreateWithPassword(ctx, b, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	repo := repository.NewHistoryRepository(pool)
	// A は 16件（→15に収まる）、B は 5件。
	for i := 0; i < 16; i++ {
		require.NoError(t, repo.RecordWithLimit(ctx, a.ID, menu.ID, domain.SearchModeSingle, 15))
	}
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.RecordWithLimit(ctx, b.ID, menu.ID, domain.SearchModeSingle, 15))
	}

	var countA, countB int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM search_histories WHERE user_id=$1`, a.ID.String()).Scan(&countA))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM search_histories WHERE user_id=$1`, b.ID.String()).Scan(&countB))
	require.Equal(t, 15, countA)
	require.Equal(t, 5, countB, "他ユーザーの履歴は FIFO の影響を受けない")
}

func TestHistoryRepository_FIFO_Over15Individually(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "fifo21@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	repo := repository.NewHistoryRepository(pool)
	for i := 0; i < 21; i++ {
		require.NoError(t, repo.RecordWithLimit(ctx, u.ID, menu.ID, domain.SearchModeSingle, 15))
	}

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM search_histories WHERE user_id=$1`, u.ID.String()).Scan(&count))
	require.Equal(t, 15, count, "21件入れても15件に収まる")
}

func TestHistoryRepository_FIFO_TiebreakOnEqualTimestamp(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "fifo-tie@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	// 同じ searched_at で15件を直接入れる（週間一括登録が同時刻になる状況を再現）。
	// 挿入順に id を覚える。seq は挿入順に自動採番される。
	sameTime := time.Date(2021, 6, 1, 12, 0, 0, 0, time.UTC)
	ids := make([]string, 15)
	for i := range ids {
		ids[i] = uuid.NewString()
		_, err := pool.Exec(ctx,
			`INSERT INTO search_histories (id, user_id, menu_id, search_mode, searched_at)
			 VALUES ($1, $2, $3, 'single', $4)`,
			ids[i], u.ID.String(), menu.ID.String(), sameTime)
		require.NoError(t, err)
	}

	// now() の新しい1件を通常経路で足す。合計16件、prune で1件消える。
	repo := repository.NewHistoryRepository(pool)
	require.NoError(t, repo.RecordWithLimit(ctx, u.ID, menu.ID, domain.SearchModeSingle, 15))

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM search_histories WHERE user_id=$1`, u.ID.String()).Scan(&count))
	require.Equal(t, 15, count)

	// searched_at が同値でも、消えるのは最初に入れた1件（seq 最小）。
	var firstExists, secondExists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM search_histories WHERE id=$1)`, ids[0]).Scan(&firstExists))
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM search_histories WHERE id=$1)`, ids[1]).Scan(&secondExists))
	require.False(t, firstExists, "同時刻でも最初に入れた履歴が消えるべき（タイブレーク）")
	require.True(t, secondExists, "2番目以降は残るべき")
}
