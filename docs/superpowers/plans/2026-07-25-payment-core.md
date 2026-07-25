# 決済コア（Stripe サブスク課金）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 利用者が Stripe を通じてプレミアムに加入し、正しく課金される購入フロー一式（申込確認画面→Stripe Checkout→Webhook 同期→5日トライアル→past_due 猶予）を実装する。

**Architecture:** 自前の申込確認画面 → Stripe Checkout（ホスト型）でカード入力 → Stripe Webhook を真実の源として `subscriptions` 行へ状態同期。`stripe-go` は薄い `PaymentGateway` adapter に隔離し、service/handler は自前 port 越しに叩く（CI で実 Stripe を叩かない）。エンタイトルメントは `trialing` と `past_due` 猶予を premium として評価するよう修正。

**Tech Stack:** Go（echo v4, pgx v5, golang-migrate, testcontainers, stripe-go）、React 19（react-router, TanStack Query, Vitest+MSW）、Postgres（Neon）。

## Global Constraints

- 価格 **¥300（税込）月額**、通貨 `jpy`。
- トライアル **5日**、**初回加入のみ**（加入行が過去に存在しないユーザーだけ付与）。
- past_due 猶予 **7日**（`PaymentGracePeriod = 7 * 24 * time.Hour`）、猶予中も premium（利用規約4条7項）。
- 解約導線の文言は **「アカウント設定 > プランの管理」**（申込確認画面が表示する。画面自体は別サブプロジェクト）。
- **カード番号を自社サーバに通さない**（Stripe Checkout ホスト型のみ）。
- 本物の行は `provider = "stripe"` + 非NULL `provider_subscription_id` + `provider_customer_id`。
- Webhook は **Stripe の値をそのまま upsert**（月数計算しない。真実は Stripe）。`SubscriptionService.Grant/Revoke` は CLI 用に据え置く。
- **真実の源は Webhook**。復帰ページは `/auth/me` をポーリングして反映を待つ。
- **CI で実 Stripe を叩かない**（fake gateway / `stripe.GenerateTestSignedPayload`）。
- `current_period_end` は **subscription item 側**（`sub.Items.Data[0].CurrentPeriodEnd`。近年の Stripe API で subscription 直下から移動）。`Status` / `CancelAtPeriodEnd` は subscription 直下。
- Go module: `github.com/yuuyakim/menu-planner/backend`。
- マイグレーション `000012` は**本番 Neon で手動実行**してからバックエンドをデプロイ（000011 と同じ運用）。
- 環境変数 `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` / `STRIPE_PRICE_ID` は**起動時 fatal**（`JWT_SECRET` と同じ扱い）。
- TDD（🔴 test → 🟢 impl を別コミット）、1挙動1コミット、DRY / YAGNI。

---

### Task 1: ドメイン — trialing / stripe / provider_customer_id / GivesPremiumAt / 猶予

**Files:**
- Modify: `backend/internal/domain/subscription.go`
- Test: `backend/internal/domain/subscription_test.go`

**Interfaces:**
- Produces:
  - `domain.SubscriptionTrialing SubscriptionStatus = "trialing"`
  - `domain.ProviderStripe = "stripe"`
  - `domain.PaymentGracePeriod = 7 * 24 * time.Hour`
  - `Subscription.ProviderCustomerID string`（構造体フィールド追加）
  - `func (s Subscription) GivesPremiumAt(now time.Time) bool`
