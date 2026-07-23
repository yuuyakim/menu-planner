package service_test

import (
	"context"
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
