package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/random/randomtest"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// テスト用の献立マスタ。ジャンルと難易度の組み合わせが重ならないようにしてある。
func testMenus() []domain.Menu {
	return []domain.Menu{
		newMenu("肉じゃが", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("茶碗蒸し", domain.GenreJapanese, domain.DifficultyElaborate),
		newMenu("ハンバーグ", domain.GenreWestern, domain.DifficultyNormal),
		newMenu("麻婆豆腐", domain.GenreChinese, domain.DifficultyEasy),
	}
}

func TestSuggestMenu_genre指定で該当ジャンルのみ候補になる(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	repo := newFakeMenuRepository(menus...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	// 和食は2件。乱数源が 0 / 1 を返せば、それぞれ順に選ばれる。
	got, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.GenreJapanese),
	})

	require.NoError(t, err)
	assert.Equal(t, domain.GenreJapanese, got.Genre)
	assert.Equal(t, "肉じゃが", got.Name)
	assert.Equal(t, genrePtr(domain.GenreJapanese), repo.lastFilter.Genre, "条件がそのまま repository に渡ること")
}

func TestSuggestMenu_difficulty指定で該当難易度のみ候補になる(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(testMenus()...)
	// easy は「肉じゃが」「麻婆豆腐」の2件。2件目を選ばせる。
	svc := newTestService(repo, randomtest.NewFixed(1))

	got, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Difficulty: difficultyPtr(domain.DifficultyEasy),
	})

	require.NoError(t, err)
	assert.Equal(t, domain.DifficultyEasy, got.Difficulty)
	assert.Equal(t, "麻婆豆腐", got.Name)
}

func TestSuggestMenu_両方指定で両方に合うもののみ(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(testMenus()...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	got, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Genre:      genrePtr(domain.GenreJapanese),
		Difficulty: difficultyPtr(domain.DifficultyEasy),
	})

	require.NoError(t, err)
	assert.Equal(t, "肉じゃが", got.Name)
	assert.Equal(t, genrePtr(domain.GenreJapanese), repo.lastFilter.Genre)
	assert.Equal(t, difficultyPtr(domain.DifficultyEasy), repo.lastFilter.Difficulty)
}

func TestSuggestMenu_両方nilで全件が候補(t *testing.T) {
	t.Parallel()

	menus := testMenus()

	// 乱数源が返す添字ごとに、マスタ全件がそのまま候補になっていることを確かめる。
	for i, want := range menus {
		t.Run(want.Name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeMenuRepository(menus...)
			svc := newTestService(repo, randomtest.NewFixed(i))

			got, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{})

			require.NoError(t, err)
			assert.Equal(t, want.Name, got.Name)
			assert.Nil(t, repo.lastFilter.Genre)
			assert.Nil(t, repo.lastFilter.Difficulty)
		})
	}
}

func TestSuggestMenu_不正なgenreはErrInvalidGenre(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(testMenus()...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	_, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.Genre("フレンチ")),
	})

	assert.ErrorIs(t, err, domain.ErrInvalidGenre)
	assert.Equal(t, 0, repo.filterCalls, "条件が不正ならDBに問い合わせないこと")
}

func TestSuggestMenu_不正なdifficultyはErrInvalidDifficulty(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(testMenus()...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	_, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Difficulty: difficultyPtr(domain.Difficulty("むずかしい")),
	})

	assert.ErrorIs(t, err, domain.ErrInvalidDifficulty)
	assert.Equal(t, 0, repo.filterCalls, "条件が不正ならDBに問い合わせないこと")
}

func TestSuggestMenu_候補0件でErrNoMenuFound(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		menus  []domain.Menu
		filter domain.MenuFilter
	}{
		"マスタが空":         {menus: nil, filter: domain.MenuFilter{}},
		"条件に合うものが1件も無い": {menus: testMenus(), filter: domain.MenuFilter{Genre: genrePtr(domain.GenreOther)}},
		"組み合わせに合うものが無い": {
			menus:  testMenus(),
			filter: domain.MenuFilter{Genre: genrePtr(domain.GenreChinese), Difficulty: difficultyPtr(domain.DifficultyElaborate)},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeMenuRepository(tt.menus...)
			svc := newTestService(repo, randomtest.NewFixed(0))

			_, err := svc.SuggestMenu(context.Background(), tt.filter)

			assert.ErrorIs(t, err, service.ErrNoMenuFound)
			assert.NotErrorIs(t, err, service.ErrNoCandidates, "Pick の内部事情は外に漏らさないこと")
		})
	}
}

func TestSuggestMenu_候補1件ならそれが返る(t *testing.T) {
	t.Parallel()

	// 境界値。0件と1件の境目で ErrNoMenuFound にならないことを確かめる。
	repo := newFakeMenuRepository(testMenus()...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	// 洋食は「ハンバーグ」の1件だけ。
	got, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.GenreWestern),
	})

	require.NoError(t, err)
	assert.Equal(t, "ハンバーグ", got.Name)
}

func TestSuggestMenu_repositoryのエラーがラップされて返る(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("DBへの接続に失敗しました")
	repo := newFakeMenuRepository(testMenus()...)
	repo.err = sentinel
	svc := newTestService(repo, randomtest.NewFixed(0))

	_, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{})

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "原因を errors.Is で辿れること")
	assert.NotEqual(t, sentinel.Error(), err.Error(), "文脈が付与されていること")
}

func TestSuggestMenu_ErrNoMenuFoundとrepositoryのエラーが区別できる(t *testing.T) {
	t.Parallel()

	// 呼び出し側は「該当なし(422)」と「DB障害(500)」を出し分ける必要がある。
	// どちらも errors.Is で取り違えないことを固定する。
	dbErr := errors.New("DBへの接続に失敗しました")

	failing := newFakeMenuRepository(testMenus()...)
	failing.err = dbErr
	_, gotDBErr := newTestService(failing, randomtest.NewFixed(0)).
		SuggestMenu(context.Background(), domain.MenuFilter{})

	_, gotEmptyErr := newTestService(newFakeMenuRepository(), randomtest.NewFixed(0)).
		SuggestMenu(context.Background(), domain.MenuFilter{})

	assert.ErrorIs(t, gotDBErr, dbErr)
	assert.NotErrorIs(t, gotDBErr, service.ErrNoMenuFound, "DB障害を該当なしと誤認しないこと")

	assert.ErrorIs(t, gotEmptyErr, service.ErrNoMenuFound)
	assert.NotErrorIs(t, gotEmptyErr, dbErr)
}

func TestSuggestMenu_乱数源のエラーは該当なしと区別できる(t *testing.T) {
	t.Parallel()

	// 乱数源の故障は候補が無いことを意味しない。500 に倒すべきなので混同させない。
	sentinel := errors.New("乱数源の故障")
	svc := newTestService(newFakeMenuRepository(testMenus()...), failingRandomizer{err: sentinel})

	_, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{})

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.NotErrorIs(t, err, service.ErrNoMenuFound)
}

