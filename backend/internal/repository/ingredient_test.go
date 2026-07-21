package repository_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// 実装がインターフェースを満たすことをコンパイル時に保証する。
var _ service.IngredientRepository = (*repository.IngredientRepository)(nil)

// menuIDByName はシード済みの献立名からIDを引く。
func menuIDByName(t *testing.T, pool *pgxpool.Pool, name string) domain.MenuID {
	t.Helper()
	var raw string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT id FROM menus WHERE name=$1`, name).Scan(&raw))
	id, err := domain.ParseMenuID(raw)
	require.NoError(t, err)
	return id
}

// namesOf は取得した対応から食材名だけを取り出す。
func namesOf(items []service.MenuIngredient) []string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Ingredient.Name)
	}
	return names
}

func TestIngredientRepository_FindByMenuIDs_SingleMenu(t *testing.T) {
	pool := seedAll(t)
	repo := repository.NewIngredientRepository(pool)

	id := menuIDByName(t, pool, "肉じゃが")
	got, err := repo.FindByMenuIDs(context.Background(), []domain.MenuID{id})
	require.NoError(t, err)

	// カナ順で安定して返る（service 側でカテゴリ順に並べ替える前提）。
	assert.Equal(t, []string{"糸こんにゃく", "じゃがいも", "玉ねぎ", "にんじん", "豚こま切れ肉"}, namesOf(got))
	for _, it := range got {
		assert.Equal(t, id, it.MenuID, "どの献立の食材かが分かる")
		assert.NoError(t, it.Ingredient.Validate(), "取得した食材が妥当")
	}
}

func TestIngredientRepository_FindByMenuIDs_MultipleMenus(t *testing.T) {
	// 買い物リストの主経路。複数の献立をまとめて1回で引く。
	pool := seedAll(t)
	repo := repository.NewIngredientRepository(pool)

	nikujaga := menuIDByName(t, pool, "肉じゃが")
	oyakodon := menuIDByName(t, pool, "親子丼")

	got, err := repo.FindByMenuIDs(context.Background(),
		[]domain.MenuID{nikujaga, oyakodon})
	require.NoError(t, err)

	// 両方の献立の食材が、それぞれの献立IDつきで返る。
	byMenu := map[domain.MenuID][]string{}
	for _, it := range got {
		byMenu[it.MenuID] = append(byMenu[it.MenuID], it.Ingredient.Name)
	}
	assert.Len(t, byMenu, 2)
	assert.Contains(t, byMenu[nikujaga], "じゃがいも")
	assert.Contains(t, byMenu[oyakodon], "鶏もも肉")

	// 玉ねぎは両方に使われるため、2件の対応として返る（集約は service の仕事）。
	var onionCount int
	for _, it := range got {
		if it.Ingredient.Name == "玉ねぎ" {
			onionCount++
		}
	}
	assert.Equal(t, 2, onionCount, "同じ食材でも献立ごとに1件ずつ返る")
}

func TestIngredientRepository_FindByMenuIDs_UnknownIDIsIgnored(t *testing.T) {
	// 存在しない献立IDは黙って除く。件数は呼び出し側（service）が判断する。
	pool := seedAll(t)
	repo := repository.NewIngredientRepository(pool)

	known := menuIDByName(t, pool, "親子丼")
	got, err := repo.FindByMenuIDs(context.Background(),
		[]domain.MenuID{known, domain.NewMenuID()})
	require.NoError(t, err)

	for _, it := range got {
		assert.Equal(t, known, it.MenuID)
	}
	assert.NotEmpty(t, got)
}

func TestIngredientRepository_FindByMenuIDs_Empty(t *testing.T) {
	// 空の指定でDBに問い合わせても意味がないので、空を返す。
	pool := seedAll(t)
	repo := repository.NewIngredientRepository(pool)

	got, err := repo.FindByMenuIDs(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestIngredientRepository_FindByMenuIDs_DuplicateIDs(t *testing.T) {
	// 同じ献立IDが重複していても、対応が二重に返らない。
	pool := seedAll(t)
	repo := repository.NewIngredientRepository(pool)

	id := menuIDByName(t, pool, "かぼちゃの煮物")
	got, err := repo.FindByMenuIDs(context.Background(),
		[]domain.MenuID{id, id})
	require.NoError(t, err)

	assert.Equal(t, []string{"かぼちゃ"}, namesOf(got))
}
