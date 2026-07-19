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
	lastUserID     domain.UserID
	lastMenuID     domain.MenuID
	lastMenuIDs    []domain.MenuID
	lastMode       domain.SearchMode
	lastLimit      int
	calls          int
	manyCalls      int
	listCalls      int
	deleteCalls    int
	deleteAllCalls int
	recentCalls    int
	lastHistoryID  domain.HistoryID
	entries        []domain.HistoryEntry
	recentIDs      []domain.MenuID
	err            error
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

func (s *fakeHistoryStore) List(_ context.Context, userID domain.UserID) ([]domain.HistoryEntry, error) {
	s.lastUserID = userID
	s.listCalls++
	return s.entries, s.err
}

func (s *fakeHistoryStore) RecentMenuIDs(_ context.Context, userID domain.UserID) ([]domain.MenuID, error) {
	s.lastUserID = userID
	s.recentCalls++
	return s.recentIDs, s.err
}

func (s *fakeHistoryStore) Delete(_ context.Context, userID domain.UserID, historyID domain.HistoryID) error {
	s.deleteCalls++
	s.lastUserID = userID
	s.lastHistoryID = historyID
	return s.err
}

func (s *fakeHistoryStore) DeleteAll(_ context.Context, userID domain.UserID) error {
	s.deleteAllCalls++
	s.lastUserID = userID
	return s.err
}

func TestHistoryService_Record_PassesLimit15(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	svc := service.NewHistoryService(store)

	userID := domain.NewUserID()
	menuID := domain.NewMenuID()
	require.NoError(t, svc.Record(context.Background(), userID.String(), menuID, domain.SearchModeSingle))

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

	err := svc.Record(context.Background(), domain.NewUserID().String(), domain.NewMenuID(), domain.SearchModeWeekly)
	require.Error(t, err)
}

func TestHistoryService_List_ParsesUserIDAndDelegates(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{entries: []domain.HistoryEntry{{}}}
	svc := service.NewHistoryService(store)

	userID := domain.NewUserID()
	entries, err := svc.List(context.Background(), userID.String())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, 1, store.listCalls)
	require.Equal(t, userID.String(), store.lastUserID.String())
}

func TestHistoryService_List_MalformedUserID(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	svc := service.NewHistoryService(store)

	_, err := svc.List(context.Background(), "not-a-uuid")
	require.ErrorIs(t, err, service.ErrUserNotFound)
	require.Zero(t, store.listCalls)
}

func TestHistoryService_RecentMenuIDs_Delegates(t *testing.T) {
	t.Parallel()

	want := []domain.MenuID{domain.NewMenuID(), domain.NewMenuID()}
	store := &fakeHistoryStore{recentIDs: want}
	svc := service.NewHistoryService(store)

	userID := domain.NewUserID()
	got, err := svc.RecentMenuIDs(context.Background(), userID.String())
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, 1, store.recentCalls)
	require.Equal(t, userID.String(), store.lastUserID.String())
}

func TestHistoryService_Record_MalformedUserID(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	svc := service.NewHistoryService(store)

	err := svc.Record(context.Background(), "not-a-uuid", domain.NewMenuID(), domain.SearchModeSingle)
	require.ErrorIs(t, err, service.ErrUserNotFound)
	require.Zero(t, store.calls)
}

func TestHistoryService_Delete_ParsesIDs(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	svc := service.NewHistoryService(store)

	userID := domain.NewUserID()
	histID := "018f0000-0000-7000-8000-000000000001"
	require.NoError(t, svc.Delete(context.Background(), userID.String(), histID))
	require.Equal(t, 1, store.deleteCalls)
	require.Equal(t, userID.String(), store.lastUserID.String())
	require.Equal(t, histID, store.lastHistoryID.String())
}

func TestHistoryService_Delete_InvalidHistoryID(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	svc := service.NewHistoryService(store)

	err := svc.Delete(context.Background(), domain.NewUserID().String(), "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrInvalidHistoryID)
	require.Zero(t, store.deleteCalls)
}

func TestHistoryService_DeleteAll_Delegates(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	svc := service.NewHistoryService(store)

	userID := domain.NewUserID()
	require.NoError(t, svc.DeleteAll(context.Background(), userID.String()))
	require.Equal(t, 1, store.deleteAllCalls)
	require.Equal(t, userID.String(), store.lastUserID.String())
}

func TestHistoryService_RecordMany_PassesAllAndLimit15(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	svc := service.NewHistoryService(store)

	userID := domain.NewUserID()
	menuIDs := []domain.MenuID{domain.NewMenuID(), domain.NewMenuID(), domain.NewMenuID()}
	require.NoError(t, svc.RecordMany(context.Background(), userID.String(), menuIDs, domain.SearchModeWeekly))

	require.Equal(t, 1, store.manyCalls)
	require.Len(t, store.lastMenuIDs, 3)
	require.Equal(t, domain.SearchModeWeekly, store.lastMode)
	require.Equal(t, 15, store.lastLimit)
}
