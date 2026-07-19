package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeHistoryStore は HistoryStore を代用し、渡された limit を記録する。
type fakeHistoryStore struct {
	lastUserID  domain.UserID
	lastMenuID  domain.MenuID
	lastMenuIDs []domain.MenuID
	lastMode    domain.SearchMode
	lastLimit   int
	calls       int
	manyCalls   int
	err         error
}

func (s *fakeHistoryStore) RecordWithLimit(_ context.Context, userID domain.UserID, menuID domain.MenuID, mode domain.SearchMode, limit int) error {
	s.calls++
	s.lastUserID = userID
	s.lastMenuID = menuID
	s.lastMode = mode
	s.lastLimit = limit
	return s.err
}

func (s *fakeHistoryStore) RecordManyWithLimit(_ context.Context, userID domain.UserID, menuIDs []domain.MenuID, mode domain.SearchMode, limit int) error {
	s.manyCalls++
	s.lastUserID = userID
	s.lastMenuIDs = menuIDs
	s.lastMode = mode
	s.lastLimit = limit
	return s.err
}

func TestHistoryService_Record_PassesLimit15(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	svc := service.NewHistoryService(store)

	userID := domain.NewUserID()
	menuID := domain.NewMenuID()
	require.NoError(t, svc.Record(context.Background(), userID, menuID, domain.SearchModeSingle))

	require.Equal(t, 1, store.calls)
	require.Equal(t, userID.String(), store.lastUserID.String())
	require.Equal(t, menuID.String(), store.lastMenuID.String())
	require.Equal(t, domain.SearchModeSingle, store.lastMode)
	// 保持件数(15)は業務ルールとして service が決める。
	require.Equal(t, 15, store.lastLimit)
	require.Equal(t, 15, service.HistoryLimit)
}

func TestHistoryService_Record_PropagatesError(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{err: errors.New("DB爆発")}
	svc := service.NewHistoryService(store)

	err := svc.Record(context.Background(), domain.NewUserID(), domain.NewMenuID(), domain.SearchModeWeekly)
	require.Error(t, err)
}

func TestHistoryService_RecordMany_PassesAllAndLimit15(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	svc := service.NewHistoryService(store)

	userID := domain.NewUserID()
	menuIDs := []domain.MenuID{domain.NewMenuID(), domain.NewMenuID(), domain.NewMenuID()}
	require.NoError(t, svc.RecordMany(context.Background(), userID, menuIDs, domain.SearchModeWeekly))

	require.Equal(t, 1, store.manyCalls)
	require.Len(t, store.lastMenuIDs, 3)
	require.Equal(t, domain.SearchModeWeekly, store.lastMode)
	require.Equal(t, 15, store.lastLimit)
}
