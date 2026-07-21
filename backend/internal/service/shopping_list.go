package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// maxShoppingListMenus は買い物リストにまとめられる献立の上限。
// 週間献立が7日分なので、それに合わせる（spec.md 5.5）。
const maxShoppingListMenus = 7

var (
	// ErrInvalidMenuIDs は献立の指定が0件、または上限を超えていることを表す。
	// 呼び出し側はこれを 400 に変換する。
	ErrInvalidMenuIDs = errors.New("献立の指定が不正です")

	// ErrMenuNotFoundInList は指定された献立の中に存在しないものがあることを表す。
	// 呼び出し側はこれを 404 に変換する。
	ErrMenuNotFoundInList = errors.New("指定された献立が見つかりません")
)

// ShoppingItem は買い物リストの1項目。
type ShoppingItem struct {
	// Ingredient は買う食材。
	Ingredient domain.Ingredient
	// UsedIn はその食材を使う献立。分量を持たない設計（spec.md 14.2）の補償として、
	// 「何のために買うか」を利用者が判断できるようにする。
	UsedIn []domain.Menu
}

// ShoppingListService は献立の集合から買い物リストを組み立てる。
type ShoppingListService struct {
	menus       MenuRepository
	ingredients IngredientRepository
}

// NewShoppingListService は ShoppingListService を生成する。
func NewShoppingListService(m MenuRepository, i IngredientRepository) *ShoppingListService {
	return &ShoppingListService{menus: m, ingredients: i}
}

// Build は指定した献立の食材をまとめた買い物リストを返す。
//
// 同じ食材が複数の献立に出てきたら1件にまとめ、UsedIn にその献立を並べる。
// 並びはカテゴリ順、同カテゴリ内はカナ順（売り場を回る順に近づける）。
func (s *ShoppingListService) Build(ctx context.Context, ids []domain.MenuID) ([]ShoppingItem, error) {
	unique := uniqueMenuIDs(ids)
	if len(unique) == 0 || len(unique) > maxShoppingListMenus {
		return nil, fmt.Errorf("%w: %d件（1〜%d件で指定してください）",
			ErrInvalidMenuIDs, len(unique), maxShoppingListMenus)
	}

	// 献立の存在を先に確かめる。**黙って無視すると、頼んだ献立の食材が
	// 抜けたリストを渡してしまう**ため、1件でも欠けていればエラーにする。
	menus, err := s.menus.FindByIDs(ctx, unique)
	if err != nil {
		return nil, fmt.Errorf("献立の取得に失敗しました: %w", err)
	}
	if len(menus) != len(unique) {
		return nil, fmt.Errorf("%w: 指定%d件のうち%d件しか存在しません",
			ErrMenuNotFoundInList, len(unique), len(menus))
	}

	byID := make(map[domain.MenuID]domain.Menu, len(menus))
	for _, m := range menus {
		byID[m.ID] = m
	}

	// 献立ごとに引くと件数分の往復になるため、まとめて1回で取る。
	pairs, err := s.ingredients.FindByMenuIDs(ctx, unique)
	if err != nil {
		return nil, fmt.Errorf("食材の取得に失敗しました: %w", err)
	}

	return aggregate(pairs, byID), nil
}

// uniqueMenuIDs は重複を除いた献立IDを、渡された順で返す。
// 順序を保つのは、同じ入力で同じ結果を返せるようにするため。
func uniqueMenuIDs(ids []domain.MenuID) []domain.MenuID {
	seen := make(map[domain.MenuID]struct{}, len(ids))
	out := make([]domain.MenuID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// aggregate は献立×食材の対応を食材ごとにまとめ、表示順に並べる。
func aggregate(pairs []MenuIngredient, menus map[domain.MenuID]domain.Menu) []ShoppingItem {
	index := make(map[domain.IngredientID]*ShoppingItem, len(pairs))
	order := make([]domain.IngredientID, 0, len(pairs))

	for _, p := range pairs {
		menu, ok := menus[p.MenuID]
		if !ok {
			// 存在確認済みなので通常は起きない。取りこぼしても
			// 「使う献立が分からない食材」を出すよりは落とす。
			continue
		}
		item, ok := index[p.Ingredient.ID]
		if !ok {
			index[p.Ingredient.ID] = &ShoppingItem{
				Ingredient: p.Ingredient,
				UsedIn:     []domain.Menu{menu},
			}
			order = append(order, p.Ingredient.ID)
			continue
		}
		// 同じ献立が二重に入らないようにする（重複IDを除いていても、
		// 1献立に同じ食材が2行ある異常データへの備え）。
		if !containsMenu(item.UsedIn, menu.ID) {
			item.UsedIn = append(item.UsedIn, menu)
		}
	}

	items := make([]ShoppingItem, 0, len(order))
	for _, id := range order {
		items = append(items, *index[id])
	}

	// カテゴリ順 → カナ順。repository がカナ順で返すため、
	// 安定ソートならカテゴリだけで並べ替えれば (カテゴリ, カナ) 順になるが、
	// 並び順を repository の実装に依存させないよう、ここで両方指定する。
	sort.SliceStable(items, func(a, b int) bool {
		ca, cb := items[a].Ingredient.Category.Order(), items[b].Ingredient.Category.Order()
		if ca != cb {
			return ca < cb
		}
		return items[a].Ingredient.NameKana < items[b].Ingredient.NameKana
	})
	return items
}

// containsMenu は献立が既に含まれているかを返す。
func containsMenu(menus []domain.Menu, id domain.MenuID) bool {
	for _, m := range menus {
		if m.ID == id {
			return true
		}
	}
	return false
}
