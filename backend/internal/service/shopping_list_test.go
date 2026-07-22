package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeIngredientRepo は献立IDごとの食材を定型で返す。
type fakeIngredientRepo struct {
	byMenu   map[domain.MenuID][]domain.Ingredient
	all      []domain.Ingredient
	err      error
	lastIDs  []domain.MenuID
	callCont int
	allCalls int
}

// FindAll は食材マスタ全件（spec.md 2.9）。この fake では all をそのまま返す。
func (r *fakeIngredientRepo) FindAll(_ context.Context) ([]domain.Ingredient, error) {
	r.allCalls++
	if r.err != nil {
		return nil, r.err
	}
	return r.all, nil
}

// FindByIDs は all の中から指定IDのものを返す。存在しないIDは黙って落とす
// （本物の repository と同じ振る舞い。件数の判断は service の仕事）。
func (r *fakeIngredientRepo) FindByIDs(_ context.Context, ids []domain.IngredientID) ([]domain.Ingredient, error) {
	if r.err != nil {
		return nil, r.err
	}
	want := make(map[domain.IngredientID]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := []domain.Ingredient{}
	for _, i := range r.all {
		if want[i.ID] {
			out = append(out, i)
		}
	}
	return out, nil
}

// FindMenuIDsByIngredientIDs は byMenu を走査し、指定食材を1つでも使う献立を返す。
func (r *fakeIngredientRepo) FindMenuIDsByIngredientIDs(_ context.Context, ids []domain.IngredientID) ([]domain.MenuID, error) {
	if r.err != nil {
		return nil, r.err
	}
	want := make(map[domain.IngredientID]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := []domain.MenuID{}
	for menuID, ings := range r.byMenu {
		for _, i := range ings {
			if want[i.ID] {
				out = append(out, menuID)
				break
			}
		}
	}
	return out, nil
}

func (r *fakeIngredientRepo) FindByMenuIDs(_ context.Context, ids []domain.MenuID) ([]service.MenuIngredient, error) {
	r.callCont++
	r.lastIDs = ids
	if r.err != nil {
		return nil, r.err
	}
	var out []service.MenuIngredient
	for _, id := range ids {
		for _, ing := range r.byMenu[id] {
			out = append(out, service.MenuIngredient{MenuID: id, Ingredient: ing})
		}
	}
	return out, nil
}

func ingredient(name, kana string, c domain.IngredientCategory) domain.Ingredient {
	return domain.Ingredient{
		ID:       domain.NewIngredientID(),
		Name:     name,
		NameKana: kana,
		Category: c,
	}
}

func menuNamed(name string) domain.Menu {
	return domain.Menu{
		ID:          domain.NewMenuID(),
		Name:        name,
		NameKana:    name,
		Genre:       domain.GenreJapanese,
		Difficulty:  domain.DifficultyEasy,
		Description: name + "の説明",
	}
}

// shoppingListFixture は「肉じゃが」「親子丼」の2献立を用意する。
// 玉ねぎは両方に使われ、集約の検証に使う。
func shoppingListFixture() (*fakeMenuRepoForList, *fakeIngredientRepo, domain.Menu, domain.Menu) {
	nikujaga := menuNamed("肉じゃが")
	oyakodon := menuNamed("親子丼")

	onion := ingredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable)
	menus := &fakeMenuRepoForList{
		menus: map[domain.MenuID]domain.Menu{
			nikujaga.ID: nikujaga,
			oyakodon.ID: oyakodon,
		},
	}
	ings := &fakeIngredientRepo{
		byMenu: map[domain.MenuID][]domain.Ingredient{
			nikujaga.ID: {
				onion,
				ingredient("豚こま切れ肉", "ぶたこまぎれにく", domain.CategoryMeat),
				ingredient("じゃがいも", "じゃがいも", domain.CategoryVegetable),
			},
			oyakodon.ID: {
				onion,
				ingredient("鶏もも肉", "とりももにく", domain.CategoryMeat),
				ingredient("米", "こめ", domain.CategoryStaple),
			},
		},
	}
	return menus, ings, nikujaga, oyakodon
}

// fakeMenuRepoForList は献立の存在確認だけに使う簡易 repository。
type fakeMenuRepoForList struct {
	menus map[domain.MenuID]domain.Menu
	err   error
}

func (r *fakeMenuRepoForList) FindByID(_ context.Context, id domain.MenuID) (*domain.Menu, error) {
	m, ok := r.menus[id]
	if !ok {
		return nil, errors.New("見つかりません")
	}
	return &m, nil
}

func (r *fakeMenuRepoForList) FindByIDs(_ context.Context, ids []domain.MenuID) ([]domain.Menu, error) {
	if r.err != nil {
		return nil, r.err
	}
	var out []domain.Menu
	for _, id := range ids {
		if m, ok := r.menus[id]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *fakeMenuRepoForList) FindByFilter(_ context.Context, _ domain.MenuFilter) ([]domain.Menu, error) {
	return nil, errors.New("使わない")
}

func newListService(m service.MenuRepository, i service.IngredientRepository) *service.ShoppingListService {
	return service.NewShoppingListService(m, i)
}

// itemNames は買い物リストの食材名を並び順のまま返す。
func itemNames(items []service.ShoppingItem) []string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Ingredient.Name)
	}
	return names
}

