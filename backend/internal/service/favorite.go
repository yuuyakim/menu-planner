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

// FavoriteStore はお気に入りの永続化を抽象化する。実装は internal/repository。
type FavoriteStore interface {
	// Add はお気に入りを1件追加する。
	// 同じ (user, menu) が既にあれば ErrFavoriteExists、
	// 献立が存在しなければ repository.ErrMenuNotFound を返す。
	Add(ctx context.Context, userID domain.UserID, menuID domain.MenuID) error
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
