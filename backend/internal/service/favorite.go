package service

import (
	"context"
	"errors"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrFavoriteExists は同じ献立が既にお気に入りに入っていることを表す（409）。
// 判定は DB の一意制約が担い、repository がこのエラーに変換する。
// アプリ側で「先に SELECT して無ければ INSERT」とすると、同時実行で
// すり抜ける余地が残るため、制約違反を受けてから変換する。
var ErrFavoriteExists = errors.New("この献立は既にお気に入りに登録されています")

// ErrFavoriteNotFound はそのユーザーのお気に入りに指定の献立が無いことを表す（404）。
//
// 他人のお気に入りを消そうとした場合もこれになる。お気に入りは
// (ユーザー, 献立) で一意なので、APIは自分の行しか指せず、他人の行に
// 到達する経路が無い。「他人のものだ」と伝える 403 は、他人が何を
// 登録しているかを漏らすことにもなるため返さない。
var ErrFavoriteNotFound = errors.New("お気に入りが見つかりません")

// FavoriteStore はお気に入りの永続化を抽象化する。実装は internal/repository。
type FavoriteStore interface {
	// Add はお気に入りを1件追加する。
	// 同じ (user, menu) が既にあれば ErrFavoriteExists、
	// 献立が存在しなければ repository.ErrMenuNotFound を返す。
	Add(ctx context.Context, userID domain.UserID, menuID domain.MenuID) error

	// List はユーザーのお気に入りを新しい順に返す。該当が無ければ空スライス。
	List(ctx context.Context, userID domain.UserID) ([]domain.Favorite, error)

	// Delete はお気に入りを1件削除する。該当が無ければ ErrFavoriteNotFound を返す。
	Delete(ctx context.Context, userID domain.UserID, menuID domain.MenuID) error
}

// FavoriteService はお気に入りの操作を担う。
// 履歴と違い件数の上限が無く、自動削除もしない（spec.md 2.6）。
type FavoriteService struct {
	store FavoriteStore
}

// NewFavoriteService は FavoriteService を生成する。
func NewFavoriteService(store FavoriteStore) *FavoriteService {
	return &FavoriteService{store: store}
}

// Add は認証済みユーザーのお気に入りに献立を1件追加する。
// userID は認証ミドルウェアが載せた検証済みの値。壊れていれば ErrUserNotFound（401）。
// menuID はリクエストボディ由来なので、不正なら ErrInvalidMenuID（400）。
func (s *FavoriteService) Add(ctx context.Context, userID, menuID string) error {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	mid, err := domain.ParseMenuID(menuID)
	if err != nil {
		return err
	}
	return s.store.Add(ctx, uid, mid)
}

// List は認証済みユーザーのお気に入りを新しい順に返す。
// 履歴と違い件数の上限が無いので、全件返す。
func (s *FavoriteService) List(ctx context.Context, userID string) ([]domain.Favorite, error) {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return s.store.List(ctx, uid)
}

// Delete は認証済みユーザーのお気に入りから献立を1件外す。
// 自分が登録していない献立は ErrFavoriteNotFound（404）。
func (s *FavoriteService) Delete(ctx context.Context, userID, menuID string) error {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	mid, err := domain.ParseMenuID(menuID)
	if err != nil {
		return err
	}
	return s.store.Delete(ctx, uid, mid)
}
