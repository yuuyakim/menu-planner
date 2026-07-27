# プレミアム加入導線の再建 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `/checkout`（加入画面）へ到達できる導線を作り、`PremiumLock` の仕様違反を解消したうえで、公開の料金ページ `/pricing` を新設する。

**Architecture:** バックエンドに価格の公開エンドポイント `GET /billing/plan`（認証不要）を1本足し、価格リテラルを `BillingService.Plan()` の1箇所に集約する。フロントは `PremiumLock` を実際のリンクに差し替え（文脈内導線）、`/pricing` とフッター・ホームの入口を追加する（常設導線）。

**Tech Stack:** Go 1.x + Echo（backend）／React + react-router + TanStack Query + Tailwind（frontend）／Vitest + MSW（unit）／Playwright（E2E）／OpenAPI + openapi-typescript（型生成）

**設計:** `docs/superpowers/specs/2026-07-27-upgrade-entrypoints-design.md`

**ブランチ:** `feat/upgrade-entrypoints`（`main` から分岐済み）

## Global Constraints

- **TDD。** テストを先に書いて失敗を確認し、それからコードを書く。**コミットは各 Step の指示に従う**（テストと実装を同じコミットに含める Step がある。RED → GREEN を踏むことが要件で、コミットの分割は要件としない）。
- **コミット接頭辞は日本語本文で `feat:` / `fix:` / `test:` / `docs:`。** 既存の履歴に合わせる。
- **`api/openapi.yaml` が API 仕様の正。** 変更したら必ず `make gen-api` で `frontend/src/api/schema.d.ts` を再生成してコミットする。CI が再生成して差分が出たら落ちる。
- **コメントは「なぜ」を書く。** 何をしているかはコードが語る。既存ファイルの密度と文体（和文・常体）に合わせる。
- **保存上限の数値（50件）をフロントエンドに書かない。** `spec.md`「上限の数値を返さない理由」に従う。
- **価格は 300、通貨は `jpy`、トライアルは 5 日。** それぞれ `backend/internal/service/billing.go` と `backend/cmd/server/main.go:158` が現に持っている値。計画中でこれらを直書きするのはバックエンドの1箇所とテストの期待値のみ。
- **`frontend/src/features/legal/content/tokushoho.md` は触らない。** 法務文書は版として固定する。

## ファイル構成

| ファイル | 責務 | 変更 |
| --- | --- | --- |
| `backend/internal/service/billing.go` | 価格の定義（`Plan()`）と申込導線 | 修正 |
| `backend/internal/handler/billing.go` | 課金APIの受け口。公開ルートの登録 | 修正 |
| `api/openapi.yaml` | API 仕様の正 | 修正 |
| `frontend/src/features/billing/api.ts` | 課金APIのクライアント | 修正 |
| `frontend/src/test/handlers.ts` | MSW の既定応答 | 修正 |
| `frontend/src/features/premium/PremiumLock.tsx` | ロック時の文脈内導線 | 修正 |
| `frontend/src/features/pricing/PricingPage.tsx` | 料金と比較表の公開ページ | **新規** |
| `frontend/src/components/Footer.tsx` | 全ページ共通の常設リンク | 修正 |
| `frontend/src/features/home/HomePage.tsx` | ホームの紹介カード | 修正 |
| `frontend/src/app/App.tsx` | ルーティング | 修正 |
| `spec.md` | 仕様の正 | 修正 |

---

### Task 1: 価格の公開エンドポイント `GET /billing/plan`

価格リテラルを `Plan()` に集約し、認証不要の公開ルートを1本足す。`/pricing` を未ログインに見せるための土台。

**Files:**
- Modify: `backend/internal/service/billing.go:28-36`（`PreviewResult` の隣に `PlanInfo` を足す）, `:74-89`（`Preview`）
- Modify: `backend/internal/service/billing_test.go`（末尾に追加）
- Modify: `backend/internal/handler/billing.go:17-23`（interface）, `:40-48`（ルート）, `:50-58`（DTO の隣）
- Modify: `backend/internal/handler/billing_test.go:38-60`（fake）, 末尾（テスト）
- Modify: `api/openapi.yaml:790`（paths）, `:1390`（schemas）, `spec.md:862`（5.7 の表）
- Regenerate: `frontend/src/api/schema.d.ts`

**Interfaces:**
- Consumes: なし（最初のタスク）
- Produces:
  - `service.PlanInfo{ Price int; Currency string; TrialDays int }`
  - `func (s *BillingService) Plan() service.PlanInfo`
  - `GET /api/v1/billing/plan` → `{"price":300,"currency":"jpy","trialDays":5}`（認証不要）

- [ ] **Step 1: service の失敗するテストを書く**

`backend/internal/service/billing_test.go` の末尾に足す。既存の `newBilling` ヘルパ（`:63`）と `validUID`（`:61`）をそのまま使う。

```go
func TestBilling_Plan_PublicInfo(t *testing.T) {
	svc := newBilling(&fakeStore{}, fakeEnt{plan: domain.PlanFree}, &fakeGateway{})
	got := svc.Plan()
	if got.Price != 300 || got.Currency != "jpy" || got.TrialDays != 5 {
		t.Errorf("plan 不正: %+v", got)
	}
}

// 価格の定義が1箇所であることを守る。Plan() だけ直して Preview が古い値を
// 返す、という取りこぼしを検知する。値を直書きで比べず、両者を突き合わせる。
func TestBilling_Preview_UsesPlanValues(t *testing.T) {
	svc := newBilling(&fakeStore{found: false}, fakeEnt{plan: domain.PlanFree}, &fakeGateway{})
	plan := svc.Plan()
	got, err := svc.Preview(context.Background(), validUID)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if got.Price != plan.Price || got.Currency != plan.Currency || got.TrialDays != plan.TrialDays {
		t.Errorf("Preview は Plan と同じ値を返すべき。preview=%+v plan=%+v", got, plan)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/service/ -run 'TestBilling_Plan|TestBilling_Preview_UsesPlanValues' -v`
