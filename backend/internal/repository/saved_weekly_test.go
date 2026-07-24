package repository_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// buildWeek は7日分の DayMenu を作る。日ごとに別の献立を割り当てる。
func buildWeek(t *testing.T, pool *pgxpool.Pool, prefix string) []domain.DayMenu {
	t.Helper()
	days := make([]domain.DayMenu, 0, domain.WeekLength)
	for day := 1; day <= domain.WeekLength; day++ {
		m := insertMenu(t, pool, prefix+string(rune('0'+day)),
			domain.GenreJapanese, domain.DifficultyEasy)
		days = append(days, domain.DayMenu{Day: day, Menu: m})
	}
	return days
}

func TestSavedWeeklyRepository_保存した週が7日分そのまま戻る(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "saved-roundtrip@example.com")
	days := buildWeek(t, pool, "献立")

	id, err := repo.Save(ctx, u.ID, days)
	require.NoError(t, err)
	require.False(t, id.IsZero())

	got, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)

	saved := got[0]
	assert.Equal(t, id, saved.ID)
	require.Len(t, saved.Days, domain.WeekLength)
	// 日の順に並び、献立も対応どおりに戻る。
	for i, want := range days {
		assert.Equal(t, want.Day, saved.Days[i].Day)
		assert.Equal(t, want.Menu.ID, saved.Days[i].Menu.ID)
		assert.Equal(t, want.Menu.Name, saved.Days[i].Menu.Name)
		// 役割まで復元されること（JOIN 経路の列の取り残し検出）。
		assert.Equal(t, want.Menu.Role, saved.Days[i].Menu.Role)
		assert.True(t, saved.Days[i].Menu.Role.Valid(), "役割が空のまま返っていないこと")
	}
	assert.False(t, saved.CreatedAt.IsZero())
}

func TestSavedWeeklyRepository_一覧は新しい順(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "saved-order@example.com")

	first, err := repo.Save(ctx, u.ID, buildWeek(t, pool, "古"))
	require.NoError(t, err)
	second, err := repo.Save(ctx, u.ID, buildWeek(t, pool, "新"))
	require.NoError(t, err)

	got, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, second, got[0].ID, "新しい方が先に来るべき")
	assert.Equal(t, first, got[1].ID)
}

func TestSavedWeeklyRepository_他人の保存は見えない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	a := createUser(t, pool, "saved-owner-a@example.com")
	b := createUser(t, pool, "saved-owner-b@example.com")

	_, err := repo.Save(ctx, a.ID, buildWeek(t, pool, "A"))
	require.NoError(t, err)

	got, err := repo.List(ctx, b.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
	// nil ではなく空スライスを返す（JSONで null にしないため）。
	assert.NotNil(t, got)
}

func TestSavedWeeklyRepository_件数を数える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "saved-count@example.com")
	other := createUser(t, pool, "saved-count-other@example.com")

	n, err := repo.Count(ctx, u.ID)
	require.NoError(t, err)
	assert.Zero(t, n)

	_, err = repo.Save(ctx, u.ID, buildWeek(t, pool, "自"))
	require.NoError(t, err)
	_, err = repo.Save(ctx, other.ID, buildWeek(t, pool, "他"))
	require.NoError(t, err)

	n, err = repo.Count(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "他人の保存は数に入らないべき")
}

func TestSavedWeeklyRepository_削除で消える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "saved-delete@example.com")
	id, err := repo.Save(ctx, u.ID, buildWeek(t, pool, "消"))
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, u.ID, id))

	got, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSavedWeeklyRepository_他人の保存は削除できない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	a := createUser(t, pool, "saved-del-a@example.com")
	b := createUser(t, pool, "saved-del-b@example.com")

	id, err := repo.Save(ctx, a.ID, buildWeek(t, pool, "他"))
	require.NoError(t, err)

	// 存在しない扱いにする。「他人のものだ」と伝えると存在を明かすことになる。
	err = repo.Delete(ctx, b.ID, id)
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuNotFound)

	// 持ち主の側では消えていない。
	got, err := repo.List(ctx, a.ID)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestSavedWeeklyRepository_無い保存の削除は見つからない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "saved-del-missing@example.com")

	err := repo.Delete(ctx, u.ID, domain.NewSavedWeeklyMenuID())
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuNotFound)
}

func TestSavedWeeklyRepository_Find_本人の週を中身つきで返す(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "sw-find@example.com")
	days := buildWeek(t, pool, "検")
	id, err := repo.Save(ctx, u.ID, days)
	require.NoError(t, err)

	got, err := repo.Find(ctx, u.ID, id)
	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	require.Len(t, got.Days, domain.WeekLength, "中身の7日分が詰まっているべき")
	for i, want := range days {
		assert.Equal(t, want.Day, got.Days[i].Day)
		assert.Equal(t, want.Menu.ID, got.Days[i].Menu.ID)
	}
	assert.False(t, got.CreatedAt.IsZero())
}

func TestSavedWeeklyRepository_Find_他人の週は見つからない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	owner := createUser(t, pool, "sw-find-owner@example.com")
	other := createUser(t, pool, "sw-find-other@example.com")
	id, err := repo.Save(ctx, owner.ID, buildWeek(t, pool, "他"))
	require.NoError(t, err)

	_, err = repo.Find(ctx, other.ID, id)
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuNotFound, "他人の週は存在を明かさず not found")
}

func TestSavedWeeklyRepository_Find_無い週は見つからない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "sw-find-missing@example.com")
	_, err := repo.Find(ctx, u.ID, domain.NewSavedWeeklyMenuID())
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuNotFound)
}

func TestSavedWeeklyRepository_存在しない献立を含む保存は失敗し何も残さない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "saved-unknown@example.com")
	days := buildWeek(t, pool, "壊")
	// 最後の1日だけ、マスタに無い献立にすり替える。
	days[domain.WeekLength-1].Menu.ID = domain.NewMenuID()

	_, err := repo.Save(ctx, u.ID, days)
	require.ErrorIs(t, err, repository.ErrMenuNotFound)

	// 途中まで書けた親行が残っていてはいけない。7日分は1つの取引。
	got, err := repo.List(ctx, u.ID)
	require.NoError(t, err)
	assert.Empty(t, got, "失敗した保存の残骸が残ってはいけない")
}
