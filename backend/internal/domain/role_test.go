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

// 検索の絞り込みでは、未指定と "all" を役割そのものとは別に解釈する（spec.md 2.10 / 5.1）。
func TestParseRoleFilter(t *testing.T) {
	t.Parallel()

	t.Run("未指定は主菜に絞る", func(t *testing.T) {
		t.Parallel()

		// ジャンル・難易度の未指定は「すべて」だが、役割だけ意味が違う。
		// 未指定のときに一番起きてほしくないのが副菜の単品提案のため。
		got, err := domain.ParseRoleFilter("")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, domain.RoleMain, *got)
	})

	t.Run("allは絞り込まない", func(t *testing.T) {
		t.Parallel()

		got, err := domain.ParseRoleFilter("all")
		require.NoError(t, err)
		assert.Nil(t, got, "nil が「絞り込まない」を表す")
	})

	for _, r := range domain.AllRoles() {
		t.Run("役割を指定できる/"+r.String(), func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseRoleFilter(r.String())
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, r, *got)
		})
	}

	for _, s := range []string{"ALL", "dessert", "主菜", " "} {
		t.Run("未知の値は弾く/"+s, func(t *testing.T) {
			t.Parallel()

			_, err := domain.ParseRoleFilter(s)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidRole)
		})
	}
}

func TestMenuFilter_Validate_役割(t *testing.T) {
	t.Parallel()

	valid := domain.RoleSide
	require.NoError(t, domain.MenuFilter{Role: &valid}.Validate())

	invalid := domain.Role("dessert")
	require.ErrorIs(t, domain.MenuFilter{Role: &invalid}.Validate(), domain.ErrInvalidRole)

	// nil は「絞り込まない」なので有効。
	require.NoError(t, domain.MenuFilter{}.Validate())
}
