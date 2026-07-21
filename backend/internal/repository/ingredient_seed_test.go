package repository_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
)

// seedAll はシードを投入する。テスト用DBは空から始まるため毎回入れ直す。
func seedAll(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newTestPool(t)
	_, err := db.Seed(context.Background(), pool)
	require.NoError(t, err)
	return pool
}

func TestSeed_EveryMenuHasIngredients(t *testing.T) {
	// **紐付けは名前で結合しているため、綴りを1文字間違えると黙って0件になる。**
	// 「食材を持たない献立が無いこと」で取りこぼしを検出する。
	pool := seedAll(t)

	rows, err := pool.Query(context.Background(), `
		SELECT m.name
		FROM menus m
		LEFT JOIN menu_ingredients mi ON mi.menu_id = m.id
		WHERE mi.menu_id IS NULL
		ORDER BY m.name`)
	require.NoError(t, err)
	defer rows.Close()

	var orphans []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		orphans = append(orphans, name)
	}
	require.NoError(t, rows.Err())

	assert.Emptyf(t, orphans, "食材が紐付いていない献立がある（名前の綴り違いの可能性）: %v", orphans)
}

func TestSeed_EveryIngredientIsUsed(t *testing.T) {
	// どの献立からも使われていない食材は、綴り違いか消し忘れ。
	pool := seedAll(t)

	rows, err := pool.Query(context.Background(), `
		SELECT i.name
		FROM ingredients i
		LEFT JOIN menu_ingredients mi ON mi.ingredient_id = i.id
		WHERE mi.ingredient_id IS NULL
		ORDER BY i.name`)
	require.NoError(t, err)
	defer rows.Close()

	var unused []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		unused = append(unused, name)
	}
	require.NoError(t, rows.Err())

	assert.Emptyf(t, unused, "どの献立からも使われていない食材がある: %v", unused)
}

func TestSeed_NoSeasoningRegistered(t *testing.T) {
	// 調味料は食材として持たない（spec.md 14.4）。
	// カテゴリでは弾けても、名前として紛れ込むことはあるので明示的に見る。
	pool := seedAll(t)

	var count int
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT count(*) FROM ingredients
		WHERE name IN ('醤油','味噌','塩','砂糖','みりん','酒','油','だし','こしょう',
		               '小麦粉','片栗粉','パン粉','マヨネーズ','ケチャップ')`).Scan(&count))

	assert.Zero(t, count, "調味料・粉類は食材として登録しない")
}

func TestSeed_IsIdempotent(t *testing.T) {
	// 再実行しても重複しない（ON CONFLICT DO NOTHING）。
	pool := seedAll(t)
	ctx := context.Background()

	before := countRows(t, pool)

	inserted, err := db.Seed(ctx, pool)
	require.NoError(t, err)
	assert.Zero(t, inserted, "2回目は1行も入らない")
	assert.Equal(t, before, countRows(t, pool), "行数が変わらない")
}

// countRows は3テーブルの行数をまとめて返す。
func countRows(t *testing.T, pool *pgxpool.Pool) [3]int {
	t.Helper()
	ctx := context.Background()
	var c [3]int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM menus`).Scan(&c[0]))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM ingredients`).Scan(&c[1]))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM menu_ingredients`).Scan(&c[2]))
	return c
}