func TestSuggestMenu_ExcludeIDsの献立が候補から外れる(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	repo := newFakeMenuRepository(menus...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	// 和食は「肉じゃが」「茶碗蒸し」の2件。先頭の肉じゃがを除外すれば、
	// 乱数源が 0 を返しても残った茶碗蒸しが選ばれる。
	nikujaga := menus[0]
	got, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Genre:      genrePtr(domain.GenreJapanese),
		ExcludeIDs: []domain.MenuID{nikujaga.ID},
	})

	require.NoError(t, err)
	assert.Equal(t, "茶碗蒸し", got.Name)
	assert.Equal(t, []domain.MenuID{nikujaga.ID}, repo.lastFilter.ExcludeIDs, "除外IDがそのまま repository に渡ること")
}

func TestSuggestMenu_全件除外するとErrNoMenuFound(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	repo := newFakeMenuRepository(menus...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	all := make([]domain.MenuID, 0, len(menus))
	for _, m := range menus {
		all = append(all, m.ID)
	}

	_, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{ExcludeIDs: all})

	assert.ErrorIs(t, err, service.ErrNoMenuFound)
}

func TestSuggestMenu_ExcludeIDsが空なら除外しない(t *testing.T) {
	t.Parallel()

	tests := map[string][]domain.MenuID{
		"nil": nil,
		"空":   {},
	}
	for name, ids := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeMenuRepository(testMenus()...)
			svc := newTestService(repo, randomtest.NewFixed(0))

			got, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{ExcludeIDs: ids})

			require.NoError(t, err)
			assert.Equal(t, "肉じゃが", got.Name, "全件が候補のままであること")
		})
	}
}

func TestSuggestMenu_知らないIDを除外しても影響しない(t *testing.T) {
	t.Parallel()

	// 履歴に残っていた献立がマスタから消えている場合を想定する。
	repo := newFakeMenuRepository(testMenus()...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	got, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Genre:      genrePtr(domain.GenreJapanese),
		ExcludeIDs: []domain.MenuID{domain.NewMenuID()},
	})

	require.NoError(t, err)
	assert.Equal(t, "肉じゃが", got.Name)
}

func TestSuggestMenu_返る献立はマスタの内容をそのまま持つ(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(testMenus()...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	got, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.GenreWestern),
	})

	require.NoError(t, err)
	assert.Equal(t, "ハンバーグ", got.Name)
	assert.Equal(t, "てすと", got.NameKana)
	assert.Equal(t, domain.DifficultyNormal, got.Difficulty)
	assert.Equal(t, "ハンバーグの説明", got.Description)
	assert.False(t, got.ID.IsZero())
}

func TestGetMenu_IDで献立が返る(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	repo := newFakeMenuRepository(menus...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	want := menus[2] // ハンバーグ
	got, err := svc.GetMenu(context.Background(), want.ID)

	require.NoError(t, err)
	assert.Equal(t, want, *got, "マスタの内容がそのまま返ること")
	assert.Equal(t, want.ID, repo.lastID, "IDがそのまま repository に渡ること")
}

func TestGetMenu_存在しないIDはrepositoryのエラーを保ったまま返る(t *testing.T) {
	t.Parallel()

	// 呼び出し側はこのエラーを 404 に変換する。ラップしても errors.Is で
	// 辿れなくなると 500 に落ちてしまうため、同一性が保たれることを固定する。
	repo := newFakeMenuRepository(testMenus()...)
	svc := newTestService(repo, randomtest.NewFixed(0))

	_, err := svc.GetMenu(context.Background(), domain.NewMenuID())

	require.Error(t, err)
	assert.ErrorIs(t, err, errFakeMenuNotFound)
}

func TestGetMenu_repositoryの障害は存在しないことと区別できる(t *testing.T) {
	t.Parallel()

	// 「存在しない(404)」と「DB障害(500)」の出し分けが崩れないようにする。
	dbErr := errors.New("DBへの接続に失敗しました")
	repo := newFakeMenuRepository(testMenus()...)
	repo.err = dbErr
	svc := newTestService(repo, randomtest.NewFixed(0))

	_, err := svc.GetMenu(context.Background(), domain.NewMenuID())

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
	assert.NotErrorIs(t, err, errFakeMenuNotFound, "DB障害を存在しないと誤認しないこと")
	assert.NotEqual(t, dbErr.Error(), err.Error(), "文脈が付与されていること")
}

// newTestService はレシピ検索を使わないテスト用に MenuService を組み立てる。
// SuggestMenu / GetMenu の検証では gateway を使わないため、定型の fake を挿す。
func newTestService(repo service.MenuRepository, rand service.Randomizer) *service.MenuService {
	return service.NewMenuService(repo, rand, newFakeRecipeGateway(), newFakeRecipeCache())
}

func TestRecipeLinks_献立名で検索して結果を返す(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	repo := newFakeMenuRepository(menus...)
	gw := newFakeRecipeGateway(
		newRecipeLink("肉じゃがの作り方", "https://recipe.example.com/1"),
		newRecipeLink("簡単 肉じゃが", "https://cooking.example.net/2"),
		newRecipeLink("本格 肉じゃが", "https://kitchen.example.org/3"),
	)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), gw, newFakeRecipeCache())

	got, err := svc.RecipeLinks(context.Background(), menus[0].ID)

	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, "肉じゃが", gw.lastMenuName, "IDではなく献立名で検索すること")
	assert.Equal(t, 3, gw.lastLimit, "spec.md 2.3 の3件")
}

func TestRecipeLinks_存在しない献立ならgatewayを呼ばない(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(testMenus()...)
	gw := newFakeRecipeGateway()
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), gw, newFakeRecipeCache())

	_, err := svc.RecipeLinks(context.Background(), domain.NewMenuID())

	require.Error(t, err)
	assert.ErrorIs(t, err, errFakeMenuNotFound, "呼び出し側が404に変換できること")
	assert.Equal(t, 0, gw.calls, "存在しない献立でAPIを消費しないこと")
}

