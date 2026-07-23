package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseRole_有効な値(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  domain.Role
	}{
		{"main", domain.RoleMain},
		{"side", domain.RoleSide},
		{"soup", domain.RoleSoup},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseRole(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.input, got.String())
		})
	}
}

func TestParseRole_無効な値(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"MAIN",
		"主菜",
		"dessert",
		// all は絞り込みを外すためのAPIの値で、献立が持つ役割ではない（spec.md 2.10）。
		// 保存できてしまうと CHECK 制約と食い違うため、ここでは弾く。
		"all",
	}

	for _, input := range tests {
		t.Run("入力="+input, func(t *testing.T) {
			t.Parallel()

			_, err := domain.ParseRole(input)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidRole)
		})
	}
}

func TestRole_Valid(t *testing.T) {
	t.Parallel()

	assert.True(t, domain.RoleMain.Valid())
	assert.True(t, domain.RoleSide.Valid())
	assert.True(t, domain.RoleSoup.Valid())
	assert.False(t, domain.Role("").Valid())
	assert.False(t, domain.Role("all").Valid())
}

func TestAllRoles(t *testing.T) {
	t.Parallel()

	// 主菜を先頭に置く。既定が main（spec.md 2.10）であり、
	// 選択肢を並べるときも既定が先頭に来るほうが選びやすい。
	assert.Equal(t,
		[]domain.Role{domain.RoleMain, domain.RoleSide, domain.RoleSoup},
		domain.AllRoles())
}
