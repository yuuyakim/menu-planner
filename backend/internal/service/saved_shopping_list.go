package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ShoppingListDeriver は献立の集合から買い物リストを導出する。
// *ShoppingListService が満たす。差分の重ね合わせはこの導出結果に対して行う。
type ShoppingListDeriver interface {
	Build(ctx context.Context, ids []domain.MenuID) ([]ShoppingItem, error)
}

// SavedShoppingItem は保存済み週の買い物リストの1項目（差分適用後）。
//
// 導出品目（origin=derived）は Ingredient 由来の名前・カナ・カテゴリを持ち、
// 手動品目（origin=manual）は利用者が付けた名前とカテゴリを持つ（カナは名前で代用）。
type SavedShoppingItem struct {
	Name     string
	NameKana string
	Category domain.IngredientCategory
	Origin   domain.Origin
	Checked  bool
	// UsedIn はその食材を使う献立。手動品目では空。
	UsedIn []domain.Menu
}

// SavedShoppingListService は保存済み週の買い物リストを、差分を重ねて返す。
type SavedShoppingListService struct {
	deriver      ShoppingListDeriver
	saved        SavedWeeklyMenuStore
	overrides    ShoppingListOverrideStore
	entitlements Entitlements
}

// NewSavedShoppingListService は SavedShoppingListService を生成する。
func NewSavedShoppingListService(
	deriver ShoppingListDeriver, saved SavedWeeklyMenuStore,
	overrides ShoppingListOverrideStore, entitlements Entitlements,
) *SavedShoppingListService {
	return &SavedShoppingListService{
		deriver: deriver, saved: saved, overrides: overrides, entitlements: entitlements,
	}
}

// For は保存済み週の買い物リストを返す。
//
// 導出は毎回行い、premium のときだけ差分を重ねる。free では差分を無視するため
// 従来の買い物リストと同じ結果になる（設計 8.2）。
// 他人の週・存在しない週は ErrSavedWeeklyMenuNotFound（404）。
func (s *SavedShoppingListService) For(
	ctx context.Context, userID, savedWeeklyMenuID string,
) ([]SavedShoppingItem, error) {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	sid, err := domain.ParseSavedWeeklyMenuID(savedWeeklyMenuID)
	if err != nil {
		return nil, err
	}

	// 所有者検証も兼ねる。他人の週なら not found。
	week, err := s.saved.Find(ctx, uid, sid)
	if err != nil {
		return nil, err
	}

	ids := make([]domain.MenuID, 0, len(week.Days))
	for _, d := range week.Days {
		ids = append(ids, d.Menu.ID)
	}
	derived, err := s.deriver.Build(ctx, ids)
	if err != nil {
		return nil, err
	}

	base := make([]SavedShoppingItem, 0, len(derived))
	for _, it := range derived {
		base = append(base, SavedShoppingItem{
			Name:     it.Ingredient.Name,
			NameKana: it.Ingredient.NameKana,
			Category: it.Ingredient.Category,
			Origin:   domain.OriginDerived,
			UsedIn:   it.UsedIn,
		})
	}

	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !ent.CanPersistShoppingList() {
		sortSavedItems(base)
		return base, nil
	}

	overrides, err := s.overrides.FindBySavedWeeklyMenu(ctx, sid)
	if err != nil {
		return nil, err
	}
	return mergeOverlay(base, overrides), nil
}

// mergeOverlay は導出結果に差分を重ねる。
//
// 名前を項目の同一性とする。導出品目に同名の差分があればチェック/非表示を適用し、
// 導出に無い名前の手動品目を足す。hidden は表示から外す。
func mergeOverlay(base []SavedShoppingItem, overrides []domain.ShoppingListOverride) []SavedShoppingItem {
	byName := make(map[string]domain.ShoppingListOverride, len(overrides))
	for _, o := range overrides {
		byName[o.Name] = o
	}
	baseNames := make(map[string]bool, len(base))

	out := make([]SavedShoppingItem, 0, len(base)+len(overrides))
	for _, it := range base {
		baseNames[it.Name] = true
		if o, ok := byName[it.Name]; ok {
			if o.Hidden {
				continue // 消された導出品目は出さない
			}
			it.Checked = o.Checked
		}
		out = append(out, it)
	}
	for _, o := range overrides {
		if o.Origin != domain.OriginManual || o.Hidden || baseNames[o.Name] {
			continue
		}
		out = append(out, SavedShoppingItem{
			Name:     o.Name,
			NameKana: o.Name, // 手動品目はカナを持たないので名前で並べる
			Category: o.Category,
			Origin:   domain.OriginManual,
			Checked:  o.Checked,
		})
	}
	sortSavedItems(out)
	return out
}