func TestRecipeLinks_3件未満でも成功する(t *testing.T) {
	t.Parallel()

	// spec.md 2.3: 検索結果が3件未満の場合は取得できた件数のみを表示する。
	menus := testMenus()
	gw := newFakeRecipeGateway(newRecipeLink("肉じゃがの作り方", "https://recipe.example.com/1"))
	svc := service.NewMenuService(newFakeMenuRepository(menus...), randomtest.NewFixed(0), gw, newFakeRecipeCache())

	got, err := svc.RecipeLinks(context.Background(), menus[0].ID)

	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestRecipeLinks_0件でも成功する(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	svc := service.NewMenuService(newFakeMenuRepository(menus...), randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	got, err := svc.RecipeLinks(context.Background(), menus[0].ID)

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRecipeLinks_gatewayの障害はErrRecipeSearchFailedとして返る(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	gw := newFakeRecipeGateway()
	gw.err = service.ErrRecipeSearchFailed
	svc := service.NewMenuService(newFakeMenuRepository(menus...), randomtest.NewFixed(0), gw, newFakeRecipeCache())

	_, err := svc.RecipeLinks(context.Background(), menus[0].ID)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrRecipeSearchFailed, "呼び出し側が502に変換できること")
}

func TestRecipeLinks_締め切りを過ぎたらErrRecipeSearchFailedになる(t *testing.T) {
	t.Parallel()

	// gateway は context 切れを素の context エラーとして返す（呼び出し側の中断と
	// 区別するため）。それをそのまま通すと 500 に化けるので、こちらが課した
	// 締め切りによる打ち切りは「検索が失敗した」= 502 に寄せる。
	menus := testMenus()
	gw := newFakeRecipeGateway()
	gw.block = make(chan struct{}) // 閉じないので gateway は返ってこない
	svc := service.NewMenuService(newFakeMenuRepository(menus...), randomtest.NewFixed(0), gw, newFakeRecipeCache(),
		service.WithRecipeBudget(20*time.Millisecond))

	_, err := svc.RecipeLinks(context.Background(), menus[0].ID)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrRecipeSearchFailed)
	assert.NotErrorIs(t, err, context.DeadlineExceeded, "内部の締め切りを外に漏らさないこと")
}

func TestRecipeLinks_呼び出し側の中断はそのまま返す(t *testing.T) {
	t.Parallel()

	// 利用者が画面を離れた場合。502として記録する筋合いではない。
	menus := testMenus()
	gw := newFakeRecipeGateway()
	gw.block = make(chan struct{})
	svc := service.NewMenuService(newFakeMenuRepository(menus...), randomtest.NewFixed(0), gw, newFakeRecipeCache())

	ctx, cancel := context.WithCancel(context.Background())
	go cancel()

	_, err := svc.RecipeLinks(ctx, menus[0].ID)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, service.ErrRecipeSearchFailed)
}

func TestRecipeLinks_既定の締め切りは5秒(t *testing.T) {
	t.Parallel()

	// gateway 単体の最悪は 3s × 3回 + バックオフ ≒ 9.6秒。画面がそれだけ回ると
	// 体験が悪いため、レシピ取得全体に上限を課す。
	svc := service.NewMenuService(newFakeMenuRepository(), randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	assert.Equal(t, 5*time.Second, svc.RecipeBudget())
}

// 2026-07-17 12:00:00 JST 相当。キャッシュの鮮度判定を固定するための基準時刻。
var testNow = time.Date(2026, 7, 17, 3, 0, 0, 0, time.UTC)

// newCachingService は時刻を固定した MenuService を組み立てる。
func newCachingService(repo service.MenuRepository, gw service.RecipeSearchGateway, cache service.RecipeLinkCache) *service.MenuService {
	return service.NewMenuService(repo, randomtest.NewFixed(0), gw, cache,
		service.WithNow(func() time.Time { return testNow }))
}

func TestRecipeLinks_初回はgatewayを呼びキャッシュに保存する(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	gw := newFakeRecipeGateway(newRecipeLink("肉じゃがの作り方", "https://recipe.example.com/1"))
	cache := newFakeRecipeCache()
	svc := newCachingService(newFakeMenuRepository(menus...), gw, cache)

	got, err := svc.RecipeLinks(context.Background(), menus[0].ID)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, gw.calls)
	assert.Equal(t, 1, cache.saveCalls, "取得した結果を保存すること")
	assert.Equal(t, testNow, cache.lastSaved.FetchedAt, "保存時刻は service の時計に従うこと")
}

func TestRecipeLinks_2回目はgatewayを呼ばない(t *testing.T) {
	t.Parallel()

	// ここがキャッシュの目的。献立120件 × 1回に消費を抑える（spec.md 13.2）。
	menus := testMenus()
	gw := newFakeRecipeGateway(newRecipeLink("肉じゃがの作り方", "https://recipe.example.com/1"))
	cache := newFakeRecipeCache()
	svc := newCachingService(newFakeMenuRepository(menus...), gw, cache)

	first, err := svc.RecipeLinks(context.Background(), menus[0].ID)
	require.NoError(t, err)
	second, err := svc.RecipeLinks(context.Background(), menus[0].ID)
	require.NoError(t, err)

	assert.Equal(t, first, second, "同じ結果が返ること")
	assert.Equal(t, 1, gw.calls, "2回目は検索APIを叩かないこと")
}

func TestRecipeLinks_献立ごとに別のキャッシュになる(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	gw := newFakeRecipeGateway(newRecipeLink("レシピ", "https://recipe.example.com/1"))
	cache := newFakeRecipeCache()
	svc := newCachingService(newFakeMenuRepository(menus...), gw, cache)

	_, err := svc.RecipeLinks(context.Background(), menus[0].ID)
	require.NoError(t, err)
	_, err = svc.RecipeLinks(context.Background(), menus[1].ID)
	require.NoError(t, err)

	assert.Equal(t, 2, gw.calls, "別の献立なら別途取得すること")
}

func TestRecipeLinks_TTL内はキャッシュを使う(t *testing.T) {
	t.Parallel()

	tests := map[string]time.Duration{
		"直後":      0,
		"1日前":     24 * time.Hour,
		"7日ちょうど前": 7 * 24 * time.Hour, // 境界値: ちょうどはヒット
	}
	for name, age := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			menus := testMenus()
			gw := newFakeRecipeGateway()
			cache := newFakeRecipeCache()
			cache.put(menus[0].ID, testNow.Add(-age), newRecipeLink("キャッシュ", "https://cached.example.com/1"))
			svc := newCachingService(newFakeMenuRepository(menus...), gw, cache)

			got, err := svc.RecipeLinks(context.Background(), menus[0].ID)

			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, "キャッシュ", got[0].Title)
			assert.Equal(t, 0, gw.calls, "検索APIを叩かないこと")
		})
	}
}

func TestRecipeLinks_TTLを過ぎたら取り直す(t *testing.T) {
	t.Parallel()

	tests := map[string]time.Duration{
		"7日と1秒前": 7*24*time.Hour + time.Second, // 境界値: 1秒過ぎたら失効
		"30日前":   30 * 24 * time.Hour,
	}
	for name, age := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			menus := testMenus()
			gw := newFakeRecipeGateway(newRecipeLink("新しい", "https://fresh.example.com/1"))
			cache := newFakeRecipeCache()
			cache.put(menus[0].ID, testNow.Add(-age), newRecipeLink("古い", "https://stale.example.com/1"))
			svc := newCachingService(newFakeMenuRepository(menus...), gw, cache)

			got, err := svc.RecipeLinks(context.Background(), menus[0].ID)

			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, "新しい", got[0].Title)
			assert.Equal(t, 1, gw.calls)
			assert.Equal(t, testNow, cache.lastSaved.FetchedAt, "取り直した時刻で上書きすること")
		})
	}
}

func TestRecipeLinks_キャッシュの読み出しが壊れてもgatewayで応える(t *testing.T) {
	t.Parallel()

	// キャッシュは高速化の手段であって、壊れたらリクエストごと失敗させる筋合いはない。
	menus := testMenus()
	gw := newFakeRecipeGateway(newRecipeLink("新しい", "https://fresh.example.com/1"))
	cache := newFakeRecipeCache()
	cache.findErr = errors.New("キャッシュの読み出しに失敗しました")
	svc := newCachingService(newFakeMenuRepository(menus...), gw, cache)

	got, err := svc.RecipeLinks(context.Background(), menus[0].ID)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, gw.calls, "検索APIにフォールバックすること")
}

