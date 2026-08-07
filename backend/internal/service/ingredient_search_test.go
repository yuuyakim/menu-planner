package service_test

import (
	"context"
	"fmt"
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
	//
	// **これは件数だけの検証。** 25件は中身が同じなので、切ってから並べても
	// 20件になり通ってしまう。並べ替えとの順序は
	// TestSearchByIngredients_切り詰めは並べ替えの後 が受け持つ。
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

// menuWithKana は名前とカナを別々に指定した献立を返す。
//
// menuNamed はカナに名前をそのまま入れるため、カナ順の検証には使えない
// （名前順で並べても同じ結果になり、カナを見ているのか区別できない）。
func menuWithKana(name, kana string) domain.Menu {
	m := menuNamed(name)
	m.NameKana = kana
	return m
}

// searchScaleFixture は候補が上限を超える規模のデータを作る。
//
// spec は「手持ちと1つでも重なる献立」を全部拾ってから並べる。fake の
// repository は map を走査するため候補の順序は毎回変わる。**その前提で、
// 返ってきた20件の顔ぶれと並びを固定する。**
//
//	top    … 手持ち3種すべてを使う。一致3 / 不足0
//	decoy  … 手持ち1種と手持ちに無い2種。一致1 / 不足2
func searchScaleFixture(topCount, decoyCount int) (
	*fakeMenuRepoForList, *fakeIngredientRepo, []domain.IngredientID,
) {
	onion := ingredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable)
	potato := ingredient("じゃがいも", "じゃがいも", domain.CategoryVegetable)
	carrot := ingredient("にんじん", "にんじん", domain.CategoryVegetable)

	menus := &fakeMenuRepoForList{menus: map[domain.MenuID]domain.Menu{}}
	ings := &fakeIngredientRepo{
		all:    []domain.Ingredient{onion, potato, carrot},
		byMenu: map[domain.MenuID][]domain.Ingredient{},
	}

	add := func(name string, items ...domain.Ingredient) {
		m := menuWithKana(name, name)
		menus.menus[m.ID] = m
		ings.byMenu[m.ID] = items
	}
	for i := 1; i <= topCount; i++ {
		add(fmt.Sprintf("作れる%02d", i), onion, potato, carrot)
	}
	for i := 1; i <= decoyCount; i++ {
		add(fmt.Sprintf("不足あり%02d", i), onion,
			ingredient("牛肉", "ぎゅうにく", domain.CategoryMeat),
			ingredient("きゅうり", "きゅうり", domain.CategoryVegetable))
	}
	return menus, ings, []domain.IngredientID{onion.ID, potato.ID, carrot.ID}
}

func TestSearchByIngredients_切り詰めは並べ替えの後(t *testing.T) {
	t.Parallel()

	// **並べてから切る**（設計 4章・6章）。50件の候補のうち上位20件は
	// 不足0の「作れるXX」だけで、切ってから並べる実装なら「不足あり」が
	// 混ざる。件数だけを見る検証では、どちらの順でも20件になるため通ってしまう。
	menus, ings, have := searchScaleFixture(20, 30)
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{IngredientIDs: have})
	require.NoError(t, err)

	want := make([]string, 0, 20)
	for i := 1; i <= 20; i++ {
		want = append(want, fmt.Sprintf("作れる%02d", i))
	}
	// 不足0が20件、それらは一致数も同じなのでカナ順に並ぶ。顔ぶれと並びの両方を固定する。
	assert.Equal(t, want, matchNames(got.Matches))
}

// searchNearMissFixture は「あと1品」が上限を超える規模のデータを作る。
//
// 不足0の献立は1件も作らない（OnlyMakeable で必ず0件になる）。
//
//	近い   … 手持ち3種すべて＋不足1。一致3 / 不足1
//	遠い   … 手持ち1種＋不足1。      一致1 / 不足1
//	圏外   … 手持ち1種＋不足2。      一致1 / 不足2（あと1品には入らない）
func searchNearMissFixture(nearCount, farCount, outCount int) (
	*fakeMenuRepoForList, *fakeIngredientRepo, []domain.IngredientID,
) {
	onion := ingredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable)
	potato := ingredient("じゃがいも", "じゃがいも", domain.CategoryVegetable)
	carrot := ingredient("にんじん", "にんじん", domain.CategoryVegetable)
	beef := func() domain.Ingredient { return ingredient("牛肉", "ぎゅうにく", domain.CategoryMeat) }

	menus := &fakeMenuRepoForList{menus: map[domain.MenuID]domain.Menu{}}
	ings := &fakeIngredientRepo{
		all:    []domain.Ingredient{onion, potato, carrot},
		byMenu: map[domain.MenuID][]domain.Ingredient{},
	}
	add := func(name string, items ...domain.Ingredient) {
		m := menuWithKana(name, name)
		menus.menus[m.ID] = m
		ings.byMenu[m.ID] = items
	}
	for i := 1; i <= nearCount; i++ {
		add(fmt.Sprintf("近い%02d", i), onion, potato, carrot, beef())
	}
	for i := 1; i <= farCount; i++ {
		add(fmt.Sprintf("遠い%02d", i), onion, beef())
	}
	for i := 1; i <= outCount; i++ {
		add(fmt.Sprintf("圏外%02d", i), onion, beef(),
			ingredient("きゅうり", "きゅうり", domain.CategoryVegetable))
	}
	return menus, ings, []domain.IngredientID{onion.ID, potato.ID, carrot.ID}
}

