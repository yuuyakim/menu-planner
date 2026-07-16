package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseDifficulty_有効な値(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  domain.Difficulty
	}{
		{"easy", domain.DifficultyEasy},
		{"normal", domain.DifficultyNormal},
		{"elaborate", domain.DifficultyElaborate},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseDifficulty(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.input, got.String())
		})
	}
}

func TestParseDifficulty_無効な値(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"hard",
		"EASY",
		"かんたん",
	}

	for _, input := range tests {
		t.Run("入力="+input, func(t *testing.T) {
			t.Parallel()

			_, err := domain.ParseDifficulty(input)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidDifficulty)
		})
	}
}

func TestDifficulty_Valid(t *testing.T) {
	t.Parallel()

	assert.True(t, domain.DifficultyEasy.Valid())
	assert.True(t, domain.DifficultyElaborate.Valid())
	assert.False(t, domain.Difficulty("hard").Valid())
	assert.False(t, domain.Difficulty("").Valid())
}

func TestAllDifficulties_全3種を返す(t *testing.T) {
	t.Parallel()

	got := domain.AllDifficulties()
	assert.Len(t, got, 3)
	assert.Contains(t, got, domain.DifficultyEasy)
	assert.Contains(t, got, domain.DifficultyNormal)
	assert.Contains(t, got, domain.DifficultyElaborate)
}
