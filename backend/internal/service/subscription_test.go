package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func newSubscriptionSvc(store service.SubscriptionStore) *service.SubscriptionService {
	return service.NewSubscriptionService(store, func() time.Time { return fixedNow })
}

func TestSubscriptionService_Grant_期限は現在から起算する(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()

	require.NoError(t, newSubscriptionSvc(store).Grant(context.Background(), u, 1))

	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, domain.PlanPremium, sub.Plan)
	require.Equal(t, domain.SubscriptionActive, sub.Status)
	require.Equal(t, domain.ProviderManual, sub.Provider)
	require.Equal(t, fixedNow.AddDate(0, 1, 0), sub.CurrentPeriodEnd)
	require.False(t, sub.CancelAtPeriodEnd)
	require.Empty(t, sub.ProviderSubscriptionID)
}

func TestSubscriptionService_Grant_複数月(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()

	require.NoError(t, newSubscriptionSvc(store).Grant(context.Background(), u, 12))

	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, fixedNow.AddDate(0, 12, 0), sub.CurrentPeriodEnd)
}

func TestSubscriptionService_Grant_0以下の月数は拒否する(t *testing.T) {
	svc := newSubscriptionSvc(newFakeSubscriptionStore())

	for _, months := range []int{0, -1} {
		err := svc.Grant(context.Background(), domain.NewUserID(), months)
		require.ErrorIs(t, err, service.ErrInvalidGrantMonths, "%d は拒否されるべき", months)
	}
}

func TestSubscriptionService_Grant_期限切れの加入を再付与できる(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	require.NoError(t, store.Upsert(context.Background(), domain.Subscription{
		UserID:           u,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionCanceled,
		CurrentPeriodEnd: fixedNow.Add(-time.Hour),
		Provider:         domain.ProviderManual,
	}))

	require.NoError(t, newSubscriptionSvc(store).Grant(context.Background(), u, 1))

	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionActive, sub.Status, "再付与で active に戻るべき")
	require.Equal(t, fixedNow.AddDate(0, 1, 0), sub.CurrentPeriodEnd)
}

func TestSubscriptionService_Grant_有効な加入は期末から積み増す(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	end := fixedNow.AddDate(0, 10, 0)
	require.NoError(t, store.Upsert(context.Background(), domain.Subscription{
		UserID:           u,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: end,
		Provider:         domain.ProviderManual,
	}))

	require.NoError(t, newSubscriptionSvc(store).Grant(context.Background(), u, 1))

	// 現在時刻から起算すると、延長のつもりの1か月付与が残り9か月を消してしまう。
	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, end.AddDate(0, 1, 0), sub.CurrentPeriodEnd)
}

func TestSubscriptionService_Grant_月末は繰り上がらず月末に丸める(t *testing.T) {
	// 1/31 の1か月後を time.AddDate に任せると 3/3 になり、
	// 「1か月」が32〜34日になる。月末に付与した利用者だけが得をする。
	jan31 := time.Date(2026, time.January, 31, 12, 0, 0, 0, time.UTC)
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	svc := service.NewSubscriptionService(store, func() time.Time { return jan31 })

	require.NoError(t, svc.Grant(context.Background(), u, 1))

	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC), sub.CurrentPeriodEnd)
}

func TestSubscriptionService_Grant_同じ日がある月はそのまま(t *testing.T) {
	// 丸めが効きすぎて通常の付与まで前倒しにならないことの担保。
	mar15 := time.Date(2026, time.March, 15, 9, 30, 0, 0, time.UTC)
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	svc := service.NewSubscriptionService(store, func() time.Time { return mar15 })

	require.NoError(t, svc.Grant(context.Background(), u, 1))

	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, time.April, 15, 9, 30, 0, 0, time.UTC), sub.CurrentPeriodEnd)
}

func TestSubscriptionService_Grant_加入を引けなければ付与しない(t *testing.T) {
	store := newFakeSubscriptionStore()
	store.findErr = errors.New("接続に失敗しました")

	err := newSubscriptionSvc(store).Grant(context.Background(), domain.NewUserID(), 1)

	// 引けないまま書くと、残期間があったかどうか分からないまま上書きしてしまう。
	require.Error(t, err)
	require.Empty(t, store.subs, "取得に失敗したら保存しない")
}

func TestSubscriptionService_Revoke_行を消さずcanceledにする(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	svc := newSubscriptionSvc(store)
	require.NoError(t, svc.Grant(context.Background(), u, 1))

	require.NoError(t, svc.Revoke(context.Background(), u))

	// 行を消すと「いつ解約したか」の記録が失われる。
	// 後で「解約したのに課金された」と申し立てられたときの反証材料になる。
	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionCanceled, sub.Status)
}

func TestSubscriptionService_Revoke_加入が無ければ何もしない(t *testing.T) {
	store := newFakeSubscriptionStore()

	// 冪等。既に free の利用者に取消をかけても失敗させない。
	require.NoError(t, newSubscriptionSvc(store).Revoke(context.Background(), domain.NewUserID()))
}
