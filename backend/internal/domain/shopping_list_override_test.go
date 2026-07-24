package domain_test

import (
	"errors"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseOrigin(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"derived", "manual"} {
		if _, err := domain.ParseOrigin(s); err != nil {
			t.Errorf("ParseOrigin(%q) は成功すべき: %v", s, err)
		}
	}
	// 表記ゆれ・空・未知は拒否（DBの値と乖離させない）。
	for _, s := range []string{"", "Derived", " manual", "other"} {
		if _, err := domain.ParseOrigin(s); !errors.Is(err, domain.ErrInvalidOrigin) {
			t.Errorf("ParseOrigin(%q) は ErrInvalidOrigin を返すべき: %v", s, err)
		}
	}
}

func validOverride() domain.ShoppingListOverride {
	return domain.ShoppingListOverride{
		SavedWeeklyMenuID: domain.NewSavedWeeklyMenuID(),
		Name:              "にんじん",
		Category:          domain.CategoryVegetable,
		Origin:            domain.OriginDerived,
		Checked:           true,
	}
}

func TestShoppingListOverride_Validate(t *testing.T) {
	t.Parallel()

	if err := validOverride().Validate(); err != nil {
		t.Fatalf("正当な差分は通るべき: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(o *domain.ShoppingListOverride)
	}{
		{"IDが未設定", func(o *domain.ShoppingListOverride) { o.SavedWeeklyMenuID = domain.SavedWeeklyMenuID{} }},
		{"名前が空", func(o *domain.ShoppingListOverride) { o.Name = "  " }},
		{"カテゴリが不正", func(o *domain.ShoppingListOverride) { o.Category = "spice" }},
		{"originが不正", func(o *domain.ShoppingListOverride) { o.Origin = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := validOverride()
			tt.mutate(&o)
			if err := o.Validate(); !errors.Is(err, domain.ErrInvalidOverride) {
				t.Errorf("%s は ErrInvalidOverride を返すべき: %v", tt.name, err)
			}
		})
	}
}