func TestRecipeLinks_キャッシュの書き込みが失敗しても結果は返す(t *testing.T) {
	t.Parallel()

	menus := testMenus()
	gw := newFakeRecipeGateway(newRecipeLink("新しい", "https://fresh.example.com/1"))
	cache := newFakeRecipeCache()
	cache.saveErr = errors.New("キャッシュの書き込みに失敗しました")
	svc := newCachingService(newFakeMenuRepository(menus...), gw, cache)

	got, err := svc.RecipeLinks(context.Background(), menus[0].ID)

	require.NoError(t, err, "保存の失敗で検索結果を捨てないこと")
	assert.Len(t, got, 1)
}

func TestRecipeLinks_0件でもキャッシュする(t *testing.T) {
	t.Parallel()

	// 0件も「探した結果」。保存しないと、該当の無い献立で毎回APIを消費する。
	menus := testMenus()
	gw := newFakeRecipeGateway()
	cache := newFakeRecipeCache()
	svc := newCachingService(newFakeMenuRepository(menus...), gw, cache)

	_, err := svc.RecipeLinks(context.Background(), menus[0].ID)
	require.NoError(t, err)
	got, err := svc.RecipeLinks(context.Background(), menus[0].ID)
	require.NoError(t, err)

	assert.Empty(t, got)
	assert.Equal(t, 1, cache.saveCalls, "0件でも保存すること")
	assert.Equal(t, 1, gw.calls, "2回目は叩かないこと")
}

func TestRecipeLinks_gateway障害のときはキャッシュを書かない(t *testing.T) {
	t.Parallel()

	// 失敗を保存すると、TTLが切れるまで失敗を返し続けることになる。
	menus := testMenus()
	gw := newFakeRecipeGateway()
	gw.err = service.ErrRecipeSearchFailed
	cache := newFakeRecipeCache()
	svc := newCachingService(newFakeMenuRepository(menus...), gw, cache)

	_, err := svc.RecipeLinks(context.Background(), menus[0].ID)

	require.Error(t, err)
	assert.Equal(t, 0, cache.saveCalls)
}

func TestRecipeLinks_存在しない献立ならキャッシュも見ない(t *testing.T) {
	t.Parallel()

	gw := newFakeRecipeGateway()
	cache := newFakeRecipeCache()
	svc := newCachingService(newFakeMenuRepository(testMenus()...), gw, cache)

	_, err := svc.RecipeLinks(context.Background(), domain.NewMenuID())

	require.Error(t, err)
	assert.Equal(t, 0, cache.findCalls)
	assert.Equal(t, 0, gw.calls)
}

func TestRecipeLinks_既定のTTLは7日(t *testing.T) {
	t.Parallel()

	svc := service.NewMenuService(newFakeMenuRepository(), randomtest.NewFixed(0),
		newFakeRecipeGateway(), newFakeRecipeCache())

	assert.Equal(t, 7*24*time.Hour, svc.RecipeCacheTTL())
}

// weeklyMenus は週間献立のテスト用マスタ。7日分を重複なく引ける最低限として8件用意する。
func weeklyMenus() []domain.Menu {
	return []domain.Menu{
		newMenu("肉じゃが", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("茶碗蒸し", domain.GenreJapanese, domain.DifficultyElaborate),
		newMenu("ハンバーグ", domain.GenreWestern, domain.DifficultyNormal),
		newMenu("オムライス", domain.GenreWestern, domain.DifficultyEasy),
		newMenu("麻婆豆腐", domain.GenreChinese, domain.DifficultyEasy),
		newMenu("餃子", domain.GenreChinese, domain.DifficultyNormal),
		newMenu("タコライス", domain.GenreOther, domain.DifficultyEasy),
		newMenu("ガパオライス", domain.GenreOther, domain.DifficultyNormal),
	}
}

// newWeeklyService は指定の乱数列で週間献立を組み立てるサービスを返す。
func newWeeklyService(menus []domain.Menu, values ...int) *service.MenuService {
	return service.NewMenuService(newFakeMenuRepository(menus...), randomtest.NewFixed(values...),
		newFakeRecipeGateway(), newFakeRecipeCache())
}

func TestSuggestWeekly_7件返る(t *testing.T) {
	t.Parallel()

	svc := newWeeklyService(weeklyMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	assert.Len(t, got, 7, "spec.md 2.2 の7日分")
}

func TestSuggestWeekly_dayが1から7の連番(t *testing.T) {
	t.Parallel()

	svc := newWeeklyService(weeklyMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	for i, d := range got {
		assert.Equal(t, i+1, d.Day, "%d番目の day", i)
	}
}

func TestSuggestWeekly_dayは起点当日から始まる(t *testing.T) {
	t.Parallel()

	// 当日起点（spec.md 13.3）。day=1 が今日で、曜日はサーバが持たない。
	svc := newWeeklyService(weeklyMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.Equal(t, 1, got[0].Day)
	assert.Equal(t, 7, got[len(got)-1].Day)
}

func TestSuggestWeekly_乱数列に従って献立が選ばれる(t *testing.T) {
	t.Parallel()

	// 添字は「残りの候補」に対するもの。重複回避で候補が 8→7→…→2 と縮むため、
	// 毎日その時点の末尾を引かせる（7,6,5,4,3,2,1）とマスタの逆順になる。
	menus := weeklyMenus()
	svc := newWeeklyService(menus, 7, 6, 5, 4, 3, 2, 1)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	for i, d := range got {
		want := menus[len(menus)-1-i].Name
		assert.Equal(t, want, d.Menu.Name, "%d日目", i+1)
	}
}

// easyMenus は難易度が全て easy で、ジャンルがばらけたマスタ。
// 難易度で絞っても7日分を組めるようにジャンルを一巡させてある。
func easyMenus() []domain.Menu {
	genres := []domain.Genre{
		domain.GenreJapanese, domain.GenreWestern, domain.GenreChinese, domain.GenreOther,
		domain.GenreJapanese, domain.GenreWestern, domain.GenreChinese, domain.GenreOther,
	}
	menus := make([]domain.Menu, 0, len(genres))
	for i, g := range genres {
		menus = append(menus, newMenu(fmt.Sprintf("簡単%d", i+1), g, domain.DifficultyEasy))
	}
	return menus
}

func TestSuggestWeekly_条件がrepositoryに渡る(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(easyMenus()...)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{
		Difficulty: difficultyPtr(domain.DifficultyEasy),
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, repo.lastFilter.Difficulty)
	assert.Equal(t, domain.DifficultyEasy, *repo.lastFilter.Difficulty)
}

func TestSuggestWeekly_候補を1度だけ問い合わせる(t *testing.T) {
	t.Parallel()

	// 7日分それぞれでDBを叩くと7倍の負荷になる。候補は一度引いて使い回す。
	repo := newFakeMenuRepository(weeklyMenus()...)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, repo.filterCalls)
}

func TestSuggestWeekly_候補0件でErrNoMenuFound(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		menus  []domain.Menu
		filter domain.MenuFilter
	}{
		"マスタが空":      {menus: nil, filter: domain.MenuFilter{}},
		"条件に合うものが無い": {menus: weeklyMenus(), filter: domain.MenuFilter{Genre: genrePtr(domain.GenreOther), Difficulty: difficultyPtr(domain.DifficultyElaborate)}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc := newWeeklyService(tt.menus, 0)

			_, err := svc.SuggestWeekly(context.Background(), tt.filter, nil)

			assert.ErrorIs(t, err, service.ErrNoMenuFound)
			assert.NotErrorIs(t, err, service.ErrNoCandidates, "Pick の内部事情は漏らさないこと")
		})
	}
}

func TestSuggestWeekly_不正な条件はErrInvalidGenre(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(weeklyMenus()...)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.Genre("フレンチ")),
	}, nil)

	assert.ErrorIs(t, err, domain.ErrInvalidGenre)
	assert.Equal(t, 0, repo.filterCalls, "条件が不正ならDBに問い合わせないこと")
}

