package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseSearchMode(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"single", "weekly"} {
		m, err := domain.ParseSearchMode(s)
		require.NoError(t, err)
		require.Equal(t, s, m.String())
		require.True(t, m.Valid())
	}
}

func TestParseSearchMode_Invalid(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"", "SINGLE", "daily", " single"} {
		_, err := domain.ParseSearchMode(s)
		require.ErrorIs(t, err, domain.ErrInvalidSearchMode, "入力=%q", s)
	}
}
