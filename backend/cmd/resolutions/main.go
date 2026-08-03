// Command resolutions は食材の解決キャッシュを運用するためのコマンド。
//
//	go run ./cmd/resolutions purge-unresolved
//
// 食材マスタを更新したあとに流す。「マスタに無い」と保存された語を消し、
// 次回のアクセスで LLM に聞き直させる（設計 9章）。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "purge-unresolved" {
		fmt.Fprintln(os.Stderr, "usage: resolutions purge-unresolved")
		os.Exit(2)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("DBに接続できませんでした", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	n, err := repository.NewResolutionRepository(pool).DeleteUnresolved(ctx)
	if err != nil {
		slog.Error("未解決キャッシュの削除に失敗しました", "error", err)
		os.Exit(1)
	}
	slog.Info("未解決キャッシュを削除しました", "deleted", n)
}
