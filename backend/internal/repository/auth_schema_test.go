package repository_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// auth_identities のスキーマ（CHECK / UNIQUE / CASCADE）を、リポジトリを介さず
// 生SQLで直接確かめる。制約はDBが守るものなので、DBに対して検証する。

// insertUser はテスト用のユーザーを1件作り、そのIDを返す。
func insertUser(t *testing.T, pool *pgxpool.Pool, id, email string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, display_name) VALUES ($1, $2, $3)`,
		id, email, "テスト太郎")
	require.NoError(t, err)
}

func TestAuthSchema_PasswordProviderRequiresHash(t *testing.T) {
	pool := newTestPool(t)
	insertUser(t, pool, "11111111-1111-1111-1111-111111111111", "pw@example.com")

	// provider=password で password_hash が NULL は CHECK 違反。
	_, err := pool.Exec(context.Background(),
		`INSERT INTO auth_identities (id, user_id, provider, password_hash)
		 VALUES ($1, $2, 'password', NULL)`,
		"aaaaaaaa-1111-1111-1111-111111111111",
		"11111111-1111-1111-1111-111111111111")
	require.Error(t, err, "password なのに hash が無ければ拒否されるべき")

	// hash があれば通る。
	_, err = pool.Exec(context.Background(),
		`INSERT INTO auth_identities (id, user_id, provider, password_hash)
		 VALUES ($1, $2, 'password', 'bcrypthash')`,
		"aaaaaaaa-2222-2222-2222-222222222222",
		"11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
}

func TestAuthSchema_GoogleProviderRequiresUID(t *testing.T) {
	pool := newTestPool(t)
	insertUser(t, pool, "22222222-2222-2222-2222-222222222222", "g@example.com")

	// provider=google で provider_uid が NULL は CHECK 違反。
	_, err := pool.Exec(context.Background(),
		`INSERT INTO auth_identities (id, user_id, provider, provider_uid)
		 VALUES ($1, $2, 'google', NULL)`,
		"bbbbbbbb-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222")
	require.Error(t, err, "google なのに uid が無ければ拒否されるべき")

	// uid があれば通る。
	_, err = pool.Exec(context.Background(),
		`INSERT INTO auth_identities (id, user_id, provider, provider_uid)
		 VALUES ($1, $2, 'google', 'google-sub-123')`,
		"bbbbbbbb-2222-2222-2222-222222222222",
		"22222222-2222-2222-2222-222222222222")
	require.NoError(t, err)
}

func TestAuthSchema_UniqueProviderUID(t *testing.T) {
	pool := newTestPool(t)
	insertUser(t, pool, "33333333-3333-3333-3333-333333333333", "u1@example.com")
	insertUser(t, pool, "44444444-4444-4444-4444-444444444444", "u2@example.com")

	_, err := pool.Exec(context.Background(),
		`INSERT INTO auth_identities (id, user_id, provider, provider_uid)
		 VALUES ($1, $2, 'google', 'same-sub')`,
		"cccccccc-1111-1111-1111-111111111111",
		"33333333-3333-3333-3333-333333333333")
	require.NoError(t, err)

	// 同じ (provider, provider_uid) は別ユーザーでも拒否される。
	_, err = pool.Exec(context.Background(),
		`INSERT INTO auth_identities (id, user_id, provider, provider_uid)
		 VALUES ($1, $2, 'google', 'same-sub')`,
		"cccccccc-2222-2222-2222-222222222222",
		"44444444-4444-4444-4444-444444444444")
	require.Error(t, err, "(provider, provider_uid) の重複は拒否されるべき")
}

func TestAuthSchema_UniqueEmail(t *testing.T) {
	pool := newTestPool(t)
	insertUser(t, pool, "55555555-5555-5555-5555-555555555555", "dup@example.com")

	// 同じメールの2人目は拒否される。
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, display_name) VALUES ($1, $2, $3)`,
		"66666666-6666-6666-6666-666666666666", "dup@example.com", "別人")
	require.Error(t, err, "メールの重複は拒否されるべき")
}

func TestAuthSchema_DeleteUserCascadesIdentities(t *testing.T) {
	pool := newTestPool(t)
	insertUser(t, pool, "77777777-7777-7777-7777-777777777777", "cascade@example.com")
	_, err := pool.Exec(context.Background(),
		`INSERT INTO auth_identities (id, user_id, provider, password_hash)
		 VALUES ($1, $2, 'password', 'hash')`,
		"dddddddd-1111-1111-1111-111111111111",
		"77777777-7777-7777-7777-777777777777")
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(),
		`DELETE FROM users WHERE id = $1`,
		"77777777-7777-7777-7777-777777777777")
	require.NoError(t, err)

	// user を消すと identity も消える。
	var count int
	err = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM auth_identities WHERE user_id = $1`,
		"77777777-7777-7777-7777-777777777777").Scan(&count)
	require.NoError(t, err)
	require.Zero(t, count, "user 削除で identity も消えるべき")
}
