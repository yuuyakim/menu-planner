package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// サブスク撤廃後、上限はプランによらず一律。
// プランごとに分けていた頃の名残（free=10）が残っていないことを、
// free / premium / ゼロ値のすべてで確かめる。
func TestEntitlement_保存上限はプランによらず50(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ent  domain.Entitlement
	}{
		{"free", domain.NewEntitlement(domain.PlanFree)},
		{"premium", domain.NewEntitlement(domain.PlanPremium)},
		{"未知のプラン", domain.NewEntitlement(domain.Plan("pro"))},
		// ゼロ値は取得し忘れを表す。撤廃後は誰も締め出さないため、
		// ここも 50 でなければならない（0 だと1件も保存できなくなる）。
		{"ゼロ値", domain.Entitlement{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, 50, tt.ent.SavedWeeklyMenuLimit())
		})
	}
}

func TestEntitlement_買い物リストの永続化は誰でもできる(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ent  domain.Entitlement
	}{
		{"free", domain.NewEntitlement(domain.PlanFree)},
		{"premium", domain.NewEntitlement(domain.PlanPremium)},
		{"ゼロ値", domain.Entitlement{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.True(t, tt.ent.CanPersistShoppingList())
		})
	}
}

func TestEntitlement_週間献立は誰でも使える(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ent  domain.Entitlement
	}{
		{"free", domain.NewEntitlement(domain.PlanFree)},
		{"premium", domain.NewEntitlement(domain.PlanPremium)},
		{"ゼロ値", domain.Entitlement{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.True(t, tt.ent.CanUseWeeklyPlanning())
		})
	}
}

// Plan は撤廃後も残す。/auth/me が返し続けるフィールドであり、
// DB の subscriptions.plan をそのまま表す役割は変わらない。
func TestEntitlement_Planを返す(t *testing.T) {
	t.Parallel()

	require.Equal(t, domain.PlanPremium,
		domain.NewEntitlement(domain.PlanPremium).Plan())
	require.Equal(t, domain.PlanFree,
		domain.NewEntitlement(domain.PlanFree).Plan())
	require.Equal(t, domain.PlanFree,
		domain.NewEntitlement(domain.Plan("pro")).Plan(),
		"未知のプランは free に落ちる")

	var zero domain.Entitlement
	require.Equal(t, domain.PlanFree, zero.Plan())
}
