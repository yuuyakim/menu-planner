// Package db はPostgresへの接続を担う。
// 上位層はこのパッケージのインターフェースにのみ依存し、pgx の存在を知らない。
package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 既定値。Neon の無料枠はアイドル時に自動スリープするため、
// 初回接続はコールドスタートで数秒かかることを前提にする。
const (
	defaultConnectRetry  = 5
	defaultRetryInterval = 500 * time.Millisecond
	defaultMaxConns      = 10
	defaultMinConns      = 0
	defaultMaxConnLife   = time.Hour
	defaultMaxConnIdle   = 30 * time.Minute
)

// Config は接続プールの設定。
type Config struct {
	DSN string
	// ConnectRetry は初回接続の試行回数（1 なら再試行しない）。
	ConnectRetry int
	// RetryInterval はリトライの初期待ち時間。試行ごとに倍にする。
	RetryInterval time.Duration
	MaxConns      int32
}

// WithDefaults は未指定の項目を既定値で補完した Config を返す。
func (c Config) WithDefaults() Config {
	if c.ConnectRetry <= 0 {
		c.ConnectRetry = defaultConnectRetry
	}
	if c.RetryInterval <= 0 {
		c.RetryInterval = defaultRetryInterval
	}
	if c.MaxConns <= 0 {
		c.MaxConns = defaultMaxConns
	}
	return c
}

// NewPool は接続プールを生成し、疎通を確認してから返す。
// Neon のコールドスタートを考慮し、疎通確認は指数バックオフでリトライする。
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	cfg = cfg.WithDefaults()

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("DSNの解析に失敗しました: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = defaultMinConns
	poolCfg.MaxConnLifetime = defaultMaxConnLife
	poolCfg.MaxConnIdleTime = defaultMaxConnIdle

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("接続プールの生成に失敗しました: %w", err)
	}

	if err := ping(ctx, pool, cfg); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// ping は疎通確認を指数バックオフでリトライする。
func ping(ctx context.Context, pool *pgxpool.Pool, cfg Config) error {
	wait := cfg.RetryInterval
	var lastErr error

	for attempt := 1; attempt <= cfg.ConnectRetry; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("DBへの接続を中断しました: %w", errors.Join(err, lastErr))
		}

		err := pool.Ping(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt == cfg.ConnectRetry {
			break
		}

		slog.Warn("DBへの接続に失敗しました。リトライします",
			"attempt", attempt, "max", cfg.ConnectRetry, "wait", wait, "error", lastErr)

		select {
		case <-ctx.Done():
			return fmt.Errorf("DBへの接続を中断しました: %w", errors.Join(ctx.Err(), lastErr))
		case <-time.After(wait):
		}
		wait *= 2
	}

	return fmt.Errorf("DBへ接続できませんでした(%d回試行): %w", cfg.ConnectRetry, lastErr)
}
