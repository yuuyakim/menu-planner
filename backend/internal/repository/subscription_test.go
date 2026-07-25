package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func TestSubscriptionRepository_無ければErrSubscriptionNotFound(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewSubscriptionRepository(pool)

	u := createUser(t, pool, "sub-repo-none@example.com")

	_, err := repo.Find(context.Background(), u.ID)
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound,
		"行が無いのは障害ではなく free を意味する通常の結果")
}

func TestSubscriptionRepository_保存して取り出せる(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSubscriptionRepository(pool)

	u := createUser(t, pool, "sub-repo-roundtrip@example.com")
	end := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	want := domain.Subscription{
		UserID:           u.ID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: end,
		Provider:         domain.ProviderManual,
	}
	require.NoError(t, repo.Upsert(ctx, want))

	got, err := repo.Find(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u.ID, got.UserID)
	require.Equal(t, domain.PlanPremium, got.Plan)
	require.Equal(t, domain.SubscriptionActive, got.Status)
	require.WithinDuration(t, end, got.CurrentPeriodEnd, time.Second)
	require.False(t, got.CancelAtPeriodEnd)
	require.Equal(t, domain.ProviderManual, got.Provider)
	require.Empty(t, got.ProviderSubscriptionID, "手動付与は決済IDを持たない")
}

// 未知の値でエラーにすると EntitlementService がそれを返し、/auth/me が 500 になって
// ログイン済みの利用者がアプリを一切使えなくなる。000010 が CHECK 制約を張らないのも
// 「決済事業者ごとに増える値を DDL の変更なしに受けられる」ためなので、
// 読み出し側が弾いてしまうとその狙いが成立しない。
func TestSubscriptionRepository_未知の値でも締め出さない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSubscriptionRepository(pool)

	for _, tt := range []struct {
		name         string
		plan, status string
	}{
		{"未知のプラン", "pro", "active"},
		{"未知の状態", "premium", "trialing"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u := createUser(t, pool, "sub-repo-unknown-"+tt.plan+tt.status+"@example.com")
			_, err := pool.Exec(ctx,
				`INSERT INTO subscriptions (user_id, plan, status, current_period_end, provider)
				 VALUES ($1, $2, $3, $4, $5)`,
				u.ID.String(), tt.plan, tt.status, time.Now().Add(24*time.Hour), domain.ProviderManual)
			require.NoError(t, err)

			got, err := repo.Find(ctx, u.ID)
			require.NoError(t, err, "未知の値はエラーにせず読み出せるべき")

			// EntitlementService と同じ導出をして、安全側に倒れることを確かめる。
			// 未知のプランは Entitlement が free に落とし、未知の状態は GivesPremiumAt が弾く。
			effective := domain.PlanFree
			if got.GivesPremiumAt(time.Now()) {
				effective = domain.NewEntitlement(got.Plan).Plan()
			}
			require.Equal(t, domain.PlanFree, effective, "未知の値がプレミアムとして通ってはならない")
		})
	}
}

func TestSubscriptionRepository_Upsertは上書きする(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSubscriptionRepository(pool)

	u := createUser(t, pool, "sub-repo-upsert@example.com")
	end := time.Now().Add(30 * 24 * time.Hour)

	require.NoError(t, repo.Upsert(ctx, domain.Subscription{
		UserID:           u.ID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: end,
		Provider:         domain.ProviderManual,
	}))

	// 取消は行を消さずに状態を遷移させる。解約時期の記録を残すため。
	require.NoError(t, repo.Upsert(ctx, domain.Subscription{
		UserID:           u.ID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionCanceled,
		CurrentPeriodEnd: end,
		Provider:         domain.ProviderManual,
	}))

	got, err := repo.Find(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionCanceled, got.Status)
}

func TestSubscriptionRepository_決済IDを保持する(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSubscriptionRepository(pool)

	u := createUser(t, pool, "sub-repo-psid@example.com")

	require.NoError(t, repo.Upsert(ctx, domain.Subscription{
		UserID:                 u.ID,
		Plan:                   domain.PlanPremium,
		Status:                 domain.SubscriptionActive,
		CurrentPeriodEnd:       time.Now().Add(time.Hour),
		Provider:               "stripe",
		ProviderSubscriptionID: "sub_XYZ",
	}))

	got, err := repo.Find(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "stripe", got.Provider)
	require.Equal(t, "sub_XYZ", got.ProviderSubscriptionID)
}

func TestSubscriptionRepository_ProviderCustomerIDRoundTrip(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSubscriptionRepository(pool)

	u := createUser(t, pool, "sub-repo-custid@example.com")

	sub := domain.Subscription{
		UserID:                 u.ID,
		Plan:                   domain.PlanPremium,
		Status:                 domain.SubscriptionTrialing,
		CurrentPeriodEnd:       time.Now().Add(120 * time.Hour),
		Provider:               domain.ProviderStripe,
		ProviderSubscriptionID: "sub_test_123",
		ProviderCustomerID:     "cus_test_123",
	}
	if err := repo.Upsert(ctx, sub); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := repo.Find(ctx, u.ID)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.ProviderCustomerID != "cus_test_123" {
		t.Errorf("ProviderCustomerID = %q, want %q", got.ProviderCustomerID, "cus_test_123")
	}
}

func TestSubscriptionRepository_決済IDが空でも複数ユーザーがUpsertできる(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSubscriptionRepository(pool)

	// Upsert は決済IDが空文字なら NULL に変換して保存する。もしこの変換が退行して
	// 空文字のまま書き込まれると、部分UNIQUE索引は空文字を同一値とみなすため、
	// 手動付与できるのはDB全体で1人だけになり、2人目以降のUpsertがunique制約違反で失敗する。
	u1 := createUser(t, pool, "sub-repo-empty-psid-1@example.com")
	u2 := createUser(t, pool, "sub-repo-empty-psid-2@example.com")

	require.NoError(t, repo.Upsert(ctx, domain.Subscription{
		UserID:           u1.ID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: time.Now().Add(time.Hour),
		Provider:         domain.ProviderManual,
	}))
	require.NoError(t, repo.Upsert(ctx, domain.Subscription{
		UserID:           u2.ID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: time.Now().Add(time.Hour),
		Provider:         domain.ProviderManual,
	}))
}
