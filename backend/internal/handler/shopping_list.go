package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// ShoppingListUseCase は買い物リストの組み立て。実装は service にある。
type ShoppingListUseCase interface {
	Build(ctx context.Context, ids []domain.MenuID) ([]service.ShoppingItem, error)
}

// MenuIngredientsUseCase は献立1件に必要な食材の取得。
type MenuIngredientsUseCase interface {
	MenuIngredients(ctx context.Context, id domain.MenuID) ([]domain.Ingredient, error)
}

// ingredientDTO は食材のAPI表現。
// ドメインの Ingredient をそのまま返すと内部の項目が契約に漏れるため、
// 外に出す項目をここで明示する。
type ingredientDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	NameKana string `json:"nameKana"`
	Category string `json:"category"`
}

// usedInDTO は「その食材を使う献立」。買い物リストで必要な最小限に絞る。
type usedInDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// shoppingItemDTO は買い物リストの1項目。
type shoppingItemDTO struct {
	Ingredient ingredientDTO `json:"ingredient"`
	UsedIn     []usedInDTO   `json:"usedIn"`
}

type shoppingListResponse struct {
	Items []shoppingItemDTO `json:"items"`
}

type ingredientsResponse struct {
	Ingredients []ingredientDTO `json:"ingredients"`
}

// shoppingListRequest は POST /shopping-list のリクエスト（spec.md 5.5）。
type shoppingListRequest struct {
	MenuIDs []string `json:"menuIds"`
}

func toIngredientDTO(i domain.Ingredient) ingredientDTO {
	return ingredientDTO{
		ID:       i.ID.String(),
		Name:     i.Name,
		NameKana: i.NameKana,
		Category: i.Category.String(),
	}
}

// ShoppingListHandler は買い物リストのHTTP境界。
type ShoppingListHandler struct {
	svc ShoppingListUseCase
}

// NewShoppingListHandler は ShoppingListHandler を生成する。
func NewShoppingListHandler(svc ShoppingListUseCase) *ShoppingListHandler {
	return &ShoppingListHandler{svc: svc}
}

// RegisterRoutes は買い物リストAPIのルーティングを登録する。
// mw はグループ全体に前置するミドルウェア（レート制限など）。
func (h *ShoppingListHandler) RegisterRoutes(e *echo.Echo, mw ...echo.MiddlewareFunc) {
	g := e.Group(APIBasePath, mw...)
	// 状態は変えないが、献立IDの配列をボディで受けるため POST（spec.md 5.5）。
	g.POST("/shopping-list", h.Build)
}

// Build は複数の献立から買い物リストを作る。
//
//	POST /api/v1/shopping-list
func (h *ShoppingListHandler) Build(c echo.Context) error {
	var req shoppingListRequest
	if err := c.Bind(&req); err != nil {
		// 本文が壊れている。echo が 400 の HTTPError にする。
		return err
	}

	// 不正なIDをそのまま service に渡すと理由が分からなくなるため、先に弾く。
	ids := make([]domain.MenuID, 0, len(req.MenuIDs))
	for _, raw := range req.MenuIDs {
		id, err := domain.ParseMenuID(raw)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}

	items, err := h.svc.Build(c.Request().Context(), ids)
	if err != nil {
		return err
	}

	// 0件でも null ではなく [] を返す。フロントが length を見るだけで扱えるようにする。
	out := make([]shoppingItemDTO, 0, len(items))
	for _, it := range items {
		usedIn := make([]usedInDTO, 0, len(it.UsedIn))
		for _, m := range it.UsedIn {
			usedIn = append(usedIn, usedInDTO{ID: m.ID.String(), Name: m.Name})
		}
		out = append(out, shoppingItemDTO{
			Ingredient: toIngredientDTO(it.Ingredient),
			UsedIn:     usedIn,
		})
	}

	return c.JSON(http.StatusOK, shoppingListResponse{Items: out})
}

// IngredientCatalogUseCase は食材マスタそのものを扱う操作。
// 実装は service.IngredientService。
type IngredientCatalogUseCase interface {
	All(ctx context.Context) ([]domain.Ingredient, error)
	SearchByIngredients(
		ctx context.Context, in service.SearchByIngredientsInput,
	) (service.SearchByIngredientsResult, error)
}

// IngredientHandler は食材のHTTP境界。
//
// 「献立の食材」と「食材マスタ全件」で参照するものが違うため、
// use case を分けて受け取る。前者は献立×食材（ShoppingListService）、
// 後者は献立に紐づかない一覧（IngredientService）。
type IngredientHandler struct {
	svc     MenuIngredientsUseCase
	catalog IngredientCatalogUseCase
}

// NewIngredientHandler は IngredientHandler を生成する。
func NewIngredientHandler(svc MenuIngredientsUseCase, catalog IngredientCatalogUseCase) *IngredientHandler {
	return &IngredientHandler{svc: svc, catalog: catalog}
}

// RegisterRoutes は食材APIのルーティングを登録する。
func (h *IngredientHandler) RegisterRoutes(e *echo.Echo, mw ...echo.MiddlewareFunc) {
	g := e.Group(APIBasePath, mw...)
	g.GET("/menus/:id/ingredients", h.List)
	g.GET("/ingredients", h.All)
	g.POST("/menus/search-by-ingredients", h.SearchByIngredients)
}

