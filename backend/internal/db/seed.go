package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/db/seeds"
)

// ErrEmptySeed はシードSQLが空であることを表す。
var ErrEmptySeed = errors.New("シードSQLが空です")

// SeedSQL は埋め込まれた献立マスタのSQLを返す。
func SeedSQL() (string, error) {
	if seeds.MenusSQL == "" {
		return "", ErrEmptySeed
	}
	return seeds.MenusSQL, nil
}

// seedStep は投入する1つのSQLと、ログに出す名前。
type seedStep struct {
	name string
	sql  string
}

// seedSteps は投入順。**この順序に意味がある。**
// 紐付けは menus / ingredients を名前で結合するため、両方が入った後でなければ
// 0件になって黙って欠落する。
func seedSteps() []seedStep {
	return []seedStep{
		{"献立マスタ", seeds.MenusSQL},
		{"食材マスタ", seeds.IngredientsSQL},
		{"献立と食材の紐付け", seeds.MenuIngredientsSQL},
	}
}

// Seed は献立マスタ・食材マスタ・その紐付けを投入する。
// ON CONFLICT DO NOTHING により、再実行しても重複しない（冪等）。
//
// 返すのは投入した行数の合計。途中で失敗したら全体を巻き戻す
// （献立だけ入って食材が入っていない中途半端な状態を残さない）。
func Seed(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("シードのトランザクション開始に失敗しました: %w", err)
	}
	defer func() {
		// コミット済みなら Rollback は無害に失敗する。
		_ = tx.Rollback(ctx)
	}()

	var total int64
	for _, step := range seedSteps() {
		if step.sql == "" {
			return 0, fmt.Errorf("%w: %s", ErrEmptySeed, step.name)
		}
		tag, err := tx.Exec(ctx, step.sql)
		if err != nil {
			return 0, fmt.Errorf("%sの投入に失敗しました: %w", step.name, err)
		}
		slog.Info("シードを投入しました", "対象", step.name, "inserted", tag.RowsAffected())
		total += tag.RowsAffected()
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("シードのコミットに失敗しました: %w", err)
	}
	return total, nil
}
