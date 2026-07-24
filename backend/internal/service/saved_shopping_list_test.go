package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeOverrideStore は差分をメモリに持つ。
type fakeOverrideStore struct {
	byWeek map[domain.SavedWeeklyMenuID][]domain.ShoppingListOverride
	err    error
}

func newFakeOverrideStore() *fakeOverrideStore {
	return &fakeOverrideStore{byWeek: map[domain.SavedWeeklyMenuID][]domain.ShoppingListOverride{}}
}

func (s *fakeOverrideStore) FindBySavedWeeklyMenu(
	_ context.Context, id domain.SavedWeeklyMenuID,
) ([]domain.ShoppingListOverride, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.byWeek[id], nil
}

func (s *fakeOverrideStore) Replace(
	_ context.Context, id domain.SavedWeeklyMenuID, overrides []domain.ShoppingListOverride,
) error {
	if s.err != nil {
		return s.err
	}
	s.byWeek[id] = overrides
	return nil
}

// fakeDeriver は導出結果を固定で返す。導出そのものは ShoppingListService のテストで検証済み。
type fakeDeriver struct {
	items []service.ShoppingItem
	err   error
}

func (d fakeDeriver) Build(_ context.Context, _ []domain.MenuID) ([]service.ShoppingItem, error) {
	return d.items, d.err
}

// ing は導出結果1件を組み立てる。
func ing(name, kana string, cat domain.IngredientCategory) service.ShoppingItem {
	return service.ShoppingItem{
		Ingredient: domain.Ingredient{ID: domain.NewIngredientID(), Name: name, NameKana: kana, Category: cat},
	}
}

// setupForTest は For を呼ぶための一式を用意し、保存済み週のIDを返す。
//
// fakeSavedWeeklyStore / fakeEntitlements は既存（saved_weekly_test.go）のものを使う。
// 保存済み週は Save 経由で登録する（本人の週として fake が覚える）。
func setupForTest(t *testing.T, plan domain.Plan, derived []service.ShoppingItem) (
	*service.SavedShoppingListService, *fakeOverrideStore, string, string,
) {
	t.Helper()
	saved := &fakeSavedWeeklyStore{}
	uid := domain.NewUserID()
	days := []domain.DayMenu{{Day: 1, Menu: domain.Menu{ID: domain.NewMenuID()}}}
	id, err := saved.Save(context.Background(), uid, days)
	require.NoError(t, err)

	overrides := newFakeOverrideStore()
	svc := service.NewSavedShoppingListService(
		fakeDeriver{items: derived}, saved, overrides, fakeEntitlements{plan: plan})
	return svc, overrides, uid.String(), id.String()
}

func TestSavedShoppingListService_For_freeは導出そのまま(t *testing.T) {
	t.Parallel()
	derived := []service.ShoppingItem{
		ing("にんじん", "にんじん", domain.CategoryVegetable),
		ing("豚肉", "ぶたにく", domain.CategoryMeat),
	}
	svc, overrides, userID, weekID := setupForTest(t, domain.PlanFree, derived)
	// free でも差分行があっても無視される。
	wid, _ := domain.ParseSavedWeeklyMenuID(weekID)
	overrides.byWeek[wid] = []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: wid, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
	}

	items, err := svc.For(context.Background(), userID, weekID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	for _, it := range items {
		require.False(t, it.Checked, "free は差分を重ねないので全て未チェック")
	}
}

func TestSavedShoppingListService_For_premiumは差分を重ねる(t *testing.T) {
	t.Parallel()
	derived := []service.ShoppingItem{
		ing("にんじん", "にんじん", domain.CategoryVegetable),
		ing("豚肉", "ぶたにく", domain.CategoryMeat),
		ing("たまねぎ", "たまねぎ", domain.CategoryVegetable),
	}
	svc, overrides, userID, weekID := setupForTest(t, domain.PlanPremium, derived)
	wid, _ := domain.ParseSavedWeeklyMenuID(weekID)
	overrides.byWeek[wid] = []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: wid, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
		{SavedWeeklyMenuID: wid, Name: "たまねぎ", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Hidden: true},
		{SavedWeeklyMenuID: wid, Name: "牛乳", Category: domain.CategoryDairyEgg, Origin: domain.OriginManual, Checked: false},
	}

	items, err := svc.For(context.Background(), userID, weekID)
	require.NoError(t, err)

	byName := map[string]service.SavedShoppingItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	require.True(t, byName["にんじん"].Checked, "チェックが重なる")
	require.NotContains(t, byName, "たまねぎ", "hidden は表示から外れる")
	require.Contains(t, byName, "牛乳", "手動品目が足される")
	require.Equal(t, domain.OriginManual, byName["牛乳"].Origin)
	require.Contains(t, byName, "豚肉", "差分の無い導出品目は残る")
}

func TestSavedShoppingListService_For_他人の週は404(t *testing.T) {
	t.Parallel()
	svc, _, _, weekID := setupForTest(t, domain.PlanPremium, nil)
	// 別ユーザーで引く。
	_, err := svc.For(context.Background(), domain.NewUserID().String(), weekID)
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuNotFound)
}

func TestSavedShoppingListService_For_並びはカテゴリ順カナ順(t *testing.T) {
	t.Parallel()
	derived := []service.ShoppingItem{
		ing("豚肉", "ぶたにく", domain.CategoryMeat),
		ing("にんじん", "にんじん", domain.CategoryVegetable),
	}
	svc, _, userID, weekID := setupForTest(t, domain.PlanPremium, derived)
	items, err := svc.For(context.Background(), userID, weekID)
	require.NoError(t, err)
	require.Equal(t, "にんじん", items[0].Name, "野菜が肉より先")
	require.Equal(t, "豚肉", items[1].Name)
}
