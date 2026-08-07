package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// searchFixture は3献立を用意する。手持ちは「玉ねぎ・じゃがいも」を想定。
//
//	肉じゃが       … 玉ねぎ・じゃがいも・牛肉       → 一致2 / 不足1
//	ポテトサラダ   … じゃがいも・きゅうり           → 一致1 / 不足1
//	オニオンスープ … 玉ねぎ                         → 一致1 / 不足0
//	麻婆豆腐       … 豆腐（手持ちと重ならない）     → 候補に出ない
func searchFixture() (*fakeMenuRepoForList, *fakeIngredientRepo, map[string]domain.Ingredient, map[string]domain.Menu) {
	onion := ingredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable)
	potato := ingredient("じゃがいも", "じゃがいも", domain.CategoryVegetable)
	beef := ingredient("牛肉", "ぎゅうにく", domain.CategoryMeat)
	cucumber := ingredient("きゅうり", "きゅうり", domain.CategoryVegetable)
	tofu := ingredient("豆腐", "とうふ", domain.CategoryOther)

	nikujaga := menuNamed("肉じゃが")
	potesara := menuNamed("ポテトサラダ")
	soup := menuNamed("オニオンスープ")
	mapo := menuNamed("麻婆豆腐")

	menus := &fakeMenuRepoForList{menus: map[domain.MenuID]domain.Menu{
		nikujaga.ID: nikujaga, potesara.ID: potesara, soup.ID: soup, mapo.ID: mapo,
	}}
	ings := &fakeIngredientRepo{
		all: []domain.Ingredient{onion, potato, beef, cucumber, tofu},
		byMenu: map[domain.MenuID][]domain.Ingredient{
			nikujaga.ID: {onion, potato, beef},
			potesara.ID: {potato, cucumber},
			soup.ID:     {onion},
			mapo.ID:     {tofu},
		},
	}
	return menus, ings,
		map[string]domain.Ingredient{"玉ねぎ": onion, "じゃがいも": potato, "牛肉": beef, "きゅうり": cucumber, "豆腐": tofu},
		map[string]domain.Menu{"肉じゃが": nikujaga, "ポテトサラダ": potesara, "オニオンスープ": soup, "麻婆豆腐": mapo}
}

func newSearchService(m service.MenuRepository, i service.IngredientRepository) *service.IngredientService {
	return service.NewIngredientService(i, m)
}

// matchNames は候補の献立名を並び順のまま返す。
func matchNames(matches []service.MenuMatch) []string {
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m.Menu.Name)
	}
	return names
}

func TestSearchByIngredients_不足の少ない順そのあと一致の多い順(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID},
		})
	require.NoError(t, err)

	// オニオンスープ（不足0）が先頭。次は不足1が2件で、一致の多い肉じゃが（2）が
	// ポテトサラダ（1）より上に来る。
	assert.Equal(t, []string{"オニオンスープ", "肉じゃが", "ポテトサラダ"}, matchNames(got.Matches))
}

func TestSearchByIngredients_一致と不足の中身(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID},
		})
	require.NoError(t, err)

	var nikujaga *service.MenuMatch
	for i := range got.Matches {
		if got.Matches[i].Menu.Name == "肉じゃが" {
			nikujaga = &got.Matches[i]
		}
	}
	require.NotNil(t, nikujaga)

	// 一致・不足ともカテゴリ順→カナ順（買い物リストと同じ並び）。
	assert.Equal(t, []string{"じゃがいも", "玉ねぎ"},
		[]string{nikujaga.Matched[0].Name, nikujaga.Matched[1].Name})
	require.Len(t, nikujaga.Missing, 1)
	assert.Equal(t, "牛肉", nikujaga.Missing[0].Name)
}

func TestSearchByIngredients_重ならない献立は返らない(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID},
		})
	require.NoError(t, err)

	// 豆腐しか使わない麻婆豆腐は候補に出ない。返しても「その食材で作れるもの」ではない。
	assert.NotContains(t, matchNames(got.Matches), "麻婆豆腐")
	assert.Contains(t, matchNames(got.Matches), "オニオンスープ")
}