func TestSuggestWeekly_repositoryのエラーがラップされて返る(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("DBへの接続に失敗しました")
	repo := newFakeMenuRepository(weeklyMenus()...)
	repo.err = sentinel
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.NotErrorIs(t, err, service.ErrNoMenuFound, "DB障害を該当なしと誤認しないこと")
}

func TestSuggestWeekly_乱数源のエラーは該当なしと区別できる(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("乱数源の故障")
	svc := service.NewMenuService(newFakeMenuRepository(weeklyMenus()...), failingRandomizer{err: sentinel},
		newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.NotErrorIs(t, err, service.ErrNoMenuFound)
}

func TestSuggestWeekly_同一献立が週内に2度出現しない(t *testing.T) {
	t.Parallel()

	// 乱数源は常に先頭を返す。重複回避が無ければ7日とも同じ献立になる。
	svc := newWeeklyService(weeklyMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)

	seen := map[domain.MenuID]string{}
	for _, d := range got {
		if prev, dup := seen[d.Menu.ID]; dup {
			t.Fatalf("%d日目の %q が %q と重複している", d.Day, d.Menu.Name, prev)
		}
		seen[d.Menu.ID] = d.Menu.Name
	}
}

func TestSuggestWeekly_候補がちょうど7件なら7件とも異なる(t *testing.T) {
	t.Parallel()

	// 境界値。候補と日数が同じとき、1件でも重複すると7日目が埋まらない。
	menus := weeklyMenus()[:7]
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)

	names := make([]string, 0, 7)
	for _, d := range got {
		names = append(names, d.Menu.Name)
	}
	want := make([]string, 0, 7)
	for _, m := range menus {
		want = append(want, m.Name)
	}
	assert.ElementsMatch(t, want, names, "候補が全て1度ずつ使われること")
}

func TestSuggestWeekly_選ばれた献立は以降の候補から外れる(t *testing.T) {
	t.Parallel()

	// 乱数源が常に 0 を返すなら、候補の先頭から順に消費される。
	menus := weeklyMenus()[:7]
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	for i, d := range got {
		assert.Equal(t, menus[i].Name, d.Menu.Name, "%d日目", i+1)
	}
}

