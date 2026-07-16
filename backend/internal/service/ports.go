// Package service はドメインロジックを担う。
// repository / gateway の実装を知らず、本ファイルで定義したインターフェースにのみ依存する
// （依存関係逆転の原則）。これによりインフラの差し替えが service に波及しない。
package service

import (
	"context"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// MenuRepository は献立マスタへのアクセスを抽象化する。
// 実装は internal/repository にある。
type MenuRepository interface {
	// FindByID はIDで献立を1件取得する。存在しない場合はエラーを返す。
	FindByID(ctx context.Context, id domain.MenuID) (*domain.Menu, error)

	// FindByFilter は条件に合う献立を返す。該当が無い場合は空スライスを返す。
	FindByFilter(ctx context.Context, f domain.MenuFilter) ([]domain.Menu, error)
}
