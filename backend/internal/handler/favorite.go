package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// FavoriteUseCase はお気に入りAPIが必要とする操作。実装は service.FavoriteService。
type FavoriteUseCase interface {
	Add(ctx context.Context, userID, menuID string) error
	List(ctx context.Context, userID string) ([]domain.Favorite, error)
	Delete(ctx context.Context, userID, menuID string) error
}

// FavoriteHandler はお気に入りAPIの受け口。
type FavoriteHandler struct {
	svc    FavoriteUseCase
	tokens *auth.JWT
}

// NewFavoriteHandler は FavoriteHandler を生成する。
func NewFavoriteHandler(svc FavoriteUseCase, tokens *auth.JWT) *FavoriteHandler {
	return &FavoriteHandler{svc: svc, tokens: tokens}
}

// RegisterRoutes はお気に入りAPIのルーティングを登録する。
// お気に入りは本人のものだけを扱うため、すべて認証必須。
func (h *FavoriteHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath, RequireAuth(h.tokens))
	g.GET("/favorites", h.List)
	g.POST("/favorites", h.Add)
	g.DELETE("/favorites/:menuId", h.Delete)
}

// addFavoriteRequest は POST /favorites のリクエストボディ。
type addFavoriteRequest struct {
	MenuID string `json:"menuId"`
}

// favoriteResponse は追加したお気に入りのAPI表現。
// お気に入り自体のIDは外に出さない。削除も献立IDで指定するため
// （DELETE /favorites/:menuId）、利用側が知る必要があるのは献立IDだけ。
type favoriteResponse struct {
	MenuID string `json:"menuId"`
}

// Add は現在のユーザーのお気に入りに献立を追加する。
//
//	POST /api/v1/favorites  {"menuId": "..."}
//
// 重複は 409、存在しない献立は 404、未認証は 401。成功時は 201。
func (h *FavoriteHandler) Add(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	var req addFavoriteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "リクエストの形式が不正です")
	}

	if err := h.svc.Add(c.Request().Context(), userID, req.MenuID); err != nil {
		return err
	}
	// 今はたまたま同じ形なので変換で済む。入出力は別物として型は分けておく。
	return c.JSON(http.StatusCreated, favoriteResponse(req))
}

// favoriteItemDTO はお気に入り1件のAPI表現。
type favoriteItemDTO struct {
	Menu      menuDTO   `json:"menu"`
	CreatedAt time.Time `json:"createdAt"`
}

// favoritesResponse は GET /favorites のレスポンス。
type favoritesResponse struct {
	Favorites []favoriteItemDTO `json:"favorites"`
}

// List は現在のユーザーのお気に入りを新しい順に返す。
//
//	GET /api/v1/favorites
//
// 履歴と違い件数の上限が無いので全件返す（spec.md 2.6）。
func (h *FavoriteHandler) List(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	favorites, err := h.svc.List(c.Request().Context(), userID)
	if err != nil {
		return err
	}

	// 0件でも null ではなく [] を返す。
	items := make([]favoriteItemDTO, 0, len(favorites))
	for _, f := range favorites {
		items = append(items, favoriteItemDTO{
			Menu:      toMenuDTO(f.Menu),
			CreatedAt: f.CreatedAt,
		})
	}
	return c.JSON(http.StatusOK, favoritesResponse{Favorites: items})
}

// Delete は現在のユーザーのお気に入りから献立を1件外す。
//
//	DELETE /api/v1/favorites/:menuId
//
// 自分が登録していない献立は 404、未認証は 401。成功時は 204。
func (h *FavoriteHandler) Delete(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}
	if err := h.svc.Delete(c.Request().Context(), userID, c.Param("menuId")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
