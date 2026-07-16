package randomtest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/random/randomtest"
)

func TestFixed_指定した値を順に返す(t *testing.T) {
	t.Parallel()

	r := randomtest.NewFixed(2, 0, 1)

	for _, want := range []int{2, 0, 1} {
		got, err := r.Intn(3)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestFixed_値を使い切ったら先頭に戻る(t *testing.T) {
	t.Parallel()

	r := randomtest.NewFixed(1, 2)

	for _, want := range []int{1, 2, 1, 2} {
		got, err := r.Intn(3)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestFixed_呼び出し回数を数える(t *testing.T) {
	t.Parallel()

	r := randomtest.NewFixed(0)
	assert.Equal(t, 0, r.Calls())

	_, err := r.Intn(1)
	require.NoError(t, err)
	_, err = r.Intn(1)
	require.NoError(t, err)

	assert.Equal(t, 2, r.Calls())
}

func TestFixed_範囲外の値を返す設定でもそのまま返す(t *testing.T) {
	t.Parallel()

	// Pick 側の防御的チェックを試すために、あえて範囲外を返せる必要がある。
	r := randomtest.NewFixed(99)

	got, err := r.Intn(3)
	require.NoError(t, err)
	assert.Equal(t, 99, got)
}

func TestFixed_値が空ならエラー(t *testing.T) {
	t.Parallel()

	r := randomtest.NewFixed()

	_, err := r.Intn(3)
	assert.Error(t, err)
}

func TestFixed_nが1未満ならエラー(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"ゼロ": 0,
		"負":  -1,
	}
	for name, n := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := randomtest.NewFixed(0)
			_, err := r.Intn(n)
			assert.Error(t, err)
		})
	}
}
