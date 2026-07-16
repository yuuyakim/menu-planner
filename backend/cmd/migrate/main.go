// Package main はマイグレーションを適用するCLI。
//
//	go run ./cmd/migrate up | down | version
package main

import (
	"log/slog"
	"os"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if len(os.Args) != 2 {
		slog.Error("使い方: migrate <up|down|version>")
		os.Exit(2)
	}

	dir, err := db.ParseMigrateDirection(os.Args[1])
	if err != nil {
		slog.Error("引数が不正です", "error", err)
		os.Exit(2)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL が設定されていません")
		os.Exit(1)
	}

	if err := db.Migrate(dsn, dir); err != nil {
		slog.Error("マイグレーションに失敗しました", "error", err)
		os.Exit(1)
	}
}
