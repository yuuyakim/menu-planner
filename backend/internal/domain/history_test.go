package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseHistoryID(t *testing.T) {
	t.Parallel()

	_, err := domain.ParseHistoryID("not-a-uuid")
	require.ErrorIs(t, err, domain.ErrInvalidHistoryID)

	_, err = domain.ParseHistoryID("00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, domain.ErrInvalidHistoryID)

	id, err := domain.ParseHistoryID("018f0000-0000-7000-8000-000000000001")
	require.NoError(t, err)
	require.Equal(t, "018f0000-0000-7000-8000-000000000001", id.String())
}
