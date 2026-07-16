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

// Seed は献立マスタを投入する。
// ON CONFLICT DO NOTHING により、再実行しても重複しない（冪等）。
func Seed(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	sql, err := SeedSQL()
	if err != nil {
		return 0, err
	}

	tag, err := pool.Exec(ctx, sql)
	if err != nil {
		return 0, fmt.Errorf("献立マスタの投入に失敗しました: %w", err)
	}

	inserted := tag.RowsAffected()
	slog.Info("献立マスタを投入しました", "inserted", inserted)
	return inserted, nil
}