// streakMenus は先頭3件が同じジャンルのマスタ。
// 乱数源が常に先頭を返すと、連続回避が無ければ和食が3日続く。
func streakMenus() []domain.Menu {
	return []domain.Menu{
		newMenu("和1", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("和2", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("和3", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("洋1", domain.GenreWestern, domain.DifficultyEasy),
		newMenu("洋2", domain.GenreWestern, domain.DifficultyEasy),
		newMenu("中1", domain.GenreChinese, domain.DifficultyEasy),
		newMenu("他1", domain.GenreOther, domain.DifficultyEasy),
		newMenu("他2", domain.GenreOther, domain.DifficultyEasy),
	}
}

// assertNoGenreStreak は同一ジャンルが3日以上連続していないことを確かめる。
func assertNoGenreStreak(t *testing.T, week []domain.DayMenu) {
	t.Helper()

	for i := 2; i < len(week); i++ {
		if week[i].Menu.Genre == week[i-1].Menu.Genre && week[i-1].Menu.Genre == week[i-2].Menu.Genre {
			t.Errorf("%d〜%d日目でジャンル %q が3連続している（%q / %q / %q）",
				week[i-2].Day, week[i].Day, week[i].Menu.Genre,
				week[i-2].Menu.Name, week[i-1].Menu.Name, week[i].Menu.Name)
		}
	}
}

// genresOf は週間献立のジャンルを順に返す。
func genresOf(week []domain.DayMenu) []domain.Genre {
	gs := make([]domain.Genre, 0, len(week))
	for _, d := range week {
		gs = append(gs, d.Menu.Genre)
	}
	return gs
}

func TestSuggestWeekly_同一ジャンルが3日以上連続しない(t *testing.T) {
	t.Parallel()

	svc := newWeeklyService(streakMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	assertNoGenreStreak(t, got)

	// 連続回避が無ければ 和1/和2/和3 と並ぶ。3日目で和食が外れることを固定する。
	assert.Equal(t, domain.GenreJapanese, got[0].Menu.Genre)
	assert.Equal(t, domain.GenreJapanese, got[1].Menu.Genre)
	assert.NotEqual(t, domain.GenreJapanese, got[2].Menu.Genre, "3日目は和食を避けること")
}

func TestSuggestWeekly_2連続は許容される(t *testing.T) {
	t.Parallel()

	// 2連続まで許すのが仕様（spec.md 2.2 は「3日以上連続しない」）。
	// 過剰に散らして候補を無駄に狭めないことを確かめる。
	svc := newWeeklyService(streakMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	genres := genresOf(got)

	found := false
	for i := 1; i < len(genres); i++ {
		if genres[i] == genres[i-1] {
			found = true
			break
		}
	}
	assert.True(t, found, "2連続が現れること（ジャンル: %v）", genres)
}

func TestSuggestWeekly_連続の判定は直前2日だけを見る(t *testing.T) {
	t.Parallel()

	// 和食が2日続いた後に別ジャンルを挟めば、その次はまた和食を選べる。
	// 「週内で和食は2回まで」ではない。
	svc := newWeeklyService(streakMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	genres := genresOf(got)

	count := 0
	for _, g := range genres {
		if g == domain.GenreJapanese {
			count++
		}
	}
	assert.Equal(t, 3, count, "和食3件が全て使われること（ジャンル: %v）", genres)
	assertNoGenreStreak(t, got)
}

func TestSuggestWeekly_重複回避と連続回避が同時に効く(t *testing.T) {
	t.Parallel()

	svc := newWeeklyService(streakMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)

	seen := map[domain.MenuID]bool{}
	for _, d := range got {
		assert.False(t, seen[d.Menu.ID], "%q が重複している", d.Menu.Name)
		seen[d.Menu.ID] = true
	}
	assertNoGenreStreak(t, got)
}

// assertNoRelaxation は緩和が1日も起きていないことを確かめる。
func assertNoRelaxation(t *testing.T, week []domain.DayMenu) {
	t.Helper()

	for _, d := range week {
		assert.False(t, d.Relaxed(), "%d日目(%s)で緩和が起きている", d.Day, d.Menu.Name)
	}
}

// relaxedDays は緩和が起きた日を返す。
func relaxedDays(week []domain.DayMenu) []int {
	days := []int{}
	for _, d := range week {
		if d.Relaxed() {
			days = append(days, d.Day)
		}
	}
	return days
}

func TestSuggestWeekly_候補が足りていれば緩和しない(t *testing.T) {
	t.Parallel()

	// 緩和は最後の手段。8件あるなら規則を全て守れる。
	svc := newWeeklyService(weeklyMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	assertNoRelaxation(t, got)
}

func TestSuggestWeekly_候補がちょうど7件でも緩和しない(t *testing.T) {
	t.Parallel()

	// 境界値。ぴったり足りているのに緩めるようでは緩和が早すぎる。
	svc := newWeeklyService(weeklyMenus()[:7], 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	assertNoRelaxation(t, got)
}

func TestSuggestWeekly_候補6件なら緩和して重複を許す(t *testing.T) {
	t.Parallel()

	// 6件では7日を埋められない。1日だけ重複を許せば足りる。
	menus := easyMenus()[:6]
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	assert.Equal(t, []int{7}, relaxedDays(got), "7日目だけ緩和されること")
	assert.True(t, got[6].RelaxedDuplicate, "重複回避を緩めたこと")
	assert.False(t, got[6].RelaxedGenreStreak, "ジャンル連続は緩めずに済むこと")

	// 6件は全て1度ずつ使われ、7日目だけがそのいずれかと重複する。
	seen := map[domain.MenuID]int{}
	for _, d := range got {
		seen[d.Menu.ID]++
	}
	assert.Len(t, seen, 6, "6件すべてが使われること")
}

func TestSuggestWeekly_候補1件なら7日とも同じ献立(t *testing.T) {
	t.Parallel()

	// 極端値。ここで無限ループやエラーにならないことを固定する。
	menus := []domain.Menu{newMenu("肉じゃが", domain.GenreJapanese, domain.DifficultyEasy)}
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	for _, d := range got {
		assert.Equal(t, "肉じゃが", d.Menu.Name)
	}

	// 1日目は緩和不要。2日目から重複、3日目からはジャンル連続も緩む。
	assert.False(t, got[0].Relaxed(), "1日目は緩和なし")
	assert.True(t, got[1].RelaxedDuplicate, "2日目は重複を緩める")
	assert.False(t, got[1].RelaxedGenreStreak, "2日目はまだ3連続にならない")
	assert.True(t, got[2].RelaxedDuplicate, "3日目は重複を緩める")
	assert.True(t, got[2].RelaxedGenreStreak, "3日目はジャンル連続も緩める")
}

func TestSuggestWeekly_候補が全て同一ジャンルなら3連続禁止を緩和する(t *testing.T) {
	t.Parallel()

	// 4-C で「ジャンルで絞ると必ず失敗する」としていた制約の解消。
	// 件数は足りているので、緩めるのはジャンル連続だけでよい。
	menus := make([]domain.Menu, 0, 10)
	for i := range 10 {
		menus = append(menus, newMenu(fmt.Sprintf("和%d", i+1), domain.GenreJapanese, domain.DifficultyEasy))
	}
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)

	seen := map[domain.MenuID]bool{}
	for _, d := range got {
		assert.False(t, seen[d.Menu.ID], "%q が重複している", d.Menu.Name)
		seen[d.Menu.ID] = true
		assert.False(t, d.RelaxedDuplicate, "%d日目: 件数は足りるので重複は緩めないこと", d.Day)
	}
	assert.Equal(t, []int{3, 4, 5, 6, 7}, relaxedDays(got), "3日目以降はジャンル連続を緩めること")
}

func TestSuggestWeekly_ジャンルで絞っても7日分が返る(t *testing.T) {
	t.Parallel()

	// 4-C の制約の解消を、利用者に近い形（spec.md 2.2 が許すジャンル指定）で確かめる。
	menus := make([]domain.Menu, 0, 10)
	for i := range 10 {
		menus = append(menus, newMenu(fmt.Sprintf("和%d", i+1), domain.GenreJapanese, domain.DifficultyEasy))
	}
	menus = append(menus, newMenu("ハンバーグ", domain.GenreWestern, domain.DifficultyEasy))
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.GenreJapanese),
	}, nil)

	require.NoError(t, err)
	assert.Len(t, got, 7)
	for _, d := range got {
		assert.Equal(t, domain.GenreJapanese, d.Menu.Genre, "絞り込みは緩めないこと")
	}
}

func TestSuggestWeekly_絞り込み条件は緩めない(t *testing.T) {
	t.Parallel()

	// 緩めるのは重複回避と連続回避だけ。利用者が指定した条件を勝手に外して
	// 別ジャンルを混ぜたら、それは要求と違うものを返している。
	menus := []domain.Menu{
		newMenu("和1", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("洋1", domain.GenreWestern, domain.DifficultyEasy),
		newMenu("洋2", domain.GenreWestern, domain.DifficultyEasy),
	}
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.GenreJapanese),
	}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	for _, d := range got {
		assert.Equal(t, "和1", d.Menu.Name, "候補1件でも他ジャンルを混ぜないこと")
	}
}

func TestSuggestWeekly_緩和はジャンル連続を先に緩める(t *testing.T) {
	t.Parallel()

	// spec.md 2.2 はルール1(重複しない)を先に挙げており、より重要度が高い。
	// 同じ献立が週に2度出るより、同ジャンルが3日続くほうが受け入れやすい。
	// 残りの候補があるうちは重複を作らず、ジャンル連続のほうを先に緩める。
	//
	// 候補: 和1, 和2, 和3, 洋1（4件）。乱数源は常に先頭。
	//   1日目 和1 / 2日目 和2 → ここで和食が2連続
	//   3日目 残り[和3, 洋1] のうち和食を避けると洋1。緩和は起きない
	menus := []domain.Menu{
		newMenu("和1", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("和2", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("和3", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("洋1", domain.GenreWestern, domain.DifficultyEasy),
	}
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)

	// 4日目までは4件で賄えるため重複は起きない。
	for i := range 4 {
		assert.False(t, got[i].RelaxedDuplicate, "%d日目: 候補が残るうちは重複させないこと", i+1)
	}
	// 5日目以降は候補を使い切っているので重複が始まる。
	assert.True(t, got[4].RelaxedDuplicate, "5日目: 候補を使い切ったら重複を許すこと")
}

func TestSuggestWeekly_緩和しても候補0件ならErrNoMenuFound(t *testing.T) {
	t.Parallel()

	// 緩和は候補を増やす操作ではない。0件は0件のまま。
	svc := newWeeklyService(nil, 0)

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	assert.ErrorIs(t, err, service.ErrNoMenuFound)
}

// idsOf は献立のIDを返す。
func idsOf(menus ...domain.Menu) []domain.MenuID {
	ids := make([]domain.MenuID, 0, len(menus))
	for _, m := range menus {
		ids = append(ids, m.ID)
	}
	return ids
}

// namesOf は週間献立の献立名を返す。
func namesOf(week []domain.DayMenu) []string {
	names := make([]string, 0, len(week))
	for _, d := range week {
		names = append(names, d.Menu.Name)
	}
	return names
}

func TestSuggestWeekly_直近履歴の献立を避ける(t *testing.T) {
	t.Parallel()

	// 候補8件のうち先頭1件が履歴にある。残り7件で7日を埋められるので避けられる。
	menus := weeklyMenus()
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, idsOf(menus[0]))

	require.NoError(t, err)
	require.Len(t, got, 7)
	assert.NotContains(t, namesOf(got), menus[0].Name, "履歴の献立が出ないこと")
	assertNoRelaxation(t, got)
}

func TestSuggestWeekly_履歴が空なら何も避けない(t *testing.T) {
	t.Parallel()

	menus := weeklyMenus()
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, nil)

	require.NoError(t, err)
	require.Len(t, got, 7)
	assert.Equal(t, menus[0].Name, got[0].Menu.Name, "先頭から選ばれること")
	assertNoRelaxation(t, got)
}

func TestSuggestWeekly_履歴を除くと7件に満たないなら履歴除外を緩和する(t *testing.T) {
	t.Parallel()

	// 候補8件のうち2件が履歴にある。残り6件では7日を埋められないため、
	// 履歴除外を緩めて7日目を埋める。
	menus := weeklyMenus()
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, idsOf(menus[0], menus[1]))

	require.NoError(t, err)
	require.Len(t, got, 7)

	// 履歴の献立が1件だけ現れ、その日は緩和済みと分かる。
	relaxed := relaxedDays(got)
	require.Len(t, relaxed, 1, "緩和は1日だけ（献立: %v）", namesOf(got))
	assert.True(t, got[relaxed[0]-1].RelaxedHistory, "履歴除外を緩めたと分かること")
	assert.False(t, got[relaxed[0]-1].RelaxedDuplicate, "重複は緩めずに済むこと")
}

func TestSuggestWeekly_履歴除外は重複回避より先に緩む(t *testing.T) {
	t.Parallel()

	// spec.md 2.2 はルール3(履歴)に「候補が枯渇する場合はこの条件を緩和する」と
	// 明記している。最も緩めてよい条件なので、重複より先に緩める。
	//
	// 候補7件中5件が履歴にある。履歴を避けると2件しか無いが、履歴を緩めれば
	// 7件で足りるため重複は起きない。
	menus := weeklyMenus()[:7]
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{},
		idsOf(menus[0], menus[1], menus[2], menus[3], menus[4]))

	require.NoError(t, err)
	require.Len(t, got, 7)
	for _, d := range got {
		assert.False(t, d.RelaxedDuplicate, "%d日目: 履歴を緩めれば足りるので重複させないこと", d.Day)
	}
	assert.ElementsMatch(t, namesOf(got), namesOf(got), "7件が全て使われること")
}

func TestSuggestWeekly_全候補が履歴にあっても7日分を返す(t *testing.T) {
	t.Parallel()

	// 極端値。履歴を避けられないなら諦めて出す。7日分が返らないほうが困る。
	menus := weeklyMenus()
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, idsOf(menus...))

	require.NoError(t, err)
	require.Len(t, got, 7)
	for _, d := range got {
		assert.True(t, d.RelaxedHistory, "%d日目: 履歴除外を緩めたと分かること", d.Day)
		assert.False(t, d.RelaxedDuplicate, "8件あるので重複はしないこと")
	}
}

