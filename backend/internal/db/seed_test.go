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

// wantMainCounts は (ジャンル, 難易度) ごとの**主菜**の期待件数。
//
// **easy / normal は各40件、elaborate は各10件**（主菜の合計360件）。
// 2026-07-22 に「献立が少なすぎる。特に簡単・普通を増やしたい」という要望を受け、
// 全ての組で10件だったものを easy / normal だけ40件に増やしている。
// elaborate を据え置くのは、利用者レビューで「高価・入手困難なものが混ざる」と
// 指摘されており（task.md「利用者レビューからの後続タスク」C）、
// 日常的に作れる献立の比率を上げたいため。
//
// **2026-07-23 に主菜だけを数えるよう変えた（フェーズ14）。**
// **元の「各40件」には副菜・汁物が含まれていた。** 役割の列を入れて数え直すと
// 主菜は 31〜37 件で、40 ではなかった（例: 洋食×簡単は40件中9件が副菜・汁物）。
// **ここで守りたいのは既定の検索経路（role=main）が枯れないこと**なので、
// 主菜の実数を表にする。週間献立が7日分を重複なく引けるのはこの件数に依る。
//
// 数を増やしたわけではなく、内訳を明らかにしただけ。増減があれば気付けるよう
// 実数で固定する（下限だけの検査にすると、まとめて消えても通ってしまう）。
func wantMainCounts() map[[2]string]int {
	return map[[2]string]int{
		{"japanese", "easy"}: 37, {"japanese", "normal"}: 32, {"japanese", "elaborate"}: 10,
		{"western", "easy"}: 31, {"western", "normal"}: 36, {"western", "elaborate"}: 10,
		{"chinese", "easy"}: 32, {"chinese", "normal"}: 35, {"chinese", "elaborate"}: 10,
		{"other", "easy"}: 36, {"other", "normal"}: 37, {"other", "elaborate"}: 10,
	}
}

// minMainPerCell は (ジャンル, 難易度) ごとに確保する主菜の下限。
//
// シードの冒頭が掲げる「最低10件」と同じ値。週間献立は7日分を重複なく引くため、
// ジャンルと難易度を両方指定されても7件は要る。3件の余裕を持たせている。
//
// **役割を入れたとき和食×手が込んだ が9件に落ちた**（土瓶蒸しを汁物にしたため）。
// 主菜を1件補って戻している。実数の表だけだと、この種の目減りが
// 「表を書き換えれば通る」で見逃されるため、下限も別に検査する。
const minMainPerCell = 10

func TestSeedSQL_期待件数のINSERTがある(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	// 総数は役割ごとの期待値の合計。ジャンル×難易度の表は主菜しか数えないため、
	// そちらを足しても副菜・汁物の分が抜ける。
	want := 0
	for _, n := range wantRoleCounts() {
		want += n
	}

	// VALUES の各行は "(gen_random_uuid(), '" で始まる。
	// 単に "gen_random_uuid()" を数えるとコメント中の記述まで拾ってしまう。
	got := strings.Count(sql, "(gen_random_uuid(), '")
	assert.Equal(t, want, got, "献立は%d件であること", want)
}

func TestSeedSQL_ジャンルと難易度ごとの主菜の件数(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	for key, want := range wantMainCounts() {
		genre, difficulty := key[0], key[1]
		// "'japanese', 'easy', 'main'," の形で出現する
		pattern := "'" + genre + "', '" + difficulty + "', 'main',"
		got := strings.Count(sql, pattern)
		assert.Equal(t, want, got, "%s × %s の主菜は%d件であること", genre, difficulty, want)
		assert.GreaterOrEqual(t, got, minMainPerCell,
			"%s × %s の主菜は%d件以上あること（週間献立が7日分を重複なく引けない）",
			genre, difficulty, minMainPerCell)
	}
}

// 副菜・汁物のジャンル単位の下限（spec.md 2.10）。
//
// **難易度まで含めた全マスの下限は設けない。** 満たすには副菜・汁物だけで
// 4ジャンル × 3難易度 × 2役割 × 10 = 240件が要り、手の込んだ副菜の需要も薄い。
// 難易度で絞ったときの枯渇は週間献立の緩和ルール（spec.md 2.2）で吸収する。
const (
	minSidePerGenre = 10
	minSoupPerGenre = 5
)

func TestSeedSQL_ジャンルごとに副菜と汁物が足りている(t *testing.T) {
	t.Parallel()

	sql, err := db.SeedSQL()
	require.NoError(t, err)

	for _, genre := range []string{"japanese", "western", "chinese", "other"} {
		for _, tt := range []struct {
			role string
			min  int
		}{
			{"side", minSidePerGenre},
			{"soup", minSoupPerGenre},
		} {
			got := 0
			for _, d := range []string{"easy", "normal", "elaborate"} {
				got += strings.Count(sql, "'"+genre+"', '"+d+"', '"+tt.role+"',")
			}
			assert.GreaterOrEqual(t, got, tt.min,
				"%s の %s は%d件以上あること（絞ると枯れて同じ献立が繰り返し出る）",
				genre, tt.role, tt.min)
		}
	}
}

// wantRoleCounts は役割ごとの期待件数（spec.md 2.10）。
//
// **14-C で副菜・汁物を +19件した後の値**（side 30→42 / soup 15→22）。
// 主菜の +1 は和食×手が込んだ の穴埋め（minMainPerCell を参照）。
func wantRoleCounts() map[string]int {
	return map[string]int{"main": 316, "side": 42, "soup": 22}
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
