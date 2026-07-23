package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/logctx"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// SubscriptionRepository は有料プランの加入を Postgres に保存する。
type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

// NewSubscriptionRepository は SubscriptionRepository を生成する。
func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool}
}

// Find は利用者の加入を返す。該当が無い場合は service.ErrSubscriptionNotFound を返す。
func (r *SubscriptionRepository) Find(
	ctx context.Context, userID domain.UserID,
) (domain.Subscription, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT plan, status, current_period_end, cancel_at_period_end,
		        provider, provider_subscription_id
		   FROM subscriptions WHERE user_id = $1`, userID.String())

	var (
		rawPlan, rawStatus, provider string
		providerSubID                *string
		sub                          domain.Subscription
	)
	if err := row.Scan(&rawPlan, &rawStatus, &sub.CurrentPeriodEnd,
		&sub.CancelAtPeriodEnd, &provider, &providerSubID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Subscription{}, service.ErrSubscriptionNotFound
		}
		return domain.Subscription{}, fmt.Errorf("加入の取得に失敗しました: %w", err)
	}

	// **未知の値でエラーにしない。** ここで弾くと EntitlementService がそれを返し、
	// /auth/me が 500 になって、ログイン済みの利用者がアプリを一切使えなくなる。
	// 未知のプランは domain.Entitlement が free に落とし、未知の状態は
	// IsActiveAt が false にするため、そのまま通しても安全側に倒れる
	// （プレミアムとして通ってしまう経路は無い）。
	// 000010 が CHECK 制約を張らず「決済事業者ごとに増える値を DDL の変更なしに
	// 受けられる」ことを狙っているのも、この読み方があってはじめて成立する。
	//
	// ただし黙って握りつぶすとデータの壊れに気づけないため、警告だけは残す。
	sub.Plan = domain.Plan(rawPlan)
	sub.Status = domain.SubscriptionStatus(rawStatus)
	if !sub.Plan.Valid() || !sub.Status.Valid() {
		logctx.From(ctx).WarnContext(ctx, "加入に未知の値が入っています。freeとして扱います",
			slog.String("user_id", userID.String()),
			slog.String("plan", rawPlan),
			slog.String("status", rawStatus))
	}

	sub.UserID = userID
	sub.Provider = provider
	if providerSubID != nil {
		sub.ProviderSubscriptionID = *providerSubID
	}
	return sub, nil
}

// Upsert は加入を保存する。既にあれば上書きする。
func (r *SubscriptionRepository) Upsert(ctx context.Context, sub domain.Subscription) error {
	// 空文字をそのまま入れると部分UNIQUE索引が「空文字は1行だけ」を強制してしまう。
	// 決済IDを持たない手動付与は NULL にする。
	var providerSubID *string
	if sub.ProviderSubscriptionID != "" {
		providerSubID = &sub.ProviderSubscriptionID
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO subscriptions
		   (user_id, plan, status, current_period_end, cancel_at_period_end,
		    provider, provider_subscription_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (user_id) DO UPDATE SET
		   plan                     = EXCLUDED.plan,
		   status                   = EXCLUDED.status,
		   current_period_end       = EXCLUDED.current_period_end,
		   cancel_at_period_end     = EXCLUDED.cancel_at_period_end,
		   provider                 = EXCLUDED.provider,
		   provider_subscription_id = EXCLUDED.provider_subscription_id,
		   updated_at               = now()`,
		sub.UserID.String(), sub.Plan.String(), sub.Status.String(),
		sub.CurrentPeriodEnd, sub.CancelAtPeriodEnd, sub.Provider, providerSubID)
	if err != nil {
		return fmt.Errorf("加入の保存に失敗しました: %w", err)
	}
	return nil
}
