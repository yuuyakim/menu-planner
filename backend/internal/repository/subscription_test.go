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
