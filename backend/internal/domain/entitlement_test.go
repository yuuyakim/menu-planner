package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestEntitlement_プランごとの保存上限(t *testing.T) {
	free := domain.NewEntitlement(domain.PlanFree)
	require.Equal(t, 10, free.SavedWeeklyMenuLimit(),
		"free は現行の10件を据え置く。既存利用者の体験を削らない")

	premium := domain.NewEntitlement(domain.PlanPremium)
	require.Equal(t, 50, premium.SavedWeeklyMenuLimit())
}

// ゼロ値が free に落ちることは、この設計の安全装置そのもの。
// 上限をフィールドで持つと 0 件になり、既存利用者が1件も保存できなくなる。
func TestEntitlement_ゼロ値はfreeとして振る舞う(t *testing.T) {
	var zero domain.Entitlement

	require.Equal(t, domain.PlanFree, zero.Plan())
	require.Equal(t, 10, zero.SavedWeeklyMenuLimit(),
		"ゼロ値の上限が0だと既存利用者が保存できなくなる")
}

func TestEntitlement_未知のプランもfreeに落ちる(t *testing.T) {
	// DBに想定外の値が入っていた場合でも、締め出すのではなく free として扱う。
	e := domain.NewEntitlement(domain.Plan("pro"))

	require.Equal(t, domain.PlanFree, e.Plan())
	require.Equal(t, 10, e.SavedWeeklyMenuLimit())
}

func TestEntitlement_Planを返す(t *testing.T) {
	require.Equal(t, domain.PlanPremium,
		domain.NewEntitlement(domain.PlanPremium).Plan())
	require.Equal(t, domain.PlanFree,
		domain.NewEntitlement(domain.PlanFree).Plan())
}

func TestEntitlement_CanPersistShoppingList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ent  domain.Entitlement
		want bool
	}{
		{"premium は永続化できる", domain.NewEntitlement(domain.PlanPremium), true},
		{"free は永続化できない", domain.NewEntitlement(domain.PlanFree), false},
		// ゼロ値は取得し忘れを表す。free と同じく永続化できない（安全側）。
		{"ゼロ値は永続化できない", domain.Entitlement{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.ent.CanPersistShoppingList(); got != tt.want {
				t.Errorf("CanPersistShoppingList() = %v, want %v", got, tt.want)
			}
		})
	}
}
