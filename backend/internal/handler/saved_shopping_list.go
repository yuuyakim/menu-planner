package handler

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// SavedShoppingListUseCase は保存済み週の買い物リストのAPIが必要とする操作。
// 実装は service.SavedShoppingListService。
//
// PUT（差分の置き換え）は別タスクで足す。
type SavedShoppingListUseCase interface {
	For(ctx context.Context, userID, savedWeeklyMenuID string) ([]service.SavedShoppingItem, error)
}

// SavedShoppingListHandler は保存済み週の買い物リストAPIの受け口。
type SavedShoppingListHandler struct {
	svc    SavedShoppingListUseCase
	tokens *auth.JWT
}

// NewSavedShoppingListHandler は SavedShoppingListHandler を生成する。
func NewSavedShoppingListHandler(svc SavedShoppingListUseCase, tokens *auth.JWT) *SavedShoppingListHandler {
	return &SavedShoppingListHandler{svc: svc, tokens: tokens}
}

// RegisterRoutes はルーティングを登録する。保存は本人のものだけを扱うため認証必須。
func (h *SavedShoppingListHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	requireAuth := RequireAuth(h.tokens)
	g.GET("/weekly-menus/:id/shopping-list", h.Get, requireAuth)
}

// savedShoppingUsedInDTO はその食材を使う献立。
type savedShoppingUsedInDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// savedShoppingItemDTO は差分適用後の買い物リストの1項目。
type savedShoppingItemDTO struct {
	Name     string                   `json:"name"`
	Category string                   `json:"category"`
	Origin   string                   `json:"origin"`
	Checked  bool                     `json:"checked"`
	UsedIn   []savedShoppingUsedInDTO `json:"usedIn"`
}

// savedShoppingListResponse は GET /weekly-menus/:id/shopping-list のレスポンス。
type savedShoppingListResponse struct {
	Items []savedShoppingItemDTO `json:"items"`
}

// Get は保存済み週の買い物リストを差分適用後の形で返す。
//
//	GET /api/v1/weekly-menus/:id/shopping-list
//
// 他人の週・存在しない週は 404、未認証は 401。
func (h *SavedShoppingListHandler) Get(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	items, err := h.svc.For(c.Request().Context(), userID, c.Param("id"))
	if err != nil {
		return err
	}

	// 0件でも null ではなく [] を返す。
	out := make([]savedShoppingItemDTO, 0, len(items))
	for _, it := range items {
		usedIn := make([]savedShoppingUsedInDTO, 0, len(it.UsedIn))
		for _, m := range it.UsedIn {
			usedIn = append(usedIn, savedShoppingUsedInDTO{ID: m.ID.String(), Name: m.Name})
		}
		out = append(out, savedShoppingItemDTO{
			Name:     it.Name,
			Category: it.Category.String(),
			Origin:   it.Origin.String(),
			Checked:  it.Checked,
			UsedIn:   usedIn,
		})
	}
	return c.JSON(http.StatusOK, savedShoppingListResponse{Items: out})
}
