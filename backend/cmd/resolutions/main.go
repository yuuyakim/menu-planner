// Command resolutions は食材の解決まわりを運用するためのコマンド。
//
//	go run ./cmd/resolutions purge-unresolved
//	go run ./cmd/resolutions prune-counters
//
// purge-unresolved は食材マスタを更新したあとに流す。「マスタに無い」と
// 保存された語を消し、次回のアクセスで LLM に聞き直させる（設計 9章）。
//
// prune-counters は読み取りの日次カウンタのうち古い行を消す
// （レート制限の設計 6.3）。30日ぶんは残し、使用量を後から振り返れるようにする。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

// counterRetentionDays はカウンタを残す日数。
const counterRetentionDays = 30

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("DBに接続できませんでした", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	switch os.Args[1] {
	case "purge-unresolved":
		n, err := repository.NewResolutionRepository(pool).DeleteUnresolved(ctx)
		if err != nil {
			slog.Error("未解決キャッシュの削除に失敗しました", "error", err)
			os.Exit(1)
		}
		slog.Info("未解決キャッシュを削除しました", "deleted", n)

	case "prune-counters":
		// 境界は JST。サーバが UTC で動くため、日付の意味を揃える。
		jst := time.FixedZone("JST", 9*60*60)
		cutoff := time.Now().In(jst).AddDate(0, 0, -counterRetentionDays).Format("2006-01-02")
		n, err := repository.NewResolveUsageRepository(pool).DeleteOlderThan(ctx, cutoff)
		if err != nil {
			slog.Error("古い読み取りカウンタの削除に失敗しました", "error", err)
			os.Exit(1)
		}
		slog.Info("古い読み取りカウンタを削除しました", "before", cutoff, "deleted", n)

	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: resolutions purge-unresolved|prune-counters")
	os.Exit(2)
}