- Consumes: なし
- 注意: `IsActiveAt` は本タスクでは**残す**（`EntitlementService.For` がまだ使うため。Task 3 で置換・削除する）。

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/domain/subscription_test.go` に追記（既存のテストがあれば末尾に追加）:

```go
func TestSubscription_GivesPremiumAt(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	cases := []struct {
		name string
		sub  domain.Subscription
		want bool
	}{
		{"active 期間内", domain.Subscription{Status: domain.SubscriptionActive, CurrentPeriodEnd: future}, true},
		{"active 期限切れ", domain.Subscription{Status: domain.SubscriptionActive, CurrentPeriodEnd: past}, false},
		{"trialing 期間内", domain.Subscription{Status: domain.SubscriptionTrialing, CurrentPeriodEnd: future}, true},
		{"past_due 猶予内", domain.Subscription{Status: domain.SubscriptionPastDue, CurrentPeriodEnd: now.Add(-3 * 24 * time.Hour)}, true},
		{"past_due 猶予超過", domain.Subscription{Status: domain.SubscriptionPastDue, CurrentPeriodEnd: now.Add(-8 * 24 * time.Hour)}, false},
		{"canceled", domain.Subscription{Status: domain.SubscriptionCanceled, CurrentPeriodEnd: future}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sub.GivesPremiumAt(now); got != tc.want {
				t.Errorf("GivesPremiumAt() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseSubscriptionStatus_Trialing(t *testing.T) {
	got, err := domain.ParseSubscriptionStatus("trialing")
	if err != nil {
		t.Fatalf("想定外のエラー: %v", err)
	}
	if got != domain.SubscriptionTrialing {
		t.Errorf("= %q, want %q", got, domain.SubscriptionTrialing)
	}
}
```

- [ ] **Step 2: 失敗を確認**

Run: `cd backend && go test ./internal/domain/ -run 'GivesPremiumAt|Trialing' -v`
Expected: コンパイルエラー（`SubscriptionTrialing` / `GivesPremiumAt` 未定義）。

- [ ] **Step 3: 実装する**

`backend/internal/domain/subscription.go`:

- const ブロック（`:15-19`）に追加:
```go
	SubscriptionTrialing SubscriptionStatus = "trialing"
```
- `ProviderManual` の近く（`:23` 付近）に追加:
```go
// ProviderStripe は Stripe 決済で作られた加入を表す provider の値。
const ProviderStripe = "stripe"

// PaymentGracePeriod は支払い失敗（past_due）後もプレミアムを維持する猶予。
// 利用規約4条7項の「7日間の猶予期間」に対応する。実際の打ち切りは Stripe の
// 督促設定が行い、この猶予は安全弁として二重に効かせる。
const PaymentGracePeriod = 7 * 24 * time.Hour
```
- `Valid()` の switch（`:36-41`）に `SubscriptionTrialing` を追加:
```go
	case SubscriptionActive, SubscriptionTrialing, SubscriptionPastDue, SubscriptionCanceled:
		return true
```
- 構造体に `ProviderSubscriptionID` の後（`:62` 付近）へ追加:
```go
	// ProviderCustomerID は決済事業者側の顧客ID。手動付与では空。
	// 顧客の再利用と将来の解約/ポータル画面で使う。
	ProviderCustomerID string
```
- `IsActiveAt` の直後に `GivesPremiumAt` を追加（`IsActiveAt` は残す。Task 3 で置換）:
```go
// GivesPremiumAt は now 時点でこの加入がプレミアム権限を与えるかを返す。
//
// active / trialing は期間内なら premium。past_due は支払い失敗後の状態だが、
// 利用規約の猶予（PaymentGracePeriod）内はプレミアムを維持する。それ以外
// （canceled や未知の状態）は権限を与えない（安全側）。
func (s Subscription) GivesPremiumAt(now time.Time) bool {
	switch s.Status {
	case SubscriptionActive, SubscriptionTrialing:
		return s.CurrentPeriodEnd.After(now)
	case SubscriptionPastDue:
		return now.Before(s.CurrentPeriodEnd.Add(PaymentGracePeriod))
	default:
		return false
	}
}
```

- [ ] **Step 4: 成功を確認**

Run: `cd backend && go test ./internal/domain/ -v`
Expected: PASS。

- [ ] **Step 5: コミット**

```bash
git add backend/internal/domain/subscription.go backend/internal/domain/subscription_test.go
git commit -m "feat: 加入に trialing / stripe / 顧客ID / プレミアム判定と猶予を追加"
```

---

### Task 2: マイグレーション 000012 + repository の provider_customer_id

**Files:**
- Create: `backend/db/migrations/000012_add_provider_customer_id.up.sql`
- Create: `backend/db/migrations/000012_add_provider_customer_id.down.sql`
- Modify: `backend/internal/repository/subscription.go`
- Test: `backend/internal/repository/subscription_test.go`（既存の integration テストがあれば追記。無ければ新規。testcontainers を使う既存テストの形に合わせる）

**Interfaces:**
- Consumes: `domain.Subscription.ProviderCustomerID`（Task 1）
- Produces: `Find`/`Upsert` が `provider_customer_id` 列を読み書きする

- [ ] **Step 1: マイグレーションを書く**

`000012_add_provider_customer_id.up.sql`:
```sql
-- 決済事業者側の顧客ID。顧客の再利用と将来の解約/ポータル画面で使う。
-- 手動付与（provider='manual'）では NULL のまま。
ALTER TABLE subscriptions ADD COLUMN provider_customer_id text;
```
`000012_add_provider_customer_id.down.sql`:
```sql
ALTER TABLE subscriptions DROP COLUMN provider_customer_id;
```

- [ ] **Step 2: 失敗するテストを書く**

`backend/internal/repository/subscription_test.go` に、Upsert→Find で顧客IDが往復することを検証するケースを追加（既存テストのセットアップ／プール取得ヘルパを流用する）:

```go
func TestSubscriptionRepository_ProviderCustomerIDRoundTrip(t *testing.T) {
	// 既存テストと同じ方法でプールとユーザーを用意する（testcontainers）。
	pool := newTestPool(t)          // 既存ヘルパ名に合わせて調整する
	userID := insertTestUser(t, pool) // 同上
	repo := repository.NewSubscriptionRepository(pool)
	ctx := context.Background()

	sub := domain.Subscription{
		UserID:                 userID,
		Plan:                   domain.PlanPremium,
		Status:                 domain.SubscriptionTrialing,
		CurrentPeriodEnd:       time.Now().Add(120 * time.Hour),
		Provider:               domain.ProviderStripe,
		ProviderSubscriptionID: "sub_test_123",
		ProviderCustomerID:     "cus_test_123",
	}
	if err := repo.Upsert(ctx, sub); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := repo.Find(ctx, userID)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.ProviderCustomerID != "cus_test_123" {
		t.Errorf("ProviderCustomerID = %q, want %q", got.ProviderCustomerID, "cus_test_123")
	}
}
```

- [ ] **Step 3: 失敗を確認**

Run: `cd backend && go test ./internal/repository/ -run ProviderCustomerIDRoundTrip -v`
Expected: FAIL（`provider_customer_id` を Find が読まない／列が無い）。Docker が要る（testcontainers）。

- [ ] **Step 4: repository を実装する**

`backend/internal/repository/subscription.go`:

- `Find` の SELECT（`:31-34`）に列を追加:
```go
	row := r.pool.QueryRow(ctx,
		`SELECT plan, status, current_period_end, cancel_at_period_end,
		        provider, provider_subscription_id, provider_customer_id
		   FROM subscriptions WHERE user_id = $1`, userID.String())
```
- スキャン用の変数（`:36-40`）に `providerCustID *string` を追加し、`Scan` に足す:
```go
	var (
		rawPlan, rawStatus, provider string
		providerSubID, providerCustID *string
		sub                          domain.Subscription
	)
	if err := row.Scan(&rawPlan, &rawStatus, &sub.CurrentPeriodEnd,
		&sub.CancelAtPeriodEnd, &provider, &providerSubID, &providerCustID); err != nil {
```
- `providerSubID` を詰める箇所（`:69-71`）の後に追加:
```go
	if providerCustID != nil {
		sub.ProviderCustomerID = *providerCustID
	}
```
- `Upsert`（`:76-98`）に列を追加。空文字は NULL にする（顧客IDを持たない手動付与のため）:
```go
	var providerCustID *string
	if sub.ProviderCustomerID != "" {
		providerCustID = &sub.ProviderCustomerID
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO subscriptions
		   (user_id, plan, status, current_period_end, cancel_at_period_end,
		    provider, provider_subscription_id, provider_customer_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (user_id) DO UPDATE SET
		   plan                     = EXCLUDED.plan,
		   status                   = EXCLUDED.status,
		   current_period_end       = EXCLUDED.current_period_end,
		   cancel_at_period_end     = EXCLUDED.cancel_at_period_end,
		   provider                 = EXCLUDED.provider,
		   provider_subscription_id = EXCLUDED.provider_subscription_id,
		   provider_customer_id     = EXCLUDED.provider_customer_id,
		   updated_at               = now()`,
		sub.UserID.String(), sub.Plan.String(), sub.Status.String(),
		sub.CurrentPeriodEnd, sub.CancelAtPeriodEnd, sub.Provider, providerSubID, providerCustID)
```

- [ ] **Step 5: 成功を確認**

Run: `cd backend && go test ./internal/repository/ -run Subscription -v`
Expected: PASS。

- [ ] **Step 6: コミット**

```bash
git add backend/db/migrations/000012_add_provider_customer_id.up.sql \
        backend/db/migrations/000012_add_provider_customer_id.down.sql \
        backend/internal/repository/subscription.go \
        backend/internal/repository/subscription_test.go
git commit -m "feat: subscriptions に provider_customer_id 列を追加し repository を拡張"
```

---

### Task 3: EntitlementService.For を GivesPremiumAt に切替（trialing / past_due 猶予）

**Files:**
- Modify: `backend/internal/service/entitlement.go`
- Modify: `backend/internal/domain/subscription.go`（`IsActiveAt` を削除）
- Modify: `backend/internal/repository/subscription.go`（コメントの `IsActiveAt` 参照を更新）
- Test: `backend/internal/service/entitlement_test.go`

**Interfaces:**
- Consumes: `Subscription.GivesPremiumAt`（Task 1）
- Produces: `For` が trialing / past_due 猶予内を premium と評価する

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/service/entitlement_test.go` に追加（既存の fake store の形に合わせる。無ければ `SubscriptionStore` を満たす小さな fake を定義）:

```go
func TestEntitlementService_For_TrialingAndPastDue(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	uid := "11111111-1111-1111-1111-111111111111" // 既存テストの有効な UUID 生成に合わせる

	cases := []struct {
		name string
		sub  domain.Subscription
		want domain.Plan
	}{
		{"trialing 期間内は premium", domain.Subscription{Plan: domain.PlanPremium, Status: domain.SubscriptionTrialing, CurrentPeriodEnd: now.Add(48 * time.Hour)}, domain.PlanPremium},
		{"past_due 猶予内は premium", domain.Subscription{Plan: domain.PlanPremium, Status: domain.SubscriptionPastDue, CurrentPeriodEnd: now.Add(-3 * 24 * time.Hour)}, domain.PlanPremium},
		{"past_due 猶予超過は free", domain.Subscription{Plan: domain.PlanPremium, Status: domain.SubscriptionPastDue, CurrentPeriodEnd: now.Add(-8 * 24 * time.Hour)}, domain.PlanFree},
		{"canceled は free", domain.Subscription{Plan: domain.PlanPremium, Status: domain.SubscriptionCanceled, CurrentPeriodEnd: now.Add(48 * time.Hour)}, domain.PlanFree},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeSubscriptionStore{sub: tc.sub} // 既存 fake 名に合わせる
			svc := service.NewEntitlementService(store, func() time.Time { return now })
			ent, err := svc.For(context.Background(), uid)
			if err != nil {
				t.Fatalf("For: %v", err)
			}
			if ent.Plan() != tc.want {
				t.Errorf("Plan() = %q, want %q", ent.Plan(), tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: 失敗を確認**

Run: `cd backend && go test ./internal/service/ -run For_TrialingAndPastDue -v`
Expected: FAIL（trialing / past_due が現状 `IsActiveAt` で free に落ちる）。

- [ ] **Step 3: 実装する**

`backend/internal/service/entitlement.go` の `For`（`:57-60`）を置換:
```go
	if !sub.GivesPremiumAt(s.now()) {
		return free, nil
	}
	return domain.NewEntitlement(sub.Plan), nil
```

`backend/internal/domain/subscription.go` から `IsActiveAt`（`:65-71`）を削除する（唯一の呼び出し元が上記 `For` だったため未使用になる）。

`backend/internal/repository/subscription.go` の Find のコメント（`:52`）の「未知の状態は IsActiveAt が false にする」を「未知の状態は GivesPremiumAt が false にする」へ更新する。

- [ ] **Step 4: 成功を確認**

Run: `cd backend && go test ./internal/service/ ./internal/domain/ -v`
Expected: PASS。`cd backend && go build ./...` も通ること（`IsActiveAt` の未使用参照が残っていないこと）。

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/entitlement.go backend/internal/service/entitlement_test.go \
        backend/internal/domain/subscription.go backend/internal/repository/subscription.go
git commit -m "feat: エンタイトルメントで trialing と past_due 猶予を premium と評価する"
```

---

### Task 4: PaymentGateway port + Stripe adapter（stripe-go 追加）

**Files:**
- Modify: `backend/go.mod` / `backend/go.sum`（`stripe-go` 追加）
- Modify: `backend/internal/service/ports.go`（port と DTO を追加）
- Create: `backend/internal/repository/payment_stripe.go`
- Test: `backend/internal/repository/payment_stripe_test.go`

**Interfaces:**
- Consumes: `domain.SubscriptionStatus`, `domain.Subscription*`（Task 1）
- Produces:
  - `service.PaymentGateway` interface
  - `service.CheckoutParams` / `service.WebhookEvent` 構造体
  - `repository.NewStripePaymentGateway(secretKey, webhookSecret, priceID string, trialDays int64) *StripePaymentGateway` が `PaymentGateway` を満たす

- [ ] **Step 1: stripe-go を追加**

Run: `cd backend && go get github.com/stripe/stripe-go/v82`
（最新の major を https://github.com/stripe/stripe-go/releases で確認し、現行の安定版 major を使う。以降の import パスの `/v82` はその major に揃える。どの major でも `current_period_end` は subscription item 側にある前提は変わらない。）

- [ ] **Step 2: port と DTO を追加**

`backend/internal/service/ports.go` に追記（`SubscriptionStore` の近く）:

```go
// CheckoutParams は Checkout セッション作成の入力。
type CheckoutParams struct {
	UserID     string // client_reference_id と subscription metadata に入れる
	CustomerID string // 再利用する既存 Stripe Customer。空なら Stripe が新規作成する
	WithTrial  bool   // 初回加入のみ true（トライアル付与）
	SuccessURL string
	CancelURL  string
}

// WebhookEvent は Stripe の Webhook を正規化したもの。
// Type が空のイベントは処理対象外（handler / service は無視する）。
type WebhookEvent struct {
	Type              string // "subscription" のとき加入状態として処理する。空は無視
	UserID            string
	SubscriptionID    string
	CustomerID        string
	Status            domain.SubscriptionStatus
	CurrentPeriodEnd  time.Time
	CancelAtPeriodEnd bool
}

// PaymentGateway は決済事業者との境界。実装は repository.StripePaymentGateway。
// service/handler は stripe-go に直接依存せず、この port 越しに使う。
type PaymentGateway interface {
	// CreateCheckoutSession は Checkout セッションを作り、リダイレクト先 URL を返す。
	CreateCheckoutSession(ctx context.Context, p CheckoutParams) (string, error)
	// ParseWebhookEvent は署名を検証し、Stripe イベントを WebhookEvent に正規化する。
	// 署名不正・本文の解釈失敗はエラー。処理対象外のイベントは Type="" で err=nil を返す。
	ParseWebhookEvent(payload []byte, sigHeader string) (WebhookEvent, error)
}
```
（`ports.go` に `time` / `context` / `domain` の import が無ければ追加する。）

- [ ] **Step 3: 失敗するテストを書く**

`backend/internal/repository/payment_stripe_test.go`:

```go
package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

const testWebhookSecret = "whsec_test_secret"

// signedEvent は指定のイベント種別と本文で署名済みペイロードを作る。
func signedEvent(t *testing.T, eventType string, obj any) ([]byte, string) {
	t.Helper()
	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	event := map[string]any{
		"id":          "evt_test",
		"object":      "event",
		"api_version": stripe.APIVersion,
		"type":        eventType,
		"data":        map[string]any{"object": json.RawMessage(raw)},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	signed := stripe.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: testWebhookSecret})
	return signed.Payload, signed.Header
}

func TestStripeGateway_ParseWebhookEvent_SubscriptionUpdated(t *testing.T) {
	gw := repository.NewStripePaymentGateway("sk_test", testWebhookSecret, "price_test", 5)
	periodEnd := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)

	sub := map[string]any{
		"id":                   "sub_123",
		"status":               "trialing",
		"cancel_at_period_end": false,
		"customer":             map[string]any{"id": "cus_123"},
		"metadata":             map[string]any{"user_id": "user-abc"},
		"items": map[string]any{
			"data": []map[string]any{{"current_period_end": periodEnd.Unix()}},
		},
	}
	payload, header := signedEvent(t, "customer.subscription.updated", sub)

	ev, err := gw.ParseWebhookEvent(payload, header)
	if err != nil {
		t.Fatalf("ParseWebhookEvent: %v", err)
	}
	if ev.Type != "subscription" {
		t.Errorf("Type = %q, want subscription", ev.Type)
	}
	if ev.UserID != "user-abc" || ev.SubscriptionID != "sub_123" || ev.CustomerID != "cus_123" {
		t.Errorf("紐付けが不正: %+v", ev)
	}
	if ev.Status != domain.SubscriptionTrialing {
		t.Errorf("Status = %q, want trialing", ev.Status)
	}
	if !ev.CurrentPeriodEnd.Equal(periodEnd) {
		t.Errorf("CurrentPeriodEnd = %v, want %v", ev.CurrentPeriodEnd, periodEnd)
	}
}

func TestStripeGateway_ParseWebhookEvent_SubscriptionDeleted(t *testing.T) {
	gw := repository.NewStripePaymentGateway("sk_test", testWebhookSecret, "price_test", 5)
	sub := map[string]any{
		"id": "sub_123", "status": "active",
		"metadata": map[string]any{"user_id": "user-abc"},
		"items":    map[string]any{"data": []map[string]any{{"current_period_end": int64(0)}}},
	}
	payload, header := signedEvent(t, "customer.subscription.deleted", sub)

	ev, err := gw.ParseWebhookEvent(payload, header)
	if err != nil {
		t.Fatalf("ParseWebhookEvent: %v", err)
	}
	if ev.Status != domain.SubscriptionCanceled {
		t.Errorf("deleted は canceled にする。got %q", ev.Status)
	}
}

func TestStripeGateway_ParseWebhookEvent_BadSignature(t *testing.T) {
	gw := repository.NewStripePaymentGateway("sk_test", testWebhookSecret, "price_test", 5)
	if _, err := gw.ParseWebhookEvent([]byte(`{}`), "t=1,v1=deadbeef"); err == nil {
		t.Error("署名不正はエラーにすべき")
	}
}

func TestStripeGateway_ParseWebhookEvent_IgnoredType(t *testing.T) {
	gw := repository.NewStripePaymentGateway("sk_test", testWebhookSecret, "price_test", 5)
	payload, header := signedEvent(t, "invoice.created", map[string]any{"id": "in_1"})
	ev, err := gw.ParseWebhookEvent(payload, header)
	if err != nil {
		t.Fatalf("ParseWebhookEvent: %v", err)
	}
	if ev.Type != "" {
		t.Errorf("対象外イベントは Type 空にする。got %q", ev.Type)
	}
}
```

- [ ] **Step 4: 失敗を確認**

Run: `cd backend && go test ./internal/repository/ -run StripeGateway -v`
Expected: コンパイルエラー（`NewStripePaymentGateway` 未定義）。

- [ ] **Step 5: adapter を実装する**

`backend/internal/repository/payment_stripe.go`:

```go
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/subscription"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// StripePaymentGateway は service.PaymentGateway の Stripe 実装。
// stripe-go への依存はこのファイルだけに閉じ込める。
type StripePaymentGateway struct {
	webhookSecret string
	priceID       string
	trialDays     int64
}

// NewStripePaymentGateway は Stripe の秘密鍵を設定し gateway を生成する。
func NewStripePaymentGateway(secretKey, webhookSecret, priceID string, trialDays int64) *StripePaymentGateway {
	stripe.Key = secretKey
	return &StripePaymentGateway{webhookSecret: webhookSecret, priceID: priceID, trialDays: trialDays}
}

func (g *StripePaymentGateway) CreateCheckoutSession(ctx context.Context, p service.CheckoutParams) (string, error) {
	params := &stripe.CheckoutSessionParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL:        stripe.String(p.SuccessURL),
		CancelURL:         stripe.String(p.CancelURL),
		ClientReferenceID: stripe.String(p.UserID),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(g.priceID), Quantity: stripe.Int64(1)},
		},
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{"user_id": p.UserID},
		},
	}
	if p.WithTrial {
		params.SubscriptionData.TrialPeriodDays = stripe.Int64(g.trialDays)
	}
	if p.CustomerID != "" {
		params.Customer = stripe.String(p.CustomerID)
	}
	params.Context = ctx

	s, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("Checkout セッションの作成に失敗しました: %w", err)
	}
	return s.URL, nil
}

func (g *StripePaymentGateway) ParseWebhookEvent(payload []byte, sigHeader string) (service.WebhookEvent, error) {
	event, err := webhook.ConstructEvent(payload, sigHeader, g.webhookSecret)
	if err != nil {
		return service.WebhookEvent{}, fmt.Errorf("Webhook 署名の検証に失敗しました: %w", err)
	}

	switch event.Type {
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return service.WebhookEvent{}, fmt.Errorf("subscription の解釈に失敗しました: %w", err)
		}
		return normalizeSubscription(&sub, event.Type == "customer.subscription.deleted"), nil

	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &cs); err != nil {
			return service.WebhookEvent{}, fmt.Errorf("checkout session の解釈に失敗しました: %w", err)
		}
		if cs.Subscription == nil || cs.Subscription.ID == "" {
			return service.WebhookEvent{}, nil // 加入を伴わない（想定外）→無視
		}
		// session は加入の詳細を含まないため取得する（subscription.* を取りこぼした場合の保険）。
		sub, err := subscription.Get(cs.Subscription.ID, nil)
		if err != nil {
			return service.WebhookEvent{}, fmt.Errorf("subscription の取得に失敗しました: %w", err)
		}
		ev := normalizeSubscription(sub, false)
		if ev.UserID == "" {
			ev.UserID = cs.ClientReferenceID
		}
		return ev, nil

	default:
		return service.WebhookEvent{}, nil // 対象外→無視
	}
}

// normalizeSubscription は Stripe の Subscription を WebhookEvent に写す。
func normalizeSubscription(sub *stripe.Subscription, deleted bool) service.WebhookEvent {
	ev := service.WebhookEvent{
		Type:              "subscription",
		UserID:            sub.Metadata["user_id"],
		SubscriptionID:    sub.ID,
		CancelAtPeriodEnd: sub.CancelAtPeriodEnd,
	}
	if sub.Customer != nil {
		ev.CustomerID = sub.Customer.ID
	}
	// current_period_end は加入 item 側にある（Stripe API の変更）。
	if sub.Items != nil && len(sub.Items.Data) > 0 {
		ev.CurrentPeriodEnd = time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0).UTC()
	}
	if deleted {
		ev.Status = domain.SubscriptionCanceled
	} else {
		ev.Status = mapStripeStatus(sub.Status)
	}
	return ev
}

// mapStripeStatus は Stripe の status をドメインの状態に写す。
// active / trialing / past_due 以外はすべてアクセスを与えない側（canceled）に倒す。
func mapStripeStatus(s stripe.SubscriptionStatus) domain.SubscriptionStatus {
	switch s {
	case stripe.SubscriptionStatusActive:
		return domain.SubscriptionActive
	case stripe.SubscriptionStatusTrialing:
		return domain.SubscriptionTrialing
	case stripe.SubscriptionStatusPastDue:
		return domain.SubscriptionPastDue
	default:
		return domain.SubscriptionCanceled
	}
}
```

（`session.New` / `subscription.Get` のパッケージ名・引数は使用する stripe-go の major に合わせて確認する。上記は package-level API。もし採用 major が client ベース API のみの場合は、その major のドキュメントに合わせて `stripe.NewClient` 経由に読み替える。）

- [ ] **Step 6: 成功を確認**

Run: `cd backend && go test ./internal/repository/ -run StripeGateway -v`
Expected: PASS。

- [ ] **Step 7: コミット**

```bash
git add backend/go.mod backend/go.sum backend/internal/service/ports.go \
        backend/internal/repository/payment_stripe.go backend/internal/repository/payment_stripe_test.go
git commit -m "feat: Stripe を隔離する PaymentGateway port と adapter を追加"
```

---

### Task 5: BillingService（Preview / CreateCheckoutSession / HandleWebhook）

**Files:**
- Create: `backend/internal/service/billing.go`
- Test: `backend/internal/service/billing_test.go`

**Interfaces:**
- Consumes: `SubscriptionStore`（Find/Upsert）, `PaymentGateway`, `CheckoutParams`, `WebhookEvent`（Task 4）, `domain.Entitlement`（For）
- Produces:
  - `service.ErrAlreadySubscribed`, `service.ErrWebhookSignature`
  - `type PreviewResult struct { Price int; Currency string; TrialDays int; TrialEligible bool; FirstBillingAt time.Time; PlanManagementPath string }`
  - `NewBillingService(entitlements Entitlements, store SubscriptionStore, gateway PaymentGateway, successURL, cancelURL string, trialDays int, now func() time.Time) *BillingService`
  - メソッド `Preview(ctx, userID string) (PreviewResult, error)` / `CreateCheckoutSession(ctx, userID string) (string, error)` / `HandleWebhook(ctx context.Context, payload []byte, sigHeader string) error`

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/service/billing_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// --- fakes ---

type fakeStore struct {
	sub      domain.Subscription
	found    bool
	upserted *domain.Subscription
}

func (f *fakeStore) Find(_ context.Context, _ domain.UserID) (domain.Subscription, error) {
	if !f.found {
		return domain.Subscription{}, service.ErrSubscriptionNotFound
	}
	return f.sub, nil
}
func (f *fakeStore) Upsert(_ context.Context, sub domain.Subscription) error {
	f.upserted = &sub
	return nil
}

type fakeEnt struct{ plan domain.Plan }

func (f fakeEnt) For(_ context.Context, _ string) (domain.Entitlement, error) {
	return domain.NewEntitlement(f.plan), nil
}

type fakeGateway struct {
	lastParams service.CheckoutParams
	url        string
	event      service.WebhookEvent
	parseErr   error
}

func (f *fakeGateway) CreateCheckoutSession(_ context.Context, p service.CheckoutParams) (string, error) {
	f.lastParams = p
	return f.url, nil
}
func (f *fakeGateway) ParseWebhookEvent(_ []byte, _ string) (service.WebhookEvent, error) {
	return f.event, f.parseErr
}

const validUID = "11111111-1111-1111-1111-111111111111"

func newBilling(store service.SubscriptionStore, ent service.Entitlements, gw service.PaymentGateway) *service.BillingService {
	now := func() time.Time { return time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC) }
	return service.NewBillingService(ent, store, gw, "https://app/checkout/complete", "https://app/checkout", 5, now)
}

// --- tests ---

func TestBilling_CreateCheckoutSession_AlreadyPremium(t *testing.T) {
	svc := newBilling(&fakeStore{}, fakeEnt{plan: domain.PlanPremium}, &fakeGateway{})
	_, err := svc.CreateCheckoutSession(context.Background(), validUID)
	if !errors.Is(err, service.ErrAlreadySubscribed) {
		t.Fatalf("premium は already-subscribed。got %v", err)
	}
}

func TestBilling_CreateCheckoutSession_FirstTimeGetsTrial(t *testing.T) {
	gw := &fakeGateway{url: "https://stripe/session"}
	svc := newBilling(&fakeStore{found: false}, fakeEnt{plan: domain.PlanFree}, gw)
	url, err := svc.CreateCheckoutSession(context.Background(), validUID)
	if err != nil {
		t.Fatalf("CreateCheckoutSession: %v", err)
	}
	if url != "https://stripe/session" {
		t.Errorf("url = %q", url)
	}
	if !gw.lastParams.WithTrial {
		t.Error("初回はトライアル付与すべき")
	}
	if gw.lastParams.CustomerID != "" {
		t.Error("初回は Customer 再利用しない")
	}
}

func TestBilling_CreateCheckoutSession_ReturningReusesCustomerNoTrial(t *testing.T) {
	store := &fakeStore{found: true, sub: domain.Subscription{
		Status: domain.SubscriptionCanceled, ProviderCustomerID: "cus_old",
	}}
	gw := &fakeGateway{url: "https://stripe/session"}
	svc := newBilling(store, fakeEnt{plan: domain.PlanFree}, gw)
	if _, err := svc.CreateCheckoutSession(context.Background(), validUID); err != nil {
		t.Fatalf("CreateCheckoutSession: %v", err)
	}
	if gw.lastParams.WithTrial {
		t.Error("再申込はトライアル無し")
	}
	if gw.lastParams.CustomerID != "cus_old" {
		t.Errorf("Customer を再利用すべき。got %q", gw.lastParams.CustomerID)
	}
}

func TestBilling_Preview_FirstTime(t *testing.T) {
	svc := newBilling(&fakeStore{found: false}, fakeEnt{plan: domain.PlanFree}, &fakeGateway{})
	got, err := svc.Preview(context.Background(), validUID)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !got.TrialEligible || got.Price != 300 || got.TrialDays != 5 {
		t.Errorf("preview 不正: %+v", got)
	}
	want := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC) // now + 5日
	if !got.FirstBillingAt.Equal(want) {
		t.Errorf("FirstBillingAt = %v, want %v", got.FirstBillingAt, want)
	}
}

func TestBilling_HandleWebhook_UpsertsSubscription(t *testing.T) {
	store := &fakeStore{}
	gw := &fakeGateway{event: service.WebhookEvent{
		Type: "subscription", UserID: validUID, SubscriptionID: "sub_1",
		CustomerID: "cus_1", Status: domain.SubscriptionActive,
		CurrentPeriodEnd: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
	}}
	svc := newBilling(store, fakeEnt{plan: domain.PlanFree}, gw)
	if err := svc.HandleWebhook(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	if store.upserted == nil {
		t.Fatal("upsert されていない")
	}
	if store.upserted.Provider != domain.ProviderStripe ||
		store.upserted.Plan != domain.PlanPremium ||
		store.upserted.ProviderSubscriptionID != "sub_1" ||
		store.upserted.ProviderCustomerID != "cus_1" {
		t.Errorf("upsert 内容が不正: %+v", *store.upserted)
	}
}

func TestBilling_HandleWebhook_SignatureError(t *testing.T) {
	gw := &fakeGateway{parseErr: errors.New("bad sig")}
	svc := newBilling(&fakeStore{}, fakeEnt{plan: domain.PlanFree}, gw)
	err := svc.HandleWebhook(context.Background(), []byte("{}"), "sig")
	if !errors.Is(err, service.ErrWebhookSignature) {
		t.Fatalf("署名エラーは ErrWebhookSignature。got %v", err)
	}
}

func TestBilling_HandleWebhook_IgnoredEvent(t *testing.T) {
	store := &fakeStore{}
	gw := &fakeGateway{event: service.WebhookEvent{Type: ""}}
	svc := newBilling(store, fakeEnt{plan: domain.PlanFree}, gw)
	if err := svc.HandleWebhook(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	if store.upserted != nil {
		t.Error("対象外イベントは upsert しない")
	}
}
```

- [ ] **Step 2: 失敗を確認**

Run: `cd backend && go test ./internal/service/ -run Billing -v`
Expected: コンパイルエラー（`BillingService` 未定義）。

- [ ] **Step 3: 実装する**

`backend/internal/service/billing.go`:

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/logctx"
)

