# プラン管理・解約画面（アカウント設定 > プランの管理）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 利用者がアプリ内「アカウント設定 > プランの管理」で現在のプラン状態を確認し、Stripe 顧客ポータル経由で解約（期末）・支払い方法変更・請求履歴を行える導線を実装する。

**Architecture:** アプリ内 `/account` に状態表示を持ち、操作は Stripe 顧客ポータルへ委譲。状態表示は `GET /billing/subscription`、ポータル遷移は `POST /billing/portal-session`。ポータルでの解約/変更は決済コアの既存 Webhook（`customer.subscription.updated`/`deleted`）がそのまま `subscriptions` に同期する（新規 Webhook 実装なし）。

**Tech Stack:** Go（echo v4, pgx v5, stripe-go v86）、React 19（react-router, TanStack Query, Vitest+MSW）。

## Global Constraints

- 解約・カード変更・請求履歴は **Stripe 顧客ポータル**に委譲（自前UIを作らない）。カード情報は自社を通さない。
- 利用者の解約は**期末**（`cancel_at_period_end`。ポータル設定で「期末解約」にする）。即時失効の `SubscriptionService.Revoke` は据え置き（管理者用）。
- ポータルを開けるのは `provider_customer_id` が非空の加入のみ。無い場合（手動付与/未加入）は `ErrNoBillingCustomer`（409 `no-billing-customer`）＋フロントはボタン非表示。
- `GET /billing/subscription` は `EntitlementService.For` 由来の `plan`（premium/free。trialing・past_due 猶予も premium）＋生の `status`（none/trialing/active/past_due/canceled）を返す。
- 新規 Webhook 実装は不要（決済コアの `HandleWebhook` が同期）。マイグレーション不要。
- Go module: `github.com/yuuyakim/menu-planner/backend`。stripe-go は adapter（`repository/payment_stripe.go`）にのみ import。
- ブランチ: `feat/payment-core` に継続（launch バンドル）。
- TDD（🔴 test → 🟢 impl 別コミット）、1挙動1コミット、DRY / YAGNI。

---

### Task 1: PaymentGateway に顧客ポータルセッション作成を追加（port + adapter）

**Files:**
- Modify: `backend/internal/service/ports.go`（`PaymentGateway` にメソッド追加）
- Modify: `backend/internal/repository/payment_stripe.go`（Stripe 実装追加）

**Interfaces:**
- Consumes: なし（新規メソッド）
- Produces: `PaymentGateway.CreateBillingPortalSession(ctx context.Context, customerID, returnURL string) (string, error)`

**注記:** このメソッドは実 Stripe API を叩くため、決済コアの `CreateCheckoutSession` と同様にユニットテストは付けない（手動E2Eで検証）。本タスクは port 追加＋adapter 実装＋ビルド確認まで。

- [ ] **Step 1: port にメソッドを追加**

`backend/internal/service/ports.go` の `PaymentGateway` インターフェースに1行追加:
```go
	// CreateBillingPortalSession は Stripe 顧客ポータルのセッションを作り、遷移先 URL を返す。
	CreateBillingPortalSession(ctx context.Context, customerID, returnURL string) (string, error)
```

- [ ] **Step 2: adapter に実装を追加**

`backend/internal/repository/payment_stripe.go`:

- import に billingportal/session を**別名で**追加（`checkout/session` が `session` を占有しているため）:
```go
	portalsession "github.com/stripe/stripe-go/v86/billingportal/session"
```
- メソッドを追加（`ParseWebhookEvent` の後あたり）:
```go
// CreateBillingPortalSession は顧客ポータルのセッションを作り、遷移先 URL を返す。
func (g *StripePaymentGateway) CreateBillingPortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(returnURL),
	}
	params.Context = ctx
	s, err := portalsession.New(params)
	if err != nil {
		return "", fmt.Errorf("顧客ポータルセッションの作成に失敗しました: %w", err)
	}
	return s.URL, nil
}
```
（`stripe.BillingPortalSessionParams` と `billingportal/session.New` は v86 の classic API。既存 adapter が `checkout/session`.New を classic で使えているので同様に使えるはず。もし v86 で該当シンボルが異なる場合は、context7（`/stripe/stripe-go`）または `go doc github.com/stripe/stripe-go/v86/billingportal/session` で確認し、同じ挙動になるよう調整する。）

- [ ] **Step 3: ビルド確認**

