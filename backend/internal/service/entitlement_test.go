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

// fixedNow はテスト用の固定時刻。期限判定を時計に依存させない。
var fixedNow = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

func newEntitlementSvc(store service.SubscriptionStore) *service.EntitlementService {
	return service.NewEntitlementService(store, func() time.Time { return fixedNow })
}

func TestEntitlementService_未認証はfree(t *testing.T) {
	svc := newEntitlementSvc(newFakeSubscriptionStore())

	// 未認証でも献立検索は使えるため、userID が空でもエラーにしない。
	ent, err := svc.For(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, domain.PlanFree, ent.Plan())
}

func TestEntitlementService_加入が無ければfree(t *testing.T) {
	svc := newEntitlementSvc(newFakeSubscriptionStore())

	u := domain.NewUserID()
	ent, err := svc.For(context.Background(), u.String())
	require.NoError(t, err)
	require.Equal(t, domain.PlanFree, ent.Plan())
}

func TestEntitlementService_有効な加入はpremium(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	require.NoError(t, store.Upsert(context.Background(), domain.Subscription{
		UserID:           u,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: fixedNow.Add(time.Hour),
		Provider:         domain.ProviderManual,
	}))

	ent, err := newEntitlementSvc(store).For(context.Background(), u.String())
	require.NoError(t, err)
	require.Equal(t, domain.PlanPremium, ent.Plan())
	require.Equal(t, 50, ent.SavedWeeklyMenuLimit())
}

func TestEntitlementService_期限切れはfree(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	require.NoError(t, store.Upsert(context.Background(), domain.Subscription{
		UserID:           u,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: fixedNow.Add(-time.Second),
		Provider:         domain.ProviderManual,
	}))

	ent, err := newEntitlementSvc(store).For(context.Background(), u.String())
	require.NoError(t, err)
	require.Equal(t, domain.PlanFree, ent.Plan(),
		"期限切れは参照時に free に落ちる。DBは書き換えない")

	// DBを書き換えていないことを確かめる。バッチではなく参照時計算であることの担保。
	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionActive, sub.Status,
		"参照は加入の状態を書き換えてはいけない")
}

func TestEntitlementService_canceledはfree(t *testing.T) {
	// past_due は trialing / past_due 猶予のテスト
	// (TestEntitlementService_TrialingとPastDue猶予はpremium) でカバーする。
	// 猶予期間内は premium になるため、ここでは canceled のみを見る。
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	require.NoError(t, store.Upsert(context.Background(), domain.Subscription{
		UserID:           u,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionCanceled,
		CurrentPeriodEnd: fixedNow.Add(time.Hour),
		Provider:         domain.ProviderManual,
	}))

	ent, err := newEntitlementSvc(store).For(context.Background(), u.String())
	require.NoError(t, err)
	require.Equal(t, domain.PlanFree, ent.Plan(), "canceled は free であるべき")
}

func TestEntitlementService_TrialingとPastDue猶予はpremium(t *testing.T) {
	cases := []struct {
		name string
		sub  domain.Subscription
		want domain.Plan
	}{
		{
			"trialing 期間内は premium",
			domain.Subscription{Plan: domain.PlanPremium, Status: domain.SubscriptionTrialing, CurrentPeriodEnd: fixedNow.Add(48 * time.Hour)},
			domain.PlanPremium,
		},
		{
			"past_due 猶予内は premium",
			domain.Subscription{Plan: domain.PlanPremium, Status: domain.SubscriptionPastDue, CurrentPeriodEnd: fixedNow.Add(-3 * 24 * time.Hour)},
			domain.PlanPremium,
		},
		{
			"past_due 猶予超過は free",
			domain.Subscription{Plan: domain.PlanPremium, Status: domain.SubscriptionPastDue, CurrentPeriodEnd: fixedNow.Add(-8 * 24 * time.Hour)},
			domain.PlanFree,
		},
		{
			"canceled は free",
			domain.Subscription{Plan: domain.PlanPremium, Status: domain.SubscriptionCanceled, CurrentPeriodEnd: fixedNow.Add(48 * time.Hour)},
			domain.PlanFree,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeSubscriptionStore()
			u := domain.NewUserID()
			sub := tc.sub
			sub.UserID = u
			sub.Provider = domain.ProviderManual
			require.NoError(t, store.Upsert(context.Background(), sub))

			ent, err := newEntitlementSvc(store).For(context.Background(), u.String())
			require.NoError(t, err)
			require.Equal(t, tc.want, ent.Plan())
		})
	}
}

func TestEntitlementService_壊れたIDはfree(t *testing.T) {
	// トークンが壊れている場合でも、エンタイトルメントの判定は締め出しではなく
	// free への縮退で応じる。認証の失敗は認証ミドルウェアの仕事。
	ent, err := newEntitlementSvc(newFakeSubscriptionStore()).
		For(context.Background(), "not-a-uuid")
	require.NoError(t, err)
	require.Equal(t, domain.PlanFree, ent.Plan())
}

func TestEntitlementService_保存の障害はエラーにする(t *testing.T) {
	store := newFakeSubscriptionStore()
	boom := errors.New("DBが落ちている")
	store.findErr = boom

	// 「加入が無い」と「引けなかった」は別。後者を free に丸めると、
	// 障害中に課金済みの利用者が黙って free に落ちる。
	_, err := newEntitlementSvc(store).For(context.Background(), domain.NewUserID().String())
	require.ErrorIs(t, err, boom)
}
