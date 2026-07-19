package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// Add は履歴を1件記録する。FIFO（件数の維持）は行わない。
func (r *HistoryRepository) Add(ctx context.Context, userID domain.UserID, menuID domain.MenuID, mode domain.SearchMode) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO search_histories (id, user_id, menu_id, search_mode)
		 VALUES ($1, $2, $3, $4)`,
		uuid.NewString(), userID.String(), menuID.String(), mode.String()); err != nil {
		return fmt.Errorf("履歴の記録に失敗しました: %w", err)
	}
	return nil
}

// RecordWithLimit は履歴を1件記録し、そのユーザーの履歴を最新 limit 件に保つ。
// 記録と超過分の削除を同一トランザクションで行い、中途半端な件数にならないようにする。
// limit の値（15）は業務ルールとして service から渡す（spec.md 4.3）。
func (r *HistoryRepository) RecordWithLimit(ctx context.Context, userID domain.UserID, menuID domain.MenuID, mode domain.SearchMode, limit int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO search_histories (id, user_id, menu_id, search_mode)
		 VALUES ($1, $2, $3, $4)`,
		uuid.NewString(), userID.String(), menuID.String(), mode.String()); err != nil {
		return fmt.Errorf("履歴の記録に失敗しました: %w", err)
	}

	if err := pruneToLimit(ctx, tx, userID, limit); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("トランザクションのコミットに失敗しました: %w", err)
	}
	return nil
}

// pruneToLimit はユーザーの履歴を最新 limit 件に切り詰める。
// 並びは searched_at の降順、同値なら seq の降順（挿入順のタイブレーク）。
// now() はトランザクション時刻なので一括登録では searched_at が同値になり、
// seq が無いと「最新15件」が定まらない。
func pruneToLimit(ctx context.Context, tx pgx.Tx, userID domain.UserID, limit int) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM search_histories
		  WHERE user_id = $1
		    AND id NOT IN (
		        SELECT id FROM search_histories
		         WHERE user_id = $1
		         ORDER BY searched_at DESC, seq DESC
		         LIMIT $2
		    )`,
		userID.String(), limit); err != nil {
		return fmt.Errorf("履歴の切り詰めに失敗しました: %w", err)
	}
	return nil
}