// sortSavedItems はカテゴリ順→カナ順に並べる（既存 aggregate と同じ規則）。
func sortSavedItems(items []SavedShoppingItem) {
	sort.SliceStable(items, func(a, b int) bool {
		ca, cb := items[a].Category.Order(), items[b].Category.Order()
		if ca != cb {
			return ca < cb
		}
		return items[a].NameKana < items[b].NameKana
	})
}

// maxManualShoppingItems は1つの買い物リストに手で足せる品目の上限（設計 7.2）。
// 上限そのものに意味はないが、無制限の利用者由来レコードが Neon の無料枠を
// 圧迫しないための歯止め。
const maxManualShoppingItems = 100

var (
	// ErrPremiumRequired はプレミアムプランでのみ使える操作を free が試みたことを表す（403）。
	ErrPremiumRequired = errors.New("プレミアムプランが必要です")

	// ErrShoppingListItemLimitReached は手動品目が上限に達したことを表す（409）。
	ErrShoppingListItemLimitReached = errors.New("追加できる品目の上限に達しました")
)

// OverrideInput は差分1件の生の指定。APIの値をそのまま受ける（SavedDayInput と同じ流儀）。
type OverrideInput struct {
	Name     string
	Category string
	Origin   string
	Checked  bool
	Hidden   bool
}

// ReplaceOverrides は保存済み週の差分を丸ごと置き換える（設計 3.5）。
//
// free は ErrPremiumRequired（403）。他人の週は ErrSavedWeeklyMenuNotFound（404）。
// 手動品目が上限超なら ErrShoppingListItemLimitReached（409）。
// 名前の重複・不正なカテゴリ/由来は domain.ErrInvalidOverride（400）。
func (s *SavedShoppingListService) ReplaceOverrides(
	ctx context.Context, userID, savedWeeklyMenuID string, inputs []OverrideInput,
) error {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	sid, err := domain.ParseSavedWeeklyMenuID(savedWeeklyMenuID)
	if err != nil {
		return err
	}

	// 権限を先に見る。free には保存経路を一切開かない。
	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return err
	}
	if !ent.CanPersistShoppingList() {
		return ErrPremiumRequired
	}

	// 所有者検証。他人の週なら 404。
	if _, err := s.saved.Find(ctx, uid, sid); err != nil {
		return err
	}

	overrides := make([]domain.ShoppingListOverride, 0, len(inputs))
	seen := make(map[string]bool, len(inputs))
	manual := 0
	for _, in := range inputs {
		o := domain.ShoppingListOverride{
			SavedWeeklyMenuID: sid,
			Name:              strings.TrimSpace(in.Name),
			// Parse は使わず、Validate に不正値を弾かせる（不正は 400=ErrInvalidOverride に寄せる。
			// ParseIngredientCategory の ErrInvalidIngredientCategory は DB 読み取り用で 500 扱いのため）。
			Category: domain.IngredientCategory(strings.TrimSpace(in.Category)),
			Origin:   domain.Origin(strings.TrimSpace(in.Origin)),
			Checked:  in.Checked,
			Hidden:   in.Hidden,
		}
		if err := o.Validate(); err != nil {
			return err
		}
		if seen[o.Name] {
			return fmt.Errorf("%w: 品目名が重複しています: %q", domain.ErrInvalidOverride, o.Name)
		}
		seen[o.Name] = true
		if o.Origin == domain.OriginManual {
			manual++
		}
		overrides = append(overrides, o)
	}
	if manual > maxManualShoppingItems {
		return fmt.Errorf("%w: 追加できる品目は%d件までです", ErrShoppingListItemLimitReached, maxManualShoppingItems)
	}

	return s.overrides.Replace(ctx, sid, overrides)
}

// ShoppingListDeriver は *ShoppingListService が満たすことをコンパイル時に確認する。
var _ ShoppingListDeriver = (*ShoppingListService)(nil)
