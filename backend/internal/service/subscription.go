package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrInvalidGrantMonths は付与する月数が1未満であることを表す。
var ErrInvalidGrantMonths = errors.New("付与する月数は1以上である必要があります")

// SubscriptionService は加入の付与と取消を担う。
//
// **CLI と将来の決済Webhook の両方がここを通る。** 状態遷移を一箇所に集めることで、
// 手動付与と決済由来の付与が別々のロジックに分かれることを防ぐ。
type SubscriptionService struct {
	store SubscriptionStore
	now   func() time.Time
}

// NewSubscriptionService は SubscriptionService を生成する。
func NewSubscriptionService(store SubscriptionStore, now func() time.Time) *SubscriptionService {
	if now == nil {
		now = time.Now
	}
	return &SubscriptionService{store: store, now: now}
}

// Grant は利用者に months か月のプレミアムを付与する。
//
// 期限は現在時刻から起算する。既存の加入があれば上書きするため、
// 期限切れや取消済みの利用者にも再付与できる。
func (s *SubscriptionService) Grant(ctx context.Context, userID domain.UserID, months int) error {
	if months < 1 {
		return fmt.Errorf("%w: %d", ErrInvalidGrantMonths, months)
	}

	return s.store.Upsert(ctx, domain.Subscription{
		UserID:           userID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: s.now().AddDate(0, months, 0),
		Provider:         domain.ProviderManual,
	})
}

// Revoke は加入を即時失効させる。
//
// **これは利用者都合の解約ではない。** 誤付与の是正や規約違反への対応といった
// 運用上の取消を想定しているため、期末まで待たない。利用者が自分の意思で解約する
// 場合は CancelAtPeriodEnd を立てて期末に失効させる（決済フェーズで実装）。
//
// 行は消さずに状態を遷移させる。解約時期の記録は、後に
// 「解約したのに課金された」と申し立てられたときの反証材料になる。
//
// 加入が無い場合は何もせず成功とする（冪等）。
func (s *SubscriptionService) Revoke(ctx context.Context, userID domain.UserID) error {
	sub, err := s.store.Find(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return err
	}

	sub.Status = domain.SubscriptionCanceled
	return s.store.Upsert(ctx, sub)
}
