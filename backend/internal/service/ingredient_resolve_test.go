package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// countingResolver は「何回呼ばれたか」を数えるスタブ。
// ①②で解けたときに Gateway が呼ばれないことを検証するために使う。
type countingResolver struct {
	calls   int
	mapping map[string]string
	err     error
}

func (r *countingResolver) Resolve(
	_ context.Context, words []string, _ []string,
) ([]service.GatewayResolution, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	out := make([]service.GatewayResolution, 0, len(words))
	for _, w := range words {
		out = append(out, service.GatewayResolution{Word: w, Name: r.mapping[w]})
	}
	return out, nil
}

// fakeResolutionRepo は解決キャッシュの最小のインメモリ実装。
type fakeResolutionRepo struct {
	data  map[string]*domain.IngredientID
	saved []string
}

func (r *fakeResolutionRepo) FindByWords(
	_ context.Context, words []string,
) (map[string]*domain.IngredientID, error) {
	out := map[string]*domain.IngredientID{}
	for _, w := range words {
		if v, ok := r.data[w]; ok {
			out[w] = v
		}
	}
	return out, nil
}

func (r *fakeResolutionRepo) Save(
	_ context.Context, word string, id *domain.IngredientID,
) error {
	if r.data == nil {
		r.data = map[string]*domain.IngredientID{}
	}
	r.data[word] = id
	r.saved = append(r.saved, word)
	return nil
}

// testCatalog はテスト用の食材マスタ。
func testCatalog(t *testing.T) []domain.Ingredient {
	t.Helper()
	mk := func(name, kana string, c domain.IngredientCategory) domain.Ingredient {
		return domain.Ingredient{
			ID: domain.NewIngredientID(), Name: name, NameKana: kana, Category: c,
		}
	}
	return []domain.Ingredient{
		mk("玉ねぎ", "たまねぎ", domain.CategoryVegetable),
		mk("豚肉", "ぶたにく", domain.CategoryMeat),
		mk("卵", "たまご", domain.CategoryDairyEgg),
	}
}

func TestResolve_ExactMatchOnly(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw)

	t.Run("全語が完全一致ならGatewayを呼ばない", func(t *testing.T) {
		got, err := svc.Resolve(ctx, "玉ねぎ、卵")
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if gw.calls != 0 {
			t.Errorf("Gateway が呼ばれています: %d回", gw.calls)
		}
		if len(got.Resolved) != 2 {
			t.Fatalf("2件解決するべきです: %+v", got.Resolved)
		}
		if len(got.Unresolved) != 0 {
			t.Errorf("未解決は0件であるべきです: %v", got.Unresolved)
		}
		if got.Degraded {
			t.Error("縮退していないのに Degraded が立っています")
		}
	})

	t.Run("カタカナ表記も完全一致で解ける", func(t *testing.T) {
		got, err := svc.Resolve(ctx, "タマネギ")
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got.Resolved) != 1 || got.Resolved[0].Ingredient.Name != "玉ねぎ" {
			t.Errorf("カナ一致で解けていません: %+v", got.Resolved)
		}
	})

	t.Run("元の語をそのまま返す", func(t *testing.T) {
		got, _ := svc.Resolve(ctx, "タマネギ")
		if got.Resolved[0].Word != "タマネギ" {
			t.Errorf("利用者が書いた語を返すべきです: %q", got.Resolved[0].Word)
		}
	})

	t.Run("重複した語は1件にまとめる", func(t *testing.T) {
		got, _ := svc.Resolve(ctx, "玉ねぎ、たまねぎ")
		if len(got.Resolved) != 1 {
			t.Errorf("同じ食材は1件にまとめるべきです: %+v", got.Resolved)
		}
	})

	t.Run("マスタに無い語は未解決に落ちる", func(t *testing.T) {
		got, err := svc.Resolve(ctx, "玉ねぎ、マツタケ")
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got.Resolved) != 1 {
			t.Errorf("解決できた分は返すべきです: %+v", got.Resolved)
		}
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "マツタケ" {
			t.Errorf("元の語のまま未解決に落とすべきです: %v", got.Unresolved)
		}
	})

	t.Run("空テキストはエラー", func(t *testing.T) {
		_, err := svc.Resolve(ctx, "  、 ")
		if !errors.Is(err, service.ErrEmptyResolveText) {
			t.Errorf("ErrEmptyResolveText を返すべきです: %v", err)
		}
	})
}
