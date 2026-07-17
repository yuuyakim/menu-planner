// Package main は献立提案APIサーバのエントリポイント。
// 依存の組み立て（DI）とサーバのライフサイクル管理のみを行う。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/random"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// dbConnectTimeout は起動時のDB接続に費やしてよい上限。
// db.NewPool はコールドスタート（Neonのスリープ復帰など）を見込んで指数バックオフで
// リトライするため、その分の猶予を持たせる。ホスト名が解決できない場合などは
// リトライが尽きる前にこの上限が先に効いて起動を打ち切る。
const dbConnectTimeout = 30 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("サーバの起動に失敗しました", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// DBに繋がらないまま起動すると、献立APIが全て500を返すサーバが
	// ヘルスチェックだけ通る状態になる。起動時に失敗させて気付けるようにする。
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL が設定されていません")
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), dbConnectTimeout)
	defer dbCancel()

	pool, err := db.NewPool(dbCtx, db.Config{DSN: dsn})
	if err != nil {
		return fmt.Errorf("DBへの接続に失敗しました: %w", err)
	}
	defer pool.Close()
	slog.Info("DBに接続しました")

	menuRepo := repository.NewMenuRepository(pool)
	menuSvc := service.NewMenuService(menuRepo, random.NewCrypto())
	menuHandler := handler.NewMenuHandler(menuSvc)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// 全てのエラーレスポンスを RFC 7807 形式に統一する
	e.HTTPErrorHandler = handler.ErrorHandler()

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{env("FRONTEND_ORIGIN", "http://localhost:5173")},
		AllowCredentials: true,
	}))

	health := handler.NewHealthHandler()
	e.GET("/health", health.Health)
	menuHandler.RegisterRoutes(e)

	addr := ":" + env("PORT", "8080")

	go func() {
		slog.Info("サーバを起動しました", "addr", addr)
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("サーバが異常終了しました", "error", err)
			os.Exit(1)
		}
	}()

	// SIGTERM/SIGINT を受けたら処理中のリクエストを捌き切ってから終了する。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("シャットダウンを開始します")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return e.Shutdown(ctx)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
