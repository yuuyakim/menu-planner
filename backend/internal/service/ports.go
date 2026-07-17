// Package service はドメインロジックを担う。
// repository / gateway の実装を知らず、本ファイルで定義したインターフェースにのみ依存する
// （依存関係逆転の原則）。これによりインフラの差し替えが service に波及しない。
package service

import (
	"context"
	"errors"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrRecipeSearchFailed は外部の検索APIから結果を得られなかったことを表す。
// 呼び出し側はこれを 502 に変換する。
//
// インターフェースの持ち主である service 側で定義する。実装（internal/gateway）で
// 定義すると、service がそれを参照できず（gateway → service の一方向依存のため
// 逆向きは循環になる）、この層で失敗を判定できなくなる。
var ErrRecipeSearchFailed = errors.New("レシピの検索に失敗しました")

// MenuRepository は献立マスタへのアクセスを抽象化する。
// 実装は internal/repository にある。
type MenuRepository interface {
	// FindByID はIDで献立を1件取得する。存在しない場合はエラーを返す。
	FindByID(ctx context.Context, id domain.MenuID) (*domain.Menu, error)

	// FindByIDs は複数のIDで献立をまとめて取得する。
	// 見つからないIDは黙って除く（呼び出し側が件数で判断できる）。
	// 1件ずつ FindByID を呼ぶとID数だけ問い合わせが飛ぶため、まとめて引く。
	FindByIDs(ctx context.Context, ids []domain.MenuID) ([]domain.Menu, error)

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

// ErrRecipeCacheMiss はキャッシュに該当が無いことを表す。
// 障害ではなく通常の結果なので、呼び出し側は検索APIに問い合わせればよい。
var ErrRecipeCacheMiss = errors.New("レシピのキャッシュがありません")

// CachedRecipeLinks はキャッシュされたレシピリンクと、それを取得した時刻。
// 鮮度の判定は保存側ではなく service が行うため、時刻をそのまま返す。
type CachedRecipeLinks struct {
	Links     []domain.RecipeLink
	FetchedAt time.Time
}

// RecipeLinkCache はレシピリンクのキャッシュを抽象化する。
// 実装は internal/repository にある。
//
// キャッシュのキーが献立IDであるため、この抽象は献立IDを知っている層
// （service）に置く。RecipeSearchGateway は献立名しか受け取らないため、
// gateway をキャッシュで包む形にはできない。
type RecipeLinkCache interface {
	// Find は献立IDに対応するキャッシュを返す。
	// 該当が無い場合は ErrRecipeCacheMiss を返す。
	Find(ctx context.Context, id domain.MenuID) (CachedRecipeLinks, error)

	// Save はキャッシュを保存する。既存があれば上書きする。
	Save(ctx context.Context, id domain.MenuID, links []domain.RecipeLink, fetchedAt time.Time) error
}

// Randomizer は乱数源を抽象化する。
// service 自身が乱数を生成すると提案結果が毎回変わりテストが書けないため、
// 乱数源を外から注入できるようにする。実装は internal/random にある。
type Randomizer interface {
	// Intn は [0, n) の整数を返す。n が 1 未満の場合はエラーを返す。
	Intn(n int) (int, error)
}
