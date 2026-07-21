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

// MenuIngredient は「どの献立に、どの食材を使うか」1件分の対応。
//
// 買い物リストでは同じ食材が複数の献立に出るため、食材だけでなく
// **どの献立で使うかも必要**になる（spec.md 5.5 の usedIn）。
type MenuIngredient struct {
	MenuID     domain.MenuID
	Ingredient domain.Ingredient
}

// IngredientRepository は献立に紐づく食材へのアクセスを抽象化する。
// 実装は internal/repository にある。
type IngredientRepository interface {
	// FindByMenuIDs は指定した献立に使う食材を、献立との対応つきで返す。
	//
	// 1件ずつ引くと献立の数だけ問い合わせが飛ぶため、まとめて引く。
	// 存在しない献立IDは黙って除く（件数の判断は呼び出し側の仕事）。
	// 並びは食材のカナ順。カテゴリ順への並べ替えは domain の知識なので
	// service 側で行う（SQLに CASE を書かない）。
	FindByMenuIDs(ctx context.Context, ids []domain.MenuID) ([]MenuIngredient, error)
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

// ErrEmailTaken はメールアドレスが既に登録済みであることを表す。
// 呼び出し側はこれを 409 に変換する。インターフェースの持ち主である
// service 側で定義し、repository はこれを返す。
var ErrEmailTaken = errors.New("メールアドレスは既に登録されています")

// ErrUserNotFound はIDに対応するユーザーが存在しないことを表す。
// 有効なトークンが指すユーザーが消えている場合などに使う。
var ErrUserNotFound = errors.New("ユーザーが見つかりません")

// ErrCredentialNotFound はメールに対応するパスワード認証が無いことを表す。
// 「ユーザーが存在しない」場合と「存在するが Google 認証のみ」場合の
// どちらもこれで表す。呼び出し側（Login）はこれとパスワード不一致を
// 区別せず同じ結果に丸め、ユーザーの存在を推測されないようにする。
var ErrCredentialNotFound = errors.New("パスワード認証が見つかりません")

// PasswordCredential はパスワード照合に必要な、ユーザーとハッシュの組。
type PasswordCredential struct {
	User         domain.User
	PasswordHash string
}

// UserRepository はユーザーと認証情報の永続化を抽象化する。
// 実装は internal/repository にある。
type UserRepository interface {
	// CreateWithPassword は user とパスワード認証の identity を
	// 1トランザクションで作る。片方だけ残る状態を防ぐため必ず両方一括で作る。
	// メールが既に使われている場合は ErrEmailTaken を返す。
	CreateWithPassword(ctx context.Context, u domain.User, passwordHash string) error

	// FindPasswordCredential はメールに対応するパスワード認証を返す。
	// ユーザーが居ない、または Google 認証のみでパスワードを持たない場合は
	// ErrCredentialNotFound を返す。
	FindPasswordCredential(ctx context.Context, email domain.Email) (PasswordCredential, error)

	// FindByID はIDでユーザーを取得する。存在しない場合は ErrUserNotFound を返す。
	FindByID(ctx context.Context, id domain.UserID) (domain.User, error)

	// FindOrCreateGoogleUser は Google 認証のユーザーを取得または作成する。
	//  1. (provider=google, provider_uid=sub) が既にあればそのユーザー。
	//  2. 無ければ email で既存ユーザーを探し、あれば google の identity を足す
	//     （パスワードと Google の共存。spec.md 1.4）。
	//  3. どちらも無ければ user と google identity を新規作成する。
	// displayName は新規作成時の表示名。
	FindOrCreateGoogleUser(ctx context.Context, sub string, email domain.Email, displayName string) (domain.User, error)
}

// PasswordHasher はパスワードのハッシュ化と検証を抽象化する。
// 実装は internal/auth にある。service にハッシュ手段を直接持たせず注入するのは、
// テストで差し替えられるようにし、乱数源と同じく依存を外に出すため。
type PasswordHasher interface {
	// Hash は平文パスワードをハッシュ化する。長さなどの検証もここで行う。
	Hash(plain string) (string, error)
	// Verify は平文がハッシュと一致するか検証する。
	Verify(hash, plain string) error
}

// Randomizer は乱数源を抽象化する。
// service 自身が乱数を生成すると提案結果が毎回変わりテストが書けないため、
// 乱数源を外から注入できるようにする。実装は internal/random にある。
type Randomizer interface {
	// Intn は [0, n) の整数を返す。n が 1 未満の場合はエラーを返す。
	Intn(n int) (int, error)
}
