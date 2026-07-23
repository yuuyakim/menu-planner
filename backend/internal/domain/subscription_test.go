package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseSubscriptionStatus_既知の状態を受け付ける(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want domain.SubscriptionStatus
	}{
		{"active", domain.SubscriptionActive},
		{"past_due", domain.SubscriptionPastDue},
		{"canceled", domain.SubscriptionCanceled},
	} {
		got, err := domain.ParseSubscriptionStatus(tt.in)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}
}

func TestParseSubscriptionStatus_未知の値は拒否する(t *testing.T) {
	for _, in := range []string{"", "ACTIVE", "cancelled", "trialing"} {
		_, err := domain.ParseSubscriptionStatus(in)
		require.ErrorIs(t, err, domain.ErrInvalidSubscriptionStatus, "%q は拒否されるべき", in)
	}
}

func TestSubscription_IsActiveAt(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

	t.Run("active かつ期限内なら有効", func(t *testing.T) {
		s := domain.Subscription{
			Status:           domain.SubscriptionActive,
			CurrentPeriodEnd: now.Add(24 * time.Hour),
		}
		require.True(t, s.IsActiveAt(now))
	})

	t.Run("active でも期限切れなら無効", func(t *testing.T) {
		s := domain.Subscription{
			Status:           domain.SubscriptionActive,
			CurrentPeriodEnd: now.Add(-time.Second),
		}
		require.False(t, s.IsActiveAt(now))
	})

	t.Run("期限ちょうどは無効", func(t *testing.T) {
		// 期限は「その時刻まで」であり、到達した時点で切れる。
		s := domain.Subscription{
			Status:           domain.SubscriptionActive,
			CurrentPeriodEnd: now,
		}
		require.False(t, s.IsActiveAt(now))
	})

	t.Run("canceled は期限内でも無効", func(t *testing.T) {
		s := domain.Subscription{
			Status:           domain.SubscriptionCanceled,
			CurrentPeriodEnd: now.Add(24 * time.Hour),
		}
		require.False(t, s.IsActiveAt(now))
	})

	t.Run("past_due は期限内でも無効", func(t *testing.T) {
		s := domain.Subscription{
			Status:           domain.SubscriptionPastDue,
			CurrentPeriodEnd: now.Add(24 * time.Hour),
		}
		require.False(t, s.IsActiveAt(now))
	})

	t.Run("ゼロ値は無効", func(t *testing.T) {
		var s domain.Subscription
		require.False(t, s.IsActiveAt(now))
	})
}