func TestSuggestWeekly_履歴に無いIDを渡しても影響しない(t *testing.T) {
	t.Parallel()

	// マスタから消えた献立が履歴に残っている場合を想定する。
	menus := weeklyMenus()
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{},
		[]domain.MenuID{domain.NewMenuID()})

	require.NoError(t, err)
	require.Len(t, got, 7)
	assertNoRelaxation(t, got)
}

func TestSuggestWeekly_履歴は絞り込みに使わない(t *testing.T) {
	t.Parallel()

	// 履歴は「可能な限り避ける」弱い条件。repository の ExcludeIDs に渡すと
	// SQLで消えてしまい、緩和できなくなる。
	menus := weeklyMenus()
	repo := newFakeMenuRepository(menus...)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, idsOf(menus[0]))

	require.NoError(t, err)
	assert.Empty(t, repo.lastFilter.ExcludeIDs, "履歴を repository に渡さないこと")
}

func TestSuggestWeekly_連続を緩めても履歴は避ける(t *testing.T) {
	t.Parallel()

	// 候補が全て同一ジャンルだと3日目から連続を緩めざるを得ない。そのときでも
	// 履歴の献立まで持ち出す必要は無い。候補8件・履歴1件なら残り7件で埋まる。
	//
	// 「連続だけ緩めて履歴は避ける」段階が無いと、連続を緩める場面で履歴の
	// 献立が選ばれてしまう。
	menus := make([]domain.Menu, 0, 8)
	for i := range 8 {
		menus = append(menus, newMenu(fmt.Sprintf("和%d", i+1), domain.GenreJapanese, domain.DifficultyEasy))
	}
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{}, idsOf(menus[0]))

	require.NoError(t, err)
	require.Len(t, got, 7)
	assert.NotContains(t, namesOf(got), "和1", "履歴の献立を避けられること")
	for _, d := range got {
		assert.False(t, d.RelaxedHistory, "%d日目: 履歴は緩めずに済むこと", d.Day)
		assert.False(t, d.RelaxedDuplicate, "%d日目: 重複も緩めずに済むこと", d.Day)
	}
	assert.Equal(t, []int{3, 4, 5, 6, 7}, relaxedDays(got), "緩めるのはジャンル連続だけ")
}

// rerollWeek は引き直しのテスト用に「現在の週」を組み立てる。
func rerollWeek(menus []domain.Menu, indexes ...int) []domain.MenuID {
	ids := make([]domain.MenuID, 0, len(indexes))
	for _, i := range indexes {
		ids = append(ids, menus[i].ID)
	}
	return ids
}

func TestRerollDay_指定日だけ変わり他の日は保持される(t *testing.T) {
	t.Parallel()

	// 週は候補8件のうち先頭7件。3日目を引き直すと、使われていない8件目が入る。
	menus := weeklyMenus()
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(menus, 0)

	got, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

	require.NoError(t, err)
	assert.Equal(t, 3, got.Day, "指定した日が返ること")
	assert.Equal(t, menus[7].Name, got.Menu.Name, "週で使われていない献立が選ばれること")
	assert.False(t, got.Relaxed())
}

func TestRerollDay_引き直し後も重複回避が効く(t *testing.T) {
	t.Parallel()

	// 他の6日で使われている献立は選ばれない。
	menus := weeklyMenus()
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(menus, 0)

	got, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

	require.NoError(t, err)
	for i, id := range week {
		if i == 2 {
			continue // 引き直した日
		}
		assert.NotEqual(t, id, got.Menu.ID, "%d日目と重複している", i+1)
	}
}

