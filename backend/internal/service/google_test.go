package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func TestUpsertGoogleUser_PassesFieldsToRepo(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	user, err := svc.UpsertGoogleUser(context.Background(), service.GoogleUser{
		Sub:         "google-sub-1",
		Email:       "Taro@Example.com",
		DisplayName: "太郎",
	})
	require.NoError(t, err)
	require.Equal(t, "google-sub-1", repo.lastGoogleSub)
	// メールは正規化して渡す。
	require.Equal(t, "taro@example.com", repo.lastGoogleEmail)
	require.Equal(t, "太郎", repo.lastGoogleName)
	require.Equal(t, "taro@example.com", user.Email.String())
}

func TestUpsertGoogleUser_DerivesNameWhenEmpty(t *testing.T) {
	t.Parallel()

	// Google が名前を返さない場合はメールのローカル部から表示名を作る。
	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	_, err := svc.UpsertGoogleUser(context.Background(), service.GoogleUser{
		Sub:         "google-sub-2",
		Email:       "hanako@example.com",
		DisplayName: "",
	})
	require.NoError(t, err)
	require.Equal(t, "hanako", repo.lastGoogleName)
}

func TestUpsertGoogleUser_InvalidEmail(t *testing.T) {
	t.Parallel()

	repo := newFakeUserRepository()
	svc := newAuthService(repo)

	_, err := svc.UpsertGoogleUser(context.Background(), service.GoogleUser{
		Sub:         "google-sub-3",
		Email:       "not-an-email",
		DisplayName: "誰か",
	})
	require.ErrorIs(t, err, domain.ErrInvalidEmail)
	require.Zero(t, repo.googleCalls, "不正メールは repository に渡さない")
}
