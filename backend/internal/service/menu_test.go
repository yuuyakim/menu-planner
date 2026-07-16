package service_test

import (
	"context"
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