Expected: コンパイルエラー `svc.Plan undefined (type *service.BillingService has no field or method Plan)`

- [ ] **Step 3: `PlanInfo` と `Plan()` を実装する**

`backend/internal/service/billing.go` の `PreviewResult`（`:28-36`）の**直前**に足す。

```go
// PlanInfo は誰にでも同じ、プランの公開情報。
// 個人に依る値（トライアル適格・初回課金日）は含めない。未ログインに返すため。
type PlanInfo struct {
	Price     int
	Currency  string
	TrialDays int
}
```

`Preview`（`:74`）の**直前**に足す。

```go
// Plan はプランの公開情報を返す。
//
// **価格の定義はここだけに置く。** Preview もこれを通すことで、
// 値上げのときに直す場所が1つで済む。
func (s *BillingService) Plan() PlanInfo {
	return PlanInfo{Price: 300, Currency: "jpy", TrialDays: s.trialDays}
}
```

- [ ] **Step 4: `Preview` を `Plan()` 経由に書き換える**

`backend/internal/service/billing.go:74-89` を丸ごと置き換える。価格リテラルが `Preview` から消えることが要点。

```go
// Preview は申込確認画面の表示値を返す。
func (s *BillingService) Preview(ctx context.Context, userID string) (PreviewResult, error) {
	eligible, _, err := s.trialEligibility(ctx, userID)
	if err != nil {
		return PreviewResult{}, err
	}
	plan := s.Plan()
	first := s.now()
	if eligible {
		first = first.Add(time.Duration(plan.TrialDays) * 24 * time.Hour)
	}
	return PreviewResult{
		Price: plan.Price, Currency: plan.Currency, TrialDays: plan.TrialDays,
		TrialEligible: eligible, FirstBillingAt: first,
		PlanManagementPath: planManagementPath,
	}, nil
}
```

- [ ] **Step 5: service のテストが通ることを確認する**

Run: `cd backend && go test ./internal/service/ -v`
Expected: PASS。既存の `TestBilling_Preview_FirstTime` も引き続き通る（`Price != 300` を見ているため、集約で値が変わっていないことの裏づけになる）

- [ ] **Step 6: コミット**

```bash
git add backend/internal/service/billing.go backend/internal/service/billing_test.go
git commit -m "feat: プランの公開情報 Plan() を足し価格の定義を1箇所に集約する"
```

- [ ] **Step 7: handler の失敗するテストを書く**

`backend/internal/handler/billing_test.go` の `fakeBilling` に メソッドを足す（`:38` の `Preview` の**直前**）。

```go
func (f *fakeBilling) Plan() service.PlanInfo {
	return service.PlanInfo{Price: 300, Currency: "jpy", TrialDays: 5}
}
```

ファイル末尾にテストを足す。

```go
// 料金の提示は未ログインにも見せる。認証を付けると /pricing が成り立たない。
func TestBillingHandler_Plan_NoAuthReturns200(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{}
	e, _ := billingApp(t, svc)

	rec := doBillingRequest(t, e, http.MethodGet, "/api/v1/billing/plan", "", "")

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Price     int    `json:"price"`
		Currency  string `json:"currency"`
		TrialDays int    `json:"trialDays"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, 300, body.Price)
	assert.Equal(t, "jpy", body.Currency)
	assert.Equal(t, 5, body.TrialDays)
}

