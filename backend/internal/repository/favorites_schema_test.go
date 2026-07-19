package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

// favorites のスキーマ（UNIQUE / CASCADE）を生SQLで直接確かめる。
// 制約はDBが守るものなので、DBに対して検証する。

// createUser はテスト用のユーザーをパスワード認証つきで作る。
func createUser(t *testing.T, pool *pgxpool.Pool, email string) domain.User {
	t.Helper()
	u := newUser(t, email)
	require.NoError(t, repository.NewUserRepository(pool).CreateWithPassword(context.Background(), u, "hash"))
	return u
}

func TestFavoritesSchema_UniqueUserMenu(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "fav-uniq@example.com")
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	_, err := pool.Exec(ctx,
		`INSERT INTO favorites (id, user_id, menu_id) VALUES ($1, $2, $3)`,
		uuid.NewString(), u.ID.String(), menu.ID.String())
	require.NoError(t, err)

	// 同じ (user_id, menu_id) の2件目は拒否される。
	_, err = pool.Exec(ctx,
		`INSERT INTO favorites (id, user_id, menu_id) VALUES ($1, $2, $3)`,
		uuid.NewString(), u.ID.String(), menu.ID.String())
	require.Error(t, err, "同じ献立の二重登録は拒否されるべき")
}

func TestFavoritesSchema_DifferentUsersSameMenu(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	a := createUser(t, pool, "fav-a@example.com")
	b := createUser(t, pool, "fav-b@example.com")
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	// 別ユーザーなら同じ献立を登録できる。
	_, err := pool.Exec(ctx,
		`INSERT INTO favorites (id, user_id, menu_id) VALUES ($1, $2, $3)`,
		uuid.NewString(), a.ID.String(), menu.ID.String())
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO favorites (id, user_id, menu_id) VALUES ($1, $2, $3)`,
		uuid.NewString(), b.ID.String(), menu.ID.String())
	require.NoError(t, err, "別ユーザーなら同じ献立を登録できるべき")
}

func TestFavoritesSchema_CascadeOnUserDelete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "fav-cascade@example.com")
	menu := insertMenu(t, pool, "親子丼", domain.GenreJapanese, domain.DifficultyEasy)

	_, err := pool.Exec(ctx,
		`INSERT INTO favorites (id, user_id, menu_id) VALUES ($1, $2, $3)`,
		uuid.NewString(), u.ID.String(), menu.ID.String())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID.String())
	require.NoError(t, err)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM favorites WHERE user_id=$1`, u.ID.String()).Scan(&count))
	require.Zero(t, count, "user 削除でお気に入りも消えるべき")
}
