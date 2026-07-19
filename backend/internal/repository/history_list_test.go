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

func TestHistoryRepository_List_NewestFirst(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "list@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	m1 := insertMenu(t, pool, "古い献立", domain.GenreJapanese, domain.DifficultyEasy)
	m2 := insertMenu(t, pool, "新しい献立", domain.GenreWestern, domain.DifficultyNormal)

	// m1 を過去、m2 を新しい時刻で直接入れる。
	_, err := pool.Exec(ctx,
		`INSERT INTO search_histories (id, user_id, menu_id, search_mode, searched_at)
		 VALUES ($1,$2,$3,'single',$4)`,
		uuid.NewString(), u.ID.String(), m1.ID.String(), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO search_histories (id, user_id, menu_id, search_mode, searched_at)
		 VALUES ($1,$2,$3,'weekly',$4)`,
		uuid.NewString(), u.ID.String(), m2.ID.String(), time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	repo := repository.NewHistoryRepository(pool)
	entries, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	// 新しい順（m2 が先）。献立情報が JOIN されている。
	require.Equal(t, m2.ID.String(), entries[0].Menu.ID.String())
	require.Equal(t, "新しい献立", entries[0].Menu.Name)
	require.Equal(t, domain.SearchModeWeekly, entries[0].Mode)
	require.Equal(t, m1.ID.String(), entries[1].Menu.ID.String())
	require.False(t, entries[0].ID.String() == "")
}

func TestHistoryRepository_List_OnlyOwnUser(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	a := newUser(t, "list-a@example.com")
	b := newUser(t, "list-b@example.com")
	require.NoError(t, userRepo.CreateWithPassword(ctx, a, "hash"))
	require.NoError(t, userRepo.CreateWithPassword(ctx, b, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	repo := repository.NewHistoryRepository(pool)
	require.NoError(t, repo.Add(ctx, a.ID, menu.ID, domain.SearchModeSingle))
	require.NoError(t, repo.Add(ctx, b.ID, menu.ID, domain.SearchModeSingle))
	require.NoError(t, repo.Add(ctx, b.ID, menu.ID, domain.SearchModeSingle))

	entries, err := repo.List(ctx, a.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1, "自分の履歴だけが返るべき")
}

func TestHistoryRepository_RecentMenuIDs_Distinct(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "recent@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	m1 := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	m2 := insertMenu(t, pool, "カレー", domain.GenreOther, domain.DifficultyEasy)

	repo := repository.NewHistoryRepository(pool)
	// m1 を2回、m2 を1回記録。
	require.NoError(t, repo.Add(ctx, u.ID, m1.ID, domain.SearchModeSingle))
	require.NoError(t, repo.Add(ctx, u.ID, m1.ID, domain.SearchModeSingle))
	require.NoError(t, repo.Add(ctx, u.ID, m2.ID, domain.SearchModeSingle))

	ids, err := repo.RecentMenuIDs(ctx, u.ID)
	require.NoError(t, err)
	// 重複を除いて2件。
	require.Len(t, ids, 2)
	got := map[string]bool{ids[0].String(): true, ids[1].String(): true}
	require.True(t, got[m1.ID.String()])
	require.True(t, got[m2.ID.String()])
}

func TestHistoryRepository_List_Empty(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "list-empty@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))

	repo := repository.NewHistoryRepository(pool)
	entries, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, entries, "0件でも nil ではなく空スライス")
	require.Empty(t, entries)
}
