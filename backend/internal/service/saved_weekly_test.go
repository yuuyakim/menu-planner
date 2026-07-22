package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeSavedWeeklyStore は SavedWeeklyMenuStore を代用する。
type fakeSavedWeeklyStore struct {
	saveCalls  int
	lastUserID domain.UserID
	lastDays   []domain.DayMenu
	count      int
	countErr   error
	saveErr    error
}

func (s *fakeSavedWeeklyStore) Save(
	_ context.Context, userID domain.UserID, days []domain.DayMenu,
) (domain.SavedWeeklyMenuID, error) {
	s.saveCalls++
	s.lastUserID = userID
	s.lastDays = days
	if s.saveErr != nil {
		return domain.SavedWeeklyMenuID{}, s.saveErr
	}
	return domain.NewSavedWeeklyMenuID(), nil
}

func (s *fakeSavedWeeklyStore) List(
	_ context.Context, _ domain.UserID,
) ([]domain.SavedWeeklyMenu, error) {
	return nil, nil
}

func (s *fakeSavedWeeklyStore) Count(_ context.Context, _ domain.UserID) (int, error) {
	return s.count, s.countErr
}

func (s *fakeSavedWeeklyStore) Delete(
	_ context.Context, _ domain.UserID, _ domain.SavedWeeklyMenuID,
) error {
	return nil
}

// weekInput は7日分の指定を作る。
func weekInput() []service.SavedDayInput {
	in := make([]service.SavedDayInput, 0, domain.WeekLength)
	for day := 1; day <= domain.WeekLength; day++ {
		in = append(in, service.SavedDayInput{Day: day, MenuID: domain.NewMenuID().String()})
	}
	return in
}

func TestSavedWeeklyService_Save_7日分を渡す(t *testing.T) {
	t.Parallel()

	store := &fakeSavedWeeklyStore{}
	svc := service.NewSavedWeeklyMenuService(store)
	userID := domain.NewUserID()
	in := weekInput()

	id, err := svc.Save(context.Background(), userID.String(), in)
	require.NoError(t, err)
	require.False(t, id.IsZero())

	require.Equal(t, 1, store.saveCalls)
	assert.Equal(t, userID.String(), store.lastUserID.String())
	require.Len(t, store.lastDays, domain.WeekLength)
	for i, want := range in {
		assert.Equal(t, want.Day, store.lastDays[i].Day)
		assert.Equal(t, want.MenuID, store.lastDays[i].Menu.ID.String())
	}
}

func TestSavedWeeklyService_Save_日数が7でなければ拒否(t *testing.T) {
	t.Parallel()

	// 過不足のどちらも受け付けない。0件も含める。
	for _, n := range []int{0, 6, 8} {
		store := &fakeSavedWeeklyStore{}
		svc := service.NewSavedWeeklyMenuService(store)

		in := weekInput()
		if n < domain.WeekLength {
			in = in[:n]
		} else {
			in = append(in, service.SavedDayInput{Day: 1, MenuID: domain.NewMenuID().String()})
		}

		_, err := svc.Save(context.Background(), domain.NewUserID().String(), in)
		require.ErrorIs(t, err, service.ErrInvalidWeek, "%d日分は拒否されるべき", n)
		assert.Zero(t, store.saveCalls, "検証に落ちたら保存しないべき")
	}
}

func TestSavedWeeklyService_Save_日の重複は拒否(t *testing.T) {
	t.Parallel()

	store := &fakeSavedWeeklyStore{}
	svc := service.NewSavedWeeklyMenuService(store)

	in := weekInput()
	in[3].Day = in[2].Day // 7件のまま、日だけを重複させる

	_, err := svc.Save(context.Background(), domain.NewUserID().String(), in)
	require.ErrorIs(t, err, service.ErrInvalidDay)
	assert.Zero(t, store.saveCalls)
}

func TestSavedWeeklyService_Save_日の範囲外は拒否(t *testing.T) {
	t.Parallel()

	for _, day := range []int{0, 8, -1} {
		store := &fakeSavedWeeklyStore{}
		svc := service.NewSavedWeeklyMenuService(store)

		in := weekInput()
		in[0].Day = day

		_, err := svc.Save(context.Background(), domain.NewUserID().String(), in)
		require.ErrorIs(t, err, service.ErrInvalidDay, "%d日目は拒否されるべき", day)
		assert.Zero(t, store.saveCalls)
	}
}

func TestSavedWeeklyService_Save_壊れた献立IDは拒否(t *testing.T) {
	t.Parallel()

	store := &fakeSavedWeeklyStore{}
	svc := service.NewSavedWeeklyMenuService(store)

	in := weekInput()
	in[5].MenuID = "not-a-uuid"

	_, err := svc.Save(context.Background(), domain.NewUserID().String(), in)
	require.ErrorIs(t, err, domain.ErrInvalidMenuID)
	assert.Zero(t, store.saveCalls)
}

func TestSavedWeeklyService_Save_上限に達していたら断る(t *testing.T) {
	t.Parallel()

	// 上限ちょうどの状態で保存しようとすると 409 相当になる。
	store := &fakeSavedWeeklyStore{count: service.SavedWeeklyMenuLimit}
	svc := service.NewSavedWeeklyMenuService(store)

	_, err := svc.Save(context.Background(), domain.NewUserID().String(), weekInput())
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuLimitReached)
	assert.Zero(t, store.saveCalls, "上限に達していたら書かないべき")
}

func TestSavedWeeklyService_Save_上限の1つ手前なら通る(t *testing.T) {
	t.Parallel()

	// 境界の確認。ちょうど上限に達する保存は成功する。
	store := &fakeSavedWeeklyStore{count: service.SavedWeeklyMenuLimit - 1}
	svc := service.NewSavedWeeklyMenuService(store)

	_, err := svc.Save(context.Background(), domain.NewUserID().String(), weekInput())
	require.NoError(t, err)
	assert.Equal(t, 1, store.saveCalls)
}

func TestSavedWeeklyService_Save_壊れたユーザーIDは認証エラー(t *testing.T) {
	t.Parallel()

	store := &fakeSavedWeeklyStore{}
	svc := service.NewSavedWeeklyMenuService(store)

	_, err := svc.Save(context.Background(), "not-a-uuid", weekInput())
	require.ErrorIs(t, err, service.ErrUserNotFound)
	assert.Zero(t, store.saveCalls)
}

func TestSavedWeeklyService_Save_件数の取得に失敗したら保存しない(t *testing.T) {
	t.Parallel()

	// 上限を確かめられないまま書くと、際限なく溜まる余地を残す。
	sentinel := errors.New("DBが落ちている")
	store := &fakeSavedWeeklyStore{countErr: sentinel}
	svc := service.NewSavedWeeklyMenuService(store)

	_, err := svc.Save(context.Background(), domain.NewUserID().String(), weekInput())
	require.ErrorIs(t, err, sentinel)
	assert.Zero(t, store.saveCalls)
}
