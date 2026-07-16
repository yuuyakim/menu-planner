package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/random/randomtest"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func TestPick_乱数源が指した候補を返す(t *testing.T) {
	t.Parallel()

	candidates := []string{"肉じゃが", "麻婆豆腐", "カルボナーラ"}

	for i, want := range candidates {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			got, err := service.Pick(randomtest.NewFixed(i), candidates)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestPick_候補が1件なら必ずそれを返す(t *testing.T) {
	t.Parallel()

	r := randomtest.NewFixed(0)

	got, err := service.Pick(r, []string{"肉じゃが"})
	require.NoError(t, err)
	assert.Equal(t, "肉じゃが", got)
}

func TestPick_候補が空ならエラー(t *testing.T) {
	t.Parallel()

	r := randomtest.NewFixed(0)

	_, err := service.Pick(r, []string{})
	assert.ErrorIs(t, err, service.ErrNoCandidates)
	assert.Equal(t, 0, r.Calls(), "候補が空なら乱数源を引く必要はない")
}

func TestPick_候補がnilならエラー(t *testing.T) {
	t.Parallel()

	_, err := service.Pick[string](randomtest.NewFixed(0), nil)
	assert.ErrorIs(t, err, service.ErrNoCandidates)
}

func TestPick_乱数源のエラーがラップされて返る(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("乱数源の故障")

	_, err := service.Pick(failingRandomizer{err: sentinel}, []string{"肉じゃが", "麻婆豆腐"})
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.NotErrorIs(t, err, service.ErrNoCandidates, "候補はあるので候補なしとは区別できること")
}

func TestPick_乱数源が範囲外の値を返したらエラー(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"上限を超える": 2,
		"負":      -1,
	}
	for name, v := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// パニック（範囲外アクセス）ではなくエラーとして扱う。
			_, err := service.Pick(randomtest.NewFixed(v), []string{"肉じゃが", "麻婆豆腐"})
			assert.Error(t, err)
		})
	}
}

// failingRandomizer は常にエラーを返す Randomizer。
type failingRandomizer struct {
	err error
}

func (r failingRandomizer) Intn(int) (int, error) {
	return 0, r.err
}
