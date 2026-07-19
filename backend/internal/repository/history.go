package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
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

// List はユーザーの履歴を新しい順に返す。献立の情報を JOIN して読み取りモデルにする。
// 該当が無い場合は空スライスを返す（nilではない）。
// FIFO で最大15件しか残らないため件数の上限指定は不要。
func (r *HistoryRepository) List(ctx context.Context, userID domain.UserID) ([]domain.HistoryEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT h.id, h.search_mode, h.searched_at,
		        m.id, m.name, m.name_kana, m.genre, m.difficulty, m.description
		   FROM search_histories h
		   JOIN menus m ON m.id = h.menu_id
		  WHERE h.user_id = $1
		  ORDER BY h.searched_at DESC, h.seq DESC`, userID.String())
	if err != nil {
		return nil, fmt.Errorf("履歴の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	entries := make([]domain.HistoryEntry, 0)
	for rows.Next() {
		entry, err := scanHistoryEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("履歴の読み取りに失敗しました: %w", err)
	}
	return entries, nil
}

// RecentMenuIDs は履歴に含まれる献立IDを新しい順・重複なしで返す。
// FIFO で最大15件しか残らないため、これが直近の献立に相当する。
func (r *HistoryRepository) RecentMenuIDs(ctx context.Context, userID domain.UserID) ([]domain.MenuID, error) {
	// DISTINCT で献立IDを一意にする。同じ献立が複数回出ても除外対象は1つ。
	// 並びは新しい順だが、除外集合として使うので順序自体は重要ではない。
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT menu_id FROM search_histories WHERE user_id = $1`,
		userID.String())
	if err != nil {
		return nil, fmt.Errorf("直近履歴の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	ids := make([]domain.MenuID, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("直近履歴の読み取りに失敗しました: %w", err)
		}
		id, err := domain.ParseMenuID(raw)
		if err != nil {
			return nil, fmt.Errorf("DBの献立IDが不正です: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("直近履歴の読み取りに失敗しました: %w", err)
	}
	return ids, nil
}

// Delete は履歴を1件削除する。存在しなければ service.ErrHistoryNotFound、
// 他人のものなら service.ErrHistoryForbidden を返す。
//
// 先に所有者を確認してから消す。「存在しない(404)」と「他人のもの(403)」を
// 呼び出し側が区別できるようにするため、DELETE の件数だけでは判別しない。
func (r *HistoryRepository) Delete(ctx context.Context, userID domain.UserID, historyID domain.HistoryID) error {
	var owner string
	err := r.pool.QueryRow(ctx,
		`SELECT user_id FROM search_histories WHERE id = $1`, historyID.String()).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return service.ErrHistoryNotFound
	}
	if err != nil {
		return fmt.Errorf("履歴の取得に失敗しました: %w", err)
	}
	if owner != userID.String() {
		return service.ErrHistoryForbidden
	}

	if _, err := r.pool.Exec(ctx,
		`DELETE FROM search_histories WHERE id = $1`, historyID.String()); err != nil {
		return fmt.Errorf("履歴の削除に失敗しました: %w", err)
	}
	return nil
}

// DeleteAll はユーザーの履歴を全件削除する。0件でもエラーにしない。
func (r *HistoryRepository) DeleteAll(ctx context.Context, userID domain.UserID) error {
	if _, err := r.pool.Exec(ctx,
		`DELETE FROM search_histories WHERE user_id = $1`, userID.String()); err != nil {
		return fmt.Errorf("履歴の全件削除に失敗しました: %w", err)
	}
	return nil
}

// scanHistoryEntry は1行を HistoryEntry に読む。
func scanHistoryEntry(row pgx.Row) (domain.HistoryEntry, error) {
	var (
		histID, mode                              string
		searchedAt                                time.Time
		menuID, name, kana, genre, diff, descript string
	)
	if err := row.Scan(&histID, &mode, &searchedAt,
		&menuID, &name, &kana, &genre, &diff, &descript); err != nil {
		return domain.HistoryEntry{}, fmt.Errorf("履歴の読み取りに失敗しました: %w", err)
	}

	id, err := domain.ParseHistoryID(histID)
	if err != nil {
		return domain.HistoryEntry{}, fmt.Errorf("DBの履歴IDが不正です: %w", err)
	}
	m, err := domain.ParseSearchMode(mode)
	if err != nil {
		return domain.HistoryEntry{}, fmt.Errorf("DBの検索種別が不正です: %w", err)
	}
	menu, err := hydrateMenu(menuID, name, kana, genre, diff, descript)
	if err != nil {
		return domain.HistoryEntry{}, err
	}
	return domain.HistoryEntry{ID: id, Menu: menu, Mode: m, SearchedAt: searchedAt}, nil
}

// hydrateMenu は献立の各カラムからドメインの Menu を組み立てる。
func hydrateMenu(id, name, kana, genre, diff, descript string) (domain.Menu, error) {
	menuID, err := domain.ParseMenuID(id)
	if err != nil {
		return domain.Menu{}, fmt.Errorf("DBの献立IDが不正です: %w", err)
	}
	g, err := domain.ParseGenre(genre)
	if err != nil {
		return domain.Menu{}, fmt.Errorf("DBのジャンルが不正です: %w", err)
	}
	d, err := domain.ParseDifficulty(diff)
	if err != nil {
		return domain.Menu{}, fmt.Errorf("DBの難易度が不正です: %w", err)
	}
	return domain.Menu{ID: menuID, Name: name, NameKana: kana, Genre: g, Difficulty: d, Description: descript}, nil
}

// RecordManyWithLimit は複数の献立を一括で記録し、最新 limit 件に保つ。
// 週間献立（7件）の登録に使う。全件の INSERT と切り詰めを1トランザクションで行い、
// FIFO は最後に1度だけ走らせる（挿入ごとに走らせる必要はなく無駄）。
// 途中で失敗したら全件ロールバックする。
func (r *HistoryRepository) RecordManyWithLimit(ctx context.Context, userID domain.UserID, menuIDs []domain.MenuID, mode domain.SearchMode, limit int) error {
	if len(menuIDs) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, menuID := range menuIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO search_histories (id, user_id, menu_id, search_mode)
			 VALUES ($1, $2, $3, $4)`,
			uuid.NewString(), userID.String(), menuID.String(), mode.String()); err != nil {
			return fmt.Errorf("履歴の記録に失敗しました: %w", err)
		}
	}

	// 全件入れ終わってから1度だけ切り詰める。
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
