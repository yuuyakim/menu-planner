package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// saved_weekly_menus / saved_weekly_menu_days のスキーマ（複合PK / CHECK / CASCADE）を
// 生SQLで直接確かめる。制約はDBが守るものなので、DBに対して検証する。

// insertSavedWeek は保存の親行を1件作り、そのIDを返す。
func insertSavedWeek(t *testing.T, pool *pgxpool.Pool, userID domain.UserID) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO saved_weekly_menus (id, user_id) VALUES ($1, $2)`,
		id, userID.String())
	require.NoError(t, err)
	return id
}

func TestSavedWeeklySchema_同じ週に同じ日は入らない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "saved-dup-day@example.com")
	week := insertSavedWeek(t, pool, u.ID)
	a := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	b := insertMenu(t, pool, "肉じゃが", domain.GenreJapanese, domain.DifficultyEasy)

	_, err := pool.Exec(ctx,
		`INSERT INTO saved_weekly_menu_days (saved_weekly_menu_id, day, menu_id)
		 VALUES ($1, 3, $2)`, week, a.ID.String())
	require.NoError(t, err)

	// 同じ週の「3日目」は2件目を作れない（複合主キー）。
	_, err = pool.Exec(ctx,
		`INSERT INTO saved_weekly_menu_days (saved_weekly_menu_id, day, menu_id)
		 VALUES ($1, 3, $2)`, week, b.ID.String())
	require.Error(t, err, "同じ週に同じ日が2件入ってはいけない")
}

func TestSavedWeeklySchema_別の週なら同じ日を持てる(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "saved-two-weeks@example.com")
	first := insertSavedWeek(t, pool, u.ID)
	second := insertSavedWeek(t, pool, u.ID)
	m := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	for _, week := range []string{first, second} {
		_, err := pool.Exec(ctx,
			`INSERT INTO saved_weekly_menu_days (saved_weekly_menu_id, day, menu_id)
			 VALUES ($1, 1, $2)`, week, m.ID.String())
		require.NoError(t, err, "週が違えば同じ日を持てるべき")
	}
}

func TestSavedWeeklySchema_日は1から7まで(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "saved-day-range@example.com")
	week := insertSavedWeek(t, pool, u.ID)
	m := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	// 範囲外は入らない。0 と 8 の両側を確かめる。
	for _, day := range []int{0, 8, -1} {
		_, err := pool.Exec(ctx,
			`INSERT INTO saved_weekly_menu_days (saved_weekly_menu_id, day, menu_id)
			 VALUES ($1, $2, $3)`, week, day, m.ID.String())
		require.Error(t, err, "%d 日目は範囲外なので拒否されるべき", day)
	}

	// 両端は入る。
	for _, day := range []int{1, domain.WeekLength} {
		_, err := pool.Exec(ctx,
			`INSERT INTO saved_weekly_menu_days (saved_weekly_menu_id, day, menu_id)
			 VALUES ($1, $2, $3)`, week, day, m.ID.String())
		require.NoError(t, err, "%d 日目は範囲内なので入るべき", day)
	}
}

func TestSavedWeeklySchema_週を消すと中身も消える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "saved-cascade-week@example.com")
	week := insertSavedWeek(t, pool, u.ID)
	m := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)
	_, err := pool.Exec(ctx,
		`INSERT INTO saved_weekly_menu_days (saved_weekly_menu_id, day, menu_id)
		 VALUES ($1, 1, $2)`, week, m.ID.String())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM saved_weekly_menus WHERE id=$1`, week)
	require.NoError(t, err)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM saved_weekly_menu_days WHERE saved_weekly_menu_id=$1`,
		week).Scan(&count))
	require.Zero(t, count, "週を消したら中身も消えるべき")
}

func TestSavedWeeklySchema_ユーザーを消すと保存も消える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "saved-cascade-user@example.com")
	insertSavedWeek(t, pool, u.ID)

	_, err := pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID.String())
	require.NoError(t, err)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM saved_weekly_menus WHERE user_id=$1`,
		u.ID.String()).Scan(&count))
	require.Zero(t, count, "ユーザー削除で保存も消えるべき")
}

func TestSavedWeeklySchema_存在しない献立は保存できない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "saved-unknown-menu@example.com")
	week := insertSavedWeek(t, pool, u.ID)

	// 献立マスタとの不整合をDBが防ぐ。jsonb ではなく正規化した理由（spec.md 4.2）。
	_, err := pool.Exec(ctx,
		`INSERT INTO saved_weekly_menu_days (saved_weekly_menu_id, day, menu_id)
		 VALUES ($1, 1, $2)`, week, uuid.NewString())
	require.Error(t, err, "存在しない献立は外部キーで拒否されるべき")
}
