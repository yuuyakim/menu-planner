package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
)

func TestHashPassword_HashAndVerify(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("correct horse")
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	// 平文がそのまま保存されていないこと。
	require.NotContains(t, hash, "correct horse")

	// cost が spec.md の 12 であること。
	cost, err := bcrypt.Cost([]byte(hash))
	require.NoError(t, err)
	require.Equal(t, 12, cost)

	// 正しいパスワードで検証が通ること。
	require.NoError(t, auth.VerifyPassword(hash, "correct horse"))
}

func TestHashPassword_DifferentEachTime(t *testing.T) {
	t.Parallel()

	// bcrypt はソルトを含むため、同じパスワードでも毎回異なるハッシュになる。
	h1, err := auth.HashPassword("same password")
	require.NoError(t, err)
	h2, err := auth.HashPassword("same password")
	require.NoError(t, err)

	require.NotEqual(t, h1, h2, "同じパスワードでもハッシュは毎回異なるべき")

	// どちらのハッシュでも検証は通る。
	require.NoError(t, auth.VerifyPassword(h1, "same password"))
	require.NoError(t, auth.VerifyPassword(h2, "same password"))
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("right password")
	require.NoError(t, err)

	err = auth.VerifyPassword(hash, "wrong password")
	require.ErrorIs(t, err, auth.ErrPasswordMismatch)
}

func TestHashPassword_TooShort(t *testing.T) {
	t.Parallel()

	// 7文字は拒否、8文字は通る（境界値）。
	_, err := auth.HashPassword("1234567")
	require.ErrorIs(t, err, auth.ErrPasswordTooShort)

	_, err = auth.HashPassword("12345678")
	require.NoError(t, err)

	// 長さは「文字数」で数える。全角8文字はバイト数では8を超えるが通る。
	_, err = auth.HashPassword("パスワード八文字")
	require.NoError(t, err)

	// 全角7文字は拒否。
	_, err = auth.HashPassword("七文字未満だよ")
	require.ErrorIs(t, err, auth.ErrPasswordTooShort)
}

func TestHashPassword_TooLong(t *testing.T) {
	t.Parallel()

	// bcrypt は72バイトを超える入力を扱えない。黙って切り詰めると別々の
	// パスワードが同じハッシュになりうるため、明示的に拒否する。
	exactly72 := strings.Repeat("a", 72)
	_, err := auth.HashPassword(exactly72)
	require.NoError(t, err, "72バイトちょうどは通るべき")

	over72 := strings.Repeat("a", 73)
	_, err = auth.HashPassword(over72)
	require.ErrorIs(t, err, auth.ErrPasswordTooLong)
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	t.Parallel()

	// bcrypt 形式でない文字列は検証に使えない。ミスマッチとは区別する。
	err := auth.VerifyPassword("not-a-bcrypt-hash", "whatever")
	require.Error(t, err)
	require.False(t, errors.Is(err, auth.ErrPasswordMismatch),
		"壊れたハッシュは不一致ではなくエラーとして扱うべき")
}