Run: `cd backend && go build ./... && go vet ./...`
Expected: 成功（`portalsession` が使われていること、未使用 import が無いこと）。

- [ ] **Step 4: コミット**

```bash
git add backend/internal/service/ports.go backend/internal/repository/payment_stripe.go
git commit -m "feat: PaymentGateway に Stripe 顧客ポータルセッション作成を追加"
```

---

### Task 2: BillingService に Subscription 取得と PortalSession 作成を追加

**Files:**
- Modify: `backend/internal/service/billing.go`
- Modify: `backend/cmd/server/main.go`（`NewBillingService` の呼び出しに returnURL 引数を追加）
- Test: `backend/internal/service/billing_test.go`

**Interfaces:**
- Consumes: `SubscriptionStore.Find`, `Entitlements.For`, `PaymentGateway.CreateBillingPortalSession`（Task 1）, `ErrSubscriptionNotFound`
- Produces:
  - `service.SubscriptionView struct { Plan, Status string; CurrentPeriodEnd time.Time; CancelAtPeriodEnd, HasPortal bool }`
  - `service.ErrNoBillingCustomer`
  - `(*BillingService).Subscription(ctx, userID string) (SubscriptionView, error)`
  - `(*BillingService).CreatePortalSession(ctx, userID string) (string, error)`
  - `NewBillingService` に `accountURL string` 引数を追加（`cancelURL` の次）

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/service/billing_test.go` に追記（既存の `fakeStore`/`fakeEnt`/`fakeGateway`/`newBilling`/`validUID` を再利用。`fakeGateway` に portal 用フィールドが無ければ足す）:

```go
// fakeGateway に以下を追加（既存の struct/メソッドに追記）:
//   portalURL        string
//   portalCustomerID string
//   portalReturnURL  string
// func (f *fakeGateway) CreateBillingPortalSession(_ context.Context, customerID, returnURL string) (string, error) {
//     f.portalCustomerID = customerID; f.portalReturnURL = returnURL; return f.portalURL, nil
// }
// newBilling は NewBillingService の新シグネチャに合わせ accountURL "https://app/account" を渡すよう更新する。

func TestBilling_Subscription_NoRow(t *testing.T) {
	svc := newBilling(&fakeStore{found: false}, fakeEnt{plan: domain.PlanFree}, &fakeGateway{})
	v, err := svc.Subscription(context.Background(), validUID)
	if err != nil {
		t.Fatalf("Subscription: %v", err)
	}
	if v.Plan != "free" || v.Status != "none" || v.HasPortal {
		t.Errorf("行なしは free/none/hasPortal=false。got %+v", v)
	}
}

func TestBilling_Subscription_PremiumActive(t *testing.T) {
	end := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{found: true, sub: domain.Subscription{
		Plan: domain.PlanPremium, Status: domain.SubscriptionActive,
		CurrentPeriodEnd: end, ProviderCustomerID: "cus_1",
	}}
	svc := newBilling(store, fakeEnt{plan: domain.PlanPremium}, &fakeGateway{})
	v, err := svc.Subscription(context.Background(), validUID)
	if err != nil {
		t.Fatalf("Subscription: %v", err)
	}
	if v.Plan != "premium" || v.Status != "active" || !v.HasPortal || !v.CurrentPeriodEnd.Equal(end) {
		t.Errorf("不正: %+v", v)
	}
}

func TestBilling_CreatePortalSession_NoCustomer(t *testing.T) {
	// customer 空（手動付与相当）
	store := &fakeStore{found: true, sub: domain.Subscription{Status: domain.SubscriptionActive}}
	svc := newBilling(store, fakeEnt{plan: domain.PlanPremium}, &fakeGateway{})
	if _, err := svc.CreatePortalSession(context.Background(), validUID); !errors.Is(err, service.ErrNoBillingCustomer) {
		t.Fatalf("customer 無しは ErrNoBillingCustomer。got %v", err)
	}
}

func TestBilling_CreatePortalSession_NoRow(t *testing.T) {
	svc := newBilling(&fakeStore{found: false}, fakeEnt{plan: domain.PlanFree}, &fakeGateway{})
	if _, err := svc.CreatePortalSession(context.Background(), validUID); !errors.Is(err, service.ErrNoBillingCustomer) {
		t.Fatalf("行なしは ErrNoBillingCustomer。got %v", err)
	}
}