func TestShoppingList_MergesSameIngredientAcrossMenus(t *testing.T) {
	// 同じ食材が複数の献立に出たら1件にまとめ、usedIn に両方を並べる。
	menus, ings, nikujaga, oyakodon := shoppingListFixture()
	svc := newListService(menus, ings)

	got, err := svc.Build(context.Background(), []domain.MenuID{nikujaga.ID, oyakodon.ID})
	require.NoError(t, err)

	var onion *service.ShoppingItem
	for i := range got {
		if got[i].Ingredient.Name == "玉ねぎ" {
			onion = &got[i]
		}
	}
	require.NotNil(t, onion, "玉ねぎが1件だけある")

	usedIn := []string{}
	for _, m := range onion.UsedIn {
		usedIn = append(usedIn, m.Name)
	}
	assert.ElementsMatch(t, []string{"肉じゃが", "親子丼"}, usedIn,
		"どの献立で使うかが分かる")
}

func TestShoppingList_OrdersByCategoryThenKana(t *testing.T) {
	// 並びはカテゴリ順（野菜→肉→魚介→卵乳→主食→その他）、同カテゴリ内はカナ順。
	// 売り場を回る順に近づけるため。
	menus, ings, nikujaga, oyakodon := shoppingListFixture()
	svc := newListService(menus, ings)

	got, err := svc.Build(context.Background(), []domain.MenuID{nikujaga.ID, oyakodon.ID})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"じゃがいも", "玉ねぎ", // 野菜（じゃがいも → たまねぎ）
		"鶏もも肉", "豚こま切れ肉", // 肉（とりももにく → ぶたこまぎれにく）
		"米", // 主食
	}, itemNames(got))
}

func TestShoppingList_DeduplicatesMenuIDs(t *testing.T) {
	// 同じ献立を二重に渡されても、usedIn が重複しない。
	menus, ings, nikujaga, _ := shoppingListFixture()
	svc := newListService(menus, ings)

	got, err := svc.Build(context.Background(), []domain.MenuID{nikujaga.ID, nikujaga.ID})
	require.NoError(t, err)

	require.NotEmpty(t, got)
	for _, it := range got {
		assert.Len(t, it.UsedIn, 1, "%s の usedIn が重複しない", it.Ingredient.Name)
	}
}

func TestShoppingList_RejectsEmpty(t *testing.T) {
	menus, ings, _, _ := shoppingListFixture()
	svc := newListService(menus, ings)

	_, err := svc.Build(context.Background(), nil)
	assert.ErrorIs(t, err, service.ErrInvalidMenuIDs, "0件は不正")
}

func TestShoppingList_RejectsTooMany(t *testing.T) {
	// 週間献立は7日分。それを超える指定は受け付けない。
	menus, ings, nikujaga, _ := shoppingListFixture()
	svc := newListService(menus, ings)

	ids := make([]domain.MenuID, 0, 8)
	for i := 0; i < 8; i++ {
		ids = append(ids, nikujaga.ID)
	}
	// 重複を除くと1件になるため、別IDで8件を作る。
	ids = ids[:0]
	for i := 0; i < 8; i++ {
		ids = append(ids, domain.NewMenuID())
	}

	_, err := svc.Build(context.Background(), ids)
	assert.ErrorIs(t, err, service.ErrInvalidMenuIDs, "8件以上は不正")
}

func TestShoppingList_UnknownMenuIsNotFound(t *testing.T) {
	// 存在しない献立IDが混ざっていたら 404 にする。
	// 黙って無視すると「頼んだ献立の食材が抜けたリスト」を渡してしまう。
	menus, ings, nikujaga, _ := shoppingListFixture()
	svc := newListService(menus, ings)

	_, err := svc.Build(context.Background(),
		[]domain.MenuID{nikujaga.ID, domain.NewMenuID()})
	assert.ErrorIs(t, err, service.ErrMenuNotFoundInList)
}

func TestShoppingList_QueriesRepositoriesOnce(t *testing.T) {
	// 献立ごとに問い合わせない（7日分で7往復させない）。
	menus, ings, nikujaga, oyakodon := shoppingListFixture()
	svc := newListService(menus, ings)

	_, err := svc.Build(context.Background(), []domain.MenuID{nikujaga.ID, oyakodon.ID})
	require.NoError(t, err)

	assert.Equal(t, 1, ings.callCont, "食材の取得は1回にまとめる")
}

func TestShoppingList_PropagatesRepositoryError(t *testing.T) {
	menus, ings, nikujaga, _ := shoppingListFixture()
	ings.err = errors.New("DB障害")
	svc := newListService(menus, ings)

	_, err := svc.Build(context.Background(), []domain.MenuID{nikujaga.ID})
	assert.Error(t, err)
}
