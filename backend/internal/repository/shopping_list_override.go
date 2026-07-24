package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ShoppingListOverrideRepository は買い物リストの差分を Postgres に保存する。
type ShoppingListOverrideRepository struct {
	pool *pgxpool.Pool
}

// NewShoppingListOverrideRepository は ShoppingListOverrideRepository を生成する。
func NewShoppingListOverrideRepository(pool *pgxpool.Pool) *ShoppingListOverrideRepository {
	return &ShoppingListOverrideRepository{pool: pool}
}

// FindBySavedWeeklyMenu は当該週の差分を name 順で返す。
func (r *ShoppingListOverrideRepository) FindBySavedWeeklyMenu(
	ctx context.Context, id domain.SavedWeeklyMenuID,
) ([]domain.ShoppingListOverride, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT name, category, origin, checked, hidden
		   FROM shopping_list_overrides
		  WHERE saved_weekly_menu_id = $1
		  ORDER BY name`, id.String())
	if err != nil {
		return nil, fmt.Errorf("買い物リストの差分の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	out := make([]domain.ShoppingListOverride, 0)
	for rows.Next() {
		var (
			name, category, origin string
			checked, hidden        bool
		)
		if err := rows.Scan(&name, &category, &origin, &checked, &hidden); err != nil {
			return nil, fmt.Errorf("買い物リストの差分の読み取りに失敗しました: %w", err)
		}
		// DBの値はアプリが書いたものなので、そのまま型に載せる（検証は書き込み側）。
		out = append(out, domain.ShoppingListOverride{
			SavedWeeklyMenuID: id,
			Name:              name,
			Category:          domain.IngredientCategory(category),
			Origin:            domain.Origin(origin),
			Checked:           checked,
			Hidden:            hidden,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("買い物リストの差分の読み取りに失敗しました: %w", err)
	}
	return out, nil
}

// Replace は当該週の差分を丸ごと置き換える。削除と挿入を1トランザクションで行う。
//
// 部分更新にせず全消し＋全入れにするのは、overlay を冪等な1リソースとして
// 扱うため（設計 3.5）。品目数は高々100件程度で、全入れ替えの負荷は問題にならない。
func (r *ShoppingListOverrideRepository) Replace(
	ctx context.Context, id domain.SavedWeeklyMenuID, overrides []domain.ShoppingListOverride,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("差分の置換を開始できませんでした: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`DELETE FROM shopping_list_overrides WHERE saved_weekly_menu_id = $1`, id.String()); err != nil {
		return fmt.Errorf("差分の削除に失敗しました: %w", err)
	}

	for _, o := range overrides {
		if _, err := tx.Exec(ctx,
			`INSERT INTO shopping_list_overrides
			   (saved_weekly_menu_id, name, category, origin, checked, hidden)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			id.String(), o.Name, o.Category.String(), o.Origin.String(), o.Checked, o.Hidden); err != nil {
			return fmt.Errorf("差分の保存に失敗しました: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("差分の置換を確定できませんでした: %w", err)
	}
	return nil
}
