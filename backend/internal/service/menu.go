package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrNoMenuFound は条件に合う献立が1件も無いことを表す。
// リクエスト自体は正しいので、呼び出し側はこれを 4xx として扱う。
var ErrNoMenuFound = errors.New("条件に合う献立が見つかりません")

// recipeCount は献立1件につき提示するレシピの件数（spec.md 2.3）。
const recipeCount = 3

// defaultRecipeBudget はレシピ取得全体に許す時間。
//
// gateway 単体の最悪は 3秒 × 3試行 + バックオフ ≒ 9.6秒（task.md 3-D）。
// 画面がそれだけ回るのは体験が悪いため、上限をここで課す。5秒あれば
// 1回目が失敗しても2回目の試行に入れる。
const defaultRecipeBudget = 5 * time.Second

// defaultRecipeCacheTTL はレシピのキャッシュを有効とみなす期間（spec.md 4.2 / 13.2）。
// レシピサイトのURLは頻繁には変わらないため長めに取る。
const defaultRecipeCacheTTL = 7 * 24 * time.Hour

// MenuService は献立の選定を担う。
type MenuService struct {
	repo         MenuRepository
	rand         Randomizer
	recipes      RecipeSearchGateway
	recipeCache  RecipeLinkCache
	recipeBudget time.Duration
	recipeTTL    time.Duration
	// now は現在時刻を返す。キャッシュの鮮度判定を固定できるよう外から差し替える。
	now func() time.Time
}

// MenuServiceOption は MenuService の任意設定。
type MenuServiceOption func(*MenuService)

// WithRecipeBudget はレシピ取得に許す時間を変える。
func WithRecipeBudget(d time.Duration) MenuServiceOption {
	return func(s *MenuService) { s.recipeBudget = d }
}

// WithRecipeCacheTTL はキャッシュを有効とみなす期間を変える。
func WithRecipeCacheTTL(d time.Duration) MenuServiceOption {
	return func(s *MenuService) { s.recipeTTL = d }
}

// WithNow は現在時刻の取得方法を変える。テストで時刻を固定するために使う。
func WithNow(now func() time.Time) MenuServiceOption {
	return func(s *MenuService) { s.now = now }
}

