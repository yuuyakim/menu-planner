package domain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParsePlan_既知のプランを受け付ける(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want domain.Plan
	}{
		{"free", domain.PlanFree},
		{"premium", domain.PlanPremium},
	} {
		got, err := domain.ParsePlan(tt.in)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}
}

func TestParsePlan_未知の値は拒否する(t *testing.T) {
	// 表記ゆれを許すとDBの値と乖離するため、完全一致のみ受け付ける。
	for _, in := range []string{"", "Free", "PREMIUM", " premium", "pro"} {
		_, err := domain.ParsePlan(in)
		require.ErrorIs(t, err, domain.ErrInvalidPlan, "%q は拒否されるべき", in)
	}
}

func TestPlan_StringはDBに入る値を返す(t *testing.T) {
	require.Equal(t, "free", domain.PlanFree.String())
	require.Equal(t, "premium", domain.PlanPremium.String())
}

func TestPlan_Valid(t *testing.T) {
	require.True(t, domain.PlanFree.Valid())
	require.True(t, domain.PlanPremium.Valid())
	require.False(t, domain.Plan("").Valid())
	require.False(t, domain.Plan("pro").Valid())
}

func TestErrInvalidPlan_sentinelとして使える(t *testing.T) {
	_, err := domain.ParsePlan("pro")
	require.True(t, errors.Is(err, domain.ErrInvalidPlan))
}
