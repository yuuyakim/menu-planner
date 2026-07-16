package db

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres ドライバの登録
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/yuuyakim/menu-planner/backend/db/migrations"
)

// MigrateDirection はマイグレーションの操作種別。
type MigrateDirection string

// 実行できる操作。
const (
	// MigrateUp は未適用のマイグレーションを全て適用する。
	MigrateUp MigrateDirection = "up"
	// MigrateDown は直近のマイグレーションを1つ戻す。
	MigrateDown MigrateDirection = "down"
	// MigrateVersion は現在のバージョンを表示するだけで変更しない。
	MigrateVersion MigrateDirection = "version"
)

// ErrInvalidDirection は未知の操作種別を表す。
var ErrInvalidDirection = errors.New("不正なマイグレーション操作です")

// ParseMigrateDirection は文字列を MigrateDirection に変換する。
func ParseMigrateDirection(s string) (MigrateDirection, error) {
	switch d := MigrateDirection(s); d {
	case MigrateUp, MigrateDown, MigrateVersion:
		return d, nil
	default:
		return "", fmt.Errorf("%w: %q (up | down | version)", ErrInvalidDirection, s)
	}
}

// MigrationFiles は埋め込まれたマイグレーションSQLのファイル名を返す。
func MigrationFiles() ([]string, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("マイグレーションの読み込みに失敗しました: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Migrate は埋め込まれたマイグレーションを DSN のDBに適用する。
func Migrate(dsn string, dir MigrateDirection) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("マイグレーション定義の読み込みに失敗しました: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("マイグレータの生成に失敗しました: %w", err)
	}
	defer func() {
		// Close はソースとDBの両方のエラーを返すが、ここでは記録のみ行う
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			slog.Warn("マイグレータのクローズに失敗しました", "source", srcErr, "db", dbErr)
		}
	}()

	switch dir {
	case MigrateVersion:
		v, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			slog.Info("マイグレーションは未適用です")
			return nil
		}
		if err != nil {
			return fmt.Errorf("バージョンの取得に失敗しました: %w", err)
		}
		slog.Info("現在のマイグレーション", "version", v, "dirty", dirty)
		return nil

	case MigrateUp:
		err = m.Up()
	case MigrateDown:
		err = m.Steps(-1)
	default:
		return fmt.Errorf("%w: %q", ErrInvalidDirection, dir)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		slog.Info("適用すべきマイグレーションはありません")
		return nil
	}
	if err != nil {
		return fmt.Errorf("マイグレーションに失敗しました(%s): %w", dir, err)
	}

	slog.Info("マイグレーションを適用しました", "direction", dir)
	return nil
}
