package gateway_test

import (
	"encoding/json"
	"os"
	"testing"
)

// evalCase は eval の正解データ1件。expected が null は「マスタに無い」を表す。
type evalCase struct {
	Input    string  `json:"input"`
	Expected *string `json:"expected"`
}

// loadEvalCases は正解データを読む。
func loadEvalCases(t *testing.T) []evalCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/ingredient_resolution_cases.json")
	if err != nil {
		t.Fatalf("正解データを読めませんでした: %v", err)
	}
	var cases []evalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("正解データを解釈できませんでした: %v", err)
	}
	return cases
}

// loadCatalogNames は食材マスタの name を返す。
//
// DBには繋がない。eval は外部APIを叩く評価であって、DBの検証ではないため。
// testdata の一覧は db/seeds/ingredients.sql から書き出したもの。
func loadCatalogNames(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("testdata/ingredient_catalog.json")
	if err != nil {
		t.Fatalf("食材リストを読めませんでした: %v", err)
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		t.Fatalf("食材リストを解釈できませんでした: %v", err)
	}
	return names
}

// TestEvalCasesAreConsistent は正解データが食材マスタと矛盾していないことを確かめる。
//
// **これは eval タグを付けない。** 外部APIを叩かないうえ、ここが壊れていると
// eval の数字そのものが無意味になるため、通常のCIで守る。
// 実際、計画のサンプルには存在しない食材名（豚肉・長ねぎ）が含まれていた。
func TestEvalCasesAreConsistent(t *testing.T) {
	cases := loadEvalCases(t)
	names := loadCatalogNames(t)

	inCatalog := make(map[string]bool, len(names))
	for _, n := range names {
		inCatalog[n] = true
	}

	if len(cases) < 50 {
		t.Errorf("正解データは50件以上にするべきです: %d件", len(cases))
	}

	seen := make(map[string]bool, len(cases))
	nullCount := 0
	for _, c := range cases {
		if seen[c.Input] {
			t.Errorf("入力語が重複しています: %q", c.Input)
		}
		seen[c.Input] = true

		if c.Expected == nil {
			nullCount++
			continue
		}
		if !inCatalog[*c.Expected] {
			t.Errorf("期待値が食材マスタに存在しません: input=%q expected=%q", c.Input, *c.Expected)
		}
	}

	// マスタ外の語が1件も無いと、「該当なしを返せるか」を測れない。
	if nullCount == 0 {
		t.Error("マスタに無い語（expected=null）を含めるべきです")
	}
}

// TestCatalogMatchesSeedCount は testdata の食材一覧がシードと同じ件数であることを確かめる。
// シードに食材を足したのに testdata を更新し忘れると、eval が古い前提で走ってしまう。
func TestCatalogMatchesSeedCount(t *testing.T) {
	names := loadCatalogNames(t)
	// spec.md 14章の食材マスタは166件。
	const wantCount = 166
	if len(names) != wantCount {
		t.Errorf("食材一覧が %d 件です（シードは %d 件）。"+
			"db/seeds/ingredients.sql を変えたら testdata も更新してください", len(names), wantCount)
	}
}