func TestBilling_CreatePortalSession_OK(t *testing.T) {
	store := &fakeStore{found: true, sub: domain.Subscription{
		Status: domain.SubscriptionActive, ProviderCustomerID: "cus_9",
	}}
	gw := &fakeGateway{portalURL: "https://stripe/portal"}
	svc := newBilling(store, fakeEnt{plan: domain.PlanPremium}, gw)
	url, err := svc.CreatePortalSession(context.Background(), validUID)
	if err != nil {
		t.Fatalf("CreatePortalSession: %v", err)
	}
	if url != "https://stripe/portal" {
		t.Errorf("url = %q", url)
	}
	if gw.portalCustomerID != "cus_9" || gw.portalReturnURL != "https://app/account" {
		t.Errorf("gateway 引数が不正: cus=%q return=%q", gw.portalCustomerID, gw.portalReturnURL)
	}
}
```

- [ ] **Step 2: 失敗を確認**

Run: `cd backend && go test ./internal/service/ -run 'Subscription|PortalSession' -v`
Expected: コンパイルエラー（`SubscriptionView` / `CreatePortalSession` / `ErrNoBillingCustomer` 未定義、`NewBillingService` の引数不一致）。

- [ ] **Step 3: 実装する**

`backend/internal/service/billing.go`:

- エラーを追加（`ErrAlreadySubscribed` の近く）:
```go
// ErrNoBillingCustomer は Stripe 顧客が紐づいていない（手動付与や未加入）ため
// 顧客ポータルを開けないことを表す。
var ErrNoBillingCustomer = errors.New("課金の顧客情報がありません")
```
- 表示用の型を追加:
```go
// SubscriptionView はプラン管理画面の表示用。
type SubscriptionView struct {
	Plan              string    // "free" | "premium"（EntitlementService 由来）
	Status            string    // "none" | "trialing" | "active" | "past_due" | "canceled"
	CurrentPeriodEnd  time.Time // 行が無ければゼロ値
	CancelAtPeriodEnd bool
	HasPortal         bool // provider_customer_id があり顧客ポータルを開けるか
}
```
- `BillingService` 構造体に `accountURL string` を追加し、`NewBillingService` の引数（`cancelURL` の次）に `accountURL string` を足して代入する:
```go
type BillingService struct {
	entitlements Entitlements
	store        SubscriptionStore
	gateway      PaymentGateway
	successURL   string
	cancelURL    string
	accountURL   string
	trialDays    int
	now          func() time.Time
}

func NewBillingService(
	entitlements Entitlements, store SubscriptionStore, gateway PaymentGateway,
	successURL, cancelURL, accountURL string, trialDays int, now func() time.Time,
) *BillingService {
	if now == nil {
		now = time.Now
	}
	return &BillingService{
		entitlements: entitlements, store: store, gateway: gateway,
		successURL: successURL, cancelURL: cancelURL, accountURL: accountURL,
		trialDays: trialDays, now: now,
	}
}
```
- メソッドを追加:
```go
// Subscription はプラン管理画面の表示値を返す。
func (s *BillingService) Subscription(ctx context.Context, userID string) (SubscriptionView, error) {
	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return SubscriptionView{}, err
	}
	view := SubscriptionView{Plan: string(ent.Plan()), Status: "none"}

	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return SubscriptionView{}, err
	}
	sub, err := s.store.Find(ctx, uid)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return view, nil // 行なし＝無料
		}
		return SubscriptionView{}, err
	}
	view.Status = string(sub.Status)
	view.CurrentPeriodEnd = sub.CurrentPeriodEnd
	view.CancelAtPeriodEnd = sub.CancelAtPeriodEnd
	view.HasPortal = sub.ProviderCustomerID != ""
	return view, nil
}

// CreatePortalSession は顧客ポータルのセッションを作り URL を返す。
// Stripe 顧客が無い（手動付与・未加入）場合は ErrNoBillingCustomer。
func (s *BillingService) CreatePortalSession(ctx context.Context, userID string) (string, error) {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return "", err
	}
	sub, err := s.store.Find(ctx, uid)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return "", ErrNoBillingCustomer
		}
		return "", err
	}
	if sub.ProviderCustomerID == "" {
		return "", ErrNoBillingCustomer
	}
	return s.gateway.CreateBillingPortalSession(ctx, sub.ProviderCustomerID, s.accountURL)
}
```
（`domain.Plan` の `String()` が `"free"`/`"premium"` を返すことは既存。`ent.Plan()` は `domain.Plan` を返すので `string(ent.Plan())` で `"free"`/`"premium"`。）

`backend/cmd/server/main.go`: `service.NewBillingService(...)` の呼び出しに `accountURL` を追加する。既存の success/cancel URL 生成の並びに合わせて:
```go
	billingSvc := service.NewBillingService(
		entitlementSvc, subscriptionRepo, paymentGateway,
		frontendOrigin+"/checkout/complete?session_id={CHECKOUT_SESSION_ID}",
		frontendOrigin+"/checkout",
		frontendOrigin+"/account",
		trialDays, time.Now)