// ErrAlreadySubscribed は既にプレミアムの利用者が加入を試みたことを表す。
var ErrAlreadySubscribed = errors.New("既にプレミアムに加入しています")

// ErrWebhookSignature は Webhook の署名検証・本文解釈に失敗したことを表す。
// handler はこれを 400 に写し、Stripe に再送させない。
var ErrWebhookSignature = errors.New("Webフックの検証に失敗しました")

// planManagementPath は解約導線の文言。特商法・利用規約が名指しする画面名。
const planManagementPath = "アカウント設定 > プランの管理"

// Entitlements は BillingService が必要とするエンタイトルメント参照。
// 実装は *EntitlementService。
type Entitlements interface {
	For(ctx context.Context, userID string) (domain.Entitlement, error)
}

// PreviewResult は申込確認画面の表示値。
type PreviewResult struct {
	Price              int
	Currency           string
	TrialDays          int
	TrialEligible      bool
	FirstBillingAt     time.Time
	PlanManagementPath string
}

// BillingService は加入の申込導線を担う。加入状態の権威は Webhook（HandleWebhook）。
type BillingService struct {
	entitlements Entitlements
	store        SubscriptionStore
	gateway      PaymentGateway
	successURL   string
	cancelURL    string
	trialDays    int
	now          func() time.Time
}

