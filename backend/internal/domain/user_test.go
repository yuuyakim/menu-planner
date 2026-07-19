package domain_test

import (
	"strings"
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

func TestNewUser_DerivesDisplayNameFromEmail(t *testing.T) {
	t.Parallel()

	// サインアップは表示名を受け取らない（spec.md 5.2）。DBは NOT NULL なので
	// メールのローカル部から導出する。
	email, err := domain.NewEmail("taro@example.com")
	require.NoError(t, err)

	u, err := domain.NewUser(email)
	require.NoError(t, err)

	require.False(t, u.ID.IsZero(), "IDが採番されるべき")
	require.Equal(t, email, u.Email)
	require.Equal(t, "taro", u.DisplayName)
}

func TestNewUser_UniqueID(t *testing.T) {
	t.Parallel()

	email, err := domain.NewEmail("a@example.com")
	require.NoError(t, err)

	u1, err := domain.NewUser(email)
	require.NoError(t, err)
	u2, err := domain.NewUser(email)
	require.NoError(t, err)

	require.NotEqual(t, u1.ID.String(), u2.ID.String(), "IDは毎回異なるべき")
}

func TestParseUserID_RejectsZeroAndGarbage(t *testing.T) {
	t.Parallel()

	_, err := domain.ParseUserID("not-a-uuid")
	require.ErrorIs(t, err, domain.ErrInvalidUserID)

	_, err = domain.ParseUserID("00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, domain.ErrInvalidUserID)

	id := domain.NewUserID()
	parsed, err := domain.ParseUserID(id.String())
	require.NoError(t, err)
	require.Equal(t, id.String(), parsed.String())
}

func TestNewUser_LongLocalPartDisplayNameIsTrimmedToLimit(t *testing.T) {
	t.Parallel()

	// 表示名の上限を超えるローカル部は切り詰める。DBの制約や画面表示のため。
	long := strings.Repeat("a", 200)
	email, err := domain.NewEmail(long + "@example.com")
	require.NoError(t, err)

	u, err := domain.NewUser(email)
	require.NoError(t, err)
	require.LessOrEqual(t, len([]rune(u.DisplayName)), 50)
	require.NotEmpty(t, u.DisplayName)
}
