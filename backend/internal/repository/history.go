package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// HistoryRepository は検索履歴の永続化を提供する。
type HistoryRepository struct {
	pool *pgxpool.Pool
}

// NewHistoryRepository は HistoryRepository を生成する。
func NewHistoryRepository(pool *pgxpool.Pool) *HistoryRepository {
	return &HistoryRepository{pool: pool}
}

// Add は履歴を1件記録する。
// FIFO（15件超の削除）はここでは行わない。件数の維持は上位（6-B）で扱う。
func (r *HistoryRepository) Add(ctx context.Context, userID domain.UserID, menuID domain.MenuID, mode domain.SearchMode) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO search_histories (id, user_id, menu_id, search_mode)
		 VALUES ($1, $2, $3, $4)`,
		uuid.NewString(), userID.String(), menuID.String(), mode.String()); err != nil {
		return fmt.Errorf("履歴の記録に失敗しました: %w", err)
	}
	return nil
}
