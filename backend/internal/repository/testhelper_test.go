package repository_test

import (
	"context"
	"log"
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
// Docker が使えない環境では空のままになり、各テストは newTestPool でスキップされる。
var sharedDSN string

// dockerAvailable は Docker が利用できるかを返す。
//
// testcontainers.SkipIfProviderIsNotHealthy は同じ判定をするが、内部で t.Skip
// （＝runtime.Goexit）を呼ぶため TestMain からは使えない。TestMain は
// テスト本体の goroutine ではないので、Goexit すると走る goroutine が無くなり
// スキップではなく deadlock でクラッシュする。判定だけを自前で行う。
func dockerAvailable() error {
	provider, err := testcontainers.ProviderDocker.GetProvider()
	if err != nil {
		return err
	}
	return provider.Health(context.Background())
}

// TestMain はパッケージ内の全テストで共有するPostgresを起動する。
func TestMain(m *testing.M) {
	// Docker が無い環境ではコンテナを起動せずテストへ進む。sharedDSN が空のまま
	// なので、DBを要するテストは newTestPool がスキップする。
	if err := dockerAvailable(); err != nil {
		log.Printf("Dockerが利用できないため、DBを要するテストをスキップします: %v", err)
		m.Run()
		return
	}

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
