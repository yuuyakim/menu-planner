package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// APIBasePath は全APIエンドポイントの接頭辞。
const APIBasePath = "/api/v1"

// MenuSuggester は献立の提案を抽象化する。実装は service.MenuService。
// handler が必要とする操作だけを宣言することで、テストで差し替えられるようにする。
type MenuSuggester interface {
	SuggestMenu(ctx context.Context, f domain.MenuFilter) (*domain.Menu, error)
}

// MenuGetter は献立の取得を抽象化する。実装は service.MenuService。
type MenuGetter interface {
	GetMenu(ctx context.Context, id domain.MenuID) (*domain.Menu, error)
}

// MenuRecipeLister は献立のレシピ取得を抽象化する。実装は service.MenuService。
type MenuRecipeLister interface {
	RecipeLinks(ctx context.Context, id domain.MenuID) ([]domain.RecipeLink, error)
}

// MenuWeeklySuggester は週間献立の提案を抽象化する。実装は service.MenuService。
type MenuWeeklySuggester interface {
	SuggestWeekly(ctx context.Context, f domain.MenuFilter, recentIDs []domain.MenuID) ([]domain.DayMenu, error)
}

// MenuDayReroller は週間献立の1日の引き直しを抽象化する。実装は service.MenuService。
type MenuDayReroller interface {
	RerollDay(ctx context.Context, f domain.MenuFilter, week []domain.MenuID, day int, recentIDs []domain.MenuID) (domain.DayMenu, error)
}

// MenuUseCase は献立APIが必要とする操作をまとめたもの。
type MenuUseCase interface {
	MenuSuggester
	MenuGetter
	MenuRecipeLister
	MenuWeeklySuggester
	MenuDayReroller
}

// MenuHistory は献立検索と履歴の連携を抽象化する。実装は service.HistoryService。
// 献立検索は未認証でも使えるため、履歴は「ログイン中なら活かす」補助的な依存。
type MenuHistory interface {
	// RecentMenuIDs は直近履歴の献立ID（除外候補）を返す。
	RecentMenuIDs(ctx context.Context, userID string) ([]domain.MenuID, error)
	// Record は提案した献立を履歴に記録する。
	Record(ctx context.Context, userID string, menuID domain.MenuID, mode domain.SearchMode) error
	// RecordMany は週間献立の複数件を履歴に記録する。
	RecordMany(ctx context.Context, userID string, menuIDs []domain.MenuID, mode domain.SearchMode) error
}

// MenuHandler は献立APIの受け口。
type MenuHandler struct {
	svc     MenuUseCase
	history MenuHistory
	tokens  *auth.JWT
}

// NewMenuHandler は MenuHandler を生成する。
// history / tokens はログイン中の利用者に履歴を連携するために使う。
func NewMenuHandler(s MenuUseCase, history MenuHistory, tokens *auth.JWT) *MenuHandler {
	return &MenuHandler{svc: s, history: history, tokens: tokens}
}

// RegisterRoutes は献立APIのルーティングを登録する。
// パスの定義をハンドラと同じ場所に置き、テストで実際のパスごと検証できるようにする。
//
// OptionalAuth は履歴を活かす提案系のルートにだけ個別に付ける。グループ全体に
// 付けると echo の method-not-allowed 判定が変わり、未対応メソッドが 405 ではなく
// 404 になってしまうため（param ルート /menus/:id との兼ね合い）。未認証でも 200 の
// まま使え、ログイン中なら userID をコンテキストから取れる（履歴の除外・記録に使う）。
func (h *MenuHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	optional := OptionalAuth(h.tokens)
	// /menus/suggest は /menus/:id と同じ階層にあるが、echo は静的なパスを
	// パラメータより優先して照合するため、:id に飲み込まれることはない。
	g.GET("/menus/suggest", h.Suggest, optional)
	// 状態は変えないが、条件をボディで受けるため POST（spec.md 5.1）。
	g.POST("/menus/suggest-weekly", h.SuggestWeekly, optional)
	g.POST("/menus/reroll-day", h.RerollDay, optional)
	g.GET("/menus/:id", h.Get)
	g.GET("/menus/:id/recipes", h.Recipes)
}

// menuDTO は献立のAPI表現。
// ドメインの Menu をそのまま返すと内部の項目（name_kana など）が
// APIの契約に漏れるため、外に出す項目をここで明示する。
type menuDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Genre       string `json:"genre"`
	Difficulty  string `json:"difficulty"`
	Description string `json:"description"`
}

// menuResponse は献立1件を返すエンドポイントの共通レスポンス。
// 献立を menu キーで包むのは、後から項目を足してもレスポンスの形が壊れないようにするため。
// suggest と :id で同じ型を使い、片方だけ形が変わることを防ぐ。
type menuResponse struct {
	Menu menuDTO `json:"menu"`
}

