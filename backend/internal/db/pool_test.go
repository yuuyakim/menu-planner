package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
)

func TestNewPool_DSNが不正ならエラー(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"空文字":       "",
		"スキーマが不正":   "not-a-dsn",
		"ポートが数値でない": "postgres://u:p@localhost:abc/db",
	}

	for name, dsn := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			pool, err := db.NewPool(ctx, db.Config{DSN: dsn})
			require.Error(t, err)
			assert.Nil(t, pool)
		})
	}
}

func TestNewPool_接続先が存在しなければエラー(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 到達できないポートを指定する。リトライを繰り返した末にエラーで返ること。
	pool, err := db.NewPool(ctx, db.Config{
		DSN:           "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1",
		ConnectRetry:  2,
		RetryInterval: 10 * time.Millisecond,
	})
	require.Error(t, err)
	assert.Nil(t, pool)
}

func TestNewPool_contextがキャンセル済みならエラー(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pool, err := db.NewPool(ctx, db.Config{
		DSN:           "postgres://u:p@127.0.0.1:1/db?sslmode=disable",
		ConnectRetry:  5,
		RetryInterval: time.Second,
	})
	require.Error(t, err)
	assert.Nil(t, pool)
}

func TestConfig_既定値が補完される(t *testing.T) {
	t.Parallel()

	c := db.Config{DSN: "postgres://u:p@h:5432/d"}.WithDefaults()

	// Neon はアイドル時にスリープするため、初回接続はリトライ前提にする。
	assert.Positive(t, c.ConnectRetry)
	assert.Positive(t, c.RetryInterval)
	assert.Positive(t, c.MaxConns)
}

func TestConfig_明示値は上書きされない(t *testing.T) {
	t.Parallel()

	c := db.Config{
		DSN:           "postgres://u:p@h:5432/d",
		ConnectRetry:  9,
		RetryInterval: 3 * time.Second,
		MaxConns:      7,
	}.WithDefaults()

	assert.Equal(t, 9, c.ConnectRetry)
	assert.Equal(t, 3*time.Second, c.RetryInterval)
	assert.Equal(t, int32(7), c.MaxConns)
}
