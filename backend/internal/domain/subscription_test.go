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
	for _, in := range []string{"", "ACTIVE", "cancelled"} {
		_, err := domain.ParseSubscriptionStatus(in)
		require.ErrorIs(t, err, domain.ErrInvalidSubscriptionStatus, "%q は拒否されるべき", in)
	}
}

func TestSubscription_GivesPremiumAt(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	cases := []struct {
		name string
		sub  domain.Subscription
		want bool
	}{
		{"active 期間内", domain.Subscription{Status: domain.SubscriptionActive, CurrentPeriodEnd: future}, true},
		{"active 期限切れ", domain.Subscription{Status: domain.SubscriptionActive, CurrentPeriodEnd: past}, false},
		{"trialing 期間内", domain.Subscription{Status: domain.SubscriptionTrialing, CurrentPeriodEnd: future}, true},
		{"past_due 猶予内", domain.Subscription{Status: domain.SubscriptionPastDue, CurrentPeriodEnd: now.Add(-3 * 24 * time.Hour)}, true},
		{"past_due 猶予超過", domain.Subscription{Status: domain.SubscriptionPastDue, CurrentPeriodEnd: now.Add(-8 * 24 * time.Hour)}, false},
		{"canceled", domain.Subscription{Status: domain.SubscriptionCanceled, CurrentPeriodEnd: future}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sub.GivesPremiumAt(now); got != tc.want {
				t.Errorf("GivesPremiumAt() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseSubscriptionStatus_Trialing(t *testing.T) {
	got, err := domain.ParseSubscriptionStatus("trialing")
	if err != nil {
		t.Fatalf("想定外のエラー: %v", err)
	}
	if got != domain.SubscriptionTrialing {
		t.Errorf("= %q, want %q", got, domain.SubscriptionTrialing)
	}
}