// NewBillingService は BillingService を生成する。
func NewBillingService(
	entitlements Entitlements, store SubscriptionStore, gateway PaymentGateway,
	successURL, cancelURL string, trialDays int, now func() time.Time,
) *BillingService {
	if now == nil {
		now = time.Now
	}
	return &BillingService{
		entitlements: entitlements, store: store, gateway: gateway,
		successURL: successURL, cancelURL: cancelURL, trialDays: trialDays, now: now,
	}
}

// Preview は申込確認画面の表示値を返す。
func (s *BillingService) Preview(ctx context.Context, userID string) (PreviewResult, error) {
	eligible, _, err := s.trialEligibility(ctx, userID)
	if err != nil {
		return PreviewResult{}, err
	}
	first := s.now()
	if eligible {
		first = first.Add(time.Duration(s.trialDays) * 24 * time.Hour)
	}
	return PreviewResult{
		Price: 300, Currency: "jpy", TrialDays: s.trialDays,
		TrialEligible: eligible, FirstBillingAt: first,
		PlanManagementPath: planManagementPath,
	}, nil
}

// CreateCheckoutSession は Checkout セッションを作り URL を返す。
func (s *BillingService) CreateCheckoutSession(ctx context.Context, userID string) (string, error) {
	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return "", err
	}
	if ent.Plan() == domain.PlanPremium {
		return "", ErrAlreadySubscribed
	}
	eligible, customerID, err := s.trialEligibility(ctx, userID)
	if err != nil {
		return "", err
	}
	return s.gateway.CreateCheckoutSession(ctx, CheckoutParams{
		UserID:     userID,
		CustomerID: customerID,
		WithTrial:  eligible,
		SuccessURL: s.successURL,
		CancelURL:  s.cancelURL,
	})
}