```

- [ ] **Step 4: 成功を確認**

Run: `cd backend && go test ./internal/service/ -run 'Subscription|PortalSession|Billing' -v && go build ./...`
Expected: PASS かつビルド成功。

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/billing.go backend/internal/service/billing_test.go backend/cmd/server/main.go
git commit -m "feat: プラン状態取得と顧客ポータルセッション作成を BillingService に追加"
```

---

### Task 3: 課金ハンドラに subscription / portal-session を追加＋エラー写像

**Files:**
- Modify: `backend/internal/handler/billing.go`
- Modify: `backend/internal/handler/problem.go`
- Modify: `backend/internal/handler/problem_coverage_test.go`
- Test: `backend/internal/handler/billing_test.go`

**Interfaces:**
- Consumes: `BillingService.Subscription`/`CreatePortalSession`（Task 2）, `service.SubscriptionView`, `service.ErrNoBillingCustomer`
- Produces: `GET /api/v1/billing/subscription`, `POST /api/v1/billing/portal-session`

- [ ] **Step 1: problem 写像を追加**

`backend/internal/handler/problem.go` の `problemMapping` に、`already-subscribed` の近くへ追加:
```go
	// 顧客ポータルを開こうとしたが Stripe 顧客が無い（手動付与・未加入）。409。
	{service.ErrNoBillingCustomer, http.StatusConflict, "no-billing-customer", "課金の顧客情報がありません"},
```

- [ ] **Step 2: 失敗するハンドラテストを書く**

`backend/internal/handler/billing_test.go` に追記（既存の `fakeBilling` に新メソッドを足す。既存の echo/httptest/認証Cookie ヘルパを流用）:

```go
// fakeBilling に追加:
//   view      service.SubscriptionView
//   viewErr   error
//   portalURL string
//   portalErr error
// func (f *fakeBilling) Subscription(context.Context, string) (service.SubscriptionView, error) { return f.view, f.viewErr }
// func (f *fakeBilling) CreatePortalSession(context.Context, string) (string, error) { return f.portalURL, f.portalErr }

func TestBillingHandler_Subscription_RequiresAuth(t *testing.T) {
	// 認証Cookieなしで GET /api/v1/billing/subscription → 401（既存の未認証テストと同じ作り）
}

func TestBillingHandler_Subscription_ReturnsView(t *testing.T) {
	// 認証あり・view={Plan:"premium",Status:"active",HasPortal:true} → 200 で
	// body に "premium" / "active" / "hasPortal":true が含まれる
}

func TestBillingHandler_PortalSession_NoBillingCustomer(t *testing.T) {
	// portalErr = service.ErrNoBillingCustomer → 409、body に "no-billing-customer"
}

func TestBillingHandler_PortalSession_ReturnsURL(t *testing.T) {
	// portalURL="https://stripe/portal" → 200 で {"url":"https://stripe/portal"}
}
```
（各本体は既存の `saved_shopping_list` / billing ハンドラテストの作法に合わせ `httptest`+echo で実装。認証必須の検証は既存 RequireAuth テストの Cookie 発行方法を流用する。）

- [ ] **Step 3: 失敗を確認**

Run: `cd backend && go test ./internal/handler/ -run 'Billing|Coverage' -v`
Expected: コンパイルエラー（新メソッド/ルート未実装）または coverage テスト失敗。

- [ ] **Step 4: ハンドラを実装する**

`backend/internal/handler/billing.go`:

