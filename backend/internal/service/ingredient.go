package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// IngredientService は食材マスタそのものを扱う（spec.md 2.9）。
//
// 買い物リスト（ShoppingListService）が「献立×食材」を扱うのに対し、
// こちらは献立に紐づかない食材の一覧を担う。手持ちの食材を選ぶ画面の
// 選択肢がこれにあたる。
type IngredientService struct {
	ingredients IngredientRepository
}

// NewIngredientService は IngredientService を生成する。
func NewIngredientService(i IngredientRepository) *IngredientService {
	return &IngredientService{ingredients: i}
}

// All は食材マスタを表示順（カテゴリ順 → カナ順）で全件返す。
//
// 166件で固定的なため、ページングも検索条件も持たない。
// 画面はこれをカテゴリごとに分けて並べる。
func (s *IngredientService) All(ctx context.Context) ([]domain.Ingredient, error) {
	items, err := s.ingredients.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("食材マスタの取得に失敗しました: %w", err)
	}
	sortIngredientsForDisplay(items)
	return items, nil
}

// sortIngredientsForDisplay は食材を表示順（カテゴリ順 → カナ順）に並べる。
//
// カテゴリの順序は domain の知識（`AllIngredientCategories`）なので、
// SQL の ORDER BY には書かない。repository はカナ順までを保証し、
// カテゴリ順はここで与える。
//
// **repository の並びに依存させないため、カナ順もここで指定する。**
// 安定ソートなのでカテゴリだけ指定しても現状は同じ結果になるが、
// repository の ORDER BY が変わった瞬間に静かに崩れる。
func sortIngredientsForDisplay(items []domain.Ingredient) {
	sort.SliceStable(items, func(a, b int) bool {
		ca, cb := items[a].Category.Order(), items[b].Category.Order()
		if ca != cb {
			return ca < cb
		}
		return items[a].NameKana < items[b].NameKana
	})
}
