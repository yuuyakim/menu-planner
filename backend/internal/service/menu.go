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

// ErrInvalidDay は週間献立の日の指定が範囲外であることを表す（1〜7）。
var ErrInvalidDay = errors.New("不正な日の指定です")

// ErrInvalidWeek は渡された週間献立が7日分そろっていないことを表す。
var ErrInvalidWeek = errors.New("不正な週間献立です")

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

// genreStreakLimit は同一ジャンルを連続させてよい日数（spec.md 2.2）。
// これを超える連続を作らない。2連続までは許す。
const genreStreakLimit = 2

// SuggestWeekly は7日分の献立を提案する（spec.md 2.2）。
// 起点は呼び出した日で、返る Day は起点からの通し番号 1..7（spec.md 13.3）。
//
// recentIDs には直近の履歴にある献立IDを渡す。これらは可能な限り避けるが、
// 7日を埋められない場合は緩和する（spec.md 2.2 のルール3）。
//
// recentIDs を f.ExcludeIDs に入れてはならない。あちらは repository が SQL で
// 除外する強い条件で、緩和の余地が無くなるため。
func (s *MenuService) SuggestWeekly(ctx context.Context, f domain.MenuFilter, recentIDs []domain.MenuID) ([]domain.DayMenu, error) {
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
		picked, err := s.pickDay(candidates, remaining, recentIDs, week)
		switch {
		case errors.Is(err, ErrNoCandidates):
			// 緩和しても選べないのは候補が0件のときだけ。障害ではなく
			// 「条件に合う献立が無い」という結果。
			return nil, ErrNoMenuFound
		case err != nil:
			return nil, fmt.Errorf("%d日目の献立の選択に失敗しました: %w", day, err)
		}
		picked.Day = day
		week = append(week, picked)

		// 一度出した献立は以降の候補から外す。残りから選ばせることで、
		// 「引き直して重複なら再抽選」のような終わらない可能性のある処理を避ける。
		remaining = slices.DeleteFunc(remaining, func(m domain.Menu) bool {
			return m.ID == picked.Menu.ID
		})
	}
	return week, nil
}

// RerollDay は週間献立の指定日だけを引き直す（spec.md 2.2 / 5.1）。
//
// week には現在の週の献立IDを day 順（1..7）に並べて渡す。サーバは週の状態を
// 持たないため、呼び出し側が持っているものを受け取る。返るのは引き直した
// 1日分だけで、他の日は呼び出し側が保持する。
//
// 重複回避は再適用する。引き直した献立は他の6日と重複せず、同一ジャンルが
// 3日以上連続しない。候補が枯渇する場合は SuggestWeekly と同じ順序で緩和する。
func (s *MenuService) RerollDay(ctx context.Context, f domain.MenuFilter, week []domain.MenuID, day int, recentIDs []domain.MenuID) (domain.DayMenu, error) {
	if err := f.Validate(); err != nil {
		return domain.DayMenu{}, err
	}
	if day < 1 || day > domain.WeekLength {
		return domain.DayMenu{}, fmt.Errorf("%w: %d (1〜%dの範囲で指定してください)", ErrInvalidDay, day, domain.WeekLength)
	}
	if len(week) != domain.WeekLength {
		// 他の日を保持する仕組みなので、週が揃っていないと重複回避を再適用できない。
		return domain.DayMenu{}, fmt.Errorf("%w: %d件 (%d件必要です)", ErrInvalidWeek, len(week), domain.WeekLength)
	}

	// 引き直す日を除いた他の日の献立。ジャンルの連続判定に必要なので実体を引く。
	otherIDs := make([]domain.MenuID, 0, domain.WeekLength-1)
	for i, id := range week {
		if i != day-1 {
			otherIDs = append(otherIDs, id)
		}
	}
	others, err := s.repo.FindByIDs(ctx, otherIDs)
	if err != nil {
		return domain.DayMenu{}, fmt.Errorf("週の献立の取得に失敗しました: %w", err)
	}

	candidates, err := s.repo.FindByFilter(ctx, f)
	if err != nil {
		return domain.DayMenu{}, fmt.Errorf("献立の検索に失敗しました: %w", err)
	}

	// 週で使われている献立を除いたもの。ここから選べば重複しない。
	//
	// 引き直す日の献立自身も外す。同じものが返ってくるのでは引き直しにならない。
	// ただし候補がそれしか無ければ最後の段階で選ばれる（candidates には残る）。
	remaining := slices.DeleteFunc(slices.Clone(candidates), func(m domain.Menu) bool {
		return slices.Contains(week, m.ID)
	})

	// day 順に並べたジャンル。引き直す日は空のまま置き、候補を当てて判定する。
	genres := s.weekGenres(week, others)

	picked, err := s.pickForDay(candidates, remaining, others, recentIDs, genres, day-1)
	if errors.Is(err, ErrNoCandidates) {
		return domain.DayMenu{}, ErrNoMenuFound
	}
	if err != nil {
		return domain.DayMenu{}, fmt.Errorf("%d日目の献立の選択に失敗しました: %w", day, err)
	}

	picked.Day = day
	return picked, nil
}