- `BillingUseCase` インターフェースにメソッド追加:
```go
	Subscription(ctx context.Context, userID string) (service.SubscriptionView, error)
	CreatePortalSession(ctx context.Context, userID string) (string, error)
```
- `RegisterRoutes` にルート追加（`requireAuth` 付き）:
```go
	g.GET("/billing/subscription", h.Subscription, requireAuth)
	g.POST("/billing/portal-session", h.PortalSession, requireAuth)
```
- DTO とハンドラを追加:
```go
type subscriptionDTO struct {
	Plan              string  `json:"plan"`
	Status            string  `json:"status"`
	CurrentPeriodEnd  *string `json:"currentPeriodEnd"`
	CancelAtPeriodEnd bool    `json:"cancelAtPeriodEnd"`
	HasPortal         bool    `json:"hasPortal"`
}

func (h *BillingHandler) Subscription(c echo.Context) error {
	userID, _ := UserIDFromContext(c)
	v, err := h.svc.Subscription(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	dto := subscriptionDTO{
		Plan: v.Plan, Status: v.Status,
		CancelAtPeriodEnd: v.CancelAtPeriodEnd, HasPortal: v.HasPortal,
	}
	if !v.CurrentPeriodEnd.IsZero() {
		s := v.CurrentPeriodEnd.Format(time.RFC3339)
		dto.CurrentPeriodEnd = &s
	}
	return c.JSON(http.StatusOK, dto)
}

func (h *BillingHandler) PortalSession(c echo.Context) error {
	userID, _ := UserIDFromContext(c)
	url, err := h.svc.CreatePortalSession(c.Request().Context(), userID)
	if err != nil {
		return err // ErrNoBillingCustomer は problem マッピングで 409
	}
	return c.JSON(http.StatusOK, map[string]string{"url": url})
}
```
（`time` は既存 import。無ければ追加。）

- [ ] **Step 5: 成功を確認**

Run: `cd backend && go test ./internal/handler/ -v && go build ./...`
Expected: PASS（Billing 新テスト＋coverage 含む）かつビルド成功。

- [ ] **Step 6: コミット**

```bash
git add backend/internal/handler/billing.go backend/internal/handler/problem.go \
        backend/internal/handler/problem_coverage_test.go backend/internal/handler/billing_test.go
git commit -m "feat: プラン状態取得と顧客ポータルのAPIを追加"
```

---

### Task 4: フロント — アカウント設定画面（プランの管理）

**Files:**
- Modify: `frontend/src/features/billing/api.ts`
- Create: `frontend/src/features/billing/AccountPage.tsx`
- Test: `frontend/src/features/billing/AccountPage.test.tsx`
- Modify: `frontend/src/app/App.tsx`（`/account` ルート）
- Modify: `frontend/src/features/auth/AuthMenu.tsx`（「アカウント設定」リンク）

**Interfaces:**
- Consumes: `apiGet`/`apiPost`（`../../api/client`）
- Produces: `getSubscription()`, `createPortalSession()`, `<AccountPage />`

- [ ] **Step 1: billing api を拡張**

`frontend/src/features/billing/api.ts` に追記:
```ts
export interface SubscriptionInfo {
  plan: 'free' | 'premium'
  status: 'none' | 'trialing' | 'active' | 'past_due' | 'canceled'
  currentPeriodEnd: string | null
  cancelAtPeriodEnd: boolean
  hasPortal: boolean
}

/** getSubscription は現在のプラン状態を取得する（表示用）。 */
export function getSubscription(): Promise<SubscriptionInfo> {
  return apiGet<SubscriptionInfo>('/billing/subscription')
}

/** createPortalSession は Stripe 顧客ポータルのセッションを作り、遷移先 URL を返す。 */
export function createPortalSession(): Promise<{ url: string }> {
  return apiPost<{ url: string }>('/billing/portal-session')
}
```

- [ ] **Step 2: 失敗するテストを書く**

`frontend/src/features/billing/AccountPage.test.tsx`（既存の `renderWithProviders`+MSW、`window.location` スタブは `CheckoutPage.test.tsx` の作法に合わせる）:

```tsx
// 主要なふるまい:
// 1. status:'active', hasPortal:true → 「次回請求」表示＋「プランを管理する」ボタン
// 2. 「プランを管理する」押下で createPortalSession が呼ばれ、返った url へ遷移
// 3. plan:'free', status:'none' → 「プレミアムにアップグレード」→ /checkout リンク（getByRole link href=/checkout）
// 4. status:'active', cancelAtPeriodEnd:true → 「〜で解約予定」表示
// 5. hasPortal:false（手動付与相当・status:'active'）→ 「プランを管理する」ボタンが出ない
```
（各ケースは `GET /billing/subscription` を MSW でモックしてレンダリング、DOM を検証。#2 は `POST /billing/portal-session` を `{url}` にモックし window.location.href を検証。）

