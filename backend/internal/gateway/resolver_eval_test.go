//go:build eval

// 実LLMを叩く評価。**CIでは走らせない**（APIキーと課金が絡むため）。
//
//	INGREDIENT_RESOLVER_PROVIDER=claude \
//	INGREDIENT_RESOLVER_API_KEY=sk-... \
//	go test -tags=eval ./internal/gateway/ -run TestResolverEval -v
package gateway_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
)

func TestResolverEval(t *testing.T) {
	cases := loadEvalCases(t)
	catalog := loadCatalogNames(t)

	resolver, err := gateway.NewResolver(gateway.ResolverConfig{
		Provider: os.Getenv("INGREDIENT_RESOLVER_PROVIDER"),
		APIKey:   os.Getenv("INGREDIENT_RESOLVER_API_KEY"),
	})
	if err != nil {
		t.Fatalf("Resolver を組み立てられませんでした: %v", err)
	}

	words := make([]string, 0, len(cases))
	for _, c := range cases {
		words = append(words, c.Input)
	}

	start := time.Now()
	answers, err := resolver.Resolve(context.Background(), words, catalog)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}

	got := make(map[string]string, len(answers))
	for _, a := range answers {
		got[a.Word] = a.Name
	}

	// 種別ごとの内訳も出す。全体の正解率だけでは、どの種別が弱いのか分からない。
	correct := 0
	missing := 0
	for _, c := range cases {
		want := ""
		if c.Expected != nil {
			want = *c.Expected
		}
		if _, ok := got[c.Input]; !ok {
			missing++
		}
		if got[c.Input] == want {
			correct++
			continue
		}
		t.Logf("誤答: input=%q want=%q got=%q", c.Input, want, got[c.Input])
	}

	rate := float64(correct) / float64(len(cases)) * 100
	t.Logf("=== eval 結果 ===")
	t.Logf("プロバイダ : %s", os.Getenv("INGREDIENT_RESOLVER_PROVIDER"))
	t.Logf("件数       : %d", len(cases))
	t.Logf("正解       : %d", correct)
	t.Logf("正解率     : %.1f%%", rate)
	t.Logf("応答漏れ   : %d（問い合わせた語が返答に含まれなかった数）", missing)
	t.Logf("所要時間   : %v", elapsed)
}
