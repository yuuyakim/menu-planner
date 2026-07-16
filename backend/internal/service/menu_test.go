package service_test

import (
	"context"
	"errors"
	"testing"

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
	svc := service.NewMenuService(repo, randomtest.NewFixed(0))

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
	svc := service.NewMenuService(repo, randomtest.NewFixed(1))

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
	svc := service.NewMenuService(repo, randomtest.NewFixed(0))

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
			svc := service.NewMenuService(repo, randomtest.NewFixed(i))

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
	svc := service.NewMenuService(repo, randomtest.NewFixed(0))

	_, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{
		Genre: genrePtr(domain.Genre("フレンチ")),
	})

	assert.ErrorIs(t, err, domain.ErrInvalidGenre)
	assert.Equal(t, 0, repo.filterCalls, "条件が不正ならDBに問い合わせないこと")
}

func TestSuggestMenu_不正なdifficultyはErrInvalidDifficulty(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(testMenus()...)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0))

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
			svc := service.NewMenuService(repo, randomtest.NewFixed(0))

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
	svc := service.NewMenuService(repo, randomtest.NewFixed(0))

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
	svc := service.NewMenuService(repo, randomtest.NewFixed(0))

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
	_, gotDBErr := service.NewMenuService(failing, randomtest.NewFixed(0)).
		SuggestMenu(context.Background(), domain.MenuFilter{})

	_, gotEmptyErr := service.NewMenuService(newFakeMenuRepository(), randomtest.NewFixed(0)).
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
	svc := service.NewMenuService(newFakeMenuRepository(testMenus()...), failingRandomizer{err: sentinel})

	_, err := svc.SuggestMenu(context.Background(), domain.MenuFilter{})

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.NotErrorIs(t, err, service.ErrNoMenuFound)
}

func TestSuggestMenu_返る献立はマスタの内容をそのまま持つ(t *testing.T) {
	t.Parallel()

	repo := newFakeMenuRepository(testMenus()...)
	svc := service.NewMenuService(repo, randomtest.NewFixed(0))

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
