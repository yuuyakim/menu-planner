package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// seedCredential は fake repo に、指定メール・パスワードのパスワード認証を仕込む。
func seedCredential(t *testing.T, repo *fakeUserRepository, email, password string) domain.User {
	t.Helper()
	e, err := domain.NewEmail(email)
	require.NoError(t, err)
	u, err := domain.NewUser(e)
	require.NoError(t, err)

	hash, err := auth.HashPassword(password)
	require.NoError(t, err)

	repo.credentials[e.String()] = service.PasswordCredential{User: u, PasswordHash: hash}
	return u
}

func TestLogin_Success(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	want := seedCredential(t, repo, "taro@example.com", "supersecret")
	svc := newAuthService(repo)

	got, err := svc.Login(context.Background(), "taro@example.com", "supersecret")
	require.NoError(t, err)
	require.Equal(t, want.ID.String(), got.ID.String())
	require.Equal(t, "taro@example.com", got.Email.String())
}

func TestLogin_NormalizesEmail(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	seedCredential(t, repo, "taro@example.com", "supersecret")
	svc := newAuthService(repo)

	// 大文字・空白つきでも同じユーザーとして照合される。
	_, err := svc.Login(context.Background(), "  TARO@Example.COM ", "supersecret")
	require.NoError(t, err)
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	seedCredential(t, repo, "taro@example.com", "supersecret")
	svc := newAuthService(repo)

	_, err := svc.Login(context.Background(), "taro@example.com", "wrongpass")
	require.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestLogin_UnknownEmail_SameErrorAsWrongPassword(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	seedCredential(t, repo, "taro@example.com", "supersecret")
	svc := newAuthService(repo)

	// 存在しないメールも、パスワード違いと同じ ErrInvalidCredentials。
	// これによりエラーの差からアカウントの有無を推測できない。
	_, errUnknown := svc.Login(context.Background(), "nobody@example.com", "whatever12")
	require.ErrorIs(t, errUnknown, service.ErrInvalidCredentials)

	_, errWrong := svc.Login(context.Background(), "taro@example.com", "wrongpass")
	require.ErrorIs(t, errWrong, service.ErrInvalidCredentials)
}

func TestLogin_GoogleOnlyUser_Rejected(t *testing.T) {
	t.Parallel()

	// Google 認証のみのユーザーはパスワード認証を持たない。repo は
	// ErrCredentialNotFound を返し、service はそれを 401 相当に丸める。
	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	_, err := svc.Login(context.Background(), "google-only@example.com", "whatever12")
	require.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestLogin_InvalidEmailFormat(t *testing.T) {
	t.Parallel()

	// 形式が不正なメールは照合以前の問題。存在推測には使えないため
	// 400 相当（ErrInvalidEmail）でよい。
	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	_, err := svc.Login(context.Background(), "not-an-email", "whatever12")
	require.ErrorIs(t, err, domain.ErrInvalidEmail)
}
