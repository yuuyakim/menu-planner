package service

import (
	"context"
	"errors"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// EntitlementService は「今この利用者が何を使えるか」を答える。
//
// 期限切れをバッチでDBに書き戻す方式は採らない。バッチが停止すると、
// 課金していない利用者がプレミアムのまま残るため。参照のたびに計算すれば
// 真実は常に1つになる。
type EntitlementService struct {
	store SubscriptionStore
	now   func() time.Time
}

// NewEntitlementService は EntitlementService を生成する。
// now は現在時刻の取得。期限判定を時計に依存させないため注入する
// （既存の Randomizer と同じ理由で外部要因を service の外に出す）。
func NewEntitlementService(store SubscriptionStore, now func() time.Time) *EntitlementService {
	if now == nil {
		now = time.Now
	}
	return &EntitlementService{store: store, now: now}
}

// For は利用者のエンタイトルメントを返す。
//
// 未認証（userID が空）や解釈できないIDは free として扱い、エラーにしない。
// 未認証でも使える機能があるため、ここで締め出すのは認証層の仕事の横取りになる。
//
// 一方、保存を引けなかった場合はエラーを返す。「加入が無い」と「引けなかった」を
// 同じ free に丸めると、障害中に課金済みの利用者が黙って free へ落ちる。
func (s *EntitlementService) For(ctx context.Context, userID string) (domain.Entitlement, error) {
	free := domain.NewEntitlement(domain.PlanFree)

	if userID == "" {
		return free, nil
	}
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return free, nil
	}

	sub, err := s.store.Find(ctx, uid)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return free, nil
		}
		return domain.Entitlement{}, err
	}

	if !sub.IsActiveAt(s.now()) {
		return free, nil
	}
	return domain.NewEntitlement(sub.Plan), nil
}
