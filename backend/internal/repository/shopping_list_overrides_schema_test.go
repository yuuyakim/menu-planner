package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// shopping_list_overrides のスキーマ（複合主キー / 既定値 / CASCADE）を
// 生SQLで直接確かめる。制約はDBが守るものなので、DBに対して検証する。

func TestShoppingListOverridesSchema_同じ週に同名は入らない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	u := createUser(t, pool, "slo-dup@example.com")
	week := insertSavedWeek(t, pool, u.ID)

	_, err := pool.Exec(ctx,
		`INSERT INTO shopping_list_overrides
		   (saved_weekly_menu_id, name, category, origin) VALUES ($1, 'にんじん', 'vegetable', 'derived')`, week)
	require.NoError(t, err)

	// 複合主キー (saved_weekly_menu_id, name) により2件目は入らない。
	_, err = pool.Exec(ctx,
		`INSERT INTO shopping_list_overrides
		   (saved_weekly_menu_id, name, category, origin) VALUES ($1, 'にんじん', 'meat', 'manual')`, week)
	require.Error(t, err, "同じ週に同名の差分は入ってはいけない")
}

func TestShoppingListOverridesSchema_既定値(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	u := createUser(t, pool, "slo-default@example.com")
	week := insertSavedWeek(t, pool, u.ID)

	// checked / hidden を省くと false、時刻は now() が入る。
	_, err := pool.Exec(ctx,
		`INSERT INTO shopping_list_overrides
		   (saved_weekly_menu_id, name, category, origin) VALUES ($1, 'たまねぎ', 'vegetable', 'derived')`, week)
	require.NoError(t, err)

	var checked, hidden bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT checked, hidden FROM shopping_list_overrides
		  WHERE saved_weekly_menu_id=$1 AND name='たまねぎ'`, week).Scan(&checked, &hidden))
	require.False(t, checked)
	require.False(t, hidden)
}

func TestShoppingListOverridesSchema_週を消すと差分も消える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	u := createUser(t, pool, "slo-cascade@example.com")
	week := insertSavedWeek(t, pool, u.ID)
	_, err := pool.Exec(ctx,
		`INSERT INTO shopping_list_overrides
		   (saved_weekly_menu_id, name, category, origin) VALUES ($1, 'にんじん', 'vegetable', 'derived')`, week)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM saved_weekly_menus WHERE id=$1`, week)
	require.NoError(t, err)

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM shopping_list_overrides WHERE saved_weekly_menu_id=$1`, week).Scan(&n))
	require.Zero(t, n, "週を消したら差分も消えるべき（CASCADE）")
}
