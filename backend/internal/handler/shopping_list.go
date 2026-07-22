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
