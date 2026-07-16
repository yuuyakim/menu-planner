package db_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
)

func TestSeedSQL_埋め込まれている(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)
	assert.NotEmpty(t, sql)
	assert.Contains(t, sql, "INSERT INTO menus")
}

func TestSeedSQL_冪等性のためON_CONFLICTを含む(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	// name の UNIQUE 制約と ON CONFLICT DO NOTHING で再実行しても重複しない
	assert.Contains(t, sql, "ON CONFLICT (name) DO NOTHING")
}

func TestSeedSQL_120件のINSERTがある(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	// VALUES の各行は "(gen_random_uuid(), '" で始まる。
	// 単に "gen_random_uuid()" を数えるとコメント中の記述まで拾ってしまう。
	got := strings.Count(sql, "(gen_random_uuid(), '")
	assert.Equal(t, 120, got, "献立は120件であること")
}

func TestSeedSQL_各ジャンルと難易度が10件ずつ(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	genres := []string{"japanese", "western", "chinese", "other"}
	difficulties := []string{"easy", "normal", "elaborate"}

	for _, g := range genres {
		for _, d := range difficulties {
			// "'japanese', 'easy'" の形で出現する
			pattern := "'" + g + "', '" + d + "'"
			got := strings.Count(sql, pattern)
			assert.Equal(t, 10, got, "%s × %s は10件であること", g, d)
		}
	}
}
