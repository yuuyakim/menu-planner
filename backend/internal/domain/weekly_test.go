package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestWeekLength_7日(t *testing.T) {
	t.Parallel()

	// spec.md 2.2 の「7日分（各日1献立、夕食想定）」。
	assert.Equal(t, 7, domain.WeekLength)
}

func TestDayMenu_Relaxed(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		day  domain.DayMenu
		want bool
	}{
		"緩和なし": {
			day:  domain.DayMenu{Day: 1},
			want: false,
		},
		"ジャンル連続を緩めた": {
			day:  domain.DayMenu{Day: 3, RelaxedGenreStreak: true},
			want: true,
		},
		"重複を緩めた": {
			day:  domain.DayMenu{Day: 7, RelaxedDuplicate: true},
			want: true,
		},
		"両方を緩めた": {
			day:  domain.DayMenu{Day: 7, RelaxedGenreStreak: true, RelaxedDuplicate: true},
			want: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.day.Relaxed())
		})
	}
}
