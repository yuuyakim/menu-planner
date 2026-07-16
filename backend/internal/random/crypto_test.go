package random_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/random"
)

func TestCrypto_nが1なら常に0を返す(t *testing.T) {
	t.Parallel()

	r := random.NewCrypto()

	for range 10 {
		got, err := r.Intn(1)
		require.NoError(t, err)
		assert.Equal(t, 0, got)
	}
}

func TestCrypto_値が範囲内に収まる(t *testing.T) {
	t.Parallel()

	r := random.NewCrypto()
	const n = 5

	for range 200 {
		got, err := r.Intn(n)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, got, 0)
		assert.Less(t, got, n)
	}
}

func TestCrypto_全ての値が出現しうる(t *testing.T) {
	t.Parallel()

	// 常に同じ値を返す実装（例: return 0）を落とすためのテスト。
	// n=3 を300回引いて特定の値が一度も出ない確率は約 (2/3)^300 で、実質ゼロ。
	r := random.NewCrypto()
	const n = 3

	seen := map[int]bool{}
	for range 300 {
		got, err := r.Intn(n)
		require.NoError(t, err)
		seen[got] = true
	}

	assert.Len(t, seen, n, "0..%d の全ての値が出現するはず: %v", n-1, seen)
}

func TestCrypto_nが1未満ならエラー(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"ゼロ": 0,
		"負":  -1,
	}
	for name, n := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := random.NewCrypto()
			_, err := r.Intn(n)
			assert.ErrorIs(t, err, random.ErrInvalidRange)
		})
	}
}
