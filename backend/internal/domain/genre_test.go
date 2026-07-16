package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseGenre_有効な値(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  domain.Genre
	}{
		{"japanese", domain.GenreJapanese},
		{"western", domain.GenreWestern},
		{"chinese", domain.GenreChinese},
		{"other", domain.GenreOther},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got, err := domain.ParseGenre(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.input, got.String())
		})
	}
}

func TestParseGenre_無効な値(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"italian",
		"JAPANESE", // 大文字は受け付けない
		" japanese",
		"japanese ",
		"和食",
	}

	for _, input := range tests {
		t.Run("入力="+input, func(t *testing.T) {
			t.Parallel()

			_, err := domain.ParseGenre(input)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidGenre)
		})
	}
}

func TestGenre_Valid(t *testing.T) {
	t.Parallel()

	assert.True(t, domain.GenreJapanese.Valid())
	assert.True(t, domain.GenreOther.Valid())
	assert.False(t, domain.Genre("italian").Valid())
	assert.False(t, domain.Genre("").Valid())
}

func TestAllGenres_全4種を返す(t *testing.T) {
	t.Parallel()

	got := domain.AllGenres()
	assert.Len(t, got, 4)
	assert.Contains(t, got, domain.GenreJapanese)
	assert.Contains(t, got, domain.GenreWestern)
	assert.Contains(t, got, domain.GenreChinese)
	assert.Contains(t, got, domain.GenreOther)
}