// Suggest は条件に合う献立を1件提案する。
//
//	GET /api/v1/menus/suggest?genre=&difficulty=
//
// ログイン中なら直近履歴の献立を避け、提案結果を履歴に記録する。
// 未認証なら履歴には触れず、これまで通り提案だけを返す。
func (h *MenuHandler) Suggest(c echo.Context) error {
	f, err := parseMenuFilter(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	userID, authed := UserIDFromContext(c)

	// ログイン中は直近履歴を除外候補に足す。
	base := f
	if authed {
		if recent, err := h.history.RecentMenuIDs(ctx, userID); err != nil {
			// 履歴の取得失敗で提案を止めない。除外なしで続ける。
			slog.Warn("直近履歴の取得に失敗しました", "error", err)
		} else {
			f.ExcludeIDs = append(f.ExcludeIDs, recent...)
		}
	}

	menu, err := h.svc.SuggestMenu(ctx, f)
	// 履歴除外で候補が尽きたら、除外を外して再試行する（履歴は可能な限り避けるが、
	// 避けた結果1件も出せないなら履歴の献立を出す。spec.md 2.2 のルール3）。
	if errors.Is(err, service.ErrNoMenuFound) && len(f.ExcludeIDs) > len(base.ExcludeIDs) {
		menu, err = h.svc.SuggestMenu(ctx, base)
	}
	if err != nil {
		return err
	}

	if authed {
		// 記録の失敗で提案を失敗させない。ログだけ残す。
		if err := h.history.Record(ctx, userID, menu.ID, domain.SearchModeSingle); err != nil {
			slog.Warn("履歴の記録に失敗しました", "error", err)
		}
	}

	return c.JSON(http.StatusOK, menuResponse{Menu: toMenuDTO(*menu)})
}

// Get はIDで献立の詳細を返す。
//
//	GET /api/v1/menus/:id
func (h *MenuHandler) Get(c echo.Context) error {
	// 不正なIDをDBに投げても0件が返るだけで理由が分からないため、先に弾く。
	id, err := domain.ParseMenuID(c.Param("id"))
	if err != nil {
		return err
	}

	menu, err := h.svc.GetMenu(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, menuResponse{Menu: toMenuDTO(*menu)})
}

// parseMenuFilter はクエリ文字列を絞り込み条件に変換する。
// 未指定と空文字はどちらも「絞り込まない」として扱う。
// フロントが未選択を genre= として送ることがあり、それを不正入力にはしないため。
func parseMenuFilter(c echo.Context) (domain.MenuFilter, error) {
	var f domain.MenuFilter

	if s := c.QueryParam("genre"); s != "" {
		g, err := domain.ParseGenre(s)
		if err != nil {
			// 受け取った値を添えて、どれが弾かれたのか呼び出し側が分かるようにする。
			return f, fmt.Errorf("%w: %q", err, s)
		}
		f.Genre = &g
	}

	if s := c.QueryParam("difficulty"); s != "" {
		d, err := domain.ParseDifficulty(s)
		if err != nil {
			return f, fmt.Errorf("%w: %q", err, s)
		}
		f.Difficulty = &d
	}

	return f, nil
}

// weeklyRequest は POST /menus/suggest-weekly のリクエスト（spec.md 5.1）。
// 未指定と null を区別しないため、どちらもポインタの nil として受ける。
type weeklyRequest struct {
	Genre      *string `json:"genre"`
	Difficulty *string `json:"difficulty"`
}

// dayMenuDTO は週間献立の1日分のAPI表現。
type dayMenuDTO struct {
	Day  int     `json:"day"`
	Menu menuDTO `json:"menu"`
}

// weeklyResponse は POST /menus/suggest-weekly のレスポンス。
type weeklyResponse struct {
	Week []dayMenuDTO `json:"week"`
}

// SuggestWeekly は7日分の献立を提案する。
//
//	POST /api/v1/menus/suggest-weekly
//
// 緩和が起きたか（domain.DayMenu.Relaxed*）は返さない。spec.md 5.1 の
// レスポンスに無いため。画面に出す必要が生じたら仕様から決める。
func (h *MenuHandler) SuggestWeekly(c echo.Context) error {
	f, err := parseWeeklyRequest(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	userID, authed := UserIDFromContext(c)

	// ログイン中は直近履歴を渡す。SuggestWeekly は候補が枯渇すれば履歴除外を
	// 緩和する（4-E）ので、単発検索と違ってハード除外ではなく第3引数で渡す。
	var recent []domain.MenuID
	if authed {
		if recent, err = h.history.RecentMenuIDs(ctx, userID); err != nil {
			slog.Warn("直近履歴の取得に失敗しました", "error", err)
			recent = nil
		}
	}

	week, err := h.svc.SuggestWeekly(ctx, f, recent)
	if err != nil {
		return err
	}

	days := make([]dayMenuDTO, 0, len(week))
	menuIDs := make([]domain.MenuID, 0, len(week))
	for _, d := range week {
		days = append(days, dayMenuDTO{Day: d.Day, Menu: toMenuDTO(d.Menu)})
		menuIDs = append(menuIDs, d.Menu.ID)
	}

	if authed {
		// 7件を1トランザクションでまとめて記録（6-C）。失敗しても提案は返す。
		if err := h.history.RecordMany(ctx, userID, menuIDs, domain.SearchModeWeekly); err != nil {
			slog.Warn("週間献立の履歴記録に失敗しました", "error", err)
		}
	}

	return c.JSON(http.StatusOK, weeklyResponse{Week: days})
}

// parseWeeklyRequest はリクエストボディを絞り込み条件に変換する。
func parseWeeklyRequest(c echo.Context) (domain.MenuFilter, error) {
	var f domain.MenuFilter

	// ボディが空でも条件なしの提案として扱う。空を不正にすると素直な使い方が
	// 弾かれるため、Bind の失敗だけを 400 にする。
	var req weeklyRequest
	if c.Request().ContentLength > 0 {
		if err := c.Bind(&req); err != nil {
			return f, err
		}
	}

	return toMenuFilter(req.Genre, req.Difficulty)
}

// toMenuFilter はボディで受けた条件を絞り込み条件に変換する。
// 未指定・null・空文字はいずれも「絞り込まない」。GET /menus/suggest と
// 揃えておく（フロントが未選択を空文字で送ることがある）。
func toMenuFilter(genre, difficulty *string) (domain.MenuFilter, error) {
	var f domain.MenuFilter

	if genre != nil && *genre != "" {
		g, err := domain.ParseGenre(*genre)
		if err != nil {
			return f, fmt.Errorf("%w: %q", err, *genre)
		}
		f.Genre = &g
	}
	if difficulty != nil && *difficulty != "" {
		d, err := domain.ParseDifficulty(*difficulty)
		if err != nil {
			return f, fmt.Errorf("%w: %q", err, *difficulty)
		}
		f.Difficulty = &d
	}
	return f, nil
}

// rerollRequest は POST /menus/reroll-day のリクエスト（spec.md 5.1）。
type rerollRequest struct {
	// Day は引き直す日（1..7）。
	Day int `json:"day"`
	// Week は現在の週の献立IDを day 順に並べたもの。サーバは週の状態を
	// 持たないため、呼び出し側が持っているものを受け取る。
	Week       []string `json:"week"`
	Genre      *string  `json:"genre"`
	Difficulty *string  `json:"difficulty"`
}

// rerollResponse は POST /menus/reroll-day のレスポンス。
// 引き直した1日分だけを返し、他の日は呼び出し側が保持する。
type rerollResponse struct {
	Menu menuDTO `json:"menu"`
}

// RerollDay は週間献立の指定日だけを引き直す。
//
//	POST /api/v1/menus/reroll-day
func (h *MenuHandler) RerollDay(c echo.Context) error {
	var req rerollRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	f, err := toMenuFilter(req.Genre, req.Difficulty)
	if err != nil {
		return err
	}

	week := make([]domain.MenuID, 0, len(req.Week))
	for _, raw := range req.Week {
		id, err := domain.ParseMenuID(raw)
		if err != nil {
			return err
		}
		week = append(week, id)
	}

	ctx := c.Request().Context()
	userID, authed := UserIDFromContext(c)

	// ログイン中は直近履歴も避ける。引き直しは確定前の絞り込み操作なので
	// 履歴には記録しない（記録は suggest / suggest-weekly の確定時のみ）。
	var recent []domain.MenuID
	if authed {
		if recent, err = h.history.RecentMenuIDs(ctx, userID); err != nil {
			slog.Warn("直近履歴の取得に失敗しました", "error", err)
			recent = nil
		}
	}

	picked, err := h.svc.RerollDay(ctx, f, week, req.Day, recent)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, rerollResponse{Menu: toMenuDTO(picked.Menu)})
}

// recipeDTO はレシピリンクのAPI表現（spec.md 5.1）。
type recipeDTO struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Domain  string `json:"domain"`
	Snippet string `json:"snippet"`
}

