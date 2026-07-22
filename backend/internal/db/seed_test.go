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

	// name の UNIQUE 制約と ON CONFLICT で再実行しても重複しない。
	// **DO NOTHING ではなく DO UPDATE（upsert）にしている。**
	// 挿入のみだと、説明文やカナの誤りを直しても、既にシード済みのDB
	// （本番など）には永久に反映されないため。
	assert.Contains(t, sql, "ON CONFLICT (name) DO UPDATE")
	assert.Contains(t, sql, "description = EXCLUDED.description",
		"修正が既存行にも反映されること")
}

// wantMenuCounts は (ジャンル, 難易度) ごとの期待件数。
//
// **easy / normal は各40件、elaborate は各10件が最終形**（合計360件）。
// 2026-07-22 に「献立が少なすぎる。特に簡単・普通を増やしたい」という要望を受け、
// 全ての組で10件だったものを easy / normal だけ40件に増やしている。
// elaborate を据え置くのは、利用者レビューで「高価・入手困難なものが混ざる」と
// 指摘されており（task.md「利用者レビューからの後続タスク」C）、
// 日常的に作れる献立の比率を上げたいため。
//
// 投入はジャンル単位で分けているため、移行中は 40 と 10 が混在する。
// 未投入のジャンルには「投入待ち」とコメントを付けている。
func wantMenuCounts() map[[2]string]int {
	return map[[2]string]int{
		{"japanese", "easy"}: 40, {"japanese", "normal"}: 40, {"japanese", "elaborate"}: 10,
		{"western", "easy"}: 40, {"western", "normal"}: 40, {"western", "elaborate"}: 10,
		{"chinese", "easy"}: 40, {"chinese", "normal"}: 40, {"chinese", "elaborate"}: 10,
		{"other", "easy"}: 40, {"other", "normal"}: 40, {"other", "elaborate"}: 10,
	}
}

func TestSeedSQL_期待件数のINSERTがある(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	want := 0
	for _, n := range wantMenuCounts() {
		want += n
	}

	// VALUES の各行は "(gen_random_uuid(), '" で始まる。
	// 単に "gen_random_uuid()" を数えるとコメント中の記述まで拾ってしまう。
	got := strings.Count(sql, "(gen_random_uuid(), '")
	assert.Equal(t, want, got, "献立は%d件であること", want)
}

func TestSeedSQL_ジャンルと難易度ごとの件数(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	for key, want := range wantMenuCounts() {
		genre, difficulty := key[0], key[1]
		// "'japanese', 'easy'" の形で出現する
		pattern := "'" + genre + "', '" + difficulty + "'"
		got := strings.Count(sql, pattern)
		assert.Equal(t, want, got, "%s × %s は%d件であること", genre, difficulty, want)
	}
}
