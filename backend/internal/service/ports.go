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

// RecipeSearchGateway はレシピ掲載ページの検索を抽象化する。
// 実装は internal/gateway にあり、検索API(Brave / Google CSE)と
// APIキー不要のスタブを差し替えられる。
type RecipeSearchGateway interface {
	// Search は献立名で検索し、上位 limit 件のリンクを返す。
	// 該当が無い場合は空スライスを返し、エラーにはしない（結果0件は障害ではない）。
	Search(ctx context.Context, menuName string, limit int) ([]domain.RecipeLink, error)
}

// Randomizer は乱数源を抽象化する。
// service 自身が乱数を生成すると提案結果が毎回変わりテストが書けないため、
// 乱数源を外から注入できるようにする。実装は internal/random にある。
type Randomizer interface {
	// Intn は [0, n) の整数を返す。n が 1 未満の場合はエラーを返す。
	Intn(n int) (int, error)
}
