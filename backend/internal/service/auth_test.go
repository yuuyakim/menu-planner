package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// サービステストは実物の auth.Hasher を注入する。長さ検証まで含めて
// 本物の振る舞いで確かめたいため（bcrypt は数十msで許容範囲）。
func newAuthService(repo service.UserRepository) *service.AuthService {
	return service.NewAuthService(repo, auth.Hasher{})
}

func TestSignUp_CreatesUserAndHashesPassword(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	user, err := svc.SignUp(context.Background(), "taro@example.com", "supersecret")
	require.NoError(t, err)

	// user が返り、repository に保存されている。
	require.Equal(t, 1, repo.calls)
	require.Equal(t, "taro@example.com", user.Email.String())
	require.Equal(t, "taro", user.DisplayName)
	require.False(t, user.ID.IsZero())

	// 保存されたのは平文ではなくハッシュ。かつ検証が通る。
	require.NotEmpty(t, repo.savedHash)
	require.NotEqual(t, "supersecret", repo.savedHash)
	require.NoError(t, auth.VerifyPassword(repo.savedHash, "supersecret"))
}

func TestSignUp_NormalizesEmail(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	user, err := svc.SignUp(context.Background(), "  Taro@Example.COM ", "supersecret")
	require.NoError(t, err)
	require.Equal(t, "taro@example.com", user.Email.String())
}

func TestSignUp_DuplicateEmail(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	repo.takenEmails["dup@example.com"] = true
	svc := newAuthService(repo)

	_, err := svc.SignUp(context.Background(), "dup@example.com", "supersecret")
	require.ErrorIs(t, err, service.ErrEmailTaken)
}

func TestSignUp_InvalidEmail(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	_, err := svc.SignUp(context.Background(), "not-an-email", "supersecret")
	require.ErrorIs(t, err, domain.ErrInvalidEmail)
	// 検証で弾かれ、repository には触れない。
	require.Zero(t, repo.calls)
}

func TestSignUp_ShortPassword(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	_, err := svc.SignUp(context.Background(), "taro@example.com", "1234567")
	require.ErrorIs(t, err, auth.ErrPasswordTooShort)
	require.Zero(t, repo.calls, "パスワード検証で弾かれ repository には触れないべき")
}

func TestSignUp_RepositoryErrorPropagates(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	repo.err = errors.New("DB爆発")
	svc := newAuthService(repo)

	_, err := svc.SignUp(context.Background(), "taro@example.com", "supersecret")
	require.Error(t, err)
	// ErrEmailTaken とは区別できる（別のDB障害）。
	require.False(t, errors.Is(err, service.ErrEmailTaken))
}
