package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestNewEmail_Valid(t *testing.T) {
	t.Parallel()

	e, err := domain.NewEmail("user@example.com")
	require.NoError(t, err)
	require.Equal(t, "user@example.com", e.String())
}

func TestNewEmail_NormalizesCaseAndSpaces(t *testing.T) {
	t.Parallel()

	// 前後の空白を除き、小文字に揃える。大小違いで同じ人が二重登録
	// できてしまわないようにするため。
	e, err := domain.NewEmail("  User@Example.COM  ")
	require.NoError(t, err)
	require.Equal(t, "user@example.com", e.String())
}

func TestNewEmail_Invalid(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"空":            "",
		"アットマーク無し":     "userexample.com",
		"ドメインにドット無し":   "user@example",
		"ローカル部無し":      "@example.com",
		"表示名つきは受け付けない": "User <user@example.com>",
		"スペースを含む":      "user @example.com",
	}
	for name, raw := range cases {
		raw := raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewEmail(raw)
			require.ErrorIs(t, err, domain.ErrInvalidEmail, "入力=%q", raw)
		})
	}
}
