package repository_test

import (
	"context"
	"fmt"
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

func TestFavoriteRepository_List_NewestFirst(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	u := createUser(t, pool, "fav-list@example.com")
	older := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	newer := insertMenu(t, pool, "カレー", domain.GenreWestern, domain.DifficultyEasy)

	require.NoError(t, repo.Add(ctx, u.ID, older.ID))
	// created_at は now()（トランザクション時刻）なので、明示的にずらして順序を確定させる。
	_, err := pool.Exec(ctx,
		`UPDATE favorites SET created_at = now() - interval '1 hour' WHERE menu_id = $1`,
		older.ID.String())
	require.NoError(t, err)
	require.NoError(t, repo.Add(ctx, u.ID, newer.ID))

	got, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "カレー", got[0].Menu.Name, "新しい順に返るべき")
	require.Equal(t, "親子丼", got[1].Menu.Name)
	require.False(t, got[0].CreatedAt.IsZero())
	// 役割まで復元されること。menus を JOIN する経路は献立の列を
	// 独自に並べており、列を足したときに取り残されやすい。
	require.Equal(t, newer.Role, got[0].Menu.Role)
	require.True(t, got[0].Menu.Role.Valid(), "役割が空のまま返っていないこと")
}

func TestFavoriteRepository_List_OnlyOwn(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	me := createUser(t, pool, "fav-me@example.com")
	other := createUser(t, pool, "fav-other@example.com")
	mine := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	theirs := insertMenu(t, pool, "カレー", domain.GenreWestern, domain.DifficultyEasy)

	require.NoError(t, repo.Add(ctx, me.ID, mine.ID))
	require.NoError(t, repo.Add(ctx, other.ID, theirs.ID))

	got, err := repo.List(ctx, me.ID)
	require.NoError(t, err)
	require.Len(t, got, 1, "他ユーザーのお気に入りは返らない")
	require.Equal(t, "親子丼", got[0].Menu.Name)
}

func TestFavoriteRepository_List_EmptyIsSlice(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	u := createUser(t, pool, "fav-empty@example.com")

	got, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestFavoriteRepository_NoAutoPruneOver15(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	u := createUser(t, pool, "fav-many@example.com")

	// 履歴は15件でFIFO削除されるが、お気に入りに上限は無い（spec.md 2.6）。
	const total = 20
	for i := 0; i < total; i++ {
		m := insertMenu(t, pool, fmt.Sprintf("献立%02d", i), domain.GenreJapanese, domain.DifficultyEasy)
		require.NoError(t, repo.Add(ctx, u.ID, m.ID))
	}

	got, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, got, total, "15件を超えても自動削除されない")
}

func TestFavoriteRepository_Delete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	u := createUser(t, pool, "fav-del@example.com")
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	require.NoError(t, repo.Add(ctx, u.ID, menu.ID))

	require.NoError(t, repo.Delete(ctx, u.ID, menu.ID))

	got, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestFavoriteRepository_Delete_NotFound(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	u := createUser(t, pool, "fav-del-none@example.com")
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	err := repo.Delete(ctx, u.ID, menu.ID)
	require.ErrorIs(t, err, service.ErrFavoriteNotFound)
}

func TestFavoriteRepository_Delete_OtherUsersSurvives(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewFavoriteRepository(pool)

	me := createUser(t, pool, "fav-del-me@example.com")
	other := createUser(t, pool, "fav-del-other@example.com")
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	require.NoError(t, repo.Add(ctx, other.ID, menu.ID))

	// 削除は (user_id, menu_id) で絞るので、他人の行には触れない。
	// 自分は登録していないので「見つからない」＝404。
	err := repo.Delete(ctx, me.ID, menu.ID)
	require.ErrorIs(t, err, service.ErrFavoriteNotFound)

	got, err := repo.List(ctx, other.ID)
	require.NoError(t, err)
	require.Len(t, got, 1, "他ユーザーのお気に入りは消えない")
}