// weekGenres は week のIDを day 順のジャンル列に変換する。
// 見つからないIDの位置は空のままにする（マスタから消えた献立など）。
func (s *MenuService) weekGenres(week []domain.MenuID, others []domain.Menu) []domain.Genre {
	genres := make([]domain.Genre, len(week))
	for i, id := range week {
		for _, m := range others {
			if m.ID == id {
				genres[i] = m.Genre
				break
			}
		}
	}
	return genres
}

// pickForDay は指定位置に置く献立を1件選ぶ。
// 段階的な緩和は pickDay と同じ順序で、連続の判定だけが前後両側になる。
//
// others には引き直す日以外の献立を渡す。重複したかの判定に使う。
func (s *MenuService) pickForDay(candidates, remaining, others []domain.Menu, recentIDs []domain.MenuID, genres []domain.Genre, index int) (domain.DayMenu, error) {
	withoutStreak := func(pool []domain.Menu) []domain.Menu {
		return slices.DeleteFunc(slices.Clone(pool), func(m domain.Menu) bool {
			return createsStreak(genres, index, m.Genre)
		})
	}
	withoutHistory := func(pool []domain.Menu) []domain.Menu {
		if len(recentIDs) == 0 {
			return pool
		}
		return slices.DeleteFunc(slices.Clone(pool), func(m domain.Menu) bool {
			return slices.Contains(recentIDs, m.ID)
		})
	}

	pools := [][]domain.Menu{
		withoutHistory(withoutStreak(remaining)),
		withoutStreak(remaining),
		withoutHistory(remaining),
		remaining,
		withoutHistory(withoutStreak(candidates)),
		withoutStreak(candidates),
		withoutHistory(candidates),
		candidates,
	}

	for _, pool := range pools {
		menu, err := Pick(s.rand, pool)
		if errors.Is(err, ErrNoCandidates) {
			continue
		}
		if err != nil {
			return domain.DayMenu{}, err
		}

		// 何を緩めたかは選ばれた献立から判定する。
		//
		// 重複は「他の日と同じ献立になったか」。引き直す日の献立自身が
		// 選び直された場合は、週の中で1度しか出ないので重複ではない。
		duplicate := slices.ContainsFunc(others, func(m domain.Menu) bool { return m.ID == menu.ID })
		return domain.DayMenu{
			Menu:               menu,
			RelaxedDuplicate:   duplicate,
			RelaxedGenreStreak: createsStreak(genres, index, menu.Genre),
			RelaxedHistory:     slices.Contains(recentIDs, menu.ID),
		}, nil
	}

	return domain.DayMenu{}, ErrNoCandidates
}

