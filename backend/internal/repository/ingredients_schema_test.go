package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// insertIngredient は食材を1件入れ、そのIDを返す。
func insertIngredient(t *testing.T, pool *pgxpool.Pool, name, kana, category string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO ingredients (id, name, name_kana, category) VALUES ($1,$2,$3,$4)`,
		id, name, kana, category)
	require.NoError(t, err)
	return id
}

func TestIngredientsSchema_AcceptsAllCategories(t *testing.T) {
	pool := newTestPool(t)

	for _, c := range domain.AllIngredientCategories() {
		insertIngredient(t, pool, "食材-"+c.String(), "しょくざい", c.String())
	}
}

func TestIngredientsSchema_RejectsSeasoningCategory(t *testing.T) {
	// 調味料は食材として持たない（spec.md 14.4）。
	// アプリ側だけでなくDBでも弾き、誤ったデータが入らないようにする。
	pool := newTestPool(t)

	_, err := pool.Exec(context.Background(),
		`INSERT INTO ingredients (id, name, name_kana, category) VALUES ($1,'醤油','しょうゆ','seasoning')`,
		uuid.NewString())
	assert.Error(t, err, "seasoning は受け付けない")
}

func TestIngredientsSchema_RejectsBlankNameAndKana(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`INSERT INTO ingredients (id, name, name_kana, category) VALUES ($1,'   ','たまねぎ','vegetable')`,
		uuid.NewString())
	assert.Error(t, err, "空白だけの名前は受け付けない")

	_, err = pool.Exec(ctx,
		`INSERT INTO ingredients (id, name, name_kana, category) VALUES ($1,'玉ねぎ','  ','vegetable')`,
		uuid.NewString())
	assert.Error(t, err, "空白だけのカナは受け付けない")
}

func TestIngredientsSchema_NameIsUnique(t *testing.T) {
	// シードを冪等にするための制約。
	pool := newTestPool(t)
	insertIngredient(t, pool, "重複テスト玉ねぎ", "たまねぎ", "vegetable")

	_, err := pool.Exec(context.Background(),
		`INSERT INTO ingredients (id, name, name_kana, category) VALUES ($1,'重複テスト玉ねぎ','たまねぎ','vegetable')`,
		uuid.NewString())
	assert.Error(t, err, "同名の食材は二重に入らない")
}

func TestMenuIngredients_LinksMenuAndIngredient(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	menu := insertMenu(t, pool, "紐付けテスト肉じゃが", domain.GenreJapanese, domain.DifficultyEasy)
	ing := insertIngredient(t, pool, "紐付けテストじゃがいも", "じゃがいも", "vegetable")

	_, err := pool.Exec(ctx,
		`INSERT INTO menu_ingredients (menu_id, ingredient_id) VALUES ($1,$2)`,
		menu.ID.String(), ing)
	require.NoError(t, err)

	// 同じ組み合わせは二重に入らない（主キー）。
	_, err = pool.Exec(ctx,
		`INSERT INTO menu_ingredients (menu_id, ingredient_id) VALUES ($1,$2)`,
		menu.ID.String(), ing)
	assert.Error(t, err, "同じ献立に同じ食材は二重に入らない")
}

func TestMenuIngredients_DeletingMenuRemovesLinks(t *testing.T) {
	// 献立を消したら紐付けも消える（ON DELETE CASCADE）。
	pool := newTestPool(t)
	ctx := context.Background()

	menu := insertMenu(t, pool, "CASCADE検証献立", domain.GenreJapanese, domain.DifficultyEasy)
	ing := insertIngredient(t, pool, "CASCADE検証食材", "けんしょう", "vegetable")
	_, err := pool.Exec(ctx,
		`INSERT INTO menu_ingredients (menu_id, ingredient_id) VALUES ($1,$2)`,
		menu.ID.String(), ing)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM menus WHERE id=$1`, menu.ID.String())
	require.NoError(t, err)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM menu_ingredients WHERE ingredient_id=$1`, ing).Scan(&count))
	assert.Equal(t, 0, count, "献立を消したら紐付けも消える")
}

func TestMenuIngredients_CannotDeleteIngredientInUse(t *testing.T) {
	// 使われている食材は消せない（ON DELETE RESTRICT）。
	// 消せてしまうと、献立から食材が黙って欠落する。
	pool := newTestPool(t)
	ctx := context.Background()

	menu := insertMenu(t, pool, "RESTRICT検証献立", domain.GenreJapanese, domain.DifficultyEasy)
	ing := insertIngredient(t, pool, "RESTRICT検証食材", "けんしょう", "vegetable")
	_, err := pool.Exec(ctx,
		`INSERT INTO menu_ingredients (menu_id, ingredient_id) VALUES ($1,$2)`,
		menu.ID.String(), ing)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM ingredients WHERE id=$1`, ing)
	assert.Error(t, err, "使われている食材は消せない")
}
