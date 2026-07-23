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

// wantRoleCounts は役割ごとの期待件数（spec.md 2.10）。
//
// **14-B 時点の値。既存360件を分類しただけで、拡充はしていない。**
// 副菜31件・汁物15件は全体の13%で、ジャンル×難易度で絞ると1桁になる。
// この状態で side を選べるようにしても同じ献立が繰り返し出るため、
// 14-C で side を各ジャンル10件以上・soup を各ジャンル5件以上まで増やす。
// **増やしたらこの表も更新すること。**
func wantRoleCounts() map[string]int {
	return map[string]int{"main": 315, "side": 30, "soup": 15}
}

func TestSeedSQL_役割ごとの件数(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	total := 0
	for role, want := range wantRoleCounts() {
		// "'easy', 'main', '" の形で出現する。難易度に続く位置で数えることで、
		// 説明文に同じ語が現れても拾わない。
		got := strings.Count(sql, "', '"+role+"', '")
		assert.Equal(t, want, got, "役割 %s は%d件であること", role, want)
		total += want
	}

	// 役割の付け漏れを検出する。行数と一致しなければ、どれかの行に role が無い。
	rows := strings.Count(sql, "(gen_random_uuid(), '")
	assert.Equal(t, rows, total, "全ての行に役割が付いていること")
}

// TestSeedSQL_説明文と役割が矛盾しない は、説明文が「単品で一食になる」と
// 主張しているのに main 以外が付いている行を検出する。
//
// **コブサラダで実際に起きた。** 説明文に「これだけで主菜になる」と書いてあるのに
// side を付けており、分類基準（単品で夕食が成立するか）と正面から矛盾していた。
// 360件を目視で分類する限り同じ取り違えは必ず再発するため、機械で拾う。
//
// 語句は「一食として成立する」と明言しているものだけに絞る。
// 「これだけで作れる」のような手軽さの表現まで拾うと誤検出になる。
func TestSeedSQL_説明文と役割が矛盾しない(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	claimsMain := []string{"主菜になる", "一食になる", "主菜として"}

	for _, line := range strings.Split(sql, "\n") {
		if !strings.HasPrefix(line, "(gen_random_uuid(), '") {
			continue
		}
		if strings.Contains(line, "', 'main', '") {
			continue
		}
		for _, phrase := range claimsMain {
			assert.NotContains(t, line, phrase,
				"説明文が%qと言っているのに main 以外が付いている", phrase)
		}
	}
}

func TestSeedSQL_役割の付け替えが既存行にも反映される(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	// 役割は後から見直す前提の列（分類は主観が入る）。ON CONFLICT に
	// 含めないと、シード済みのDBでは初回の分類のまま固定されてしまう。
	assert.Contains(t, sql, "role        = EXCLUDED.role",
		"役割の付け替えが既存行にも反映されること")
}