func TestSearchByIngredients_あと1品も上位20件で打ち切る(t *testing.T) {
	t.Parallel()

	// あと1品の候補が25件。上限は matches と同じ20件（spec.md 5.6）。
	menus, ings, have := searchNearMissFixture(5, 20, 3)
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{IngredientIDs: have, OnlyMakeable: true})
	require.NoError(t, err)

	require.Empty(t, got.Matches, "不足0の献立は用意していない")
	assert.Len(t, got.NearMisses, 20)
	for _, m := range got.NearMisses {
		assert.Len(t, m.Missing, 1, "不足がちょうど1件でない: %s", m.Menu.Name)
		assert.NotContains(t, m.Menu.Name, "圏外", "不足2件が混ざっている")
	}
}

func TestSearchByIngredients_あと1品は一致の多い順(t *testing.T) {
	t.Parallel()

	// 不足はどれも1件で並ばないため、一致の多い順に固定する（設計 3章）。
	// ここも切り詰めより先に並べないと、一致3の「近い」が20件から漏れる。
	menus, ings, have := searchNearMissFixture(5, 20, 3)
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{IngredientIDs: have, OnlyMakeable: true})
	require.NoError(t, err)

	want := make([]string, 0, 20)
	for i := 1; i <= 5; i++ {
		want = append(want, fmt.Sprintf("近い%02d", i))
	}
	// 一致1どうしは不足も同数なのでカナ順。20件で切れるので 遠い16〜20 は落ちる。
	for i := 1; i <= 15; i++ {
		want = append(want, fmt.Sprintf("遠い%02d", i))
	}
	assert.Equal(t, want, matchNames(got.NearMisses))
}

// searchKanaTieFixture は一致数も不足数も全く同じ献立を6件作る。
//
// **名前順とカナ順を逆にしてある。** 名前で並べても偶然通ることがないよう、
// 期待する並びは名前の逆順になる。
func searchKanaTieFixture() (*fakeMenuRepoForList, *fakeIngredientRepo, domain.IngredientID) {
	onion := ingredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable)

	menus := &fakeMenuRepoForList{menus: map[domain.MenuID]domain.Menu{}}
	ings := &fakeIngredientRepo{
		all:    []domain.Ingredient{onion},
		byMenu: map[domain.MenuID][]domain.Ingredient{},
	}
	const n = 6
	for i := 1; i <= n; i++ {
		m := menuWithKana(fmt.Sprintf("献立%d", i), fmt.Sprintf("かな%d", n+1-i))
		menus.menus[m.ID] = m
		// 一致1 / 不足1 で全件そろえる。差が付くのはカナだけ。
		ings.byMenu[m.ID] = []domain.Ingredient{
			onion, ingredient("牛肉", "ぎゅうにく", domain.CategoryMeat),
		}
	}
	return menus, ings, onion.ID
}

func TestSearchByIngredients_同数ならカナ順(t *testing.T) {
	t.Parallel()

	// 設計 6章「さらに同数ならカナ順」。両方の並び順で第3のキーが効く。
	// fake の repository は map を走査するので、カナで並べていなければ
	// 順序は毎回変わる。
	for _, by := range []service.MatchSort{service.SortMissingAsc, service.SortMatchedDesc} {
		t.Run(string(by), func(t *testing.T) {
			t.Parallel()

			menus, ings, onionID := searchKanaTieFixture()
			svc := newSearchService(menus, ings)

			got, err := svc.SearchByIngredients(context.Background(),
				service.SearchByIngredientsInput{
					IngredientIDs: []domain.IngredientID{onionID},
					Sort:          by,
				})
			require.NoError(t, err)

			// カナは かな1..かな6 の順。名前は逆に振ってあるので 献立6..献立1。
			assert.Equal(t,
				[]string{"献立6", "献立5", "献立4", "献立3", "献立2", "献立1"},
				matchNames(got.Matches))
		})
	}
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

func TestSearchByIngredients_作れるものが0件ならあと1品を返す(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	// じゃがいもだけ。不足0の献立は存在しない。
	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["じゃがいも"].ID},
			OnlyMakeable:  true,
		})
	require.NoError(t, err)

	assert.Empty(t, got.Matches)
	// ポテトサラダは不足1（きゅうり）。肉じゃがは不足2（玉ねぎ・牛肉）なので入らない。
	assert.Equal(t, []string{"ポテトサラダ"}, matchNames(got.NearMisses))
}

func TestSearchByIngredients_あと1品は不足ちょうど1件だけ(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["じゃがいも"].ID},
			OnlyMakeable:  true,
		})
	require.NoError(t, err)

	for _, m := range got.NearMisses {
		assert.Len(t, m.Missing, 1, "不足が1件でない献立が混ざっている: %s", m.Menu.Name)
	}
}

func TestSearchByIngredients_候補があるならあと1品は返さない(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	// 玉ねぎ＋じゃがいもならオニオンスープが不足0で見つかる。
	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID},
			OnlyMakeable:  true,
		})
	require.NoError(t, err)

	require.NotEmpty(t, got.Matches)
	assert.Empty(t, got.NearMisses, "候補があるのに あと1品 を返している")
}

func TestSearchByIngredients_絞っていなければあと1品は常に空(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	// OnlyMakeable でなければ、たとえ結果が0件でも nearMisses は埋めない。
	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["じゃがいも"].ID},
		})
	require.NoError(t, err)

	assert.NotEmpty(t, got.Matches)
	assert.Empty(t, got.NearMisses)
}
