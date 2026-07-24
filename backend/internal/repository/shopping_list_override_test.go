package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func TestShoppingListOverrideRepository_置換と取得(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewShoppingListOverrideRepository(pool)

	u := createUser(t, pool, "slo-repo@example.com")
	weekStr := insertSavedWeek(t, pool, u.ID)
	week, err := domain.ParseSavedWeeklyMenuID(weekStr)
	require.NoError(t, err)

	// 最初は空。
	got, err := repo.FindBySavedWeeklyMenu(ctx, week)
	require.NoError(t, err)
	require.Empty(t, got)

	// 2件入れる。
	overrides := []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: week, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
		{SavedWeeklyMenuID: week, Name: "牛乳", Category: domain.CategoryDairyEgg, Origin: domain.OriginManual, Checked: false},
	}
	require.NoError(t, repo.Replace(ctx, week, overrides))

	got, err = repo.FindBySavedWeeklyMenu(ctx, week)
	require.NoError(t, err)
	require.Len(t, got, 2)
	// name 順で返る。
	require.Equal(t, "にんじん", got[0].Name)
	require.True(t, got[0].Checked)
	require.Equal(t, domain.OriginManual, got[1].Origin)
	require.Equal(t, week, got[0].SavedWeeklyMenuID)
}

func TestShoppingListOverrideRepository_置換は丸ごと入れ替える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewShoppingListOverrideRepository(pool)

	u := createUser(t, pool, "slo-replace@example.com")
	weekStr := insertSavedWeek(t, pool, u.ID)
	week, _ := domain.ParseSavedWeeklyMenuID(weekStr)

	require.NoError(t, repo.Replace(ctx, week, []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: week, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
	}))
	// 2回目は前の差分を消して新しいものだけにする。
	require.NoError(t, repo.Replace(ctx, week, []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: week, Name: "たまねぎ", Category: domain.CategoryVegetable, Origin: domain.OriginManual, Checked: false},
	}))

	got, err := repo.FindBySavedWeeklyMenu(ctx, week)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "たまねぎ", got[0].Name, "前の差分は消えているべき")
}

func TestShoppingListOverrideRepository_空で置換すると全部消える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewShoppingListOverrideRepository(pool)

	u := createUser(t, pool, "slo-clear@example.com")
	weekStr := insertSavedWeek(t, pool, u.ID)
	week, _ := domain.ParseSavedWeeklyMenuID(weekStr)

	require.NoError(t, repo.Replace(ctx, week, []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: week, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
	}))
	require.NoError(t, repo.Replace(ctx, week, nil))

	got, err := repo.FindBySavedWeeklyMenu(ctx, week)
	require.NoError(t, err)
	require.Empty(t, got, "空で置換したら全部消えるべき")
}
