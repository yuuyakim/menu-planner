// Package main は献立マスタを投入するCLI。
//
//	go run ./cmd/seed
//
// 再実行しても重複しない（冪等）。
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL が設定されていません")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, db.Config{DSN: dsn})
	if err != nil {
		slog.Error("DBへの接続に失敗しました", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if _, err := db.Seed(ctx, pool); err != nil {
		slog.Error("シードの投入に失敗しました", "error", err)
		os.Exit(1)
	}
}
