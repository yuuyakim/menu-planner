// Package main はプレミアムプランを手で付与・取消するCLI。
//
//	go run ./cmd/grant -email=foo@example.com -months=1   # 付与
//	go run ./cmd/grant -email=foo@example.com -revoke     # 即時取消
//
// 決済を導入するまでの唯一の付与手段。SQLを直接書かず service を通すことで、
// 将来の決済Webhook と同じ状態遷移を経由させる。
//
// 手動付与は決済事業者側の履歴に残らないため、ここが出すログが唯一の記録になる。
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	email := flag.String("email", "", "対象の利用者のメールアドレス")
	months := flag.Int("months", 1, "付与する月数")
	revoke := flag.Bool("revoke", false, "付与ではなく即時取消を行う")
	flag.Parse()

	if *email == "" {
		slog.Error("-email が必要です")
		os.Exit(1)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL が設定されていません")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, db.Config{DSN: dsn})
	if err != nil {
		slog.Error("DBへの接続に失敗しました", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	addr, err := domain.NewEmail(*email)
	if err != nil {
		slog.Error("メールアドレスが不正です", "error", err)
		os.Exit(1)
	}

	// メール→UserID の解決はCLIの責務。service を UserID 起点にしておくことで、
	// 顧客IDしか持たない将来の決済Webhook が不自然な逆引きを強いられない。
	user, err := repository.NewUserRepository(pool).FindByEmail(ctx, addr)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			slog.Error("該当する利用者が居ません", "email", *email)
		} else {
			slog.Error("利用者の取得に失敗しました", "error", err)
		}
		os.Exit(1)
	}

	svc := service.NewSubscriptionService(repository.NewSubscriptionRepository(pool), time.Now)

	if *revoke {
		if err := svc.Revoke(ctx, user.ID); err != nil {
			slog.Error("取消に失敗しました", "error", err)
			os.Exit(1)
		}
		slog.Info("プレミアムを取り消しました",
			"user_id", user.ID.String(), "email", *email)
		return
	}

	if err := svc.Grant(ctx, user.ID, *months); err != nil {
		slog.Error("付与に失敗しました", "error", err)
		os.Exit(1)
	}
	slog.Info("プレミアムを付与しました",
		"user_id", user.ID.String(), "email", *email, "months", *months)
}
