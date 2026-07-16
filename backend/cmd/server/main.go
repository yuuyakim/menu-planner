// Package main は献立提案APIサーバのエントリポイント。
// 依存の組み立て（DI）とサーバのライフサイクル管理のみを行う。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/yuuyakim/menu-planner/backend/internal/handler"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("サーバの起動に失敗しました", "error", err)
		os.Exit(1)
	}
}

func run() error {
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
