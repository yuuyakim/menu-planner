package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeFavoriteStore は FavoriteStore を代用する。
type fakeFavoriteStore struct {
	lastUserID domain.UserID
	lastMenuID domain.MenuID
	addCalls   int
	err        error
}

func (s *fakeFavoriteStore) Add(_ context.Context, userID domain.UserID, menuID domain.MenuID) error {
	s.addCalls++
	s.lastUserID = userID
	s.lastMenuID = menuID
	return s.err
}

func TestFavoriteService_Add_Delegates(t *testing.T) {
	t.Parallel()

	store := &fakeFavoriteStore{}
	svc := service.NewFavoriteService(store)

	userID := domain.NewUserID()
	menuID := domain.NewMenuID()
	require.NoError(t, svc.Add(context.Background(), userID.String(), menuID.String()))

	require.Equal(t, 1, store.addCalls)
	require.Equal(t, userID.String(), store.lastUserID.String())
	require.Equal(t, menuID.String(), store.lastMenuID.String())
}

func TestFavoriteService_Add_MalformedUserID(t *testing.T) {
	t.Parallel()

	store := &fakeFavoriteStore{}
	svc := service.NewFavoriteService(store)

	err := svc.Add(context.Background(), "not-a-uuid", domain.NewMenuID().String())
	require.ErrorIs(t, err, service.ErrUserNotFound)
	require.Zero(t, store.addCalls)
}

func TestFavoriteService_Add_MalformedMenuID(t *testing.T) {
	t.Parallel()

	store := &fakeFavoriteStore{}
	svc := service.NewFavoriteService(store)

	// 献立IDはリクエストボディ由来なので、検証できずに 400 相当のエラーになる。
	err := svc.Add(context.Background(), domain.NewUserID().String(), "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrInvalidMenuID)
	require.Zero(t, store.addCalls)
}

func TestFavoriteService_Add_PropagatesStoreError(t *testing.T) {
	t.Parallel()

	// 重複は store（＝DBの一意制約）が判定し、service はそのまま通す。
	store := &fakeFavoriteStore{err: service.ErrFavoriteExists}
	svc := service.NewFavoriteService(store)

	err := svc.Add(context.Background(), domain.NewUserID().String(), domain.NewMenuID().String())
	require.ErrorIs(t, err, service.ErrFavoriteExists)
}