- [ ] **Step 3: 失敗を確認**

Run: `cd frontend && npx vitest run src/features/billing/AccountPage.test.tsx`
Expected: FAIL（`AccountPage` 未実装）。

- [ ] **Step 4: 画面を実装する**

`frontend/src/features/billing/AccountPage.tsx`（既存ページのローディング/エラー・`kon-*` スタイル、`CheckoutPage` の `formatJst`・ボタンスタイルに合わせる）:

```tsx
import { Link } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'

import { ErrorMessage } from '../../components/ErrorMessage'
import { getSubscription, createPortalSession, type SubscriptionInfo } from './api'

function formatJst(iso: string): string {
  return new Intl.DateTimeFormat('ja-JP', {
    year: 'numeric', month: 'long', day: 'numeric', timeZone: 'Asia/Tokyo',
  }).format(new Date(iso))
}

function planLabel(s: SubscriptionInfo): string {
  if (s.plan !== 'premium') {
    return s.status === 'canceled' ? '解約済み（無料プラン）' : '無料プラン'
  }
  const end = s.currentPeriodEnd ? formatJst(s.currentPeriodEnd) : ''
  if (s.status === 'trialing') return `プレミアム（無料お試し中）／初回課金 ${end}`
  if (s.status === 'past_due') return 'プレミアム（お支払いの確認中。カード情報の更新をお願いします）'
  if (s.cancelAtPeriodEnd) return `プレミアム（${end}で解約予定。それまでご利用いただけます）`
  return `プレミアム（次回請求 ${end}）`
}

export function AccountPage() {
  const sub = useQuery({ queryKey: ['billing', 'subscription'], queryFn: getSubscription })
  const portal = useMutation({
    mutationFn: createPortalSession,
    onSuccess: ({ url }) => {
      window.location.href = url
    },
  })

  if (sub.isPending) return <p>読み込み中…</p>
  if (sub.error) return <ErrorMessage error={sub.error} />

  const s = sub.data
  const showUpgrade = s.plan !== 'premium'

  return (
    <section className="mx-auto max-w-md">
      <h1 className="text-xl font-bold text-kon-ink">アカウント設定</h1>
      <h2 className="mt-4 font-medium text-kon-ink">プランの管理</h2>
      <p className="mt-2 text-kon-ink">{planLabel(s)}</p>

      {portal.error && <div className="mt-3"><ErrorMessage error={portal.error} /></div>}

      <div className="mt-4">
        {showUpgrade ? (
          <Link
            to="/checkout"
            className="inline-block rounded-full bg-kon-leaf px-4 py-2 font-medium text-white"
          >
            プレミアムにアップグレード
          </Link>
        ) : s.hasPortal ? (
          <button
            type="button"
            disabled={portal.isPending}
            onClick={() => portal.mutate()}
            className="rounded-full bg-kon-leaf px-4 py-2 font-medium text-white disabled:opacity-50"
          >
            プランを管理する
          </button>
        ) : null}
      </div>
    </section>
  )
}
```
（`ErrorMessage` の import パス・props は既存に合わせる。`showUpgrade` は canceled/free の両方を拾う。premium かつ hasPortal=false（手動付与）はボタン無し・状態のみ。）

- [ ] **Step 5: ルートと導線を追加**

`frontend/src/app/App.tsx`:
- import: `import { AccountPage } from '../features/billing/AccountPage'`
- `/favorites` 付近に認証必須で追加:
```tsx
            <Route
              path="/account"
              element={
                <RequireAuth>
                  <AccountPage />
                </RequireAuth>
              }
            />
```

`frontend/src/features/auth/AuthMenu.tsx`: ログイン済みメニューに「アカウント設定」への `<Link to="/account">アカウント設定</Link>` を追加（既存のログアウト等と同じ並び・スタイルに合わせる）。未ログイン時は出さない。

- [ ] **Step 6: 成功を確認**

Run: `cd frontend && npx vitest run src/features/billing/ && npx vitest run` （全体回帰）
Expected: PASS。`npx tsc -b --noEmit` clean。

- [ ] **Step 7: コミット**

```bash
git add frontend/src/features/billing/api.ts frontend/src/features/billing/AccountPage.tsx \
        frontend/src/features/billing/AccountPage.test.tsx frontend/src/app/App.tsx \
        frontend/src/features/auth/AuthMenu.tsx
git commit -m "feat: アカウント設定（プランの管理）画面と導線を追加"
```

