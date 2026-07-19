package service

import (
	"context"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// HistoryLimit はユーザーごとに保持する履歴の最大件数（spec.md 2.5・FIFO 15件）。
// 保持件数は業務ルールなので service で持ち、repository には値として渡す。
const HistoryLimit = 15

// HistoryStore は履歴の永続化を抽象化する。実装は internal/repository。
type HistoryStore interface {
	// RecordWithLimit は履歴を1件記録し、最新 limit 件に切り詰める。
	RecordWithLimit(ctx context.Context, userID domain.UserID, menuID domain.MenuID, mode domain.SearchMode, limit int) error
}

// HistoryService は検索履歴の記録を担う。
type HistoryService struct {
	store HistoryStore
}

// NewHistoryService は HistoryService を生成する。
func NewHistoryService(store HistoryStore) *HistoryService {
	return &HistoryService{store: store}
}

// Record は履歴を1件記録し、FIFO で最新 HistoryLimit 件に保つ。
func (s *HistoryService) Record(ctx context.Context, userID domain.UserID, menuID domain.MenuID, mode domain.SearchMode) error {
	return s.store.RecordWithLimit(ctx, userID, menuID, mode, HistoryLimit)
}
