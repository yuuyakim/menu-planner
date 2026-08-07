package service

import (
	"context"
	"errors"
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
	menus       MenuRepository
}

// NewIngredientService は IngredientService を生成する。
func NewIngredientService(i IngredientRepository, m MenuRepository) *IngredientService {
	return &IngredientService{ingredients: i, menus: m}
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

// maxIngredientSearchResults は食材からの検索で返す上限（spec.md 5.6）。
// 食材を多く選ぶほど候補は増えるが、一覧で見比べる用途でこれを超えても選べない。
const maxIngredientSearchResults = 20

var (
	// ErrInvalidIngredientIDs は食材の指定が0件であることを表す（400）。
	ErrInvalidIngredientIDs = errors.New("食材の指定が不正です")

	// ErrIngredientNotFound は指定された食材の中に存在しないものがあることを表す（404）。
	//
	// 黙って無視すると、利用者の意図と違う条件で検索した結果を
	// 正しい答えとして返すことになる。
	ErrIngredientNotFound = errors.New("指定された食材が見つかりません")
)

// MenuMatch は手持ちの食材に対する献立1件の当てはまり具合（spec.md 5.6）。
type MenuMatch struct {
	// Menu は候補の献立。
	Menu domain.Menu
	// Matched は手持ちと重なった食材。
	Matched []domain.Ingredient
	// Missing は足りない食材。買い足せば作れることを示す。
	Missing []domain.Ingredient
}

// MatchSort は候補の並び順（spec.md 5.6）。
//
// ゼロ値（"")は SortMissingAsc と同じ扱いにする。これにより
// つまみを指定しない呼び出しが現行と同じ挙動になる。
// 未知の値の拒否は handler が担う（値をここまで持ち込ませない）。
type MatchSort string

const (
	// SortMissingAsc は「不足の少ない順 → 一致の多い順 → カナ順」。既定。
	// 知りたいのが「いま作れるか」だから第1が不足になる。
	SortMissingAsc MatchSort = "missing_asc"

	// SortMatchedDesc は「一致の多い順 → 不足の少ない順 → カナ順」。
	// 手持ちを使い切りたい（買い足しを減らし、余らせたくない）とき。
	SortMatchedDesc MatchSort = "matched_desc"
)

// SearchByIngredientsInput は食材からの検索条件。
//
// 引数を並べず構造体にしているのは、ids に真偽値と列挙が続くと
// 呼び出し側でどれが何か読めなくなるため。
type SearchByIngredientsInput struct {
	// IngredientIDs は手持ちの食材。1件以上。重複は1件として扱う。
	IngredientIDs []domain.IngredientID
	// OnlyMakeable は true のとき、不足のある献立を返さない。
	OnlyMakeable bool
	// Sort は並び順。ゼロ値は SortMissingAsc。
	Sort MatchSort
}

// SearchByIngredientsResult は食材からの検索結果。
type SearchByIngredientsResult struct {
	// Matches は条件に合った候補。
	Matches []MenuMatch
	// NearMisses は「あと1品買えば作れる」候補。
	// OnlyMakeable かつ Matches が0件のときだけ埋まる。それ以外は空。
	NearMisses []MenuMatch
}

// SearchByIngredients は手持ちの食材で作れる献立を探す。
//
// 既定では完全一致に絞らない。献立1件の食材は平均4.4種で、それを全部持っている
// 状況はまれなため（spec.md 2.9）。in.OnlyMakeable が true のときだけ不足0に絞る。
func (s *IngredientService) SearchByIngredients(
	ctx context.Context, in SearchByIngredientsInput,
) (SearchByIngredientsResult, error) {
	empty := SearchByIngredientsResult{Matches: []MenuMatch{}, NearMisses: []MenuMatch{}}

	have := uniqueIngredientIDs(in.IngredientIDs)
	if len(have) == 0 {
		return empty, fmt.Errorf("%w: 1件以上指定してください", ErrInvalidIngredientIDs)
	}

	// 存在を先に確かめる。存在しないIDを黙って落とすと、
	// 利用者が選んだつもりの条件と違う結果を返してしまう。
	found, err := s.ingredients.FindByIDs(ctx, have)
	if err != nil {
		return empty, fmt.Errorf("食材の取得に失敗しました: %w", err)
	}
	if len(found) != len(have) {
		return empty, fmt.Errorf("%w: 指定%d件のうち%d件しか存在しません",
			ErrIngredientNotFound, len(have), len(found))
	}

	// 手持ちと1つも重ならない献立はここで落ちる（SQL側で絞る）。
	menuIDs, err := s.ingredients.FindMenuIDsByIngredientIDs(ctx, have)
	if err != nil {
		return empty, fmt.Errorf("献立の絞り込みに失敗しました: %w", err)
	}
	if len(menuIDs) == 0 {
		return empty, nil
	}

	// 不足を出すには、候補献立の「全食材」が要る。重なった分だけでは足りない。
	pairs, err := s.ingredients.FindByMenuIDs(ctx, menuIDs)
	if err != nil {
		return empty, fmt.Errorf("食材の取得に失敗しました: %w", err)
	}
	menus, err := s.menus.FindByIDs(ctx, menuIDs)
	if err != nil {
		return empty, fmt.Errorf("献立の取得に失敗しました: %w", err)
	}

	all := buildMatches(menus, pairs, have)

	matches := all
	if in.OnlyMakeable {
		matches = withMissingAtMost(all, 0)
	}
	sortMatches(matches, in.Sort)
	matches = truncateMatches(matches)

	// 行き止まりを防ぐ。ただし条件を勝手に外して再検索はしない
	// （「この中以外は出さない」という指定に反する）。
	// 0件だと明言した上で、別枠として「あと1品」を添えるだけにする。
	nearMisses := []MenuMatch{}
	if in.OnlyMakeable && len(matches) == 0 {
		// 不足0は上で0件だと分かっているので、1件以下＝ちょうど1件になる。
		nearMisses = withMissingAtMost(all, 1)
		// 不足はどれも1件で並ばないため、一致の多い順に固定する。
		sortMatches(nearMisses, SortMatchedDesc)
		nearMisses = truncateMatches(nearMisses)
	}

	return SearchByIngredientsResult{Matches: matches, NearMisses: nearMisses}, nil
}

// withMissingAtMost は不足が n 件以下の候補だけを新しいスライスで返す。
//
// 元のスライスと backing array を共有しないよう append で作り直す。
// 共有すると、返した側を並べ替えたときに元の並びまで崩れる。
func withMissingAtMost(matches []MenuMatch, n int) []MenuMatch {
	out := make([]MenuMatch, 0, len(matches))
	for _, m := range matches {
		if len(m.Missing) <= n {
			out = append(out, m)
		}
	}
	return out
}

// truncateMatches は上限（20件）で打ち切る。
//
// **必ず並べ替えの後に呼ぶ。** 手持ち50種なら不足0だけで132件出るため、
// 切ってから並べると上位を取りこぼす。
func truncateMatches(matches []MenuMatch) []MenuMatch {
	if len(matches) > maxIngredientSearchResults {
		return matches[:maxIngredientSearchResults]
	}
	return matches
}

// buildMatches は献立ごとに手持ちとの重なりと不足を組み立てる。
func buildMatches(
	menus []domain.Menu, pairs []MenuIngredient, have []domain.IngredientID,
) []MenuMatch {
	haveSet := make(map[domain.IngredientID]bool, len(have))
	for _, id := range have {
		haveSet[id] = true
	}

	byMenu := make(map[domain.MenuID][]domain.Ingredient, len(menus))
	for _, p := range pairs {
		byMenu[p.MenuID] = append(byMenu[p.MenuID], p.Ingredient)
	}

	matches := make([]MenuMatch, 0, len(menus))
	for _, m := range menus {
		matched := []domain.Ingredient{}
		missing := []domain.Ingredient{}
		for _, ing := range byMenu[m.ID] {
			if haveSet[ing.ID] {
				matched = append(matched, ing)
			} else {
				missing = append(missing, ing)
			}
		}
		// 重なりゼロはSQLで落ちているはずだが、ここでも守る。
		// 献立に食材が1件も紐づいていない場合もここで除ける。
		if len(matched) == 0 {
			continue
		}
		sortIngredientsForDisplay(matched)
		sortIngredientsForDisplay(missing)
		matches = append(matches, MenuMatch{Menu: m, Matched: matched, Missing: missing})
	}
	return matches
}

// sortMatches は候補を指定された並び順にする（spec.md 5.6）。
//
// カナ順は同値のときに並びを安定させるためだけのもの。
func sortMatches(matches []MenuMatch, by MatchSort) {
	if by == SortMatchedDesc {
		sort.SliceStable(matches, func(a, b int) bool {
			if ma, mb := len(matches[a].Matched), len(matches[b].Matched); ma != mb {
				return ma > mb
			}
			if la, lb := len(matches[a].Missing), len(matches[b].Missing); la != lb {
				return la < lb
			}
			return matches[a].Menu.NameKana < matches[b].Menu.NameKana
		})
		return
	}

	// 既定（SortMissingAsc とゼロ値）。第1が不足なのは、
	// 知りたいのが「いま作れるか」であって「手持ちを何品使うか」ではないため。
	sort.SliceStable(matches, func(a, b int) bool {
		if la, lb := len(matches[a].Missing), len(matches[b].Missing); la != lb {
			return la < lb
		}
		if ma, mb := len(matches[a].Matched), len(matches[b].Matched); ma != mb {
			return ma > mb
		}
		return matches[a].Menu.NameKana < matches[b].Menu.NameKana
	})
}

// uniqueIngredientIDs は重複を除く。順序は保つ。
func uniqueIngredientIDs(ids []domain.IngredientID) []domain.IngredientID {
	seen := make(map[domain.IngredientID]bool, len(ids))
	out := make([]domain.IngredientID, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
