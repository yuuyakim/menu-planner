package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ResolveUsageRepository は「読み取る」の日次カウンタへのアクセスを提供する。
//
// day は 2006-01-02 形式の文字列で受ける。time.Time を渡すと、
// timestamptz として送られて date 列との比較がタイムゾーン依存になりうる。
// 文字列 + $1::date なら、どの日を指しているかがコードと SQL の両方で読める。
type ResolveUsageRepository struct {
	pool *pgxpool.Pool
}

// NewResolveUsageRepository は ResolveUsageRepository を生成する。
func NewResolveUsageRepository(pool *pgxpool.Pool) *ResolveUsageRepository {
	return &ResolveUsageRepository{pool: pool}
}

// Counts は指定日の「サービス全体」と「その利用者」の回数を返す。
//
// 1往復で両方を引く。上限判定は必ず両方を見るため、分けても往復が増えるだけ。
func (r *ResolveUsageRepository) Counts(
	ctx context.Context, day, scope, subject string,
) (total, own int, err error) {
	rows, err := r.pool.Query(ctx,
		`SELECT scope, count
		   FROM resolve_usage_counters
		  WHERE usage_date = $1::date
		    AND ((scope = 'total' AND subject = '')
		      OR (scope = $2 AND subject = $3))`, day, scope, subject)
	if err != nil {
		return 0, 0, fmt.Errorf("読み取りカウンタの取得に失敗しました: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return 0, 0, fmt.Errorf("読み取りカウンタの読み取りに失敗しました: %w", err)
		}
		if s == "total" {
			total = n
			continue
		}
		own = n
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("読み取りカウンタの取得に失敗しました: %w", err)
	}
	return total, own, nil
}

// Increment は「サービス全体」と「その利用者」の回数を1つずつ加算する。
//
// 2行を1文で撃つ。別々に撃つと、片方だけ成功したときに全体と利用者の
// 帳尻が合わなくなる。
func (r *ResolveUsageRepository) Increment(
	ctx context.Context, day, scope, subject string,
) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
		 VALUES ($1::date, 'total', '', 1), ($1::date, $2, $3, 1)
		 ON CONFLICT (usage_date, scope, subject)
		 DO UPDATE SET count = resolve_usage_counters.count + 1`,
		day, scope, subject)
	if err != nil {
		return fmt.Errorf("読み取りカウンタの加算に失敗しました: %w", err)
	}
	return nil
}

// DeleteOlderThan は day より前の行を消す（設計 6.3）。
// 運用コマンドから流す。日付ごとに行が増え続けるのを止めるためだけの経路。
func (r *ResolveUsageRepository) DeleteOlderThan(
	ctx context.Context, day string,
) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM resolve_usage_counters WHERE usage_date < $1::date`, day)
	if err != nil {
		return 0, fmt.Errorf("古い読み取りカウンタの削除に失敗しました: %w", err)
	}
	return tag.RowsAffected(), nil
}