// trialEligibility は「トライアル適格か」と「再利用する Customer ID」を返す。
// 加入行が一度も無ければ適格（初回）。行があれば非適格で、その顧客IDを再利用する。
func (s *BillingService) trialEligibility(ctx context.Context, userID string) (bool, string, error) {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return false, "", err
	}
	sub, err := s.store.Find(ctx, uid)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return true, "", nil
		}
		return false, "", err
	}
	return false, sub.ProviderCustomerID, nil
}

// HandleWebhook は Webhook を検証し、加入状態を subscriptions に同期する。
func (s *BillingService) HandleWebhook(ctx context.Context, payload []byte, sigHeader string) error {
	ev, err := s.gateway.ParseWebhookEvent(payload, sigHeader)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookSignature, err)
	}
	if ev.Type == "" {
		return nil // 対象外イベント
	}
	if ev.UserID == "" {
		// 紐付けられない＝こちらの不具合。再送しても直らないのでログして無視する。
		logctx.From(ctx).WarnContext(ctx, "Webフックに user_id が無いため無視します",
			slog.String("subscription_id", ev.SubscriptionID))
		return nil
	}
	uid, err := domain.ParseUserID(ev.UserID)
	if err != nil {
		logctx.From(ctx).WarnContext(ctx, "Webフックの user_id が不正なため無視します",
			slog.String("user_id", ev.UserID))
		return nil
	}
	return s.store.Upsert(ctx, domain.Subscription{
		UserID:                 uid,
		Plan:                   domain.PlanPremium,
		Status:                 ev.Status,
		CurrentPeriodEnd:       ev.CurrentPeriodEnd,
		CancelAtPeriodEnd:      ev.CancelAtPeriodEnd,
		Provider:               domain.ProviderStripe,
		ProviderSubscriptionID: ev.SubscriptionID,
		ProviderCustomerID:     ev.CustomerID,
	})
}
```

（`logctx` の import パスは既存の repository/subscription.go に倣う: `github.com/yuuyakim/menu-planner/backend/internal/logctx`。）

- [ ] **Step 4: 成功を確認**

Run: `cd backend && go test ./internal/service/ -run Billing -v`
Expected: PASS。

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/billing.go backend/internal/service/billing_test.go
git commit -m "feat: 申込導線と Webhook 同期を担う BillingService を追加"
```

---

### Task 6: BillingHandler + main.go 配線 + STRIPE_* 設定 + problem マッピング

**Files:**
- Create: `backend/internal/handler/billing.go`
- Test: `backend/internal/handler/billing_test.go`
- Modify: `backend/internal/handler/problem.go`（`already-subscribed` を追加）
- Modify: `backend/internal/handler/problem_coverage_test.go`（`ErrWebhookSignature` を intentionallyUnmapped に）
- Modify: `backend/cmd/server/main.go`（設定読み込みと DI 配線）
- Modify: `.env.example` / `docker-compose.yml` / `DEPLOY.md`（環境変数）

**Interfaces:**
- Consumes: `BillingService`（Preview/CreateCheckoutSession/HandleWebhook）, `ErrAlreadySubscribed`, `ErrWebhookSignature`（Task 5）
- Produces: `NewBillingHandler(svc BillingUseCase, tokens *auth.JWT) *BillingHandler` + `RegisterRoutes(e *echo.Echo)`

