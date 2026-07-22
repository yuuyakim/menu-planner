package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// SavedWeeklyMenuUseCase は保存した週間献立のAPIが必要とする操作。
// 実装は service.SavedWeeklyMenuService。
type SavedWeeklyMenuUseCase interface {
	Save(ctx context.Context, userID string, days []service.SavedDayInput) (domain.SavedWeeklyMenuID, error)
	List(ctx context.Context, userID string) ([]domain.SavedWeeklyMenu, error)
	Delete(ctx context.Context, userID, id string) error
}

// SavedWeeklyMenuHandler は保存した週間献立APIの受け口。
type SavedWeeklyMenuHandler struct {
	svc    SavedWeeklyMenuUseCase
	tokens *auth.JWT
}

// NewSavedWeeklyMenuHandler は SavedWeeklyMenuHandler を生成する。
func NewSavedWeeklyMenuHandler(svc SavedWeeklyMenuUseCase, tokens *auth.JWT) *SavedWeeklyMenuHandler {
	return &SavedWeeklyMenuHandler{svc: svc, tokens: tokens}
}

// RegisterRoutes は保存した週間献立APIのルーティングを登録する。
// 保存は本人のものだけを扱うため認証必須（spec.md 2.8）。
func (h *SavedWeeklyMenuHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	// RequireAuth はグループではなくルート個別に付ける。グループに付けると
	// /api/v1 配下の未定義パスにも走り、404 であるべきものが 401 になる（Issue #73）。
	requireAuth := RequireAuth(h.tokens)
	g.GET("/weekly-menus", h.List, requireAuth)
	g.POST("/weekly-menus", h.Save, requireAuth)
	g.DELETE("/weekly-menus/:id", h.Delete, requireAuth)
}

// saveWeeklyMenuRequest は POST /weekly-menus のリクエストボディ。
type saveWeeklyMenuRequest struct {
	Days []saveWeeklyMenuDay `json:"days"`
}

// saveWeeklyMenuDay は保存する1日分の指定。
type saveWeeklyMenuDay struct {
	Day    int    `json:"day"`
	MenuID string `json:"menuId"`
}

// savedWeeklyMenuResponse は保存した週のAPI表現。
//
// 中身は送った側が既に持っているため返さない。必要なのは、
// 後で削除するときに指せるIDだけ。
type savedWeeklyMenuResponse struct {
	ID string `json:"id"`
}

// Save は現在のユーザーの週間献立を1件保存する。
//
//	POST /api/v1/weekly-menus  {"days": [{"day": 1, "menuId": "..."}, ...]}
//
// 7日分ちょうどでなければ 400、上限（10件）に達していれば 409、
// 存在しない献立を含めば 404、未認証は 401。成功時は 201。
func (h *SavedWeeklyMenuHandler) Save(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	var req saveWeeklyMenuRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "リクエストの形式が不正です")
	}

	days := make([]service.SavedDayInput, 0, len(req.Days))
	for _, d := range req.Days {
		days = append(days, service.SavedDayInput{Day: d.Day, MenuID: d.MenuID})
	}

	id, err := h.svc.Save(c.Request().Context(), userID, days)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, savedWeeklyMenuResponse{ID: id.String()})
}

// savedWeeklyMenuItemDTO は保存した週1件のAPI表現。
//
// 中身の7日分をここに含めるため、「開く」ための個別取得は要らない
// （spec.md 5.3）。1日分の形は週間献立の提案と同じ dayMenuDTO を使う。
// 画面では同じものとして扱えた方が素直なため。
type savedWeeklyMenuItemDTO struct {
	ID        string       `json:"id"`
	Days      []dayMenuDTO `json:"days"`
	CreatedAt time.Time    `json:"createdAt"`
}

// savedWeeklyMenusResponse は GET /weekly-menus のレスポンス。
type savedWeeklyMenusResponse struct {
	WeeklyMenus []savedWeeklyMenuItemDTO `json:"weeklyMenus"`
}

// List は現在のユーザーの保存を新しい順に返す。
//
//	GET /api/v1/weekly-menus
//
// 上限が10件と小さいので全件返す（spec.md 2.8）。ページングは設けない。
func (h *SavedWeeklyMenuHandler) List(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	saved, err := h.svc.List(c.Request().Context(), userID)
	if err != nil {
		return err
	}

	// 0件でも null ではなく [] を返す。
	items := make([]savedWeeklyMenuItemDTO, 0, len(saved))
	for _, s := range saved {
		days := make([]dayMenuDTO, 0, len(s.Days))
		for _, d := range s.Days {
			days = append(days, dayMenuDTO{Day: d.Day, Menu: toMenuDTO(d.Menu)})
		}
		items = append(items, savedWeeklyMenuItemDTO{
			ID:        s.ID.String(),
			Days:      days,
			CreatedAt: s.CreatedAt,
		})
	}
	return c.JSON(http.StatusOK, savedWeeklyMenusResponse{WeeklyMenus: items})
}

// Delete は現在のユーザーの保存を1件削除する。
//
//	DELETE /api/v1/weekly-menus/:id
//
// 他人のものは 404（存在を明かさない）、未認証は 401。成功時は 204。
func (h *SavedWeeklyMenuHandler) Delete(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}
	if err := h.svc.Delete(c.Request().Context(), userID, c.Param("id")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
