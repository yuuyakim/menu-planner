package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// HistoryLister は履歴一覧の取得を抽象化する。実装は service.HistoryService。
type HistoryLister interface {
	List(ctx context.Context, userID string) ([]domain.HistoryEntry, error)
}

// HistoryHandler は履歴APIの受け口。
type HistoryHandler struct {
	svc    HistoryLister
	tokens *auth.JWT
}

// NewHistoryHandler は HistoryHandler を生成する。
func NewHistoryHandler(svc HistoryLister, tokens *auth.JWT) *HistoryHandler {
	return &HistoryHandler{svc: svc, tokens: tokens}
}

// RegisterRoutes は履歴APIのルーティングを登録する。
// 履歴は本人のものだけを扱うため、すべて認証必須。
func (h *HistoryHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath, RequireAuth(h.tokens))
	g.GET("/histories", h.List)
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
