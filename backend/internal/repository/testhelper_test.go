package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
)

// sharedDSN はテスト用Postgresの接続文字列。
// コンテナの起動は数秒かかるためパッケージ内で1度だけ行い、
// 各テストはトランザクションではなくテーブルのクリアで独立性を保つ。
var sharedDSN string

// TestMain はパッケージ内の全テストで共有するPostgresを起動する。
func TestMain(m *testing.M) {
	testcontainers.SkipIfProviderIsNotHealthy(&testing.T{})

	ctx := context.Background()
	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("menu_planner_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		panic("テスト用Postgresの起動に失敗しました: " + err.Error())
	}
	defer func() { _ = testcontainers.TerminateContainer(container) }()

	sharedDSN, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("接続文字列の取得に失敗しました: " + err.Error())
	}

	if err := db.Migrate(sharedDSN, db.MigrateUp); err != nil {
		panic("マイグレーションに失敗しました: " + err.Error())
	}

	m.Run()
}

// newTestPool はテスト用の接続プールを返す。
// テスト終了時に menus をクリアし、次のテストに影響を残さない。
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if sharedDSN == "" {
		t.Skip("Dockerが利用できないためスキップします")
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.Config{DSN: sharedDSN})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(), "TRUNCATE menus")
		require.NoError(t, err)
		pool.Close()
	})

	return pool
}