- [ ] **Step 1: problem マッピングを追加**

`backend/internal/handler/problem.go` の `problemMapping` に、`ErrPremiumRequired` の近く（409 群）へ追加:
```go
	// 既にプレミアムの利用者が加入を試みた。今の状態との競合なので 409。
	{service.ErrAlreadySubscribed, http.StatusConflict, "already-subscribed", "既にプレミアムに加入しています"},
```

`backend/internal/handler/problem_coverage_test.go` の `intentionallyUnmapped`（無ければ既存の仕組みに合わせる）に追加:
```go
	// ErrWebhookSignature は Webhook でのみ使い、client へ problem+json として
	// 返さない（Stripe には素の 400 を返す）。よって写像しない。
	service.ErrWebhookSignature,
```

- [ ] **Step 2: 失敗するハンドラテストを書く**

`backend/internal/handler/billing_test.go`（既存ハンドラテストの echo セットアップ・認証Cookie発行ヘルパに合わせる）:

```go
// fakeBilling は BillingUseCase を満たす。
type fakeBilling struct {
	previewErr    error
	url           string
	createErr     error
	handleErr     error
	handleCalled  bool
}

func (f *fakeBilling) Preview(context.Context, string) (service.PreviewResult, error) {
	return service.PreviewResult{Price: 300, Currency: "jpy", TrialDays: 5, TrialEligible: true, PlanManagementPath: "アカウント設定 > プランの管理"}, f.previewErr
}
func (f *fakeBilling) CreateCheckoutSession(context.Context, string) (string, error) {
	return f.url, f.createErr
}
func (f *fakeBilling) HandleWebhook(context.Context, []byte, string) error {
	f.handleCalled = true
	return f.handleErr
}

func TestBillingHandler_CheckoutSession_RequiresAuth(t *testing.T) {
	// 認証Cookieなしで POST /api/v1/billing/checkout-session → 401
	// （既存の handler テストと同じ方法でリクエストを作る）
}

func TestBillingHandler_CheckoutSession_AlreadySubscribed(t *testing.T) {
	// createErr = service.ErrAlreadySubscribed のとき 409 を返す
}

func TestBillingHandler_CheckoutSession_ReturnsURL(t *testing.T) {
	// 認証あり・url="https://stripe/x" → 200 で {"url":"https://stripe/x"}
}

func TestBillingHandler_Webhook_BadSignature(t *testing.T) {
	// handleErr = fmt.Errorf("%w: x", service.ErrWebhookSignature) → 400
}

func TestBillingHandler_Webhook_OK(t *testing.T) {
	// handleErr = nil → 200、handleCalled = true
}

func TestBillingHandler_Webhook_ProcessingError(t *testing.T) {
	// handleErr = errors.New("db down")（署名エラーでない）→ 500
}
```
（各テスト本体は、同ディレクトリの既存ハンドラテスト（例: `saved_shopping_list` のテスト）の書き方に合わせて `httptest` + echo で実装する。認証必須の検証は既存の RequireAuth テストの Cookie 発行方法を流用する。webhook は認証不要なので Cookie 無しで叩く。）

- [ ] **Step 3: 失敗を確認**

Run: `cd backend && go test ./internal/handler/ -run Billing -v`
Expected: コンパイルエラー（`NewBillingHandler` 未定義）。

- [ ] **Step 4: ハンドラを実装する**

`backend/internal/handler/billing.go`:

```go
package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// BillingUseCase は課金APIが必要とする操作。実装は service.BillingService。
type BillingUseCase interface {
	Preview(ctx context.Context, userID string) (service.PreviewResult, error)
	CreateCheckoutSession(ctx context.Context, userID string) (string, error)
	HandleWebhook(ctx context.Context, payload []byte, sigHeader string) error
}

// BillingHandler は課金APIの受け口。
type BillingHandler struct {
	svc    BillingUseCase
	tokens *auth.JWT
}

// NewBillingHandler は BillingHandler を生成する。
func NewBillingHandler(svc BillingUseCase, tokens *auth.JWT) *BillingHandler {
	return &BillingHandler{svc: svc, tokens: tokens}
}

// RegisterRoutes はルーティングを登録する。
// preview / checkout-session は本人の加入を作るため認証必須。
// webhook は Stripe が直接叩くため認証を付けず、署名検証で守る。
func (h *BillingHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	requireAuth := RequireAuth(h.tokens)
	g.GET("/billing/preview", h.Preview, requireAuth)
	g.POST("/billing/checkout-session", h.CreateCheckoutSession, requireAuth)
	g.POST("/billing/webhook", h.Webhook)
}

type previewDTO struct {
	Price              int    `json:"price"`
	Currency           string `json:"currency"`
	TrialDays          int    `json:"trialDays"`
	TrialEligible      bool   `json:"trialEligible"`
	FirstBillingAt     string `json:"firstBillingAt"`
	PlanManagementPath string `json:"planManagementPath"`
}

func (h *BillingHandler) Preview(c echo.Context) error {
	userID, _ := UserIDFromContext(c)
	p, err := h.svc.Preview(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, previewDTO{
		Price: p.Price, Currency: p.Currency, TrialDays: p.TrialDays,
		TrialEligible: p.TrialEligible,
		FirstBillingAt: p.FirstBillingAt.Format(time.RFC3339),
		PlanManagementPath: p.PlanManagementPath,
	})
}

func (h *BillingHandler) CreateCheckoutSession(c echo.Context) error {
	userID, _ := UserIDFromContext(c)
	url, err := h.svc.CreateCheckoutSession(c.Request().Context(), userID)
	if err != nil {
		return err // ErrAlreadySubscribed は problem マッピングで 409
	}
	return c.JSON(http.StatusOK, map[string]string{"url": url})
}

func (h *BillingHandler) Webhook(c echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.NoContent(http.StatusBadRequest)
	}
	sig := c.Request().Header.Get("Stripe-Signature")
	if err := h.svc.HandleWebhook(c.Request().Context(), body, sig); err != nil {
		if errors.Is(err, service.ErrWebhookSignature) {
			return c.NoContent(http.StatusBadRequest)
		}
		return c.NoContent(http.StatusInternalServerError)
	}
	return c.NoContent(http.StatusOK)
}
```
（`time` の import を追加すること。`UserIDFromContext` / `APIBasePath` / `RequireAuth` は既存 handler の定義を使う。）

- [ ] **Step 5: main.go に設定と配線を追加**

`backend/cmd/server/main.go`:

- `JWT_SECRET` の検証（`:75-78`）の後に Stripe 設定を追加（欠落時 fatal）:
```go
	stripeSecret := os.Getenv("STRIPE_SECRET_KEY")
	stripeWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	stripePriceID := os.Getenv("STRIPE_PRICE_ID")
	if stripeSecret == "" || stripeWebhookSecret == "" || stripePriceID == "" {
		return errors.New("STRIPE_SECRET_KEY / STRIPE_WEBHOOK_SECRET / STRIPE_PRICE_ID が設定されていません")
	}
```
- `savedShoppingListHandler` の配線（`:141-144`）の後に billing の配線を追加:
```go
	const trialDays = 5
	paymentGateway := repository.NewStripePaymentGateway(
		stripeSecret, stripeWebhookSecret, stripePriceID, int64(trialDays))
	billingSvc := service.NewBillingService(
		entitlementSvc, subscriptionRepo, paymentGateway,
		frontendOrigin+"/checkout/complete?session_id={CHECKOUT_SESSION_ID}",
		frontendOrigin+"/checkout",
		trialDays, time.Now)
	billingHandler := handler.NewBillingHandler(billingSvc, tokens)
```
- ルート登録（`:184` の後）に追加:
```go
	billingHandler.RegisterRoutes(e)
```

- [ ] **Step 6: 環境変数を宣言に追加**

- `.env.example`（ルート）に追記:
```
# Stripe（テストモードのキー。https://dashboard.stripe.com/test/apikeys）
STRIPE_SECRET_KEY=sk_test_xxx
STRIPE_WEBHOOK_SECRET=whsec_xxx
STRIPE_PRICE_ID=price_xxx
```
- `docker-compose.yml` の backend サービス `environment:` に3つを追加（`${STRIPE_SECRET_KEY}` 等で `.env` から流し込む）。
- `DEPLOY.md` の環境変数マトリクスに3行追加。本番は **GCP Secret Manager → Cloud Run** に入れる旨、Webhook は Cloud Run 公開URL `/api/v1/billing/webhook` に着弾し署名で守る旨を明記。

