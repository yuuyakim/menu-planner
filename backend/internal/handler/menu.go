package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
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

// MenuUseCase は献立APIが必要とする操作をまとめたもの。
type MenuUseCase interface {
	MenuSuggester
	MenuGetter
}

// MenuHandler は献立APIの受け口。
type MenuHandler struct {
	svc MenuUseCase
}

// NewMenuHandler は MenuHandler を生成する。
func NewMenuHandler(s MenuUseCase) *MenuHandler {
	return &MenuHandler{svc: s}
}

// RegisterRoutes は献立APIのルーティングを登録する。
// パスの定義をハンドラと同じ場所に置き、テストで実際のパスごと検証できるようにする。
func (h *MenuHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	// /menus/suggest は /menus/:id と同じ階層にあるが、echo は静的なパスを
	// パラメータより優先して照合するため、:id に飲み込まれることはない。
	g.GET("/menus/suggest", h.Suggest)
	g.GET("/menus/:id", h.Get)
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
func (h *MenuHandler) Suggest(c echo.Context) error {
	f, err := parseMenuFilter(c)
	if err != nil {
		return err
	}

	menu, err := h.svc.SuggestMenu(c.Request().Context(), f)
	if err != nil {
		return err
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

func toMenuDTO(m domain.Menu) menuDTO {
	return menuDTO{
		ID:          m.ID.String(),
		Name:        m.Name,
		Genre:       m.Genre.String(),
		Difficulty:  m.Difficulty.String(),
		Description: m.Description,
	}
}
