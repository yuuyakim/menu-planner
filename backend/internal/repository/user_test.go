package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// newUser はテスト用のユーザーをメールから組み立てる。
func newUser(t *testing.T, email string) domain.User {
	t.Helper()
	e, err := domain.NewEmail(email)
	require.NoError(t, err)
	u, err := domain.NewUser(e)
	require.NoError(t, err)
	return u
}

func TestUserRepository_CreateWithPassword_PersistsBoth(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	u := newUser(t, "taro@example.com")
	require.NoError(t, repo.CreateWithPassword(ctx, u, "bcrypthash"))

	// users が1件。
	var email, displayName string
	err := pool.QueryRow(ctx,
		`SELECT email, display_name FROM users WHERE id = $1`, u.ID.String()).
		Scan(&email, &displayName)
	require.NoError(t, err)
	require.Equal(t, "taro@example.com", email)
	require.Equal(t, "taro", displayName)

	// auth_identity が password 方式で1件、ハッシュ付き。
	var provider, hash string
	err = pool.QueryRow(ctx,
		`SELECT provider, password_hash FROM auth_identities WHERE user_id = $1`, u.ID.String()).
		Scan(&provider, &hash)
	require.NoError(t, err)
	require.Equal(t, "password", provider)
	require.Equal(t, "bcrypthash", hash)
}

func TestUserRepository_CreateWithPassword_DuplicateEmail(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	u1 := newUser(t, "dup@example.com")
	require.NoError(t, repo.CreateWithPassword(ctx, u1, "hash1"))

	// 同じメールの別ユーザーは ErrEmailTaken。
	u2 := newUser(t, "dup@example.com")
	err := repo.CreateWithPassword(ctx, u2, "hash2")
	require.ErrorIs(t, err, service.ErrEmailTaken)

	// 2人目のユーザーもロールバックで残っていない（対で作るため）。
	var count int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE id = $1`, u2.ID.String()).Scan(&count)
	require.NoError(t, err)
	require.Zero(t, count, "重複時はユーザーも作られないべき")
}

func TestUserRepository_CreateWithPassword_Atomicity(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// 成功時は users と auth_identities がちょうど対で1件ずつ。
	u := newUser(t, "atomic@example.com")
	require.NoError(t, repo.CreateWithPassword(ctx, u, "hash"))

	var users, identities int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM auth_identities`).Scan(&identities))
	require.Equal(t, 1, users)
	require.Equal(t, 1, identities)
}