---

### Task 5: openapi / schema.d.ts / spec.md / 手動E2E手順 の更新

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `frontend/src/api/schema.d.ts`（再生成）
- Modify: `spec.md`
- Modify: `docs/manual-e2e-payment.md`

**Interfaces:**
- Consumes: Task 3 の API
- Produces: 契約・仕様・手順が実装と一致

- [ ] **Step 1: openapi.yaml を更新**

`api/openapi.yaml` に2エンドポイントを追加（既存の billing の書式・security・problem 参照に合わせる）:
- `GET /billing/subscription`（要認証、200: `{plan,status,currentPeriodEnd(nullable),cancelAtPeriodEnd,hasPortal}`、401）
- `POST /billing/portal-session`（要認証、200: `{url}`、401、409 `no-billing-customer`）
フィールド名はバックエンド DTO（`plan,status,currentPeriodEnd,cancelAtPeriodEnd,hasPortal` / `{url}`）と一致させる。

- [ ] **Step 2: schema.d.ts を再生成**

Run: `cd frontend && npm run gen:api`
2回実行して差分が出ない（冪等）ことを確認し、生成物をコミット対象にする。

- [ ] **Step 3: spec.md を更新**

`spec.md` の §2.11（プレミアムプラン）に「アカウント設定 > プランの管理」画面と Stripe 顧客ポータルによる解約・カード変更・請求履歴を追記。API 一覧に `GET /billing/subscription` と `POST /billing/portal-session` を追加。解約は期末（利用規約4条5項）で、ポータル操作は既存 Webhook で同期する旨を1文で記す。

- [ ] **Step 4: 手動E2E手順を追記**

`docs/manual-e2e-payment.md` に「シナリオB'（顧客ポータルで解約）」を追記: `/account` →「プランを管理する」→ Stripe ポータルで期末解約 → `customer.subscription.updated`(cancel_at_period_end=true) → `/account` 再訪で「〈日〉で解約予定」表示 → 期末 `deleted` → free。カード変更もポータルで確認。ローンチ前に **Stripe ダッシュボードで顧客ポータルを有効化**（解約=期末・支払い方法更新・請求履歴・戻りURL `/account`、test/live 両方）が必要な旨を明記。

- [ ] **Step 5: 確認**

Run: `cd frontend && npx tsc -b --noEmit`
Expected: 型エラーなし。

- [ ] **Step 6: コミット**

```bash
git add api/openapi.yaml frontend/src/api/schema.d.ts spec.md docs/manual-e2e-payment.md
git commit -m "docs: プラン管理APIを openapi/schema/spec/手動E2E に反映"
```

---

## Self-Review（計画作成後のチェック結果）

- **spec カバレッジ**: spec §4.1(GET subscription)→T2/T3、§4.2(portal-session)→T2/T3、§4.3(port/adapter)→T1、§4.4(service/ErrNoBillingCustomer)→T2/T3、§5(フロント/AccountPage/状態別表示/AuthMenu)→T4、§6(テスト)→各task、§7(ローンチ: ポータル有効化)→T5(手順明記)。§3.3(手動付与はボタン非表示＋ErrNoBillingCustomer)→T2(サービス)/T4(hasPortal=false 非表示)。
- **プレースホルダ**: 実コードを各ステップに記載。フロントのヘルパ名（`renderWithProviders`・`ErrorMessage`・`window.location` スタブ）は「既存に合わせる」と明示（環境依存の事実、実装者が確認）。
- **型整合**: `SubscriptionView`（Plan/Status/CurrentPeriodEnd/CancelAtPeriodEnd/HasPortal, T2）→ handler DTO（T3, JSON: plan/status/currentPeriodEnd/cancelAtPeriodEnd/hasPortal）→ フロント `SubscriptionInfo`（T4）で名称一致。`CreateBillingPortalSession(ctx,customerID,returnURL)`（T1）を T2 が使用。`NewBillingService` の新引数 `accountURL`（T2）は main.go 同一タスクで更新しビルドを壊さない。`ErrNoBillingCustomer`（T2）→ problem 写像（T3）。
- **注意（実装時判断）**: v86 の `billingportal/session` classic シンボル（無ければ context7/godoc で確認）。フロントのテストヘルパ名。`domain.Plan` の String が "free"/"premium" を返すこと（既存前提）。