// 個人に依る値を公開ルートに載せない。載せると未ログインに、実際の申込内容と
// ずれた「初回課金日」を見せることになる（特商法の表示は /billing/preview の責務）。
func TestBillingHandler_Plan_OmitsPersonalFields(t *testing.T) {
	t.Parallel()

	svc := &fakeBilling{}
	e, _ := billingApp(t, svc)

	rec := doBillingRequest(t, e, http.MethodGet, "/api/v1/billing/plan", "", "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "trialEligible")
	assert.NotContains(t, rec.Body.String(), "firstBillingAt")
	assert.NotContains(t, rec.Body.String(), "planManagementPath")
}
```

- [ ] **Step 8: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/handler/ -run TestBillingHandler_Plan -v`
Expected: FAIL。`fakeBilling` にメソッドが増えるだけなので**コンパイルは通り**、ルート未登録のため `Not Found`（404）が返って `require.Equal(t, http.StatusOK, rec.Code)` が落ちる

- [ ] **Step 9: interface・ルート・ハンドラを実装する**

`backend/internal/handler/billing.go:17-23` の interface に1行足す。

```go
	Plan() service.PlanInfo
```

`RegisterRoutes`（`:40-48`）のコメントとルートを更新する。**`requireAuth` を渡さない**のがこのルートの要点なので、その理由を残す。

```go
// RegisterRoutes はルーティングを登録する。
// preview / checkout-session / subscription / portal-session は本人の情報を
// 扱うため認証必須。
// plan は誰にでも同じ公開情報で、料金ページ（/pricing）を未ログインに見せる
// ために認証を付けない。
// webhook は Stripe が直接叩くため認証を付けず、署名検証で守る。
func (h *BillingHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	requireAuth := RequireAuth(h.tokens)
	g.GET("/billing/plan", h.Plan)
	g.GET("/billing/preview", h.Preview, requireAuth)
	g.POST("/billing/checkout-session", h.CreateCheckoutSession, requireAuth)
	g.POST("/billing/webhook", h.Webhook)
	g.GET("/billing/subscription", h.Subscription, requireAuth)
	g.POST("/billing/portal-session", h.PortalSession, requireAuth)
}
```

`previewDTO`（`:50-58`）の**直前**に DTO とハンドラを足す。

```go
// planDTO はプランの公開情報。個人に依る値は含めない。
type planDTO struct {
	Price     int    `json:"price"`
	Currency  string `json:"currency"`
	TrialDays int    `json:"trialDays"`
}

// Plan はプランの公開情報を返す。
//
//	GET /api/v1/billing/plan
//
// 認証は要らない。料金ページ（/pricing）が未ログインにも料金を出すために使う。
func (h *BillingHandler) Plan(c echo.Context) error {
	p := h.svc.Plan()
	return c.JSON(http.StatusOK, planDTO{
		Price: p.Price, Currency: p.Currency, TrialDays: p.TrialDays,
	})
}
```

- [ ] **Step 10: テストが通ることを確認する**

Run: `cd backend && go test ./... -cover`
Expected: PASS（全パッケージ）

- [ ] **Step 11: Lint を通す**

Run: `cd backend && go vet ./... && golangci-lint run`
Expected: 指摘なし

- [ ] **Step 12: コミット**

```bash
git add backend/internal/handler/billing.go backend/internal/handler/billing_test.go
git commit -m "feat: プランの公開情報を返す GET /billing/plan を足す"
```

- [ ] **Step 13: OpenAPI に追記する**

`api/openapi.yaml:790` の `/api/v1/billing/preview:` の**直前**に足す。`security: []` が「認証不要」の宣言（ファイル末尾 `:1481` に全体の既定 `accessCookie` がある）。

```yaml
  /api/v1/billing/plan:
    get:
      tags: [billing]
      summary: プランの公開情報を取得する
      description: |
        料金ページ（/pricing）に出す、誰にでも同じプラン情報。
        料金の提示は未ログインにも見せるため認証を要さない。
        個人に依る値（トライアル適格・初回課金日）は含めない。それらは
        /billing/preview（要認証・特商法12条の6の申込確認）が返す。
      security: []
      responses:
        '200':
          description: プランの公開情報
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/BillingPlanResponse'
```

`api/openapi.yaml:1390` の `BillingPreviewResponse:` の**直前**に足す。

```yaml
    BillingPlanResponse:
      type: object
      description: プランの公開情報（誰にでも同じ。/pricing が使う）。
      required: [price, currency, trialDays]
      properties:
        price:
          type: integer
          description: 月額料金（最小単位。currency=jpy なら円）。
          example: 300
        currency:
          type: string
          example: jpy
        trialDays:
          type: integer
          description: トライアル期間の日数。
          example: 5

```

- [ ] **Step 14: 型を再生成する**

Run: `make gen-api`
Expected: `frontend/src/api/schema.d.ts` に `/api/v1/billing/plan` と `BillingPlanResponse` が現れる

確認: `grep -n "billing/plan" frontend/src/api/schema.d.ts`

- [ ] **Step 15: `spec.md` 5.7 の表に足す**

`spec.md:862` の `| GET | \`/billing/preview\` | 必須 | ...` 行の**直前**に足す。

```markdown
| GET | `/billing/plan` | 不要 | プランの公開情報（価格・通貨・無料日数）。料金ページ `/pricing` の表示に使う。個人に依る値は含めない |
```

- [ ] **Step 16: コミット**

```bash
git add api/openapi.yaml frontend/src/api/schema.d.ts spec.md
git commit -m "docs: GET /billing/plan を openapi / schema / spec に反映"
```

---

### Task 2: `PremiumLock` に加入導線を戻す（仕様違反の解消）

`spec.md:297-298` が既に要求している導線を実装する。**この計画で最も効くタスク。** `/weekly`・`/saved-weekly` でブロックされた利用者が、その場から加入画面に進めるようになる。

**Files:**
- Modify: `frontend/src/features/billing/api.ts`（末尾）
- Modify: `frontend/src/test/handlers.ts:40`（配列末尾）
- Modify: `frontend/src/features/premium/PremiumLock.tsx`（全面）
- Modify: `frontend/src/features/premium/PremiumLock.test.tsx`（全面）

**Interfaces:**
- Consumes: `GET /api/v1/billing/plan`（Task 1）
- Produces:
  - `interface PlanInfo { price: number; currency: string; trialDays: number }`（`features/billing/api.ts`）
  - `function getPlan(): Promise<PlanInfo>`
  - クエリキー `['billing', 'plan']`
  - MSW 既定ハンドラ `GET /api/v1/billing/plan` → `{ price: 300, currency: 'jpy', trialDays: 5 }`
  - `PremiumLock` の props は不変（`title` / `description`）。呼び出し側（`WeeklyPage.tsx:122`・`SavedWeeklyPage.tsx:88`）は無改修

- [ ] **Step 1: API クライアントに `getPlan` を足す**

`frontend/src/features/billing/api.ts` の末尾に足す。既存の `getBillingPreview` と同じ流儀。

```ts
/** PlanInfo は誰にでも同じ、プランの公開情報。 */
export interface PlanInfo {
  price: number
  currency: string
  trialDays: number
}

/** getPlan はプランの公開情報を取得する（未ログインでも呼べる）。 */
export function getPlan(): Promise<PlanInfo> {
  return apiGet<PlanInfo>('/billing/plan')
}
```

- [ ] **Step 2: MSW の既定ハンドラを足す**

`frontend/src/test/handlers.ts` の配列末尾（`:39` の `),` の後、`]` の前）に足す。**既定に置くのが要点**で、これが無いと `PremiumLock` を描画する既存テスト（`WeeklyPage` / `SavedWeeklyPage`）が未処理リクエストで落ちる。

```ts
  // 料金はロック画面と料金ページが引く。既定を置かないと、PremiumLock を
  // 描画する既存のテスト（WeeklyPage / SavedWeeklyPage）が未処理リクエストで落ちる。
  http.get('/api/v1/billing/plan', () =>
    HttpResponse.json({ price: 300, currency: 'jpy', trialDays: 5 }),
  ),
```

- [ ] **Step 3: 失敗するテストを書く**

`frontend/src/features/premium/PremiumLock.test.tsx` を丸ごと置き換える。冒頭の `respondMe` は既存のものをそのまま残す。

```tsx
import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { PremiumLock } from './PremiumLock'

// respondMe は現在のユーザーの応答を仕込む。プランだけを差し替える
// （AuthMenu.test.tsx / ShoppingListPage.test.tsx と同じ流儀）。
// 未ログインは test/handlers.ts の既定（401）に任せるため、ここでは呼ばない。
function respondMe(plan: 'free' | 'premium') {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000001',
          email: 'user@example.com',
          displayName: 'ユーザー',
          plan,
        },
      }),
    ),
  )
}

const props = {
  title: '1週間まとめて計画',
  description: '1週間分の献立をまとめて計画できます。',
}

describe('PremiumLock', () => {
  it('ログイン済み free は加入画面へのリンクを出す', async () => {
    respondMe('free')
    renderWithProviders(<PremiumLock {...props} />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムにアップグレード' }),
    ).toHaveAttribute('href', '/checkout')
  })

  // 未ログインにも同じ導線を出す。/checkout は RequireAuth で守られており、
  // 押すとログイン画面を経て /checkout へ戻る。
  it('未ログインも同じ加入導線と、ログインが要る旨を出す', async () => {
    renderWithProviders(<PremiumLock {...props} />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムにアップグレード' }),
    ).toHaveAttribute('href', '/checkout')
    expect(screen.getByText('ログインが必要です')).toBeInTheDocument()
  })

  it('ログイン済み free には「ログインが必要です」を出さない', async () => {
    respondMe('free')
    renderWithProviders(<PremiumLock {...props} />)

    await screen.findByRole('link', { name: 'プレミアムにアップグレード' })
    expect(screen.queryByText('ログインが必要です')).not.toBeInTheDocument()
  })

  it('料金と無料期間を出す', async () => {
    respondMe('free')
    renderWithProviders(<PremiumLock {...props} />)

    expect(await screen.findByText(/月額300円・5日間無料/)).toBeInTheDocument()
  })

  // 料金が引けなくても導線は残す。ここで丸ごと隠すと、この修正が直そうと
  // している「加入画面に行けない」状態に戻ってしまう。
  it('料金の取得に失敗しても加入導線は残る', async () => {
    respondMe('free')
    server.use(
      http.get('/api/v1/billing/plan', () => HttpResponse.error()),
    )
    renderWithProviders(<PremiumLock {...props} />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムにアップグレード' }),
    ).toHaveAttribute('href', '/checkout')
    expect(screen.queryByText(/月額/)).not.toBeInTheDocument()
  })

  it('プランの詳細へのリンクを出す', async () => {
    respondMe('free')
    renderWithProviders(<PremiumLock {...props} />)

    expect(
      await screen.findByRole('link', { name: 'プランの詳細を見る' }),
    ).toHaveAttribute('href', '/pricing')
  })

  it('判定が付くまではローディング表示を出す', () => {
    renderWithProviders(<PremiumLock {...props} />)

    expect(screen.getByRole('status')).toBeInTheDocument()
  })
})
```

- [ ] **Step 4: テストが失敗することを確認する**

Run: `cd frontend && npx vitest run src/features/premium/PremiumLock.test.tsx`
Expected: FAIL。「ログイン済み free は加入画面へのリンクを出す」等が `Unable to find role="link"` で落ちる（現状はリンクを持たないテキストのため）

- [ ] **Step 5: `PremiumLock` を実装する**

`frontend/src/features/premium/PremiumLock.tsx` を丸ごと置き換える。冒頭の「決済フローは未実装（設計スコープ外）」コメントは**削除する**（決済は実装済みで、この記述が今回の取りこぼしの原因だった）。

```tsx
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'

import { MascotStatus } from '../../components/MascotStatus'
import { useCurrentUser } from '../auth/useCurrentUser'
import { getPlan } from '../billing/api'

type Props = {
  /** ロック中の機能名。何ができるようになるかを一言で。 */
  title: string
  /** 補足説明。どう役立つかを具体的に伝える。 */
  description: string
}

// PremiumLock はプレミアム限定機能のプレビューカード。
//
// premium 向けの分岐は持たない。呼び出し側（WeeklyPage / SavedWeeklyPage）が
// premium でないときにだけ描画するため、ここに premium の枝を書いても到達しない。
//
// 未ログインにも同じ加入導線を出す。/checkout は RequireAuth で守られており、
// 押すとログイン画面へ送られ、ログイン後に /checkout へ戻る（RequireAuth が
// state.from を残し、LoginPage がそこへ戻す）。
export function PremiumLock({ title, description }: Props) {
  const { user, isLoading } = useCurrentUser()
  const plan = useQuery({ queryKey: ['billing', 'plan'], queryFn: getPlan })

  if (isLoading) {
    return <MascotStatus>読み込み中…</MascotStatus>
  }

  return (
    <div className="mx-auto max-w-md rounded-2xl border border-kon-leaf-soft bg-white p-6 text-center">
      <p className="text-lg font-bold text-kon-ink">{title}</p>
      <p className="mt-2 text-sm text-kon-ink/70">{description}</p>

      <Link
        to="/checkout"
        className="mt-4 inline-block rounded-full bg-kon-leaf px-5 py-2 font-medium text-white hover:bg-kon-leaf/90"
      >
        プレミアムにアップグレード
      </Link>

      {/* 料金が引けないときは、この行だけを落とす。カードごと隠すと
          加入導線まで消え、この画面が直そうとした不具合に戻る。 */}
      {plan.data && (
        <p className="mt-2 text-sm text-kon-ink/70">
          月額{plan.data.price}円・{plan.data.trialDays}日間無料
        </p>
      )}

      {/* 押してからログイン画面に飛ばされるより、先に断っておく方が親切
          （HomePage の requiresAuth 表示と同じ流儀）。 */}
      {!user && <p className="mt-1 text-xs text-kon-ink/60">ログインが必要です</p>}

      <Link
        to="/pricing"
        className="mt-3 block text-sm text-kon-ink/70 underline decoration-kon-leaf underline-offset-2 hover:text-kon-ink"
      >
        プランの詳細を見る
      </Link>
    </div>
  )
}
```

- [ ] **Step 6: テストが通ることを確認する**

Run: `cd frontend && npx vitest run src/features/premium/PremiumLock.test.tsx`
Expected: PASS（7件）

- [ ] **Step 7: 巻き込み事故がないことを確認する**

`PremiumLock` を描画する既存テストが落ちていないか確かめる。

Run: `cd frontend && npx vitest run src/features/menu/WeeklyPage.test.tsx src/features/menu/SavedWeeklyPage.test.tsx`
Expected: PASS

- [ ] **Step 8: コミット**

```bash
git add frontend/src/features/billing/api.ts frontend/src/test/handlers.ts \
        frontend/src/features/premium/PremiumLock.tsx \
        frontend/src/features/premium/PremiumLock.test.tsx
git commit -m "fix: PremiumLock に加入導線を戻す（spec.md 2.11 の要求を満たす）"
```

---

### Task 3: 料金ページ `/pricing`

未ログインにも見える公開ページ。比較表と料金を出し、加入状態に応じた CTA を出す。

**Files:**
- Create: `frontend/src/features/pricing/PricingPage.tsx`
- Create: `frontend/src/features/pricing/PricingPage.test.tsx`
- Modify: `frontend/src/app/App.tsx`（import と Route）
- Create: `frontend/src/app/App.pricing.test.tsx`

**Interfaces:**
- Consumes: `getPlan()` / `PlanInfo`（Task 2）、`useCurrentUser()`（既存。`{ user, isLoading }` を返し `user.plan` は `'free' | 'premium'`）
- Produces: `export function PricingPage(): JSX.Element`、ルート `/pricing`（`RequireAuth` で包まない）

- [ ] **Step 1: 失敗するテストを書く**

`frontend/src/features/pricing/PricingPage.test.tsx` を新規作成する。

```tsx
import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { PricingPage } from './PricingPage'

function respondMe(plan: 'free' | 'premium') {
  server.use(
    http.get('/api/v1/auth/me', () =>
      HttpResponse.json({
        user: {
          id: '018f0000-0000-7000-8000-000000000002',
          email: 'user@example.com',
          displayName: 'ユーザー',
          plan,
        },
      }),
    ),
  )
}

describe('料金プラン画面', () => {
  it('未ログインでも料金と比較表が見える', async () => {
    renderWithProviders(<PricingPage />)

    expect(await screen.findByText(/月額300円/)).toBeInTheDocument()
    expect(screen.getByText('1週間の献立を組み立てる')).toBeInTheDocument()
  })

  it('未ログインには加入画面への CTA を出す', async () => {
    renderWithProviders(<PricingPage />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムを試す' }),
    ).toHaveAttribute('href', '/checkout')
  })

  // premium が /checkout を踏むと already-subscribed（409）で行き止まりになる。
  // AccountPage が同じ配慮をしているのに揃える。
  it('premium にはプラン管理への CTA を出し、加入画面へは送らない', async () => {
    respondMe('premium')
    renderWithProviders(<PricingPage />)

    expect(
      await screen.findByRole('link', { name: 'プランを管理する' }),
    ).toHaveAttribute('href', '/account')
    expect(
      screen.queryByRole('link', { name: 'プレミアムを試す' }),
    ).not.toBeInTheDocument()
  })

  it('free には加入画面への CTA を出す', async () => {
    respondMe('free')
    renderWithProviders(<PricingPage />)

    expect(
      await screen.findByRole('link', { name: 'プレミアムを試す' }),
    ).toHaveAttribute('href', '/checkout')
  })

  // 先に free 向けの CTA を描いてから差し替えると、premium の利用者に
  // 一瞬「プレミアムを試す」が見える。比較表は状態に依らないので先に出す。
  it('加入状態の判定中は CTA を出さないが、比較表は出す', () => {
    renderWithProviders(<PricingPage />)

    expect(
      screen.queryByRole('link', { name: 'プレミアムを試す' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'プランを管理する' }),
    ).not.toBeInTheDocument()
    expect(screen.getByText('1週間の献立を組み立てる')).toBeInTheDocument()
  })

  // 上限の数値をフロントが持つと二重管理になる（spec.md「上限の数値を返さない理由」）。
  it('保存件数の数値を表示しない', async () => {
    respondMe('free')
    renderWithProviders(<PricingPage />)

    await screen.findByRole('link', { name: 'プレミアムを試す' })
    expect(screen.queryByText(/50/)).not.toBeInTheDocument()
  })

  it('特定商取引法に基づく表記へのリンクを添える', async () => {
    renderWithProviders(<PricingPage />)

    expect(
      await screen.findByRole('link', { name: /特定商取引法/ }),
    ).toHaveAttribute('href', '/legal/tokushoho')
  })
})
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd frontend && npx vitest run src/features/pricing/PricingPage.test.tsx`
Expected: FAIL。`Failed to resolve import "./PricingPage"`

- [ ] **Step 3: `PricingPage` を実装する**

`frontend/src/features/pricing/PricingPage.tsx` を新規作成する。

```tsx
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'

import { useCurrentUser } from '../auth/useCurrentUser'
import { getPlan } from '../billing/api'

// features は比較表の中身。spec.md 2.11 の線引きの表が正。
//
// **保存件数（50件）は書かない。** 上限の数値をフロントが持つと二重管理に
// なるため（spec.md「上限の数値を返さない理由」）、機能の有無だけを示す。
const features = [
  { label: '献立を1食提案・レシピ・履歴・お気に入り', free: true },
  { label: '冷蔵庫の食材から探す', free: true },
  { label: '買い物リストの作成', free: true },
  { label: '1週間の献立を組み立てる', free: false },
  { label: '1日だけ引き直す', free: false },
  { label: '週間献立の保存・呼び出し', free: false },
  { label: '買い物リストのチェックを残す', free: false },
] as const

// PricingPage は料金と機能の比較を出す公開ページ。
//
// 未ログインでも見える（RequireAuth で包まない）。加入を検討する前に見る
// 画面であり、料金を知るのにログインを要求するのは筋が通らないため。
export function PricingPage() {
  const { user, isLoading } = useCurrentUser()
  const plan = useQuery({ queryKey: ['billing', 'plan'], queryFn: getPlan })

  return (
    <section className="mx-auto max-w-xl space-y-6">
      <h1 className="text-2xl font-bold text-kon-ink">料金プラン</h1>

      <p className="text-kon-ink/75">
        献立を1食ずつ探すのは無料のまま使えます。1週間分をまとめて計画したいときに
        プレミアムをどうぞ。
      </p>

      <table className="w-full border-collapse text-sm">
        <caption className="sr-only">無料プランとプレミアムプランの比較</caption>
        <thead>
          <tr className="border-b border-kon-leaf-soft">
            <th scope="col" className="py-2 text-left font-medium text-kon-ink">
              できること
            </th>
            <th scope="col" className="w-20 py-2 text-center font-medium text-kon-ink">
              無料
            </th>
            <th scope="col" className="w-28 py-2 text-center font-medium text-kon-ink">
              プレミアム
              {/* 料金が引けないときはこの行だけ落とす。表そのものは出す。 */}
              {plan.data && (
                <span className="mt-0.5 block text-xs font-normal text-kon-ink/70">
                  月額{plan.data.price}円
                </span>
              )}
            </th>
          </tr>
        </thead>
        <tbody>
          {features.map((f) => (
            <tr key={f.label} className="border-b border-kon-leaf-soft/60">
              <td className="py-2 text-kon-ink/85">{f.label}</td>
              {/* 記号だけだと読み上げで意味が伝わらないため、文字を添えて隠す。 */}
              <td className="py-2 text-center text-kon-ink">
                {f.free ? '○' : '—'}
                <span className="sr-only">{f.free ? '使える' : '使えない'}</span>
              </td>
              <td className="py-2 text-center text-kon-ink">
                ○<span className="sr-only">使える</span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {plan.data && plan.data.trialDays > 0 && (
        <p className="text-sm text-kon-ink/75">
          はじめての方は{plan.data.trialDays}日間無料でお試しいただけます。
        </p>
      )}

      {/* 判定が付くまで CTA を出さない。先に free 向けを描いて差し替えると、
          premium の利用者に一瞬「プレミアムを試す」が見える
          （AuthMenu が同じ理由で判定前の描画を避けている）。 */}
      {!isLoading &&
        (user?.plan === 'premium' ? (
          <Link
            to="/account"
            className="inline-block rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95"
          >
            プランを管理する
          </Link>
        ) : (
          <Link
            to="/checkout"
            className="inline-block rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95"
          >
            プレミアムを試す
          </Link>
        ))}

      <p className="text-sm text-kon-ink/70">
        価格・支払方法・解約の条件は{' '}
        <Link
          to="/legal/tokushoho"
          className="underline decoration-kon-leaf underline-offset-2 hover:text-kon-ink"
        >
          特定商取引法に基づく表記
        </Link>{' '}
        をご確認ください。
      </p>
    </section>
  )
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd frontend && npx vitest run src/features/pricing/PricingPage.test.tsx`
Expected: PASS（7件）

- [ ] **Step 5: ルートの失敗するテストを書く**

`frontend/src/app/App.pricing.test.tsx` を新規作成する。`App.legal.test.tsx` の流儀に倣い、既定（未ログイン）のままリダイレクトされないことを確かめる。

```tsx
import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../test/render'
import { App } from './App'

// 料金ページは未ログインでも見えて欲しい（RequireAuth で包まない）。
// 加入を検討する前に見る画面であり、ログインを要求すると意味を成さない。
describe('料金ページ（未ログイン）', () => {
  it('/pricing で料金プランを表示し、ログイン画面へ送らない', async () => {
    renderWithProviders(<App />, { route: '/pricing' })

    expect(
      await screen.findByRole('heading', { level: 1, name: '料金プラン' }),
    ).toBeVisible()
    expect(
      screen.queryByRole('heading', { level: 1, name: 'ログイン' }),
    ).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 6: テストが失敗することを確認する**

Run: `cd frontend && npx vitest run src/app/App.pricing.test.tsx`
Expected: FAIL。`/pricing` は未定義パスなので `NotFoundPage` が出て、見出しが見つからない

- [ ] **Step 7: ルートを足す**

`frontend/src/app/App.tsx` の import は**パスの昇順**に並んでいる。`../features/menu/WeeklyPage` の**後**（import 群の最後）に足す。

```tsx
import { PricingPage } from '../features/pricing/PricingPage'
```

`<Route path="/login" element={<LoginPage />} />` の**直前**に足す。

```tsx
            {/* 料金の提示は未ログインにも見せる。加入を検討する前に見る画面で、
                ログインを要求すると意味を成さない。 */}
            <Route path="/pricing" element={<PricingPage />} />
```

- [ ] **Step 8: テストが通ることを確認する**

Run: `cd frontend && npx vitest run src/app/App.pricing.test.tsx`
Expected: PASS

- [ ] **Step 9: コミット**

```bash
git add frontend/src/features/pricing/ frontend/src/app/App.tsx \
        frontend/src/app/App.pricing.test.tsx
git commit -m "feat: 料金プランページ /pricing を足す（未ログインでも見える）"
```

---

### Task 4: 常設導線（フッター・ホーム）と仕様の方針更新

`/pricing` へ辿り着ける入口を2つ置き、`spec.md` の「常設の押し売りにはせず」方針を転換する。

**Files:**
- Modify: `frontend/src/components/Footer.tsx:5-9`（`footerLinks`）
- Modify: `frontend/src/components/Footer.test.tsx`（末尾に追加）
- Modify: `frontend/src/features/home/HomePage.tsx:139`（`</ul>` の後）
- Modify: `frontend/src/features/home/HomePage.test.tsx`（末尾に追加）
- Modify: `spec.md:345-352`

**Interfaces:**
- Consumes: ルート `/pricing`（Task 3）
- Produces: なし（他タスクが依存しない終端）

- [ ] **Step 1: Footer の失敗するテストを書く**

`frontend/src/components/Footer.test.tsx` の `describe` 内、末尾に足す。

```tsx
  it('料金プランへのリンクを持つ', () => {
    renderWithProviders(<Footer />)

    expect(screen.getByRole('link', { name: '料金プラン' })).toHaveAttribute(
      'href',
      '/pricing',
    )
  })
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd frontend && npx vitest run src/components/Footer.test.tsx`
Expected: FAIL。`Unable to find role="link" and name "料金プラン"`

- [ ] **Step 3: Footer にリンクを足す**

`frontend/src/components/Footer.tsx:3-9` のコメントと配列を置き換える。

```tsx
// footerLinks は常設の導線。法務3ページは表示義務があり、料金プランは
// 加入前に料金を確かめる先。どちらもどの画面からでも辿れる必要がある。
const footerLinks = [
  { to: '/pricing', label: '料金プラン' },
  { to: '/legal/tokushoho', label: '特定商取引法に基づく表記' },
  { to: '/legal/terms', label: '利用規約' },
  { to: '/legal/privacy', label: 'プライバシーポリシー' },
] as const
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd frontend && npx vitest run src/components/Footer.test.tsx`
Expected: PASS（4件）

- [ ] **Step 5: HomePage の失敗するテストを書く**

`frontend/src/features/home/HomePage.test.tsx` の `describe` 内、末尾に足す。既存の `loggedIn()` は `plan` を返さないため、プランを指定する版を別に用意する。

```tsx
  it('free には料金プランへの案内を出す', async () => {
    server.use(
      http.get('/api/v1/auth/me', () =>
        HttpResponse.json({
          user: {
            id: '018f0000-0000-7000-8000-000000000009',
            email: 'user@example.com',
            displayName: 'キムさん',
            plan: 'free',
          },
        }),
      ),
    )
    renderWithProviders(<HomePage />)

    expect(
      await screen.findByRole('link', { name: 'プランを見る' }),
    ).toHaveAttribute('href', '/pricing')
  })

  it('未ログインにも料金プランへの案内を出す', async () => {
    renderWithProviders(<HomePage />)

    expect(
      await screen.findByRole('link', { name: 'プランを見る' }),
    ).toHaveAttribute('href', '/pricing')
  })

  // 加入済みの利用者に勧誘を出さない。
  it('premium には料金プランへの案内を出さない', async () => {
    server.use(
      http.get('/api/v1/auth/me', () =>
        HttpResponse.json({
          user: {
            id: '018f0000-0000-7000-8000-000000000009',
            email: 'user@example.com',
            displayName: 'キムさん',
            plan: 'premium',
          },
        }),
      ),
    )
    renderWithProviders(<HomePage />)

    // 先に判定が付いたことを確かめてから「無いこと」を検査する。
    // 部分一致にするのは既存テスト（:49）と同じ理由で、文言の細部に縛られないため。
    expect(await screen.findByText(/キムさん/)).toBeVisible()
    expect(
      screen.queryByRole('link', { name: 'プランを見る' }),
    ).not.toBeInTheDocument()
  })

  // 判定前に出すと、premium の利用者に一瞬勧誘が見える。
  it('判定が付くまでは案内を出さない', () => {
    renderWithProviders(<HomePage />)

    expect(
      screen.queryByRole('link', { name: 'プランを見る' }),
    ).not.toBeInTheDocument()
  })
```

- [ ] **Step 6: テストが失敗することを確認する**

Run: `cd frontend && npx vitest run src/features/home/HomePage.test.tsx`
Expected: FAIL。「free には料金プランへの案内を出す」等が `Unable to find role="link" and name "プランを見る"` で落ちる

- [ ] **Step 7: HomePage に紹介カードを足す**

`frontend/src/features/home/HomePage.tsx:139` の `</ul>` の**後**、`</section>` の**前**に足す。

```tsx
      {/* プレミアムの案内。判定が付くまでは出さない（決めつけると premium の
          利用者に一瞬勧誘が見える）。加入済みには出さない。
          押し売りにしないため、ここでは加入画面ではなく料金ページへ送る。 */}
      {!isLoading && user?.plan !== 'premium' && (
        <div className="rounded-2xl border border-kon-leaf-soft bg-white p-4 sm:flex sm:items-center sm:gap-4">
          <p className="min-w-0 flex-1 text-sm text-kon-ink/85">
            1週間分の献立をまとめて組み立てたり、買い物リストのチェックを残したりするには
            プレミアムプランをどうぞ。
          </p>
          <Link
            to="/pricing"
            className="mt-3 inline-block whitespace-nowrap rounded-full bg-kon-leaf px-4 py-1.5 text-sm font-medium text-white transition-colors hover:brightness-95 sm:mt-0"
          >
            プランを見る
          </Link>
        </div>
      )}
```

- [ ] **Step 8: テストが通ることを確認する**

Run: `cd frontend && npx vitest run src/features/home/HomePage.test.tsx`
Expected: PASS

- [ ] **Step 9: フロント全体を通す**

Run: `make test-frontend`
Expected: 型チェック・Lint・全テストが PASS

- [ ] **Step 10: コミット**

```bash
git add frontend/src/components/Footer.tsx frontend/src/components/Footer.test.tsx \
        frontend/src/features/home/HomePage.tsx frontend/src/features/home/HomePage.test.tsx
git commit -m "feat: フッターとホームに料金プランへの常設導線を足す"
```

- [ ] **Step 11: `spec.md` の方針を更新する**

`spec.md:345-354` の引用ブロックを丸ごと置き換える。

```markdown
> **アップグレード導線は文脈内を主とし、料金ページを1枚だけ常設する。**
> 決済（Stripe Checkout）と申込み確認画面 `/checkout` が実装されたため、
> 以前のように導線を一切出さない理由は無くなった。free の利用者が premium 限定
> 機能に触れた文脈での案内を主とする（例: 買い物リスト画面で永続化に触れたときの
> 案内バナー → `/checkout`、および週間画面・保存一覧を開いたときのロック付き
> プレビュー → `/checkout`。設計 `2026-07-24-weekly-premium-retier-design.md`
> §3.2・§3.6）。
>
> **加えて料金ページ `/pricing` を常設する**（フッターの1リンクと、ホームの
> 案内カード1枚。設計 `2026-07-27-upgrade-entrypoints-design.md`）。当初は
> 「常設の押し売りにはせず、その文脈を開いたときにだけ出す」としていたが、
> 同じ再編で週間献立という看板機能を free から取り上げた（上の「これは意図的な
> 『取り上げ』である」）。取り上げた以上、何がいくらで使えるのかを知る場が
> 存在しないこと自体が不誠実になる。特に未ログインの利用者には、特商法ページ
> 以外に料金を知る手段が無かった。
>
> 一方で、各画面に「プレミアムにする」ボタンを散らすような押し付けはしない。
> 常設するのは料金ページ1枚と、そこへの控えめな入口だけに留める。
> ログイン中の利用者情報を出す `AuthMenu` には、premium であることを示す
> バッジを1つ出すに留める（こちらは従来通り）。
>
> 手動テストの位置づけは5.7に記す。
```

- [ ] **Step 12: コミット**

```bash
git add spec.md
git commit -m "docs: 加入導線の方針を更新し料金ページの常設を仕様に反映"
```

---

### Task 5: E2E で導線を通しで確かめる

単体テストは各部品を見るが、「ロックされた画面から実際に加入画面へ着く」ことは通しでしか確かめられない。今回直した不具合がまさにその繋ぎ目にあったため、ここを押さえる。

**Files:**
- Modify: `frontend/e2e/premium.spec.ts`（末尾に追加）

**Interfaces:**
- Consumes: Task 2〜4 の全て
- Produces: なし（終端）

- [ ] **Step 1: E2E を書く**

`frontend/e2e/premium.spec.ts` の末尾に足す。既存の import 行に `testPassword` を加える（現状は `signUp, uniqueEmail` のみ）。

```ts
// free が週間画面でロックされたその場から加入画面へ進めること。
// この繋ぎ目が切れていたのが今回の不具合だったため、通しで押さえる。
test('free は週間献立でロックされ、その場から加入画面へ進める', async ({ page }) => {
  await signUp(page, uniqueEmail('lock'))

  await page.goto('/weekly')

  await page
    .getByRole('link', { name: 'プレミアムにアップグレード' })
    .click()

  await expect(page).toHaveURL(/\/checkout$/)
  await expect(
    page.getByRole('heading', { name: 'お申込み内容の確認' }),
  ).toBeVisible()
})

// 未ログインが料金ページから加入へ向かうと、ログインを挟んで加入画面に戻る。
// 戻り先は RequireAuth が state.from に残し、LoginPage がそこへ navigate する。
test('未ログインは料金ページからログインを経て加入画面に着く', async ({ page }) => {
  await page.goto('/pricing')

  await page.getByRole('link', { name: 'プレミアムを試す' }).click()

  await expect(page).toHaveURL(/\/login$/)

  await page.getByRole('button', { name: '新規登録はこちら' }).click()
  await page.getByLabel('メールアドレス').fill(uniqueEmail('pricing'))
  await page.getByLabel('パスワード').fill(testPassword)
  await page.getByRole('button', { name: '登録する' }).click()

  await expect(page).toHaveURL(/\/checkout$/)
  await expect(
    page.getByRole('heading', { name: 'お申込み内容の確認' }),
  ).toBeVisible()
})
```

import 行を差し替える。

```ts
import { signUp, testPassword, uniqueEmail } from './helpers'
```

- [ ] **Step 2: アプリを起動する**

Run: `make up && make seed`
Expected: コンテナが起動し、シードが入る

- [ ] **Step 3: E2E を実行する**

Run: `cd frontend && npx playwright test e2e/premium.spec.ts`
Expected: PASS（既存2件＋新規2件）

- [ ] **Step 4: コミット**

```bash
git add frontend/e2e/premium.spec.ts
git commit -m "test: 加入導線の通し（ロック→加入画面、料金ページ→ログイン→加入画面）"
```

- [ ] **Step 5: 全テストと Lint を通す**

Run: `make test && make lint`
Expected: すべて PASS、指摘なし

- [ ] **Step 6: PR を出す**

`/pre-pr-review` を先に走らせてから PR を作る。`main` へのマージは承認待ち（feature→main は自分でマージしない）。

```bash
git push -u origin feat/upgrade-entrypoints
```

PR 本文には次を含める。
- `PremiumLock` の修正は **`spec.md:297-298` が既に要求していた導線の実装**（仕様違反の解消）であること
- `/pricing` と常設導線は **`spec.md` の方針転換**であり、同 PR で `spec.md` を更新していること
- `GET /billing/plan` は**認証不要**で、個人に依る値を含まないこと