func TestSearchByIngredients_重複IDは1件として扱う(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["玉ねぎ"].ID},
		})
	require.NoError(t, err)

	var soup *service.MenuMatch
	for i := range got.Matches {
		if got.Matches[i].Menu.Name == "オニオンスープ" {
			soup = &got.Matches[i]
		}
	}
	require.NotNil(t, soup)
	// 玉ねぎを2回渡しても一致は1件。
	assert.Len(t, soup.Matched, 1)
}

func TestSearchByIngredients_0件指定は拒否(t *testing.T) {
	t.Parallel()

	menus, ings, _, _ := searchFixture()
	svc := newSearchService(menus, ings)

	_, err := svc.SearchByIngredients(context.Background(), service.SearchByIngredientsInput{})
	assert.ErrorIs(t, err, service.ErrInvalidIngredientIDs)
}

func TestSearchByIngredients_存在しない食材は見つからない(t *testing.T) {
	t.Parallel()

	// 黙って無視すると、利用者が選んだつもりの条件と違う結果を返してしまう。
	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	_, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, domain.NewIngredientID()},
		})
	assert.ErrorIs(t, err, service.ErrIngredientNotFound)
}

func TestSearchByIngredients_該当なしは空スライス(t *testing.T) {
	t.Parallel()

	// どの献立にも使われていない食材だけを選んだ場合。
	menus, ings, _, _ := searchFixture()
	lonely := ingredient("パクチー", "ぱくちー", domain.CategoryVegetable)
	ings.all = append(ings.all, lonely)
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{lonely.ID},
		})
	require.NoError(t, err)
	assert.NotNil(t, got.Matches, "nil ではなく空スライスを返す")
	assert.Empty(t, got.Matches)
}

func TestSearchByIngredients_上位20件で打ち切る(t *testing.T) {
	t.Parallel()

	// 同じ食材を使う献立を25件作る。見比べる用途で20件を超えても選べない。
	common := ingredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable)
	menus := &fakeMenuRepoForList{menus: map[domain.MenuID]domain.Menu{}}
	ings := &fakeIngredientRepo{
		all:    []domain.Ingredient{common},
		byMenu: map[domain.MenuID][]domain.Ingredient{},
	}
	for i := 0; i < 25; i++ {
		m := menuNamed("献立")
		menus.menus[m.ID] = m
		ings.byMenu[m.ID] = []domain.Ingredient{common}
	}
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{common.ID},
		})
	require.NoError(t, err)
	assert.Len(t, got.Matches, 20)
}

func TestSearchByIngredients_省略時は現行と同じ結果(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)
	ids := []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID}

	// ゼロ値の Input が、つまみを足す前の挙動と一致することを固定する。
	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{IngredientIDs: ids})
	require.NoError(t, err)

	assert.Equal(t, []string{"オニオンスープ", "肉じゃが", "ポテトサラダ"}, matchNames(got.Matches))
	assert.Empty(t, got.NearMisses)
}

func TestSearchByIngredients_作れるものだけに絞る(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID},
			OnlyMakeable:  true,
		})
	require.NoError(t, err)

	// 不足1の肉じゃが・ポテトサラダは落ちる。
	assert.Equal(t, []string{"オニオンスープ"}, matchNames(got.Matches))
	for _, m := range got.Matches {
		assert.Empty(t, m.Missing, "不足のある献立が混ざっている: %s", m.Menu.Name)
	}
}

func TestSearchByIngredients_手持ちを多く使う順(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID},
			Sort:          service.SortMatchedDesc,
		})
	require.NoError(t, err)

	// 一致数は 肉じゃが2 > ポテトサラダ1 = オニオンスープ1。
	// 同数どうしは不足の少ない方（オニオンスープ 不足0）が先。
	assert.Equal(t, []string{"肉じゃが", "オニオンスープ", "ポテトサラダ"}, matchNames(got.Matches))
}
