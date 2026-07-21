package handler

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/logctx"
)

// RequestLogging はリクエストごとに request_id を付けた logger を用意し、
// 1リクエストにつき1行のアクセスログを出す。
//
// logger を request の context に載せるため、ハンドラや service 層が
// logctx.From(ctx) / LoggerFrom(c) で取り出すと、同じ request_id が
// 全ログに伝播する。
//
// 本文もヘッダ（Cookie / Authorization）も、クエリ文字列も記録しない。
// パスワードやトークン、OAuth の code がログに漏れるのを防ぐため、
// 記録するのは method / path / status / latency / ip に限る。
//
// 前段に echo の RequestID ミドルウェアを置くこと。そこで載る X-Request-Id を読む。
func RequestLogging(logger *slog.Logger) echo.MiddlewareFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			// RequestID ミドルウェアが応答ヘッダに載せたIDを使う。
			// 無ければクライアント指定のリクエストヘッダに落ちる。
			rid := c.Response().Header().Get(echo.HeaderXRequestID)
			if rid == "" {
				rid = req.Header.Get(echo.HeaderXRequestID)
			}
			reqLogger := logger.With("request_id", rid)

			// 下位層が context 経由で同じ logger を使えるようにする。
			c.SetRequest(req.WithContext(logctx.WithLogger(req.Context(), reqLogger)))

			start := time.Now()
			err := next(c)
			latency := time.Since(start)

			// エラー時、応答ステータスはこの後 ErrorHandler が確定させるため、
			// この時点の c.Response().Status はまだ 200 のことがある。
			// クライアントが実際に受け取る値を toProblem で先取りする。
			status := c.Response().Status
			if err != nil {
				status = toProblem(err).Status
			}

			reqLogger.LogAttrs(req.Context(), slog.LevelInfo, "リクエストを処理しました",
				slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
				slog.Int("status", status),
				slog.Duration("latency", latency),
				slog.String("ip", c.RealIP()),
			)
			return err
		}
	}
}

// LoggerFrom はリクエストスコープの logger を返す。RequestLogging を通っていれば
// request_id 付き、通っていなければ slog.Default()。ハンドラから使う。
func LoggerFrom(c echo.Context) *slog.Logger {
	return logctx.From(c.Request().Context())
}
