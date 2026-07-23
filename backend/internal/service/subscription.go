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
// **有効な加入が残っていれば、その期末から積み増す。** 現在時刻から起算すると、
// 残期間の長い利用者への再付与が期間の短縮になる（10か月残っている人に
// 1か月足したつもりが1か月に縮む）。Upsert は同じ行を上書きするので履歴も残らず、
// 利用者は理由の分からないまま突然 free に落ちる。期限切れ・取消済みの加入は
// 積み増す残りが無いので、現在時刻から起算する。
func (s *SubscriptionService) Grant(ctx context.Context, userID domain.UserID, months int) error {
	if months < 1 {
		return fmt.Errorf("%w: %d", ErrInvalidGrantMonths, months)
	}

	now := s.now()
	base := now
	sub, err := s.store.Find(ctx, userID)
	switch {
	case err == nil:
		if sub.IsActiveAt(now) {
			base = sub.CurrentPeriodEnd
		}
	case errors.Is(err, ErrSubscriptionNotFound):
		// 初回付与。base は現在時刻のまま。
	default:
		return err
	}

	return s.store.Upsert(ctx, domain.Subscription{
		UserID:           userID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: addMonths(base, months),
		Provider:         domain.ProviderManual,
	})
}

// addMonths は months か月後を返す。翌月に同じ日が無ければ月末に丸める。
//
// time.AddDate は 1/31 の1か月後を 3/3 に繰り上げてしまう。「1か月付与」が
// 32〜34日になるため、月末に付与した利用者だけが得をする。決済を導入すると
// 同じズレが「課金されていない数日間だけプレミアムが有効」として表に出る。
func addMonths(t time.Time, months int) time.Time {
	shifted := t.AddDate(0, months, 0)
	// 繰り上がったかどうかは「日が変わったか」で分かる（1/31 → 3/3 は 31≠3）。
	if shifted.Day() != t.Day() {
		// 繰り上がり先の月初から1日戻せば、意図した月の末日になる。
		return shifted.AddDate(0, 0, -shifted.Day())
	}
	return shifted
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