// searchByIngredientsRequest は POST /menus/search-by-ingredients のリクエストボディ。
//
// sort をポインタで受けるのは「省略」と「空文字の指定」を区別するため。
// 前者は既定、後者は誤りとして 400 にする。
type searchByIngredientsRequest struct {
	IngredientIDs []string `json:"ingredientIds"`
	OnlyMakeable  bool     `json:"onlyMakeable"`
	Sort          *string  `json:"sort"`
}

// menuMatchDTO は候補の献立1件。手持ちとの重なりと不足を併せて返す。
type menuMatchDTO struct {
	Menu    menuDTO         `json:"menu"`
	Matched []ingredientDTO `json:"matched"`
	Missing []ingredientDTO `json:"missing"`
}

// searchByIngredientsResponse は候補の一覧。
//
// nearMisses は「あと1品買えば作れる」候補。onlyMakeable で0件だったときだけ
// 埋まる。常に配列で返す（null にすると画面側で length を見る前に落ちる）。
type searchByIngredientsResponse struct {
	Matches    []menuMatchDTO `json:"matches"`
	NearMisses []menuMatchDTO `json:"nearMisses"`
}

// parseMatchSort は並び順を解釈する。省略なら既定、未知の値は 400。
//
// 既定に丸めない。利用者の指定を黙って読み替えると、
// 違う条件で検索した結果を正しい答えとして返すことになる。
//
// **echo.NewHTTPError は使わない。** それは toProblem の総称の分岐に落ち、
// type が http-error・title が "Bad Request" になって、書いたメッセージは
// どこにも出ない。このエンドポイントの他の 400 と同じく、写像表を通して
// 固有の type と日本語の title を返す。
func parseMatchSort(raw *string) (service.MatchSort, error) {
	if raw == nil {
		return service.SortMissingAsc, nil
	}
	switch service.MatchSort(*raw) {
	case service.SortMissingAsc, service.SortMatchedDesc:
		return service.MatchSort(*raw), nil
	default:
		return "", fmt.Errorf("%w: missing_asc か matched_desc を指定してください",
			service.ErrInvalidMatchSort)
	}
}

// SearchByIngredients は手持ちの食材で作れる献立を探す。
//
//	POST /api/v1/menus/search-by-ingredients
//	{"ingredientIds": ["...", "..."], "onlyMakeable": true, "sort": "matched_desc"}
//
// 0件指定は 400、存在しない食材が含まれれば 404。未認証でも使える（spec.md 2.9）。
func (h *IngredientHandler) SearchByIngredients(c echo.Context) error {
	var req searchByIngredientsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "リクエストの形式が不正です")
	}

	sortBy, err := parseMatchSort(req.Sort)
	if err != nil {
		return err
	}

	ids := make([]domain.IngredientID, 0, len(req.IngredientIDs))
	for _, raw := range req.IngredientIDs {
		id, err := domain.ParseIngredientID(raw)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}

	res, err := h.catalog.SearchByIngredients(c.Request().Context(),
		service.SearchByIngredientsInput{
			IngredientIDs: ids,
			OnlyMakeable:  req.OnlyMakeable,
			Sort:          sortBy,
		})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, searchByIngredientsResponse{
		Matches:    toMenuMatchDTOs(res.Matches),
		NearMisses: toMenuMatchDTOs(res.NearMisses),
	})
}

// toMenuMatchDTOs は候補の並びをDTOに写す。0件でも null にしない。
func toMenuMatchDTOs(matches []service.MenuMatch) []menuMatchDTO {
	out := make([]menuMatchDTO, 0, len(matches))
	for _, m := range matches {
		out = append(out, menuMatchDTO{
			Menu:    toMenuDTO(m.Menu),
			Matched: toIngredientDTOs(m.Matched),
			Missing: toIngredientDTOs(m.Missing),
		})
	}
	return out
}

// toIngredientDTOs は食材の並びをDTOに写す。0件でも null にしない。
func toIngredientDTOs(items []domain.Ingredient) []ingredientDTO {
	out := make([]ingredientDTO, 0, len(items))
	for _, i := range items {
		out = append(out, toIngredientDTO(i))
	}
	return out
}

// All は食材マスタを表示順で全件返す。手持ちの食材を選ぶ選択肢に使う。
//
//	GET /api/v1/ingredients
//
// 166件で固定的なため、ページングも検索クエリも設けない（spec.md 5.6）。
// 未認証でも使える（検索と同じ扱い）。
func (h *IngredientHandler) All(c echo.Context) error {
	items, err := h.catalog.All(c.Request().Context())
	if err != nil {
		return fmt.Errorf("食材マスタの取得に失敗しました: %w", err)
	}

	out := make([]ingredientDTO, 0, len(items))
	for _, i := range items {
		out = append(out, toIngredientDTO(i))
	}
	return c.JSON(http.StatusOK, ingredientsResponse{Ingredients: out})
}

// List は献立に必要な食材を返す。
//
//	GET /api/v1/menus/:id/ingredients
func (h *IngredientHandler) List(c echo.Context) error {
	id, err := domain.ParseMenuID(c.Param("id"))
	if err != nil {
		return err
	}

	items, err := h.svc.MenuIngredients(c.Request().Context(), id)
	if err != nil {
		return fmt.Errorf("食材の取得に失敗しました: %w", err)
	}

	out := make([]ingredientDTO, 0, len(items))
	for _, i := range items {
		out = append(out, toIngredientDTO(i))
	}

	return c.JSON(http.StatusOK, ingredientsResponse{Ingredients: out})
}