- [ ] **Step 7: 成功を確認**

Run: `cd backend && go test ./internal/handler/ -run 'Billing|Coverage' -v && go build ./...`
Expected: PASS かつビルド成功。

- [ ] **Step 8: コミット**

```bash
git add backend/internal/handler/billing.go backend/internal/handler/billing_test.go \
        backend/internal/handler/problem.go backend/internal/handler/problem_coverage_test.go \
        backend/cmd/server/main.go .env.example docker-compose.yml DEPLOY.md
git commit -m "feat: 課金APIのハンドラと配線・Stripe設定・エラー写像を追加"
```

---

### Task 7: フロント — billing api + 申込確認画面 `/checkout`

**Files:**
- Create: `frontend/src/features/billing/api.ts`
- Create: `frontend/src/features/billing/CheckoutPage.tsx`
- Test: `frontend/src/features/billing/CheckoutPage.test.tsx`

**Interfaces:**
- Consumes: `apiGet`/`apiPost`（`../../api/client`）, `useCurrentUser`, `useQuery`/`useMutation`
- Produces: `getBillingPreview()`, `createCheckoutSession()`, `<CheckoutPage />`

- [ ] **Step 1: billing api を書く**

`frontend/src/features/billing/api.ts`:
```ts
import { apiGet, apiPost } from '../../api/client'

export interface BillingPreview {
  price: number
  currency: string
  trialDays: number
  trialEligible: boolean
  firstBillingAt: string
  planManagementPath: string
}

/** getBillingPreview は申込確認画面の表示値を取得する。 */
export function getBillingPreview(): Promise<BillingPreview> {
  return apiGet<BillingPreview>('/billing/preview')
}

/** createCheckoutSession は Checkout セッションを作り、遷移先 URL を返す。 */
export function createCheckoutSession(): Promise<{ url: string }> {
  return apiPost<{ url: string }>('/billing/checkout-session')
}
```

- [ ] **Step 2: 失敗するテストを書く**

`frontend/src/features/billing/CheckoutPage.test.tsx`（既存のページテスト＝MSW + `renderWithProviders` 相当のヘルパに合わせる）:

```tsx
// 主要なふるまい:
// 1. preview の6項目が表示される（¥300税込 / 5日トライアル / 初回課金日時 /
//    「アカウント設定 > プランの管理」/ 返金 / 規約・プライバシーのリンク）
// 2. 同意チェックが外れている間はボタンが disabled
// 3. 同意 → ボタン押下で createCheckoutSession が呼ばれ、返った url へ遷移する

it('同意するまでボタンは押せない', async () => {
  // GET /billing/preview を MSW でモック → レンダリング
  // ボタン「無料お試しを開始する」が disabled であること
  // チェックボックスを check → enabled になること
})

it('同意して押すと Checkout の url へ遷移する', async () => {
  // POST /billing/checkout-session を MSW で {url:'https://stripe/x'} に
  // window.location.href への代入を spy（既存テストの手法に合わせる）
  // → 'https://stripe/x' へ遷移することを検証
})
```

- [ ] **Step 3: 失敗を確認**

Run: `cd frontend && npx vitest run src/features/billing/CheckoutPage.test.tsx`
Expected: FAIL（`CheckoutPage` 未実装）。

- [ ] **Step 4: 画面を実装する**

`frontend/src/features/billing/CheckoutPage.tsx`（既存ページのローディング/エラー表示・見出しスタイル（`kon-*`）に合わせる）:

```tsx
import { useState } from 'react'
import { Link } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'

import { ErrorMessage } from '../../components/ErrorMessage' // 既存の表示部品に合わせる
import { getBillingPreview, createCheckoutSession } from './api'

function formatJst(iso: string): string {
  // 既存の日時表示に合わせる。無ければ Intl で JST 表示にする。
  return new Intl.DateTimeFormat('ja-JP', {
    year: 'numeric', month: 'long', day: 'numeric',
    hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Tokyo',
  }).format(new Date(iso))
}

export function CheckoutPage() {
  const [agreed, setAgreed] = useState(false)
  const preview = useQuery({ queryKey: ['billing', 'preview'], queryFn: getBillingPreview, retry: false })
  const start = useMutation({
    mutationFn: createCheckoutSession,
    onSuccess: ({ url }) => {
      window.location.href = url
    },
  })

  if (preview.isPending) return <p>読み込み中…</p>
  if (preview.error) return <ErrorMessage error={preview.error} />

  const p = preview.data
  return (
    <section className="mx-auto max-w-md">
      <h1 className="text-xl font-bold text-kon-ink">お申込み内容の確認</h1>
      <dl className="mt-4 space-y-2 text-kon-ink">
        <div><dt className="font-medium">プラン</dt><dd>プレミアム（月額 {p.price}円・税込）</dd></div>
        {p.trialEligible && (
          <div>
            <dt className="font-medium">無料お試し</dt>
            <dd>{p.trialDays}日間無料。初回課金は <strong>{formatJst(p.firstBillingAt)}</strong>。
              この日時より前に解約すれば課金されません。</dd>
          </div>
        )}
        {!p.trialEligible && (
          <div><dt className="font-medium">初回課金</dt><dd>お申込み時（{formatJst(p.firstBillingAt)}）</dd></div>
        )}
        <div><dt className="font-medium">解約方法</dt><dd>「{p.planManagementPath}」からいつでも解約できます（期末まで利用可能）。</dd></div>
        <div><dt className="font-medium">返金</dt><dd>原則として返金はできません（当方の責による場合等を除く）。</dd></div>
        <div><dt className="font-medium">お支払い</dt><dd>クレジットカード（決済代行：Stripe）。次の画面でカード情報を入力します。</dd></div>
      </dl>

      <label className="mt-4 flex items-start gap-2 text-sm text-kon-ink">
        <input type="checkbox" checked={agreed} onChange={(e) => setAgreed(e.target.checked)} className="mt-1" />
        <span>
          <Link to="/legal/terms" className="underline">利用規約</Link> と
          <Link to="/legal/privacy" className="underline"> プライバシーポリシー</Link> に同意します
        </span>
      </label>

      {start.error && <div className="mt-3"><ErrorMessage error={start.error} /></div>}

      <button
        type="button"
        disabled={!agreed || start.isPending}
        onClick={() => start.mutate()}
        className="mt-4 w-full rounded-full bg-kon-leaf px-4 py-2 font-medium text-white disabled:opacity-50"
      >
        無料お試しを開始する
      </button>
    </section>
  )
}
```
（`ErrorMessage` の import パス・props は既存の使い方に合わせる。ボタン色 `bg-kon-leaf` 等も既存の主要ボタンに揃える。返済者向けにボタン文言を出し分けたい場合でも、まずは共通で可。）

- [ ] **Step 5: 成功を確認**

Run: `cd frontend && npx vitest run src/features/billing/CheckoutPage.test.tsx`
Expected: PASS。

- [ ] **Step 6: コミット**

```bash
git add frontend/src/features/billing/api.ts \
        frontend/src/features/billing/CheckoutPage.tsx \
        frontend/src/features/billing/CheckoutPage.test.tsx
git commit -m "feat: 申込み確認画面 /checkout を追加"
```

---

### Task 8: フロント — 復帰画面 `/checkout/complete` + ルート登録

**Files:**
- Create: `frontend/src/features/billing/CheckoutCompletePage.tsx`
- Test: `frontend/src/features/billing/CheckoutCompletePage.test.tsx`
- Modify: `frontend/src/app/App.tsx`（`/checkout` と `/checkout/complete` を RequireAuth 付きで登録）

**Interfaces:**
- Consumes: `meQueryKey`（`../auth/useCurrentUser`）, `useQuery`, `fetchMe`（`../auth/api`）
- Produces: `<CheckoutCompletePage />`

- [ ] **Step 1: 失敗するテストを書く**

`frontend/src/features/billing/CheckoutCompletePage.test.tsx`:
```tsx
// 1. 初期は「お手続きを受け付けました」を表示
// 2. /auth/me が plan:'premium' を返したら「プレミアムが有効になりました」＋週間への導線
// 3. 一定回数たっても premium にならなければ「反映まで時間がかかることがあります」を表示
it('premium 反映後に成功表示になる', async () => {
  // GET /auth/me を最初 free → 次に premium を返すよう MSW を設定
  // 「プレミアムが有効になりました」が出ることを待つ
})
```

- [ ] **Step 2: 失敗を確認**

Run: `cd frontend && npx vitest run src/features/billing/CheckoutCompletePage.test.tsx`
Expected: FAIL。

