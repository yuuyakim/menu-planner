package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

var _ service.RecipeLinkCache = (*repository.RecipeLinkCache)(nil)

func testLink(t *testing.T, title, url string) domain.RecipeLink {
	t.Helper()

	link, err := domain.NewRecipeLink(title, url, title+"の説明")
	require.NoError(t, err)
	return link
}

func TestRecipeLinkCache_保存して取り出せる(t *testing.T) {
	pool := newTestPool(t)
	cache := repository.NewRecipeLinkCache(pool)
	ctx := context.Background()

	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	fetchedAt := time.Date(2026, 7, 17, 3, 0, 0, 0, time.UTC)
	links := []domain.RecipeLink{
		testLink(t, "親子丼の作り方", "https://recipe.example.com/1"),
		testLink(t, "簡単 親子丼", "https://cooking.example.net/2"),
	}

	require.NoError(t, cache.Save(ctx, menu.ID, links, fetchedAt))

	got, err := cache.Find(ctx, menu.ID)

	require.NoError(t, err)
	require.Len(t, got.Links, 2)
	assert.Equal(t, links, got.Links, "保存した内容がそのまま戻ること")
	assert.True(t, fetchedAt.Equal(got.FetchedAt), "取得時刻が保たれること: %v", got.FetchedAt)
}

func TestRecipeLinkCache_無ければErrRecipeCacheMiss(t *testing.T) {
	cache := repository.NewRecipeLinkCache(newTestPool(t))

	_, err := cache.Find(context.Background(), domain.NewMenuID())

	assert.ErrorIs(t, err, service.ErrRecipeCacheMiss)
}

func TestRecipeLinkCache_上書きできる(t *testing.T) {
	pool := newTestPool(t)
	cache := repository.NewRecipeLinkCache(pool)
	ctx := context.Background()

	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	old := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)

	require.NoError(t, cache.Save(ctx, menu.ID, []domain.RecipeLink{testLink(t, "古い", "https://old.example.com/1")}, old))
	require.NoError(t, cache.Save(ctx, menu.ID, []domain.RecipeLink{testLink(t, "新しい", "https://new.example.com/1")}, recent))

	got, err := cache.Find(ctx, menu.ID)

	require.NoError(t, err)
	require.Len(t, got.Links, 1, "行が増えず上書きされること")
	assert.Equal(t, "新しい", got.Links[0].Title)
	assert.True(t, recent.Equal(got.FetchedAt))
}

func TestRecipeLinkCache_0件も保存できる(t *testing.T) {
	// 0件も「探した結果」。保存できないと毎回APIを消費してしまう。
	pool := newTestPool(t)
	cache := repository.NewRecipeLinkCache(pool)
	ctx := context.Background()

	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	require.NoError(t, cache.Save(ctx, menu.ID, []domain.RecipeLink{}, time.Now()))

	got, err := cache.Find(ctx, menu.ID)

	require.NoError(t, err, "0件のキャッシュはミスではなくヒット")
	assert.Empty(t, got.Links)
}

func TestRecipeLinkCache_壊れたキャッシュはエラーになる(t *testing.T) {
	// DBの中身が壊れても、不正なリンクをそのまま画面に出さない。
	tests := map[string]string{
		"javascriptスキーム": `[{"title":"罠","url":"javascript:alert(1)","snippet":"x"}]`,
		"タイトルが空":         `[{"title":"","url":"https://example.com/1","snippet":"x"}]`,
		"URLが空":          `[{"title":"題","url":"","snippet":"x"}]`,
		"JSONとして壊れている":   `["文字列"]`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			pool := newTestPool(t)
			cache := repository.NewRecipeLinkCache(pool)
			ctx := context.Background()

			menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
			_, err := pool.Exec(ctx,
				`INSERT INTO recipe_link_caches (menu_id, links, fetched_at) VALUES ($1, $2, now())`,
				menu.ID.String(), raw)
			require.NoError(t, err)

			_, err = cache.Find(ctx, menu.ID)

			require.Error(t, err, "壊れたキャッシュを黙って返さないこと")
			assert.NotErrorIs(t, err, service.ErrRecipeCacheMiss, "ミスと障害を取り違えないこと")
		})
	}
}

func TestRecipeLinkCache_配列以外は表が拒否する(t *testing.T) {
	pool := newTestPool(t)
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	_, err := pool.Exec(context.Background(),
		`INSERT INTO recipe_link_caches (menu_id, links, fetched_at) VALUES ($1, $2, now())`,
		menu.ID.String(), `{"not":"an array"}`)

	require.Error(t, err, "CHECK 制約が効くこと")
}

func TestRecipeLinkCache_献立を消すとキャッシュも消える(t *testing.T) {
	pool := newTestPool(t)
	cache := repository.NewRecipeLinkCache(pool)
	ctx := context.Background()

	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	require.NoError(t, cache.Save(ctx, menu.ID, []domain.RecipeLink{testLink(t, "題", "https://example.com/1")}, time.Now()))

	_, err := pool.Exec(ctx, `DELETE FROM menus WHERE id = $1`, menu.ID.String())
	require.NoError(t, err)

	_, err = cache.Find(ctx, menu.ID)

	assert.ErrorIs(t, err, service.ErrRecipeCacheMiss, "CASCADE で消えること")
}

func TestRecipeLinkCache_存在しない献立には保存できない(t *testing.T) {
	// 外部キーが無いと、消えた献立のキャッシュが残り続ける。
	cache := repository.NewRecipeLinkCache(newTestPool(t))

	err := cache.Save(context.Background(), domain.NewMenuID(),
		[]domain.RecipeLink{testLink(t, "題", "https://example.com/1")}, time.Now())

	require.Error(t, err)
}