// NewMenuService は献立サービスを組み立てる。
func NewMenuService(repo MenuRepository, rand Randomizer, recipes RecipeSearchGateway, cache RecipeLinkCache, opts ...MenuServiceOption) *MenuService {
	s := &MenuService{
		repo:         repo,
		rand:         rand,
		recipes:      recipes,
		recipeCache:  cache,
		recipeBudget: defaultRecipeBudget,
		recipeTTL:    defaultRecipeCacheTTL,
		now:          time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// RecipeBudget はレシピ取得に許す時間を返す。
func (s *MenuService) RecipeBudget() time.Duration { return s.recipeBudget }

// RecipeCacheTTL はキャッシュを有効とみなす期間を返す。
func (s *MenuService) RecipeCacheTTL() time.Duration { return s.recipeTTL }

// SuggestMenu は条件に合う献立から1件を無作為に選んで返す。
// 条件が不正な場合は domain.ErrInvalidGenre / domain.ErrInvalidDifficulty を返す。
func (s *MenuService) SuggestMenu(ctx context.Context, f domain.MenuFilter) (*domain.Menu, error) {
	// 不正な条件をDBに投げても0件が返るだけで理由が分からないため、先に弾く。
	if err := f.Validate(); err != nil {
		return nil, err
	}

	candidates, err := s.repo.FindByFilter(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("献立の検索に失敗しました: %w", err)
	}

	menu, err := Pick(s.rand, candidates)
	switch {
	// 候補が無いのは障害ではなく「条件に合う献立が無い」という結果。
	// Pick の内部事情(ErrNoCandidates)は外に出さず、ドメインのエラーに変換する。
	case errors.Is(err, ErrNoCandidates):
		return nil, ErrNoMenuFound
	case err != nil:
		return nil, fmt.Errorf("献立の選択に失敗しました: %w", err)
	}

	return &menu, nil
}

// SuggestWeekly は7日分の献立を提案する（spec.md 2.2）。
// 起点は呼び出した日で、返る Day は起点からの通し番号 1..7（spec.md 13.3）。
//
// 同一献立は週内に2度出現しない。同ジャンルの連続回避は 4-C、候補が足りない
// 場合の緩和は 4-D で順に足す。現時点では候補が7件に満たないと ErrNoMenuFound
// になる。
func (s *MenuService) SuggestWeekly(ctx context.Context, f domain.MenuFilter) ([]domain.DayMenu, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}

	// 候補は一度だけ引いて使い回す。日ごとに問い合わせるとDBへの負荷が7倍になる。
	candidates, err := s.repo.FindByFilter(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("献立の検索に失敗しました: %w", err)
	}

	// 選んだ献立を候補から取り除いていくため、呼び出し元の値を壊さないよう複製する。
	remaining := slices.Clone(candidates)

	week := make([]domain.DayMenu, 0, domain.WeekLength)
	for day := 1; day <= domain.WeekLength; day++ {
		menu, err := Pick(s.rand, remaining)
		switch {
		case errors.Is(err, ErrNoCandidates):
			// 候補が無いのは障害ではなく「条件に合う献立が無い」という結果。
			return nil, ErrNoMenuFound
		case err != nil:
			return nil, fmt.Errorf("%d日目の献立の選択に失敗しました: %w", day, err)
		}
		week = append(week, domain.DayMenu{Day: day, Menu: menu})

		// 一度出した献立は以降の候補から外す。残りから選ばせることで、
		// 「引き直して重複なら再抽選」のような終わらない可能性のある処理を避ける。
		remaining = slices.DeleteFunc(remaining, func(m domain.Menu) bool {
			return m.ID == menu.ID
		})
	}
	return week, nil
}

// RecipeLinks は献立のレシピ掲載ページを最大3件返す。
// 献立が存在しない場合は repository のエラーを包んで返す（呼び出し側で 404）。
// 検索に失敗した場合は ErrRecipeSearchFailed を返す（同 502）。
//
// 結果が0件でもエラーにしない。該当が無いことは障害ではなく、
// 呼び出し側は空のレシピ欄として表示できる（spec.md 2.3）。
func (s *MenuService) RecipeLinks(ctx context.Context, id domain.MenuID) ([]domain.RecipeLink, error) {
	// 検索語は献立名なので、まず献立を引く。存在しないIDで外部APIを
	// 消費しないよう、ここで先に弾く。
	menu, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("献立の取得に失敗しました(id=%s): %w", id, err)
	}

	if links, ok := s.cachedLinks(ctx, id); ok {
		return links, nil
	}

	searchCtx, cancel := context.WithTimeout(ctx, s.recipeBudget)
	defer cancel()

	links, err := s.recipes.Search(searchCtx, menu.Name, recipeCount)
	if err != nil {
		// 呼び出し側の中断（利用者が画面を離れたなど）は失敗として扱わず、
		// そのまま返す。502 として記録する筋合いではないため。
		if ctx.Err() != nil {
			return nil, fmt.Errorf("レシピの取得を中断しました(%q): %w", menu.Name, ctx.Err())
		}
		// ここに来る時点で原因は外部API側。gateway は自身の締め切り超過を
		// 素の context エラーで返すため、こちらの締め切りによる打ち切りも
		// 含めて「検索が失敗した」に寄せる。
		if errors.Is(err, ErrRecipeSearchFailed) {
			return nil, fmt.Errorf("レシピの取得に失敗しました(%q): %w", menu.Name, err)
		}
		// 原因は文字列として添えるに留め、エラーとしては包まない。%w で包むと、
		// こちらが課した締め切りの DeadlineExceeded が呼び出し側まで伝播し、
		// 「利用者が中断した」との区別がつかなくなるため。
		return nil, fmt.Errorf("レシピの取得に失敗しました(%q): %w (原因: %s)", menu.Name, ErrRecipeSearchFailed, err.Error())
	}

	// 0件も「探した結果」なので保存する。保存しないと、該当の無い献立で
	// 毎回APIを消費することになる。
	//
	// 失敗を保存しないのは意図的。TTLが切れるまで失敗を返し続けてしまうため、
	// エラー時はここに到達しない。
	if err := s.recipeCache.Save(ctx, id, links, s.now()); err != nil {
		// キャッシュは高速化の手段であって、保存できないことは
		// 検索結果を捨てる理由にならない。
		slog.Warn("レシピのキャッシュを保存できませんでした",
			"menu_id", id, "menu_name", menu.Name, "error", err)
	}
	return links, nil
}

// cachedLinks は有効なキャッシュがあればそれを返す。
//
// キャッシュは高速化の手段であって、読めないことはリクエストを失敗させる
// 理由にならない。障害も期限切れも「無い」として扱い、検索APIに任せる。
func (s *MenuService) cachedLinks(ctx context.Context, id domain.MenuID) ([]domain.RecipeLink, bool) {
	cached, err := s.recipeCache.Find(ctx, id)
	switch {
	case errors.Is(err, ErrRecipeCacheMiss):
		return nil, false
	case err != nil:
		slog.Warn("レシピのキャッシュを読めませんでした。検索APIに問い合わせます",
			"menu_id", id, "error", err)
		return nil, false
	}

	// 取得から TTL を過ぎたものは使わない。ちょうど TTL の時点はまだ有効とする。
	if s.now().Sub(cached.FetchedAt) > s.recipeTTL {
		return nil, false
	}
	return cached.Links, true
}

// GetMenu はIDで献立を1件返す。
// 献立が存在しない場合とDB障害は repository が別のエラーで表現しており、
// 呼び出し側が 404 と 500 を出し分けられるよう、原因を包んだまま返す。
func (s *MenuService) GetMenu(ctx context.Context, id domain.MenuID) (*domain.Menu, error) {
	menu, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("献立の取得に失敗しました(id=%s): %w", id, err)
	}
	return menu, nil
}
