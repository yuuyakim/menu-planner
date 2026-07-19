package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// HistoryUseCase は履歴APIが必要とする操作をまとめたもの。実装は service.HistoryService。
type HistoryUseCase interface {
	List(ctx context.Context, userID string) ([]domain.HistoryEntry, error)
	Delete(ctx context.Context, userID, historyID string) error
	DeleteAll(ctx context.Context, userID string) error
}

// HistoryHandler は履歴APIの受け口。
type HistoryHandler struct {
	svc    HistoryUseCase
	tokens *auth.JWT
}

// NewHistoryHandler は HistoryHandler を生成する。
func NewHistoryHandler(svc HistoryUseCase, tokens *auth.JWT) *HistoryHandler {
	return &HistoryHandler{svc: svc, tokens: tokens}
}

// RegisterRoutes は履歴APIのルーティングを登録する。
// 履歴は本人のものだけを扱うため、すべて認証必須。
func (h *HistoryHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath, RequireAuth(h.tokens))
	g.GET("/histories", h.List)
	// 静的な /histories は :id より優先されるので飲み込まれない。
	g.DELETE("/histories", h.DeleteAll)
	g.DELETE("/histories/:id", h.Delete)
}

// historyItemDTO は履歴1件のAPI表現。
type historyItemDTO struct {
	ID         string    `json:"id"`
	Menu       menuDTO   `json:"menu"`
	SearchMode string    `json:"searchMode"`
	SearchedAt time.Time `json:"searchedAt"`
}

// historiesResponse は GET /histories のレスポンス。
type historiesResponse struct {
	Histories []historyItemDTO `json:"histories"`
}

// List は現在のユーザーの履歴を新しい順に返す。
//
//	GET /api/v1/histories
func (h *HistoryHandler) List(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}

	entries, err := h.svc.List(c.Request().Context(), userID)
	if err != nil {
		return err
	}

	// 0件でも null ではなく [] を返す。
	items := make([]historyItemDTO, 0, len(entries))
	for _, e := range entries {
		items = append(items, historyItemDTO{
			ID:         e.ID.String(),
			Menu:       toMenuDTO(e.Menu),
			SearchMode: e.Mode.String(),
			SearchedAt: e.SearchedAt,
		})
	}
	return c.JSON(http.StatusOK, historiesResponse{Histories: items})
}

// Delete は現在のユーザーの履歴を1件削除する。
//
//	DELETE /api/v1/histories/:id
//
// 存在しない履歴は 404、他人の履歴は 403。成功時は 204。
func (h *HistoryHandler) Delete(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}
	if err := h.svc.Delete(c.Request().Context(), userID, c.Param("id")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteAll は現在のユーザーの履歴を全件削除する。
//
//	DELETE /api/v1/histories
//
// 0件でも成功（冪等）。成功時は 204。
func (h *HistoryHandler) DeleteAll(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}
	if err := h.svc.DeleteAll(c.Request().Context(), userID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
