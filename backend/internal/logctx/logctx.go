// Package logctx はリクエストスコープの logger を context に載せて運ぶ。
//
// handler がリクエストごとに request_id を付けた logger を context に入れ、
// service など下位層が From(ctx) で取り出して使う。これにより request_id が
// 層をまたいで全ログに伝播する。handler パッケージに置くと service から参照できず
// import 循環になるため、どちらからも依存できる中立な場所に置く。
package logctx

import (
	"context"
	"log/slog"
)

// loggerKey は context に logger を載せる際のキー。
// 他のパッケージの値と衝突しないよう、非公開の型を使う。
type loggerKey struct{}

// WithLogger は logger を載せた新しい context を返す。
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// From は context に載っている logger を返す。無ければ slog.Default()。
// 呼び出し側が nil チェックをしなくて済むよう、常に使える logger を返す。
func From(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}