// recipesResponse は GET /menus/:id/recipes のレスポンス。
type recipesResponse struct {
	Recipes []recipeDTO `json:"recipes"`
}

// Recipes は献立のレシピ掲載ページを返す。
//
//	GET /api/v1/menus/:id/recipes
func (h *MenuHandler) Recipes(c echo.Context) error {
	id, err := domain.ParseMenuID(c.Param("id"))
	if err != nil {
		return err
	}

	links, err := h.svc.RecipeLinks(c.Request().Context(), id)
	if err != nil {
		return err
	}

	// 0件でも null ではなく [] を返す。フロントが length を見るだけで
	// 扱えるようにするため。
	recipes := make([]recipeDTO, 0, len(links))
	for _, l := range links {
		recipes = append(recipes, recipeDTO{
			Title:   l.Title,
			URL:     l.URL,
			Domain:  l.Domain,
			Snippet: l.Snippet,
		})
	}
	return c.JSON(http.StatusOK, recipesResponse{Recipes: recipes})
}

func toMenuDTO(m domain.Menu) menuDTO {
	return menuDTO{
		ID:          m.ID.String(),
		Name:        m.Name,
		Genre:       m.Genre.String(),
		Difficulty:  m.Difficulty.String(),
		Description: m.Description,
	}
}
