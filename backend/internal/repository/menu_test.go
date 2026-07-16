package repository_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func ptr[T any](v T) *T { return &v }

// insertMenu はテスト用の献立を1件投入する。
func insertMenu(t *testing.T, pool *pgxpool.Pool, name string, g domain.Genre, d domain.Difficulty) domain.Menu {
	t.Helper()

	m := domain.Menu{
		ID:          domain.NewMenuID(),
		Name:        name,
		NameKana:    name + "かな",
		Genre:       g,
		Difficulty:  d,
		Description: name + "の説明",
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO menus (id, name, name_kana, genre, difficulty, description)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		m.ID.String(), m.Name, m.NameKana, m.Genre.String(), m.Difficulty.String(), m.Description)
	require.NoError(t, err)
	return m
}

func TestMenuRepository_FindByID_存在する(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	want := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	got, err := repo.FindByID(context.Background(), want.ID)
	require.NoError(t, err)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, "親子丼", got.Name)
	assert.Equal(t, domain.GenreJapanese, got.Genre)
	assert.Equal(t, domain.DifficultyEasy, got.Difficulty)
	assert.NotEmpty(t, got.Description)
}

func TestMenuRepository_FindByID_存在しない(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	got, err := repo.FindByID(context.Background(), domain.NewMenuID())
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrMenuNotFound)
	assert.Nil(t, got)
}

func TestMenuRepository_FindByFilter_genreのみ指定(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	insertMenu(t, pool, "肉じゃが", domain.GenreJapanese, domain.DifficultyNormal)
	insertMenu(t, pool, "ナポリタン", domain.GenreWestern, domain.DifficultyEasy)

	got, err := repo.FindByFilter(context.Background(), domain.MenuFilter{
		Genre: ptr(domain.GenreJapanese),
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, m := range got {
		assert.Equal(t, domain.GenreJapanese, m.Genre)
	}
}

func TestMenuRepository_FindByFilter_difficultyのみ指定(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	insertMenu(t, pool, "ナポリタン", domain.GenreWestern, domain.DifficultyEasy)
	insertMenu(t, pool, "肉じゃが", domain.GenreJapanese, domain.DifficultyNormal)

	got, err := repo.FindByFilter(context.Background(), domain.MenuFilter{
		Difficulty: ptr(domain.DifficultyEasy),
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, m := range got {
		assert.Equal(t, domain.DifficultyEasy, m.Difficulty)
	}
}

func TestMenuRepository_FindByFilter_両方指定(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	want := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	insertMenu(t, pool, "肉じゃが", domain.GenreJapanese, domain.DifficultyNormal)
	insertMenu(t, pool, "ナポリタン", domain.GenreWestern, domain.DifficultyEasy)

	got, err := repo.FindByFilter(context.Background(), domain.MenuFilter{
		Genre:      ptr(domain.GenreJapanese),
		Difficulty: ptr(domain.DifficultyEasy),
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, want.ID, got[0].ID)
}

func TestMenuRepository_FindByFilter_両方nilで全件(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	insertMenu(t, pool, "ナポリタン", domain.GenreWestern, domain.DifficultyNormal)
	insertMenu(t, pool, "麻婆豆腐", domain.GenreChinese, domain.DifficultyElaborate)

	got, err := repo.FindByFilter(context.Background(), domain.MenuFilter{})
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

func TestMenuRepository_FindByFilter_該当0件は空スライス(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	got, err := repo.FindByFilter(context.Background(), domain.MenuFilter{
		Genre: ptr(domain.GenreChinese),
	})
	require.NoError(t, err)
	assert.Empty(t, got)
	// nil ではなく空スライスを返すこと（呼び出し側で len() や range が安全に使える）
	assert.NotNil(t, got)
}

func TestMenuRepository_FindByFilter_ExcludeIDsで除外(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	a := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	b := insertMenu(t, pool, "牛丼", domain.GenreJapanese, domain.DifficultyEasy)
	c := insertMenu(t, pool, "かつ丼", domain.GenreJapanese, domain.DifficultyEasy)

	got, err := repo.FindByFilter(context.Background(), domain.MenuFilter{
		Genre:      ptr(domain.GenreJapanese),
		ExcludeIDs: []domain.MenuID{a.ID, b.ID},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, c.ID, got[0].ID)
}

func TestMenuRepository_FindByFilter_ExcludeIDsが空なら除外しない(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	insertMenu(t, pool, "牛丼", domain.GenreJapanese, domain.DifficultyEasy)

	got, err := repo.FindByFilter(context.Background(), domain.MenuFilter{
		ExcludeIDs: []domain.MenuID{},
	})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestMenuRepository_FindByFilter_全件除外すると0件(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	a := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	b := insertMenu(t, pool, "牛丼", domain.GenreJapanese, domain.DifficultyEasy)

	got, err := repo.FindByFilter(context.Background(), domain.MenuFilter{
		ExcludeIDs: []domain.MenuID{a.ID, b.ID},
	})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestMenuRepository_FindByFilter_不正な条件はエラー(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	got, err := repo.FindByFilter(context.Background(), domain.MenuFilter{
		Genre: ptr(domain.Genre("italian")),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidGenre)
	assert.Nil(t, got)
}

func TestMenuRepository_FindByFilter_名前順で安定して返る(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewMenuRepository(pool)

	insertMenu(t, pool, "牛丼", domain.GenreJapanese, domain.DifficultyEasy)
	insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	first, err := repo.FindByFilter(context.Background(), domain.MenuFilter{})
	require.NoError(t, err)
	second, err := repo.FindByFilter(context.Background(), domain.MenuFilter{})
	require.NoError(t, err)

	// 順序が不定だとランダム選択の再現性が取れないため、常に同じ順序で返すこと
	require.Len(t, first, 2)
	assert.Equal(t, first[0].ID, second[0].ID)
	assert.Equal(t, first[1].ID, second[1].ID)
}