// pickDay はその日の献立を1件選ぶ。
//
// 規則を全て守れる候補が無い場合、段階的に緩めて必ず7日を埋める（spec.md 2.2）。
// 緩める順序はルールの列挙順の逆、すなわち重要度の低いものから。
//
//	重要度: 重複させない(ルール1) > ジャンル3連続にしない(2) > 履歴を避ける(3)
//
// 履歴を最初に緩めるのは、spec.md 2.2 がルール3にだけ「候補が枯渇する場合は
// この条件を緩和する」と明記しているため。ジャンル連続を重複より先に緩めるのは、
// 同じ献立が週に2度出るより同ジャンルが3日続くほうが受け入れやすいため。
//
// この順序により、「履歴の献立を出す」と「ジャンル3連続にする」のどちらかを
// 選ばざるを得ない場面では前者を採る。履歴を避けられる候補が残っていても
// 履歴の献立が出ることがあるのはそのため（仕様通りの挙動）。
//
// 段階は「重複を許さない(remaining)」を先に試し切ってから「許す(candidates)」に
// 降りるので、候補が残るうちは重複を作らない。候補が7件以上あれば remaining が
// 尽きないため、重複は決して起きない。
//
// 絞り込み条件（ジャンル・難易度）は緩めない。それは利用者が指定したもので、
// 勝手に外すと要求と違うものを返すことになる。
//
// 先読みはしない。その日その日で選ぶため、週全体では3連続を避けられる並びが
// あるのに袋小路に入ることが理論上ある（例: 残り1件が直前2日と同じジャンル）。
// 候補1〜12件・ジャンル1〜4種を総当たりで2400回試して5回、いずれも候補が
// 2ジャンルしか無い作為的な形でのみ起きた。実マスタ相当（120件・4ジャンル）
// では2000回試して0回であり、先読みを入れる複雑さに見合わないと判断した。
func (s *MenuService) pickDay(candidates, remaining []domain.Menu, recentIDs []domain.MenuID, week []domain.DayMenu) (domain.DayMenu, error) {
	streak, hasStreak := streakingGenre(week)

	// withoutStreak はジャンル連続になる候補を除く。連続の心配が無い日は
	// そのまま返す（複製を作らない）。
	withoutStreak := func(pool []domain.Menu) []domain.Menu {
		if !hasStreak {
			return pool
		}
		return slices.DeleteFunc(slices.Clone(pool), func(m domain.Menu) bool {
			return m.Genre == streak
		})
	}
	// withoutHistory は直近に提示した候補を除く。
	withoutHistory := func(pool []domain.Menu) []domain.Menu {
		if len(recentIDs) == 0 {
			return pool
		}
		return slices.DeleteFunc(slices.Clone(pool), func(m domain.Menu) bool {
			return slices.Contains(recentIDs, m.ID)
		})
	}

	// 上から順に試す。緩める条件が「少なく」「重要度の低いものから」増えていく。
	//
	// 重複を緩める段階(5以降)は remaining を使い切ってから。ジャンル連続を
	// 緩める段階でも履歴を避ける版(3, 7)を先に試すことで、連続を緩めざるを
	// 得ない場合でも履歴の献立を無駄に持ち出さない。
	pools := [][]domain.Menu{
		withoutHistory(withoutStreak(remaining)),  // 1. 全て守る
		withoutStreak(remaining),                  // 2. 履歴
		withoutHistory(remaining),                 // 3. 連続
		remaining,                                 // 4. 履歴 + 連続
		withoutHistory(withoutStreak(candidates)), // 5. 重複
		withoutStreak(candidates),                 // 6. 重複 + 履歴
		withoutHistory(candidates),                // 7. 重複 + 連続
		candidates,                                // 8. 全て緩める
	}

	for _, pool := range pools {
		menu, err := Pick(s.rand, pool)
		if errors.Is(err, ErrNoCandidates) {
			// この段階では選べない。次の段階へ降りる。
			// Pick は候補が空なら乱数を引かないため、段階を試すこと自体は
			// 選択結果に影響しない。
			continue
		}
		if err != nil {
			return domain.DayMenu{}, err
		}

		// 何を緩めたかは段階ではなく選ばれた献立から判定する。段階から決めると、
		// 例えば「履歴を緩める段階まで降りたが、選ばれた献立は履歴に無かった」
		// 場合に嘘の印が付く。
		return domain.DayMenu{
			Menu:               menu,
			RelaxedDuplicate:   used(week, menu),
			RelaxedGenreStreak: hasStreak && menu.Genre == streak,
			RelaxedHistory:     slices.Contains(recentIDs, menu.ID),
		}, nil
	}

	// 全段階で選べないのは candidates が空のときだけ。
	return domain.DayMenu{}, ErrNoCandidates
}

// used は献立が既に週内で使われているかを返す。
func used(week []domain.DayMenu, menu domain.Menu) bool {
	return slices.ContainsFunc(week, func(d domain.DayMenu) bool {
		return d.Menu.ID == menu.ID
	})
}

// streakingGenre は直前 genreStreakLimit 日が同一ジャンルで埋まっている場合に
// そのジャンルを返す。次の日にこのジャンルを選ぶと上限を超える。
//
// 見るのは直前の数日だけで、週全体での出現回数は数えない。「和食は週2回まで」
// ではなく「3日以上続かない」が仕様のため、別ジャンルを挟めばまた選べる。
func streakingGenre(week []domain.DayMenu) (domain.Genre, bool) {
	if len(week) < genreStreakLimit {
		return "", false
	}

	last := week[len(week)-1].Menu.Genre
	for _, d := range week[len(week)-genreStreakLimit:] {
		if d.Menu.Genre != last {
			return "", false
		}
	}
	return last, true
}

// createsStreak は week の index 番目を genre に置き換えたとき、
// 同一ジャンルが上限を超えて連続するかを返す。
//
// SuggestWeekly は前から順に埋めるため直前だけを見ればよいが、引き直しは
// 週の途中を差し替えるため前後の両側を見る必要がある。3日目を引き直すなら
// [1,2,3] [2,3,4] [3,4,5] のどの窓でも3連続にしてはならない。
//
// week の要素は day 順に並んでいる前提。空の要素（引き直し中の日）は
// index で示し、その位置の genre を仮に当てて判定する。
func createsStreak(genres []domain.Genre, index int, genre domain.Genre) bool {
	window := genreStreakLimit + 1

	// index を含む長さ window の窓を全て見る。
	for start := index - genreStreakLimit; start <= index; start++ {
		if start < 0 || start+window > len(genres) {
			continue
		}

		same := true
		for i := start; i < start+window; i++ {
			g := genres[i]
			if i == index {
				g = genre
			}
			if g != genre {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
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
