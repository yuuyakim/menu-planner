package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func TestHistoryRepository_Add_Records(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	// 履歴は user と menu を参照する。先に両方を用意する。
	u := newUser(t, "hist@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	repo := repository.NewHistoryRepository(pool)
	require.NoError(t, repo.Add(ctx, u.ID, menu.ID, domain.SearchModeSingle))

	var count int
	var mode string
	err := pool.QueryRow(ctx,
		`SELECT count(*), max(search_mode) FROM search_histories WHERE user_id=$1 AND menu_id=$2`,
		u.ID.String(), menu.ID.String()).Scan(&count, &mode)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, "single", mode)
}

func TestHistoryRepository_Add_CascadesOnUserDelete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "cascade-hist@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	menu := insertMenu(t, pool, "麻婆豆腐", domain.GenreChinese, domain.DifficultyNormal)

	repo := repository.NewHistoryRepository(pool)
	require.NoError(t, repo.Add(ctx, u.ID, menu.ID, domain.SearchModeWeekly))

	// user を消すと履歴も消える。
	_, err := pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID.String())
	require.NoError(t, err)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM search_histories WHERE user_id=$1`, u.ID.String()).Scan(&count))
	require.Zero(t, count, "user 削除で履歴も消えるべき")
}

// 履歴の一覧は menus を JOIN して献立を組み立てる。列を足したときに
// この経路が取り残されると、献立の一部が空のまま返る。
// role を足した際に実際に取り残されたため、復元を検証する。
func TestHistoryRepository_List_献立の役割まで復元する(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := newUser(t, "hist-role@example.com")
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(ctx, u, "hash"))
	// 既定の main ではなく side を使う。main だと DEFAULT と区別が付かない。
	menu := insertMenuWithRole(t, pool, "ポテトサラダ",
		domain.GenreWestern, domain.DifficultyEasy, domain.RoleSide)

	repo := repository.NewHistoryRepository(pool)
	require.NoError(t, repo.Add(ctx, u.ID, menu.ID, domain.SearchModeSingle))

	got, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, domain.RoleSide, got[0].Menu.Role)
	require.True(t, got[0].Menu.Role.Valid(), "役割が空のまま返っていないこと")
}
