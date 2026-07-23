package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// subscriptions のスキーマ（主キー / CASCADE / 部分UNIQUE）を生SQLで直接確かめる。
// 制約はDBが守るものなので、DBに対して検証する。

// insertSubscription は加入を1件入れる。provider_subscription_id は空文字なら NULL にする。
func insertSubscription(
	t *testing.T, pool *pgxpool.Pool, userID domain.UserID, providerSubID string,
) error {
	t.Helper()
	var psid *string
	if providerSubID != "" {
		psid = &providerSubID
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO subscriptions
		   (user_id, plan, status, current_period_end, provider, provider_subscription_id)
		 VALUES ($1, 'premium', 'active', now() + interval '30 days', 'manual', $2)`,
		userID.String(), psid)
	return err
}

func TestSubscriptionsSchema_1ユーザーに2件は入らない(t *testing.T) {
	pool := newTestPool(t)

	u := createUser(t, pool, "sub-pk@example.com")

	require.NoError(t, insertSubscription(t, pool, u.ID, ""))
	// user_id が主キー。複数同時加入は仕様にない。
	require.Error(t, insertSubscription(t, pool, u.ID, ""),
		"1ユーザーに2件目の加入が入ってはいけない")
}

func TestSubscriptionsSchema_ユーザーを消すと加入も消える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "sub-cascade@example.com")
	require.NoError(t, insertSubscription(t, pool, u.ID, ""))

	_, err := pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID.String())
	require.NoError(t, err)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM subscriptions WHERE user_id=$1`, u.ID.String()).Scan(&count))
	require.Zero(t, count, "ユーザー削除で加入も消えるべき")
}

func TestSubscriptionsSchema_同じ決済IDは2行入らない(t *testing.T) {
	pool := newTestPool(t)

	a := createUser(t, pool, "sub-dup-a@example.com")
	b := createUser(t, pool, "sub-dup-b@example.com")

	require.NoError(t, insertSubscription(t, pool, a.ID, "sub_ABC"))
	// 将来 Webhook が同じイベントを二度配送しても、DBが二重適用を弾く。
	require.Error(t, insertSubscription(t, pool, b.ID, "sub_ABC"),
		"同じ決済IDが2行に入ってはいけない")
}

func TestSubscriptionsSchema_決済IDがNULLなら複数行入る(t *testing.T) {
	pool := newTestPool(t)

	a := createUser(t, pool, "sub-null-a@example.com")
	b := createUser(t, pool, "sub-null-b@example.com")

	// 手動付与は決済IDを持たない。部分索引なので NULL は重複と見なさない。
	require.NoError(t, insertSubscription(t, pool, a.ID, ""))
	require.NoError(t, insertSubscription(t, pool, b.ID, ""),
		"手動付与（決済IDなし）は何件でも入るべき")
}

func TestSubscriptionsSchema_存在しないユーザーには入らない(t *testing.T) {
	pool := newTestPool(t)

	orphan, err := domain.ParseUserID(uuid.NewString())
	require.NoError(t, err)

	require.Error(t, insertSubscription(t, pool, orphan, ""),
		"存在しないユーザーの加入は外部キーで拒否されるべき")
}

func TestSubscriptionsSchema_既定値(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	u := createUser(t, pool, "sub-defaults@example.com")
	require.NoError(t, insertSubscription(t, pool, u.ID, ""))

	var cancelAtPeriodEnd bool
	var createdAt, updatedAt time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT cancel_at_period_end, created_at, updated_at
		   FROM subscriptions WHERE user_id=$1`, u.ID.String()).
		Scan(&cancelAtPeriodEnd, &createdAt, &updatedAt))

	require.False(t, cancelAtPeriodEnd, "解約予約の既定は false であるべき")
	require.False(t, createdAt.IsZero())
	require.False(t, updatedAt.IsZero())
}