func TestRerollDay_前後どちらの連続も避ける(t *testing.T) {
	t.Parallel()

	// 3日目を引き直す。1-2日目が和食、4-5日目も和食なので、
	// 和食を選ぶと [1,2,3] と [3,4,5] の両方で3連続になる。
	//
	// 前方だけを見る実装（SuggestWeekly の作り）だと 4-5日目の和食を見落とす。
	menus := []domain.Menu{
		newMenu("和1", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("和2", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("洋1", domain.GenreWestern, domain.DifficultyEasy), // 3日目（引き直す）
		newMenu("和3", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("和4", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("中1", domain.GenreChinese, domain.DifficultyEasy),
		newMenu("他1", domain.GenreOther, domain.DifficultyEasy),
		// 引き直しの候補。和食を選ぶと両側で3連続になる。
		newMenu("和5", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("洋2", domain.GenreWestern, domain.DifficultyEasy),
	}
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(menus, 0)

	got, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

	require.NoError(t, err)
	assert.Equal(t, "洋2", got.Menu.Name, "和食を選ぶと前後どちらでも3連続になる")
	assert.False(t, got.RelaxedGenreStreak)
}

func TestRerollDay_挟まれた形の連続も避ける(t *testing.T) {
	t.Parallel()

	// 2日目を引き直す。1日目と3日目が和食なので、和食を選ぶと [1,2,3] で3連続。
	// 前方（1日目）だけでは2連続にならないため、後方を見ないと見落とす。
	menus := []domain.Menu{
		newMenu("和1", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("洋1", domain.GenreWestern, domain.DifficultyEasy), // 2日目（引き直す）
		newMenu("和2", domain.GenreJapanese, domain.DifficultyEasy),
		newMenu("中1", domain.GenreChinese, domain.DifficultyEasy),
		newMenu("他1", domain.GenreOther, domain.DifficultyEasy),
		newMenu("中2", domain.GenreChinese, domain.DifficultyEasy),
		newMenu("他2", domain.GenreOther, domain.DifficultyEasy),
		newMenu("和3", domain.GenreJapanese, domain.DifficultyEasy), // 候補（和食）
		newMenu("洋2", domain.GenreWestern, domain.DifficultyEasy),  // 候補（洋食）
	}
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(menus, 0)

	got, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 2, nil)

	require.NoError(t, err)
	assert.Equal(t, "洋2", got.Menu.Name, "和食を選ぶと1日目と3日目に挟まれて3連続になる")
}

func TestRerollDay_範囲外のdayはエラー(t *testing.T) {
	t.Parallel()

	menus := weeklyMenus()
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(menus, 0)

	for _, day := range []int{0, -1, 8, 100} {
		t.Run(fmt.Sprintf("day=%d", day), func(t *testing.T) {
			t.Parallel()

			_, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, day, nil)

			assert.ErrorIs(t, err, service.ErrInvalidDay)
		})
	}
}

func TestRerollDay_週の件数が7でなければエラー(t *testing.T) {
	t.Parallel()

	// 他の日を保持する仕組みなので、週が揃っていないと重複回避を再適用できない。
	menus := weeklyMenus()
	svc := newWeeklyService(menus, 0)

	tests := map[string][]domain.MenuID{
		"空":   {},
		"6件":  rerollWeek(menus, 0, 1, 2, 3, 4, 5),
		"8件":  rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6, 7),
		"nil": nil,
	}
	for name, week := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

			assert.ErrorIs(t, err, service.ErrInvalidWeek)
		})
	}
}

func TestRerollDay_候補が週と同数なら同じ献立が返る(t *testing.T) {
	t.Parallel()

	// 候補が週の7件しか無い場合、引き直しの選択肢は
	//   (a) 同じ献立を返す（規則は全て守る）
	//   (b) 別の献立を返す（他の日と重複する）
	// の二択になる。重複回避は spec.md 2.2 のルール1で最も重要なので、
	// 規則を破らず据え置く。画面上は「変わらなかった」と見える。
	menus := weeklyMenus()[:7]
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(menus, 0)

	got, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

	require.NoError(t, err)
	assert.Equal(t, menus[2].ID, got.Menu.ID, "元の献立のまま")
	assert.False(t, got.Relaxed(), "規則は何も破っていない")
}

func TestRerollDay_候補に余裕があれば重複させずに引き直せる(t *testing.T) {
	t.Parallel()

	// 8件あれば未使用が1件残るので、重複も据え置きも起きない。
	menus := weeklyMenus()
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(menus, 0)

	got, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

	require.NoError(t, err)
	assert.Equal(t, menus[7].ID, got.Menu.ID, "未使用の献立が入ること")
	assert.False(t, got.RelaxedDuplicate)
}

func TestRerollDay_履歴を避ける(t *testing.T) {
	t.Parallel()

	menus := weeklyMenus()
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	// 8件目（唯一の未使用）が履歴にある。他に選べないので緩和して出す。
	svc := newWeeklyService(menus, 0)

	got, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, idsOf(menus[7]))

	require.NoError(t, err)
	assert.Equal(t, menus[7].Name, got.Menu.Name)
	assert.True(t, got.RelaxedHistory, "履歴を緩めたと分かること")
}

func TestRerollDay_不正な条件はエラー(t *testing.T) {
	t.Parallel()

	menus := weeklyMenus()
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(menus, 0)

	_, err := svc.RerollDay(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.Genre("フレンチ")),
	}, week, 3, nil)

	assert.ErrorIs(t, err, domain.ErrInvalidGenre)
}

func TestRerollDay_候補0件でErrNoMenuFound(t *testing.T) {
	t.Parallel()

	menus := weeklyMenus()
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(nil, 0)

	_, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

	assert.ErrorIs(t, err, service.ErrNoMenuFound)
}

func TestRerollDay_他の日はまとめて引く(t *testing.T) {
	t.Parallel()

	// 6日分を1件ずつ引くと問い合わせが6回になる。
	menus := weeklyMenus()
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	repo := newFakeMenuRepository(menus...)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, repo.idsCalls, "FindByIDs は1回")
	assert.Equal(t, 0, repo.idCalls, "FindByID を繰り返さないこと")
}

func TestRerollDay_同じ献立を返さない(t *testing.T) {
	t.Parallel()

	// 引き直して同じものが返るのでは引き直しにならない。
	// 引き直す日の献立自身も候補から外す。
	menus := weeklyMenus()
	week := rerollWeek(menus, 0, 1, 2, 3, 4, 5, 6)
	svc := newWeeklyService(menus, 0)

	got, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

	require.NoError(t, err)
	assert.NotEqual(t, menus[2].ID, got.Menu.ID, "3日目の元の献立(%s)が返っている", menus[2].Name)
}

func TestRerollDay_候補が1件しか無ければ同じ献立が返る(t *testing.T) {
	t.Parallel()

	// 極端値。他に選びようが無いなら諦めて同じものを返す。エラーにはしない。
	// 週の他の日にも同じ献立が並ぶ状況なので、重複の印が付く。
	menus := []domain.Menu{newMenu("肉じゃが", domain.GenreJapanese, domain.DifficultyEasy)}
	week := make([]domain.MenuID, 7)
	for i := range week {
		week[i] = menus[0].ID
	}
	svc := newWeeklyService(menus, 0)

	got, err := svc.RerollDay(context.Background(), domain.MenuFilter{}, week, 3, nil)

	require.NoError(t, err)
	assert.Equal(t, "肉じゃが", got.Menu.Name)
	assert.True(t, got.RelaxedDuplicate, "他の日にも同じ献立があるため重複の印が付く")
}
