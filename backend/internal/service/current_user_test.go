package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func TestCurrentUser_ReturnsUser(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	user := seedCredential(t, repo, "taro@example.com", "supersecret")
	svc := newAuthService(repo)

	got, err := svc.CurrentUser(context.Background(), user.ID.String())
	require.NoError(t, err)
	require.Equal(t, user.ID.String(), got.ID.String())
	require.Equal(t, "taro@example.com", got.Email.String())
}

func TestCurrentUser_UnknownID(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	_, err := svc.CurrentUser(context.Background(), domain.NewUserID().String())
	require.ErrorIs(t, err, service.ErrUserNotFound)
}

func TestCurrentUser_MalformedID(t *testing.T) {
	t.Parallel()

	// トークンの sub は自分が署名した UUID のはずだが、壊れていても
	// パニックせずセッション不正（ErrUserNotFound）として扱う。
	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	_, err := svc.CurrentUser(context.Background(), "not-a-uuid")
	require.ErrorIs(t, err, service.ErrUserNotFound)
}
