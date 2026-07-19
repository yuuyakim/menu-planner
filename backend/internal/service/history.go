package service

import (
	"context"
	"errors"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// HistoryLimit はユーザーごとに保持する履歴の最大件数（spec.md 2.5・FIFO 15件）。
// 保持件数は業務ルールなので service で持ち、repository には値として渡す。
const HistoryLimit = 15

// 履歴削除にまつわるエラー。
var (
	// ErrHistoryNotFound は指定した履歴が存在しないことを表す（404）。
	ErrHistoryNotFound = errors.New("履歴が見つかりません")
	// ErrHistoryForbidden は他人の履歴を操作しようとしたことを表す（403）。
	ErrHistoryForbidden = errors.New("この履歴を操作する権限がありません")
)

// HistoryStore は履歴の永続化を抽象化する。実装は internal/repository。
type HistoryStore interface {
	// RecordWithLimit は履歴を1件記録し、最新 limit 件に切り詰める。
	RecordWithLimit(ctx context.Context, userID domain.UserID, menuID domain.MenuID, mode domain.SearchMode, limit int) error

	// RecordManyWithLimit は複数の献立を一括記録し、最新 limit 件に切り詰める。
	RecordManyWithLimit(ctx context.Context, userID domain.UserID, menuIDs []domain.MenuID, mode domain.SearchMode, limit int) error

	// List はユーザーの履歴を新しい順に返す。該当が無ければ空スライス。
	List(ctx context.Context, userID domain.UserID) ([]domain.HistoryEntry, error)

	// RecentMenuIDs は履歴に含まれる献立IDを返す（検索時の除外候補に使う）。
	RecentMenuIDs(ctx context.Context, userID domain.UserID) ([]domain.MenuID, error)

	// Delete は履歴を1件削除する。存在しなければ ErrHistoryNotFound、
	// 他人のものなら ErrHistoryForbidden を返す。
	Delete(ctx context.Context, userID domain.UserID, historyID domain.HistoryID) error

	// DeleteAll はユーザーの履歴を全件削除する。0件でも成功。
	DeleteAll(ctx context.Context, userID domain.UserID) error
}

// HistoryService は検索履歴の記録を担う。
type HistoryService struct {
	store HistoryStore
}

// NewHistoryService は HistoryService を生成する。
func NewHistoryService(store HistoryStore) *HistoryService {
	return &HistoryService{store: store}
}

// Record は履歴を1件記録し、FIFO で最新 HistoryLimit 件に保つ。
// userID は認証ミドルウェアが載せた検証済みの値。壊れていれば ErrUserNotFound。
func (s *HistoryService) Record(ctx context.Context, userID string, menuID domain.MenuID, mode domain.SearchMode) error {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	return s.store.RecordWithLimit(ctx, uid, menuID, mode, HistoryLimit)
}

// RecordMany は複数の献立を一括記録し、FIFO で最新 HistoryLimit 件に保つ。
// 週間献立の確定時に7件をまとめて記録するのに使う。
func (s *HistoryService) RecordMany(ctx context.Context, userID string, menuIDs []domain.MenuID, mode domain.SearchMode) error {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	return s.store.RecordManyWithLimit(ctx, uid, menuIDs, mode, HistoryLimit)
}

// RecentMenuIDs は直近履歴に含まれる献立IDを返す。検索時の除外候補に使う。
// FIFO で最大15件しか残らないため、これが「直近15件」の献立に相当する。
func (s *HistoryService) RecentMenuIDs(ctx context.Context, userID string) ([]domain.MenuID, error) {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return s.store.RecentMenuIDs(ctx, uid)
}

// List は認証済みユーザーの履歴を新しい順に返す。
// userID は認証ミドルウェアが検証したトークンの sub。壊れていれば
// ErrUserNotFound（呼び出し側で 401）に丸める。
func (s *HistoryService) List(ctx context.Context, userID string) ([]domain.HistoryEntry, error) {
	id, err := domain.ParseUserID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return s.store.List(ctx, id)
}

// Delete は認証済みユーザーの履歴を1件削除する。
// 不正な履歴IDは ErrInvalidHistoryID（400）、存在しなければ ErrHistoryNotFound（404）、
// 他人のものなら ErrHistoryForbidden（403）。
func (s *HistoryService) Delete(ctx context.Context, userID, historyID string) error {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	hid, err := domain.ParseHistoryID(historyID)
	if err != nil {
		return err
	}
	return s.store.Delete(ctx, uid, hid)
}

// DeleteAll は認証済みユーザーの履歴を全件削除する。
func (s *HistoryService) DeleteAll(ctx context.Context, userID string) error {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	return s.store.DeleteAll(ctx, uid)
}