- [ ] **Step 3: 実装する**

`frontend/src/features/billing/CheckoutCompletePage.tsx`:
```tsx
import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'

import { fetchMe } from '../auth/api'
import { meQueryKey } from '../auth/useCurrentUser'

const maxPolls = 10

export function CheckoutCompletePage() {
  const me = useQuery({
    queryKey: meQueryKey,
    queryFn: fetchMe,
    retry: false,
    // premium になるまで数秒おきに取り直す。なったら止める。
    refetchInterval: (query) =>
      query.state.data?.plan === 'premium' || (query.state.dataUpdateCount ?? 0) >= maxPolls
        ? false
        : 2000,
  })

  const active = me.data?.plan === 'premium'
  const exhausted = (me.dataUpdatedAt && (me.errorUpdateCount + me.dataUpdateCount) >= maxPolls) ?? false

  if (active) {
    return (
      <section className="mx-auto max-w-md text-center">
        <h1 className="text-xl font-bold text-kon-ink">プレミアムが有効になりました</h1>
        <p className="mt-2 text-kon-ink">ありがとうございます。1週間の献立をまとめて計画できます。</p>
        <Link to="/weekly" className="mt-4 inline-block rounded-full bg-kon-leaf px-4 py-2 font-medium text-white">
          1週間の献立へ
        </Link>
      </section>
    )
  }

  return (
    <section className="mx-auto max-w-md text-center">
      <h1 className="text-xl font-bold text-kon-ink">お手続きを受け付けました</h1>
      <p className="mt-2 text-kon-ink">
        {exhausted
          ? '反映まで少し時間がかかることがあります。しばらくして再読み込みしてください。'
          : 'プレミアムの有効化を確認しています…'}
      </p>
    </section>
  )
}
```
（`refetchInterval` / 打ち切り条件の具体は TanStack Query の実バージョンの API に合わせて調整する。要点は「premium になるか上限回数で停止」。）

- [ ] **Step 4: App.tsx にルートを登録**

`frontend/src/app/App.tsx`:
- import 追加:
```tsx
import { CheckoutPage } from '../features/billing/CheckoutPage'
import { CheckoutCompletePage } from '../features/billing/CheckoutCompletePage'
```
- `/favorites` の Route の近くに、認証必須で追加（加入は本人に紐づくため）:
```tsx
            <Route
              path="/checkout"
              element={
                <RequireAuth>
                  <CheckoutPage />
                </RequireAuth>
              }
            />
            <Route
              path="/checkout/complete"
              element={
                <RequireAuth>
                  <CheckoutCompletePage />
                </RequireAuth>
              }
            />
```

- [ ] **Step 5: 成功を確認**

Run: `cd frontend && npx vitest run src/features/billing/`
Expected: PASS。

- [ ] **Step 6: コミット**

```bash
git add frontend/src/features/billing/CheckoutCompletePage.tsx \
        frontend/src/features/billing/CheckoutCompletePage.test.tsx \
        frontend/src/app/App.tsx
git commit -m "feat: 決済復帰画面 /checkout/complete とルートを追加"
```

---

### Task 9: フロント — 既存アップグレード CTA を `/checkout` に接続

**Files:**
- Modify: plan-split で追加された既存のアップグレード CTA（保存上限 409 時・買い物リスト初回チェック時の導線）。場所は grep で特定する。
- Test: 対応する既存テスト（CTA を持つコンポーネントのテスト）に、`/checkout` への導線があることを追加。

**Interfaces:**
- Consumes: `react-router` の `Link`
- Produces: 既存 CTA が `/checkout` へ遷移する

- [ ] **Step 1: CTA の場所を特定する**

Run: `cd frontend && grep -rn "プレミアム\|アップグレード" src/features/menu`
plan-split で入れた「保存上限に達したときの案内」「買い物リスト初回チェック時の案内」の CTA を見つける。

- [ ] **Step 2: 失敗するテストを書く**

該当コンポーネントの既存テストに、CTA が `/checkout` への `<Link>`（または遷移）を持つことを検証するケースを追加する（例: `getByRole('link', { name: /プレミアム|アップグレード/ })` の `href` が `/checkout`）。

- [ ] **Step 3: 失敗を確認**

Run: `cd frontend && npx vitest run <該当テスト>`
Expected: FAIL。

- [ ] **Step 4: CTA を `/checkout` へ接続する**

該当 CTA を `react-router` の `<Link to="/checkout">…</Link>`（またはボタン→`navigate('/checkout')`）にする。文言は既存を踏襲（「プレミアムにアップグレード」等）。テキストだけだった箇所にリンクを与える。

- [ ] **Step 5: 成功を確認**

Run: `cd frontend && npx vitest run <該当テスト>`
Expected: PASS。

- [ ] **Step 6: コミット**

```bash
git add frontend/src/features/menu/
git commit -m "feat: 既存のアップグレード導線を /checkout に接続"
```

---

### Task 10: openapi.yaml / schema.d.ts / spec.md の更新

**Files:**
- Modify: `backend/api/openapi.yaml`（または既存の OpenAPI 定義の場所）
- Modify: `frontend/src/api/schema.d.ts`（生成物。再生成する）
- Modify: `spec.md`（プレミアム表・API 一覧・課金の説明）

**Interfaces:**
- Consumes: 全タスクの API
- Produces: 契約と仕様書が実装と一致する

- [ ] **Step 1: openapi.yaml を更新**

`GET /billing/preview`（200: preview オブジェクト、401）、`POST /billing/checkout-session`（200: `{url}`、401、409 `already-subscribed`）、`POST /billing/webhook`（200/400、認証なし）を追加する。既存の security / problem の書式に合わせる。

- [ ] **Step 2: schema.d.ts を再生成**

Run: `cd frontend && npm run generate:api`（プロジェクトの生成コマンドに合わせる。`package.json` の scripts を確認）
生成された差分をコミット対象にする。

- [ ] **Step 3: spec.md を更新**

- プレミアム機能表・API 一覧に課金導線（preview / checkout-session / webhook）を追記。
- 「加入は Stripe Checkout（ホスト型）で行い、状態の真実は Webhook」「trialing / past_due 猶予を premium と評価」「解約画面は別途（アカウント設定 > プランの管理）」を記述。
- **手動テストの位置づけ**を明記: 実 Stripe を使う E2E は CI に含めず、`docs/legal/checkout-display.md §6` の手動チェックリスト（テストカード＋`stripe listen/trigger`）で確認する、と1段落残す（隠さない）。

- [ ] **Step 4: 確認**

Run: `cd frontend && npx tsc --noEmit`（schema 変更で型が壊れていないこと。既存の型チェックコマンドに合わせる）
Expected: エラーなし。

- [ ] **Step 5: コミット**

```bash
git add backend/api/openapi.yaml frontend/src/api/schema.d.ts spec.md
git commit -m "docs: 課金APIを openapi / schema / spec に反映"
```

---

## Self-Review（計画作成後のチェック結果）

- **spec カバレッジ**: spec §4（データモデル）→T1/T2、§4.3（判定）→T1/T3、§5.1 preview→T5/T6/T7、§5.2 checkout-session→T5/T6、§5.3 webhook→T4/T5/T6、§5.4 port→T4、§6 service→T5、§7 フロント→T7/T8/T9、§8 設定→T4/T6、§9 テスト→各タスクの test、§10 ロールアウト→T2(migration)/T6(env)/T10(手動テスト明記)。解約画面（§2 対象外）は計画に含めない（正しい）。
- **プレースホルダ**: 実コードを各ステップに記載。フロントのヘルパ名（`renderWithProviders` 等）と一部の既存部品（`ErrorMessage`）は「既存に合わせる」と明示。これは環境依存の事実であり、実装者がファイルを読んで確定する。
- **型整合**: `WebhookEvent`/`CheckoutParams`（T4）を T5 が消費、`PreviewResult`/`ErrAlreadySubscribed`/`ErrWebhookSignature`（T5）を T6 が消費、`BillingPreview`/`createCheckoutSession`（T7）を T8/T9 が利用。`current_period_end` は item 側（T4 で吸収）→ ドメインは `time.Time` 1本（T1）で一貫。
- **注意（実装時の判断が要る箇所）**: stripe-go の major と API 形（package-level vs client）；フロントのテストヘルパ名と `window.location` の spy 方法；`intentionallyUnmapped` の実際の仕組み（無ければ problem_coverage_test の既存方式に合わせる）；`refetchInterval` の停止条件（TanStack Query の実バージョン）。いずれも該当タスク内に注記済み。
