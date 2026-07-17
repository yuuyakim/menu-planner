package service_test

import (
	"context"
	"errors"
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

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{})

	require.NoError(t, err)
	assert.Len(t, got, 7, "spec.md 2.2 の7日分")
}

func TestSuggestWeekly_dayが1から7の連番(t *testing.T) {
	t.Parallel()

	svc := newWeeklyService(weeklyMenus(), 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{})

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

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{})

	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.Equal(t, 1, got[0].Day)
	assert.Equal(t, 7, got[len(got)-1].Day)
}

func TestSuggestWeekly_乱数列に従って献立が選ばれる(t *testing.T) {
	t.Parallel()

	menus := weeklyMenus()
	// 候補8件から順に添字 0..6 を引かせる。
	svc := newWeeklyService(menus, 0, 1, 2, 3, 4, 5, 6)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{})

	require.NoError(t, err)
	require.Len(t, got, 7)
	for i, d := range got {
		assert.Equal(t, menus[i].Name, d.Menu.Name, "%d日目", i+1)
	}
}

func TestSuggestWeekly_条件がrepositoryに渡る(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(weeklyMenus()...)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.GenreJapanese),
	})

	require.NoError(t, err)
	require.NotNil(t, repo.lastFilter.Genre)
	assert.Equal(t, domain.GenreJapanese, *repo.lastFilter.Genre)
}

func TestSuggestWeekly_候補を1度だけ問い合わせる(t *testing.T) {
	t.Parallel()

	// 7日分それぞれでDBを叩くと7倍の負荷になる。候補は一度引いて使い回す。
	repo := newFakeMenuRepository(weeklyMenus()...)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{})

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

			_, err := svc.SuggestWeekly(context.Background(), tt.filter)

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
	})

	assert.ErrorIs(t, err, domain.ErrInvalidGenre)
	assert.Equal(t, 0, repo.filterCalls, "条件が不正ならDBに問い合わせないこと")
}

func TestSuggestWeekly_repositoryのエラーがラップされて返る(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("DBへの接続に失敗しました")
	repo := newFakeMenuRepository(weeklyMenus()...)
	repo.err = sentinel
	svc := service.NewMenuService(repo, randomtest.NewFixed(0), newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{})

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.NotErrorIs(t, err, service.ErrNoMenuFound, "DB障害を該当なしと誤認しないこと")
}

func TestSuggestWeekly_乱数源のエラーは該当なしと区別できる(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("乱数源の故障")
	svc := service.NewMenuService(newFakeMenuRepository(weeklyMenus()...), failingRandomizer{err: sentinel},
		newFakeRecipeGateway(), newFakeRecipeCache())

	_, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{})

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.NotErrorIs(t, err, service.ErrNoMenuFound)
}

func TestSuggestWeekly_骨格の段階では重複を許す(t *testing.T) {
	t.Parallel()

	// 重複回避は 4-B で入れる。この段階では候補1件でも7日分を返せることを
	// 固定しておき、4-B での挙動の変化が差分として見えるようにする。
	menus := []domain.Menu{newMenu("肉じゃが", domain.GenreJapanese, domain.DifficultyEasy)}
	svc := newWeeklyService(menus, 0)

	got, err := svc.SuggestWeekly(context.Background(), domain.MenuFilter{})

	require.NoError(t, err)
	require.Len(t, got, 7)
	for _, d := range got {
		assert.Equal(t, "肉じゃが", d.Menu.Name)
	}
}
