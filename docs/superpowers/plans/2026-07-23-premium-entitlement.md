# プレミアムプラン基盤（エンタイトルメント）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 決済を実装せずに「誰がプレミアムか」を保持・判定し、週間献立の保存上限をプラン由来にする。将来の決済 Webhook が同じサービス層に載る形にする。

**Architecture:** `subscriptions` テーブルに加入状態を持ち、`EntitlementService` が参照のたびに有効性を計算する（バッチで書き換えない）。上限値は `domain.Entitlement` のメソッドで導出し、ゼロ値が free に落ちるようにする。付与・取消は `SubscriptionService` に集約し、CLI `cmd/grant` がそれを呼ぶ。

**Tech Stack:** Go 1.26 / echo v4 / pgx v5 / golang-migrate v4 / PostgreSQL 17 / testcontainers / React 19 / TypeScript / Vitest / Playwright

**設計の正:** `docs/superpowers/specs/2026-07-23-premium-entitlement-design.md`

## Global Constraints

- **ブランチ構成**: `main` → `feature/premium` → 作業ブランチ。作業ブランチのPRの base は必ず `feature/premium`。`main` 宛てのPRは出さない。
- **マイグレーション番号は `000010`**。`main` は `000008` までで、`000009_add_menu_role` は未マージの `feature/menu-role` にのみ存在する。**`feature/premium` → `main` のPRを出す前に `000009` が `main` に入っていることを確認する。** 先に `000010` を本番へ適用すると、後から来る `000009` は golang-migrate に永久に無視される。
- **プランごとの上限**: free = 10件（現行維持）、premium = 50件。既存利用者の体験を削らない。
- **上限値はコードに持つ**。DBにもAPIレスポンスにも置かない。
- **TDD**: 各タスクは 🔴失敗するテスト → 🟢最小実装 の順。1タスク = 1PR（🔴+🟢の対）。
- **ローカルの `go test` に `-race` は付けない**（cgo=gcc が必要。CI の Linux では有効）。
- **`api/openapi.yaml` が API 仕様の正**。変更したら `make gen-api` で TS 型を再生成してコミットする（CIが差分で落とす）。
- **エラーはインターフェースの持ち主である `service` 側で定義**し、`repository` がそれを返す（`ports.go` 冒頭の方針）。
- **PR を出す前に `/code-review` を必ず走らせる**。指摘はその場で直す。
- **コメントは既存コードに合わせて「なぜそうしたか」を書く**。何をしているかの逐語訳は書かない。

## File Structure

| ファイル | 責務 |
| --- | --- |
| `backend/internal/domain/plan.go` | 新規。`Plan` 値オブジェクト |
| `backend/internal/domain/entitlement.go` | 新規。`Entitlement` と上限の導出 |
| `backend/internal/domain/subscription.go` | 新規。`Subscription` エンティティと `SubscriptionStatus` |
| `backend/db/migrations/000010_create_subscriptions.{up,down}.sql` | 新規。テーブルと部分UNIQUE索引 |
| `backend/internal/repository/subscription.go` | 新規。`Find` / `Upsert` |
| `backend/internal/repository/user.go` | 変更。`FindByEmail` を追加（CLI が email→UserID を解決するため） |
| `backend/internal/service/ports.go` | 変更。`SubscriptionStore` / `Entitlements` / `ErrSubscriptionNotFound` を追加。`UserRepository` に `FindByEmail` を追加 |
| `backend/internal/service/entitlement.go` | 新規。`EntitlementService.For` |
| `backend/internal/service/subscription.go` | 新規。`SubscriptionService.Grant` / `Revoke` |
| `backend/internal/service/saved_weekly.go` | 変更。上限をプラン由来にし、エラー型を差し替え |
| `backend/internal/handler/problem.go` | 変更。上限エラーの title を件数非依存にする |
| `backend/internal/handler/auth.go` | 変更。`userDTO` に `plan` を追加 |
| `backend/cmd/grant/main.go` | 新規。付与・取消のCLI |
| `backend/cmd/server/main.go` | 変更。配線 |
| `api/openapi.yaml` | 変更。`User` に `plan` を追加 |
| `frontend/src/features/auth/AuthMenu.tsx` | 変更。premium バッジ |
| `frontend/e2e/premium.spec.ts` | 新規。E2E |
| `spec.md` | 変更。2.11 / 4.2 / 15章 |

---

### Task 1: `domain.Plan`

**Files:**
- Create: `backend/internal/domain/plan.go`
- Test: `backend/internal/domain/plan_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `domain.Plan`（`string` の別名型）、`domain.PlanFree` / `domain.PlanPremium`、`domain.ParsePlan(string) (Plan, error)`、`(Plan).Valid() bool`、`(Plan).String() string`、`domain.ErrInvalidPlan`

`domain/genre.go` の書き方をそのまま踏襲する。表記ゆれを許すとDBの値と乖離するため完全一致のみ受け付ける。

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/domain/plan_test.go`:

```go
package domain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParsePlan_既知のプランを受け付ける(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want domain.Plan
	}{
		{"free", domain.PlanFree},
		{"premium", domain.PlanPremium},
	} {
		got, err := domain.ParsePlan(tt.in)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}
}

func TestParsePlan_未知の値は拒否する(t *testing.T) {
	// 表記ゆれを許すとDBの値と乖離するため、完全一致のみ受け付ける。
	for _, in := range []string{"", "Free", "PREMIUM", " premium", "pro"} {
		_, err := domain.ParsePlan(in)
		require.ErrorIs(t, err, domain.ErrInvalidPlan, "%q は拒否されるべき", in)
	}
}

func TestPlan_StringはDBに入る値を返す(t *testing.T) {
	require.Equal(t, "free", domain.PlanFree.String())
	require.Equal(t, "premium", domain.PlanPremium.String())
}

func TestPlan_Valid(t *testing.T) {
	require.True(t, domain.PlanFree.Valid())
	require.True(t, domain.PlanPremium.Valid())
	require.False(t, domain.Plan("").Valid())
	require.False(t, domain.Plan("pro").Valid())
}

func TestErrInvalidPlan_sentinelとして使える(t *testing.T) {
	_, err := domain.ParsePlan("pro")
	require.True(t, errors.Is(err, domain.ErrInvalidPlan))
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/domain/ -run TestParsePlan -v`
Expected: FAIL — `undefined: domain.ParsePlan`（コンパイルエラー）

- [ ] **Step 3: 最小の実装を書く**

`backend/internal/domain/plan.go`:

```go
package domain

import "errors"

// ErrInvalidPlan は文字列が既知のプランに一致しないことを表す。
var ErrInvalidPlan = errors.New("不正なプランです")

// Plan は利用者の契約プラン。
type Plan string

// 定義済みのプラン。DBの subscriptions.plan に格納される値と一致する。
const (
	PlanFree    Plan = "free"
	PlanPremium Plan = "premium"
)

// ParsePlan は文字列を Plan に変換する。
// 表記ゆれを許容するとDBの値と乖離するため、完全一致のみを受け付ける。
func ParsePlan(s string) (Plan, error) {
	p := Plan(s)
	if !p.Valid() {
		return "", ErrInvalidPlan
	}
	return p, nil
}

// Valid は定義済みのプランかどうかを返す。
func (p Plan) Valid() bool {
	switch p {
	case PlanFree, PlanPremium:
		return true
	default:
		return false
	}
}

// String は DB およびAPIで用いる文字列表現を返す。
func (p Plan) String() string {
	return string(p)
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd backend && go test ./internal/domain/ -run 'TestParsePlan|TestPlan|TestErrInvalidPlan' -v`
Expected: PASS（5テスト）

- [ ] **Step 5: コミット**

```bash
git add backend/internal/domain/plan.go backend/internal/domain/plan_test.go
git commit -m "feat: プランの値オブジェクトを追加"
```

---

### Task 2: `domain.Entitlement`

**Files:**
- Create: `backend/internal/domain/entitlement.go`
- Test: `backend/internal/domain/entitlement_test.go`

**Interfaces:**
- Consumes: `domain.Plan` / `domain.PlanFree` / `domain.PlanPremium`（Task 1）
- Produces: `domain.Entitlement`（非公開フィールド `plan Plan` のみを持つ構造体）、`domain.NewEntitlement(Plan) Entitlement`、`(Entitlement).Plan() Plan`、`(Entitlement).SavedWeeklyMenuLimit() int`

**このタスクの要点はゼロ値の扱いにある。** 上限を `SavedWeeklyMenuLimit int` というフィールドで持つと、
取得し忘れた `Entitlement{}` のゼロ値が「上限0件」を意味し、既存利用者が1件も保存できなくなる。
`plan` を非公開にしてメソッドで導出すれば、ゼロ値の `plan` は空文字となり free と同じ扱いに落ちる。

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/domain/entitlement_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestEntitlement_プランごとの保存上限(t *testing.T) {
	free := domain.NewEntitlement(domain.PlanFree)
	require.Equal(t, 10, free.SavedWeeklyMenuLimit(),
		"free は現行の10件を据え置く。既存利用者の体験を削らない")

	premium := domain.NewEntitlement(domain.PlanPremium)
	require.Equal(t, 50, premium.SavedWeeklyMenuLimit())
}

// ゼロ値が free に落ちることは、この設計の安全装置そのもの。
// 上限をフィールドで持つと 0 件になり、既存利用者が1件も保存できなくなる。
func TestEntitlement_ゼロ値はfreeとして振る舞う(t *testing.T) {
	var zero domain.Entitlement

	require.Equal(t, domain.PlanFree, zero.Plan())
	require.Equal(t, 10, zero.SavedWeeklyMenuLimit(),
		"ゼロ値の上限が0だと既存利用者が保存できなくなる")
}

func TestEntitlement_未知のプランもfreeに落ちる(t *testing.T) {
	// DBに想定外の値が入っていた場合でも、締め出すのではなく free として扱う。
	e := domain.NewEntitlement(domain.Plan("pro"))

	require.Equal(t, domain.PlanFree, e.Plan())
	require.Equal(t, 10, e.SavedWeeklyMenuLimit())
}

func TestEntitlement_Planを返す(t *testing.T) {
	require.Equal(t, domain.PlanPremium,
		domain.NewEntitlement(domain.PlanPremium).Plan())
	require.Equal(t, domain.PlanFree,
		domain.NewEntitlement(domain.PlanFree).Plan())
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/domain/ -run TestEntitlement -v`
Expected: FAIL — `undefined: domain.NewEntitlement`（コンパイルエラー）

- [ ] **Step 3: 最小の実装を書く**

`backend/internal/domain/entitlement.go`:

```go
package domain

// 週間献立の保存上限。プランごとの値（spec.md 2.8 / 2.11）。
//
// 上限値は仕様であってデータなので、DBではなくコードに置く。
// DBに置くと変更のたびにマイグレーションが要り、テストもDBの状態に依存する。
const (
	freeSavedWeeklyMenuLimit    = 10
	premiumSavedWeeklyMenuLimit = 50
)

// Entitlement は「今この利用者が何をどれだけ使えるか」を表す。
//
// **上限をフィールドではなくメソッドで導出するのがこの型の要点。**
// 仮に SavedWeeklyMenuLimit を int のフィールドで持たせると、取得し忘れた
// Entitlement{} のゼロ値が「上限0件」を意味し、既存利用者が1件も保存できなくなる。
// plan を非公開にしてメソッドで導出すれば、ゼロ値の plan は空文字となり
// free と同じ扱いに落ちる。安全側の既定を型で保証する。
type Entitlement struct {
	plan Plan
}

// NewEntitlement はプランから Entitlement を組み立てる。
func NewEntitlement(p Plan) Entitlement {
	return Entitlement{plan: p}
}

// Plan は契約プランを返す。
// premium 以外は全て free として扱う（ゼロ値・DBの想定外の値を含む）。
func (e Entitlement) Plan() Plan {
	if e.plan == PlanPremium {
		return PlanPremium
	}
	return PlanFree
}

// SavedWeeklyMenuLimit は保存できる週間献立の件数を返す。
func (e Entitlement) SavedWeeklyMenuLimit() int {
	if e.Plan() == PlanPremium {
		return premiumSavedWeeklyMenuLimit
	}
	return freeSavedWeeklyMenuLimit
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd backend && go test ./internal/domain/ -run TestEntitlement -v`
Expected: PASS（4テスト）

- [ ] **Step 5: コミット**

```bash
git add backend/internal/domain/entitlement.go backend/internal/domain/entitlement_test.go
git commit -m "feat: エンタイトルメントと上限の導出を追加"
```

---

### Task 3: `domain.Subscription`

**Files:**
- Create: `backend/internal/domain/subscription.go`
- Test: `backend/internal/domain/subscription_test.go`

**Interfaces:**
- Consumes: `domain.Plan`（Task 1）、`domain.UserID`（既存）
- Produces: `domain.SubscriptionStatus`、`domain.SubscriptionActive` / `SubscriptionPastDue` / `SubscriptionCanceled`、`domain.ParseSubscriptionStatus(string) (SubscriptionStatus, error)`、`(SubscriptionStatus).Valid() bool`、`(SubscriptionStatus).String() string`、`domain.ErrInvalidSubscriptionStatus`、`domain.ProviderManual`（定数 `"manual"`）、構造体 `domain.Subscription{UserID, Plan, Status, CurrentPeriodEnd, CancelAtPeriodEnd, Provider, ProviderSubscriptionID}`、`(Subscription).IsActiveAt(time.Time) bool`

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/domain/subscription_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseSubscriptionStatus_既知の状態を受け付ける(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want domain.SubscriptionStatus
	}{
		{"active", domain.SubscriptionActive},
		{"past_due", domain.SubscriptionPastDue},
		{"canceled", domain.SubscriptionCanceled},
	} {
		got, err := domain.ParseSubscriptionStatus(tt.in)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}
}

func TestParseSubscriptionStatus_未知の値は拒否する(t *testing.T) {
	for _, in := range []string{"", "ACTIVE", "cancelled", "trialing"} {
		_, err := domain.ParseSubscriptionStatus(in)
		require.ErrorIs(t, err, domain.ErrInvalidSubscriptionStatus, "%q は拒否されるべき", in)
	}
}

func TestSubscription_IsActiveAt(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

	t.Run("active かつ期限内なら有効", func(t *testing.T) {
		s := domain.Subscription{
			Status:           domain.SubscriptionActive,
			CurrentPeriodEnd: now.Add(24 * time.Hour),
		}
		require.True(t, s.IsActiveAt(now))
	})

	t.Run("active でも期限切れなら無効", func(t *testing.T) {
		s := domain.Subscription{
			Status:           domain.SubscriptionActive,
			CurrentPeriodEnd: now.Add(-time.Second),
		}
		require.False(t, s.IsActiveAt(now))
	})

	t.Run("期限ちょうどは無効", func(t *testing.T) {
		// 期限は「その時刻まで」であり、到達した時点で切れる。
		s := domain.Subscription{
			Status:           domain.SubscriptionActive,
			CurrentPeriodEnd: now,
		}
		require.False(t, s.IsActiveAt(now))
	})

	t.Run("canceled は期限内でも無効", func(t *testing.T) {
		s := domain.Subscription{
			Status:           domain.SubscriptionCanceled,
			CurrentPeriodEnd: now.Add(24 * time.Hour),
		}
		require.False(t, s.IsActiveAt(now))
	})

	t.Run("past_due は期限内でも無効", func(t *testing.T) {
		s := domain.Subscription{
			Status:           domain.SubscriptionPastDue,
			CurrentPeriodEnd: now.Add(24 * time.Hour),
		}
		require.False(t, s.IsActiveAt(now))
	})

	t.Run("ゼロ値は無効", func(t *testing.T) {
		var s domain.Subscription
		require.False(t, s.IsActiveAt(now))
	})
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/domain/ -run 'TestParseSubscriptionStatus|TestSubscription_IsActiveAt' -v`
Expected: FAIL — `undefined: domain.SubscriptionStatus`（コンパイルエラー）

- [ ] **Step 3: 最小の実装を書く**

`backend/internal/domain/subscription.go`:

```go
package domain

import (
	"errors"
	"time"
)

// ErrInvalidSubscriptionStatus は文字列が既知の加入状態に一致しないことを表す。
var ErrInvalidSubscriptionStatus = errors.New("不正な加入状態です")

// SubscriptionStatus は加入の状態。
type SubscriptionStatus string

// 定義済みの加入状態。DBの subscriptions.status に格納される値と一致する。
const (
	SubscriptionActive   SubscriptionStatus = "active"
	SubscriptionPastDue  SubscriptionStatus = "past_due"
	SubscriptionCanceled SubscriptionStatus = "canceled"
)

// ProviderManual は運用者が手で付与した加入を表す provider の値。
// 決済を導入したら "stripe" などが増える。
const ProviderManual = "manual"

// ParseSubscriptionStatus は文字列を SubscriptionStatus に変換する。
func ParseSubscriptionStatus(s string) (SubscriptionStatus, error) {
	st := SubscriptionStatus(s)
	if !st.Valid() {
		return "", ErrInvalidSubscriptionStatus
	}
	return st, nil
}

// Valid は定義済みの状態かどうかを返す。
func (s SubscriptionStatus) Valid() bool {
	switch s {
	case SubscriptionActive, SubscriptionPastDue, SubscriptionCanceled:
		return true
	default:
		return false
	}
}

// String は DB で用いる文字列表現を返す。
func (s SubscriptionStatus) String() string {
	return string(s)
}

// Subscription は1利用者の加入。1利用者につき高々1件（DBでは user_id が主キー）。
type Subscription struct {
	UserID           UserID
	Plan             Plan
	Status           SubscriptionStatus
	CurrentPeriodEnd time.Time
	// CancelAtPeriodEnd は解約予約中かどうか。利用者都合の解約は即時失効させず、
	// 期末まで使えるようにする（即時失効は返金の争いを招く）。
	// 書き込む経路は決済フェーズで作る。
	CancelAtPeriodEnd bool
	// Provider は加入を作った経路。手動付与は ProviderManual。
	Provider string
	// ProviderSubscriptionID は決済事業者側の加入ID。手動付与では空。
	ProviderSubscriptionID string
}

// IsActiveAt は指定時刻に加入が有効かを返す。
//
// 期限切れをバッチでDBに書き戻すことはせず、参照のたびにここで判定する。
// バッチが停止すると、課金していない利用者がプレミアムのまま残るため。
func (s Subscription) IsActiveAt(t time.Time) bool {
	return s.Status == SubscriptionActive && s.CurrentPeriodEnd.After(t)
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd backend && go test ./internal/domain/ -run 'TestParseSubscriptionStatus|TestSubscription_IsActiveAt' -v`
Expected: PASS（2テスト、サブテスト6件）

- [ ] **Step 5: コミット**

```bash
git add backend/internal/domain/subscription.go backend/internal/domain/subscription_test.go
git commit -m "feat: 加入エンティティと状態を追加"
```

---

### Task 4: マイグレーション `000010_create_subscriptions`

**Files:**
- Create: `backend/db/migrations/000010_create_subscriptions.up.sql`
- Create: `backend/db/migrations/000010_create_subscriptions.down.sql`
- Test: `backend/internal/repository/subscriptions_schema_test.go`

**Interfaces:**
- Consumes: `users` テーブル（既存）
- Produces: テーブル `subscriptions`、部分UNIQUE索引 `subscriptions_provider_subscription_id_key`

制約はDBが守るものなので、既存の `saved_weekly_schema_test.go` と同じく生SQLで直接検証する。
テストヘルパー `newTestPool` / `createUser` は `testhelper_test.go` と既存テストにある。

> **着手前に確認**: `ls backend/db/migrations/` で `000009` が存在しないこと。
> 存在する場合は Global Constraints の指示に従い番号を調整する。

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/repository/subscriptions_schema_test.go`:

```go
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
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/repository/ -run TestSubscriptionsSchema -v`
Expected: FAIL — `relation "subscriptions" does not exist`
（Docker が無い環境では SKIP になる。その場合は Docker を起動してから実行する）

- [ ] **Step 3: マイグレーションを書く**

`backend/db/migrations/000010_create_subscriptions.up.sql`:

```sql
-- 有料プランの加入（設計 4.1）。free はレコードを持たない。
-- 行が無いこと自体が free を意味するため、「無料の加入」を作る責務が生じない。
CREATE TABLE subscriptions (
    -- 1利用者につき高々1件。複数同時加入は仕様にないため主キーで固定する。
    user_id                  uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    plan                     text        NOT NULL,
    -- active / past_due / canceled。CHECK ではなくアプリ側で検証する
    -- （決済事業者ごとに増える値を DDL の変更なしに受けられるようにするため）。
    status                   text        NOT NULL,
    current_period_end       timestamptz NOT NULL,
    -- 解約予約。利用者都合の解約は即時失効させず期末まで使えるようにする。
    -- 書き込む経路は決済フェーズで作るため、今は既定値のまま使われる。
    cancel_at_period_end     boolean     NOT NULL DEFAULT false,
    -- 加入を作った経路。現在は 'manual'（運用者の手動付与）のみ。
    provider                 text        NOT NULL,
    -- 決済事業者側の加入ID。手動付与では NULL。
    provider_subscription_id text,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now()
);

-- 決済事業者の加入IDは高々1行にしか対応しない。将来 Webhook が同じイベントを
-- 二度配送しても、DBが二重適用を弾く。今は張るコストがゼロだが、後から張ると
-- 既存データの重複を掃除する必要が出るため先に入れる。
--
-- 手動付与は provider_subscription_id が NULL になる。NULL 同士は一意制約で
-- 重複と見なされないが、意図を明示するため部分索引にする。
CREATE UNIQUE INDEX subscriptions_provider_subscription_id_key
    ON subscriptions (provider_subscription_id)
    WHERE provider_subscription_id IS NOT NULL;
```

`backend/db/migrations/000010_create_subscriptions.down.sql`:

```sql
-- 索引はテーブルと一緒に落ちる。
DROP TABLE IF EXISTS subscriptions;
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd backend && go test ./internal/repository/ -run TestSubscriptionsSchema -v`
Expected: PASS（6テスト）

- [ ] **Step 5: down が通ることを確認する**

Run: `cd backend && go run ./cmd/migrate up && go run ./cmd/migrate down && go run ./cmd/migrate up`
Expected: エラーなく完了する（`DATABASE_URL` が要る。`make up` 済みなら `make migrate` / `make migrate-down` でもよい）

- [ ] **Step 6: コミット**

```bash
git add backend/db/migrations/000010_create_subscriptions.up.sql \
        backend/db/migrations/000010_create_subscriptions.down.sql \
        backend/internal/repository/subscriptions_schema_test.go
git commit -m "feat: subscriptions テーブルを追加"
```

---

### Task 5: `SubscriptionStore` ポートと `SubscriptionRepository`

**Files:**
- Modify: `backend/internal/service/ports.go`（末尾に追加）
- Create: `backend/internal/repository/subscription.go`
- Modify: `backend/internal/repository/interface_test.go`（適合検査を1行追加）
- Test: `backend/internal/repository/subscription_test.go`

**Interfaces:**
- Consumes: `domain.Subscription` / `domain.ParsePlan` / `domain.ParseSubscriptionStatus`（Task 1・3）
- Produces: `service.ErrSubscriptionNotFound`、`service.SubscriptionStore` インターフェース（`Find(ctx, domain.UserID) (domain.Subscription, error)` / `Upsert(ctx, domain.Subscription) error`）、`repository.NewSubscriptionRepository(*pgxpool.Pool) *SubscriptionRepository`

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/repository/subscription_test.go`:

```go
package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func TestSubscriptionRepository_無ければErrSubscriptionNotFound(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewSubscriptionRepository(pool)

	u := createUser(t, pool, "sub-repo-none@example.com")

	_, err := repo.Find(context.Background(), u.ID)
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound,
		"行が無いのは障害ではなく free を意味する通常の結果")
}

func TestSubscriptionRepository_保存して取り出せる(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSubscriptionRepository(pool)

	u := createUser(t, pool, "sub-repo-roundtrip@example.com")
	end := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	want := domain.Subscription{
		UserID:           u.ID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: end,
		Provider:         domain.ProviderManual,
	}
	require.NoError(t, repo.Upsert(ctx, want))

	got, err := repo.Find(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u.ID, got.UserID)
	require.Equal(t, domain.PlanPremium, got.Plan)
	require.Equal(t, domain.SubscriptionActive, got.Status)
	require.WithinDuration(t, end, got.CurrentPeriodEnd, time.Second)
	require.False(t, got.CancelAtPeriodEnd)
	require.Equal(t, domain.ProviderManual, got.Provider)
	require.Empty(t, got.ProviderSubscriptionID, "手動付与は決済IDを持たない")
}

func TestSubscriptionRepository_Upsertは上書きする(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSubscriptionRepository(pool)

	u := createUser(t, pool, "sub-repo-upsert@example.com")
	end := time.Now().Add(30 * 24 * time.Hour)

	require.NoError(t, repo.Upsert(ctx, domain.Subscription{
		UserID:           u.ID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: end,
		Provider:         domain.ProviderManual,
	}))

	// 取消は行を消さずに状態を遷移させる。解約時期の記録を残すため。
	require.NoError(t, repo.Upsert(ctx, domain.Subscription{
		UserID:           u.ID,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionCanceled,
		CurrentPeriodEnd: end,
		Provider:         domain.ProviderManual,
	}))

	got, err := repo.Find(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionCanceled, got.Status)
}

func TestSubscriptionRepository_決済IDを保持する(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSubscriptionRepository(pool)

	u := createUser(t, pool, "sub-repo-psid@example.com")

	require.NoError(t, repo.Upsert(ctx, domain.Subscription{
		UserID:                 u.ID,
		Plan:                   domain.PlanPremium,
		Status:                 domain.SubscriptionActive,
		CurrentPeriodEnd:       time.Now().Add(time.Hour),
		Provider:               "stripe",
		ProviderSubscriptionID: "sub_XYZ",
	}))

	got, err := repo.Find(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "stripe", got.Provider)
	require.Equal(t, "sub_XYZ", got.ProviderSubscriptionID)
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/repository/ -run TestSubscriptionRepository -v`
Expected: FAIL — `undefined: repository.NewSubscriptionRepository`（コンパイルエラー）

- [ ] **Step 3: ポートを追加する**

`backend/internal/service/ports.go` の末尾に追記:

```go
// ErrSubscriptionNotFound は利用者に加入が無いことを表す。
//
// 障害ではなく「free である」という通常の結果を意味する。
// インターフェースの持ち主である service 側で定義し、repository がこれを返す。
var ErrSubscriptionNotFound = errors.New("加入が見つかりません")

// SubscriptionStore は有料プランの加入の永続化を抽象化する。
// 実装は internal/repository にある。
type SubscriptionStore interface {
	// Find は利用者の加入を返す。該当が無い場合は ErrSubscriptionNotFound を返す。
	Find(ctx context.Context, userID domain.UserID) (domain.Subscription, error)

	// Upsert は加入を保存する。既にあれば上書きする。
	// 取消は行の削除ではなく status の遷移で表すため、Delete は設けない。
	Upsert(ctx context.Context, sub domain.Subscription) error
}
```

- [ ] **Step 4: リポジトリを実装する**

`backend/internal/repository/subscription.go`:

```go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
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

	plan, err := domain.ParsePlan(rawPlan)
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("DBのプランが不正です: %w", err)
	}
	status, err := domain.ParseSubscriptionStatus(rawStatus)
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("DBの加入状態が不正です: %w", err)
	}

	sub.UserID = userID
	sub.Plan = plan
	sub.Status = status
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
```

- [ ] **Step 5: テストが通ることを確認する**

Run: `cd backend && go test ./internal/repository/ -run TestSubscriptionRepository -v`
Expected: PASS（4テスト）

- [ ] **Step 6: インターフェース適合を確認する**

`backend/internal/repository/interface_test.go` に既存の適合検査がある。同じ形で1行追加する:

```go
var _ service.SubscriptionStore = (*repository.SubscriptionRepository)(nil)
```

Run: `cd backend && go build ./... && go test ./internal/repository/ -run TestSubscription -v`
Expected: PASS

- [ ] **Step 7: コミット**

```bash
git add backend/internal/service/ports.go backend/internal/repository/subscription.go \
        backend/internal/repository/subscription_test.go backend/internal/repository/interface_test.go
git commit -m "feat: 加入のリポジトリとポートを追加"
```

---

### Task 6: `UserRepository.FindByEmail`

**Files:**
- Modify: `backend/internal/service/ports.go`（`UserRepository` インターフェースにメソッド追加）
- Modify: `backend/internal/repository/user.go`（実装追加）
- Modify: `backend/internal/service/fake_user_repository_test.go`（fake に追随）
- Test: `backend/internal/repository/user_test.go`（テスト追加）

**Interfaces:**
- Consumes: `domain.Email` / `domain.User`（既存）
- Produces: `UserRepository.FindByEmail(ctx, domain.Email) (domain.User, error)`。該当が無ければ `service.ErrUserNotFound`

CLI がメールアドレスから `UserID` を解決するために要る。既存の `FindPasswordCredential` は
`auth_identities` を内部結合するため **Google 認証のみの利用者を引けず**、この用途には使えない。

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/repository/user_test.go` に追記:

```go
func TestUserRepository_FindByEmail_メールで引ける(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewUserRepository(pool)

	u := createUser(t, pool, "find-by-email@example.com")

	got, err := repo.FindByEmail(ctx, u.Email)
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID)
	require.Equal(t, u.Email, got.Email)
}

func TestUserRepository_FindByEmail_居なければErrUserNotFound(t *testing.T) {
	pool := newTestPool(t)
	repo := repository.NewUserRepository(pool)

	addr, err := domain.NewEmail("nobody-here@example.com")
	require.NoError(t, err)

	_, err = repo.FindByEmail(context.Background(), addr)
	require.ErrorIs(t, err, service.ErrUserNotFound)
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/repository/ -run TestUserRepository_FindByEmail -v`
Expected: FAIL — `repo.FindByEmail undefined`（コンパイルエラー）

- [ ] **Step 3: ポートに追加する**

`backend/internal/service/ports.go` の `UserRepository` インターフェースに追記:

```go
	// FindByEmail はメールでユーザーを取得する。存在しない場合は ErrUserNotFound を返す。
	//
	// FindPasswordCredential は auth_identities を内部結合するため Google 認証のみの
	// 利用者を引けない。認証方式によらずユーザーを特定したい用途（CLI の付与対象の
	// 解決など）ではこちらを使う。
	FindByEmail(ctx context.Context, email domain.Email) (domain.User, error)
```

- [ ] **Step 4: 実装する**

`backend/internal/repository/user.go` の `FindByID` の直後に追記:

```go
// FindByEmail はメールでユーザーを取得する。
// 存在しない場合は service.ErrUserNotFound を返す。
func (r *UserRepository) FindByEmail(ctx context.Context, email domain.Email) (domain.User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, email, display_name FROM users WHERE email = $1`, email.String())

	var rawID, mail, displayName string
	if err := row.Scan(&rawID, &mail, &displayName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, service.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("ユーザーの取得に失敗しました: %w", err)
	}

	userID, err := domain.ParseUserID(rawID)
	if err != nil {
		return domain.User{}, fmt.Errorf("DBのユーザーIDが不正です: %w", err)
	}
	addr, err := domain.NewEmail(mail)
	if err != nil {
		return domain.User{}, fmt.Errorf("DBのメールが不正です: %w", err)
	}

	return domain.User{ID: userID, Email: addr, DisplayName: displayName}, nil
}
```

- [ ] **Step 5: fake を追随させる**

`backend/internal/service/fake_user_repository_test.go` の `FindByID` の直後に足す。
既存 fake は `credentials`（メール→`PasswordCredential`）にユーザーを持つので、
`FindByID` と同じくそこから引く:

```go
func (r *fakeUserRepository) FindByEmail(_ context.Context, email domain.Email) (domain.User, error) {
	cred, ok := r.credentials[email.String()]
	if !ok {
		return domain.User{}, service.ErrUserNotFound
	}
	return cred.User, nil
}
```

Run: `cd backend && go build ./... && go vet ./...`
Expected: エラーなし

- [ ] **Step 6: テストが通ることを確認する**

Run: `cd backend && go test ./internal/repository/ ./internal/service/ -run 'FindByEmail|Test' -count=1`
Expected: PASS（既存テストも含め全て通ること）

- [ ] **Step 7: コミット**

```bash
git add backend/internal/service/ports.go backend/internal/repository/user.go \
        backend/internal/repository/user_test.go backend/internal/service/fake_user_repository_test.go
git commit -m "feat: メールでユーザーを引けるようにする"
```

---

### Task 7: `service.EntitlementService`

**Files:**
- Create: `backend/internal/service/entitlement.go`
- Create: `backend/internal/service/fake_subscription_store_test.go`
- Test: `backend/internal/service/entitlement_test.go`

**Interfaces:**
- Consumes: `service.SubscriptionStore` / `service.ErrSubscriptionNotFound`（Task 5）、`domain.Entitlement`（Task 2）、`domain.Subscription`（Task 3）
- Produces: `service.NewEntitlementService(SubscriptionStore, func() time.Time) *EntitlementService`、`(*EntitlementService).For(ctx, userID string) (domain.Entitlement, error)`

現在時刻を関数で注入するのは、既存の `Randomizer` と同じ理由（外部要因を service の外に出してテスト可能にする）。

- [ ] **Step 1: fake ストアを書く**

`backend/internal/service/fake_subscription_store_test.go`:

```go
package service_test

import (
	"context"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeSubscriptionStore は加入の保存をメモリ上で模す。
type fakeSubscriptionStore struct {
	subs map[string]domain.Subscription
	// findErr が非nilなら Find がそれを返す。障害時の振る舞いを見るため。
	findErr error
}

func newFakeSubscriptionStore() *fakeSubscriptionStore {
	return &fakeSubscriptionStore{subs: map[string]domain.Subscription{}}
}

func (f *fakeSubscriptionStore) Find(
	_ context.Context, userID domain.UserID,
) (domain.Subscription, error) {
	if f.findErr != nil {
		return domain.Subscription{}, f.findErr
	}
	sub, ok := f.subs[userID.String()]
	if !ok {
		return domain.Subscription{}, service.ErrSubscriptionNotFound
	}
	return sub, nil
}

func (f *fakeSubscriptionStore) Upsert(_ context.Context, sub domain.Subscription) error {
	f.subs[sub.UserID.String()] = sub
	return nil
}
```

- [ ] **Step 2: 失敗するテストを書く**

`backend/internal/service/entitlement_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fixedNow はテスト用の固定時刻。期限判定を時計に依存させない。
var fixedNow = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

func newEntitlementSvc(store service.SubscriptionStore) *service.EntitlementService {
	return service.NewEntitlementService(store, func() time.Time { return fixedNow })
}

func TestEntitlementService_未認証はfree(t *testing.T) {
	svc := newEntitlementSvc(newFakeSubscriptionStore())

	// 未認証でも献立検索は使えるため、userID が空でもエラーにしない。
	ent, err := svc.For(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, domain.PlanFree, ent.Plan())
}

func TestEntitlementService_加入が無ければfree(t *testing.T) {
	svc := newEntitlementSvc(newFakeSubscriptionStore())

	u := domain.NewUserID()
	ent, err := svc.For(context.Background(), u.String())
	require.NoError(t, err)
	require.Equal(t, domain.PlanFree, ent.Plan())
}

func TestEntitlementService_有効な加入はpremium(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	require.NoError(t, store.Upsert(context.Background(), domain.Subscription{
		UserID:           u,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: fixedNow.Add(time.Hour),
		Provider:         domain.ProviderManual,
	}))

	ent, err := newEntitlementSvc(store).For(context.Background(), u.String())
	require.NoError(t, err)
	require.Equal(t, domain.PlanPremium, ent.Plan())
	require.Equal(t, 50, ent.SavedWeeklyMenuLimit())
}

func TestEntitlementService_期限切れはfree(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	require.NoError(t, store.Upsert(context.Background(), domain.Subscription{
		UserID:           u,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionActive,
		CurrentPeriodEnd: fixedNow.Add(-time.Second),
		Provider:         domain.ProviderManual,
	}))

	ent, err := newEntitlementSvc(store).For(context.Background(), u.String())
	require.NoError(t, err)
	require.Equal(t, domain.PlanFree, ent.Plan(),
		"期限切れは参照時に free に落ちる。DBは書き換えない")

	// DBを書き換えていないことを確かめる。バッチではなく参照時計算であることの担保。
	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionActive, sub.Status,
		"参照は加入の状態を書き換えてはいけない")
}

func TestEntitlementService_canceledとpast_dueはfree(t *testing.T) {
	for _, status := range []domain.SubscriptionStatus{
		domain.SubscriptionCanceled,
		domain.SubscriptionPastDue,
	} {
		store := newFakeSubscriptionStore()
		u := domain.NewUserID()
		require.NoError(t, store.Upsert(context.Background(), domain.Subscription{
			UserID:           u,
			Plan:             domain.PlanPremium,
			Status:           status,
			CurrentPeriodEnd: fixedNow.Add(time.Hour),
			Provider:         domain.ProviderManual,
		}))

		ent, err := newEntitlementSvc(store).For(context.Background(), u.String())
		require.NoError(t, err)
		require.Equal(t, domain.PlanFree, ent.Plan(), "%s は free であるべき", status)
	}
}

func TestEntitlementService_壊れたIDはfree(t *testing.T) {
	// トークンが壊れている場合でも、エンタイトルメントの判定は締め出しではなく
	// free への縮退で応じる。認証の失敗は認証ミドルウェアの仕事。
	ent, err := newEntitlementSvc(newFakeSubscriptionStore()).
		For(context.Background(), "not-a-uuid")
	require.NoError(t, err)
	require.Equal(t, domain.PlanFree, ent.Plan())
}

func TestEntitlementService_保存の障害はエラーにする(t *testing.T) {
	store := newFakeSubscriptionStore()
	boom := errors.New("DBが落ちている")
	store.findErr = boom

	// 「加入が無い」と「引けなかった」は別。後者を free に丸めると、
	// 障害中に課金済みの利用者が黙って free に落ちる。
	_, err := newEntitlementSvc(store).For(context.Background(), domain.NewUserID().String())
	require.ErrorIs(t, err, boom)
}
```

- [ ] **Step 3: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/service/ -run TestEntitlementService -v`
Expected: FAIL — `undefined: service.NewEntitlementService`（コンパイルエラー）

- [ ] **Step 4: 実装する**

`backend/internal/service/entitlement.go`:

```go
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
// now は現在時刻の取得。期限判定を時計に依存させないため注入する。
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
```

- [ ] **Step 5: テストが通ることを確認する**

Run: `cd backend && go test ./internal/service/ -run TestEntitlementService -v`
Expected: PASS（7テスト）

- [ ] **Step 6: コミット**

```bash
git add backend/internal/service/entitlement.go backend/internal/service/entitlement_test.go \
        backend/internal/service/fake_subscription_store_test.go
git commit -m "feat: エンタイトルメントを参照時に計算する"
```

---

### Task 8: `service.SubscriptionService`

**Files:**
- Create: `backend/internal/service/subscription.go`
- Test: `backend/internal/service/subscription_test.go`

**Interfaces:**
- Consumes: `service.SubscriptionStore`（Task 5）、`domain.Subscription`（Task 3）
- Produces: `service.NewSubscriptionService(SubscriptionStore, func() time.Time) *SubscriptionService`、`(*SubscriptionService).Grant(ctx, domain.UserID, months int) error`、`(*SubscriptionService).Revoke(ctx, domain.UserID) error`、`service.ErrInvalidGrantMonths`

**引数が `UserID` でメールアドレスでないことが要点。** 将来の Webhook が決済事業者から受け取るのは
顧客IDであってメールアドレスではない。メール起点にすると Webhook 側が不自然な逆引きを強いられる。
メール→`UserID` の解決は CLI の責務（Task 9）。

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/service/subscription_test.go`:

```go
package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func newSubscriptionSvc(store service.SubscriptionStore) *service.SubscriptionService {
	return service.NewSubscriptionService(store, func() time.Time { return fixedNow })
}

func TestSubscriptionService_Grant_期限は現在から起算する(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()

	require.NoError(t, newSubscriptionSvc(store).Grant(context.Background(), u, 1))

	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, domain.PlanPremium, sub.Plan)
	require.Equal(t, domain.SubscriptionActive, sub.Status)
	require.Equal(t, domain.ProviderManual, sub.Provider)
	require.Equal(t, fixedNow.AddDate(0, 1, 0), sub.CurrentPeriodEnd)
	require.False(t, sub.CancelAtPeriodEnd)
	require.Empty(t, sub.ProviderSubscriptionID)
}

func TestSubscriptionService_Grant_複数月(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()

	require.NoError(t, newSubscriptionSvc(store).Grant(context.Background(), u, 12))

	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, fixedNow.AddDate(0, 12, 0), sub.CurrentPeriodEnd)
}

func TestSubscriptionService_Grant_0以下の月数は拒否する(t *testing.T) {
	svc := newSubscriptionSvc(newFakeSubscriptionStore())

	for _, months := range []int{0, -1} {
		err := svc.Grant(context.Background(), domain.NewUserID(), months)
		require.ErrorIs(t, err, service.ErrInvalidGrantMonths, "%d は拒否されるべき", months)
	}
}

func TestSubscriptionService_Grant_期限切れの加入を再付与できる(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	require.NoError(t, store.Upsert(context.Background(), domain.Subscription{
		UserID:           u,
		Plan:             domain.PlanPremium,
		Status:           domain.SubscriptionCanceled,
		CurrentPeriodEnd: fixedNow.Add(-time.Hour),
		Provider:         domain.ProviderManual,
	}))

	require.NoError(t, newSubscriptionSvc(store).Grant(context.Background(), u, 1))

	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionActive, sub.Status, "再付与で active に戻るべき")
	require.Equal(t, fixedNow.AddDate(0, 1, 0), sub.CurrentPeriodEnd)
}

func TestSubscriptionService_Revoke_行を消さずcanceledにする(t *testing.T) {
	store := newFakeSubscriptionStore()
	u := domain.NewUserID()
	svc := newSubscriptionSvc(store)
	require.NoError(t, svc.Grant(context.Background(), u, 1))

	require.NoError(t, svc.Revoke(context.Background(), u))

	// 行を消すと「いつ解約したか」の記録が失われる。
	// 後で「解約したのに課金された」と申し立てられたときの反証材料になる。
	sub, err := store.Find(context.Background(), u)
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionCanceled, sub.Status)
}

func TestSubscriptionService_Revoke_加入が無ければ何もしない(t *testing.T) {
	store := newFakeSubscriptionStore()

	// 冪等。既に free の利用者に取消をかけても失敗させない。
	require.NoError(t, newSubscriptionSvc(store).Revoke(context.Background(), domain.NewUserID()))
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/service/ -run TestSubscriptionService -v`
Expected: FAIL — `undefined: service.NewSubscriptionService`（コンパイルエラー）

- [ ] **Step 3: 実装する**

`backend/internal/service/subscription.go`:

```go
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
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd backend && go test ./internal/service/ -run TestSubscriptionService -v`
Expected: PASS（6テスト）

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/subscription.go backend/internal/service/subscription_test.go
git commit -m "feat: 加入の付与と取消をサービスに集約する"
```

---

### Task 9: CLI `cmd/grant` と Makefile

**Files:**
- Create: `backend/cmd/grant/main.go`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `service.NewSubscriptionService`（Task 8）、`repository.NewUserRepository` / `FindByEmail`（Task 6）、`repository.NewSubscriptionRepository`（Task 5）
- Produces: 実行可能な CLI。他のコードから参照されない

`cmd/seed/main.go` と同じ構成にする（`DATABASE_URL` を読み、`slog` のJSONハンドラを既定にし、
失敗したら `os.Exit(1)`）。CLI にはテストを書かない。ロジックは全て Task 7・8 で検証済みで、
このファイルは配線と入出力だけを持つ。

- [ ] **Step 1: 実装する**

`backend/cmd/grant/main.go`:

```go
// Package main はプレミアムプランを手で付与・取消するCLI。
//
//	go run ./cmd/grant -email=foo@example.com -months=1   # 付与
//	go run ./cmd/grant -email=foo@example.com -revoke     # 即時取消
//
// 決済を導入するまでの唯一の付与手段。SQLを直接書かず service を通すことで、
// 将来の決済Webhook と同じ状態遷移を経由させる。
//
// 手動付与は決済事業者側の履歴に残らないため、ここが出すログが唯一の記録になる。
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	email := flag.String("email", "", "対象の利用者のメールアドレス")
	months := flag.Int("months", 1, "付与する月数")
	revoke := flag.Bool("revoke", false, "付与ではなく即時取消を行う")
	flag.Parse()

	if *email == "" {
		slog.Error("-email が必要です")
		os.Exit(1)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL が設定されていません")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, db.Config{DSN: dsn})
	if err != nil {
		slog.Error("DBへの接続に失敗しました", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	addr, err := domain.NewEmail(*email)
	if err != nil {
		slog.Error("メールアドレスが不正です", "error", err)
		os.Exit(1)
	}

	// メール→UserID の解決はCLIの責務。service を UserID 起点にしておくことで、
	// 顧客IDしか持たない将来の決済Webhook が不自然な逆引きを強いられない。
	user, err := repository.NewUserRepository(pool).FindByEmail(ctx, addr)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			slog.Error("該当する利用者が居ません", "email", *email)
		} else {
			slog.Error("利用者の取得に失敗しました", "error", err)
		}
		os.Exit(1)
	}

	svc := service.NewSubscriptionService(repository.NewSubscriptionRepository(pool), time.Now)

	if *revoke {
		if err := svc.Revoke(ctx, user.ID); err != nil {
			slog.Error("取消に失敗しました", "error", err)
			os.Exit(1)
		}
		slog.Info("プレミアムを取り消しました",
			"user_id", user.ID.String(), "email", *email)
		return
	}

	if err := svc.Grant(ctx, user.ID, *months); err != nil {
		slog.Error("付与に失敗しました", "error", err)
		os.Exit(1)
	}
	slog.Info("プレミアムを付与しました",
		"user_id", user.ID.String(), "email", *email, "months", *months)
}
```

- [ ] **Step 2: ビルドが通ることを確認する**

Run: `cd backend && go build ./... && go vet ./...`
Expected: エラーなし

- [ ] **Step 3: Makefile にターゲットを足す**

`.PHONY` 行の末尾に `grant revoke` を足し、`seed` ターゲットの直後に追記:

```makefile
# 決済を導入するまでの唯一の付与手段。EMAIL は必須、MONTHS の既定は1。
grant: ## プレミアムを付与する (make grant EMAIL=foo@example.com MONTHS=1)
	docker compose run --rm backend go run ./cmd/grant -email=$(EMAIL) -months=$(or $(MONTHS),1)

revoke: ## プレミアムを即時取り消す (make revoke EMAIL=foo@example.com)
	docker compose run --rm backend go run ./cmd/grant -email=$(EMAIL) -revoke
```

`help` ターゲットの `@echo` 一覧にも2行足す:

```makefile
	@echo "  grant          grant premium (EMAIL=... MONTHS=1)"
	@echo "  revoke         revoke premium (EMAIL=...)"
```

- [ ] **Step 4: 実際に動かして確認する**

```bash
make up && make migrate
# 適当な利用者を作る（未登録なら画面 http://localhost:5173/login から登録する）
make grant EMAIL=foo@example.com MONTHS=1
```
Expected: `{"level":"INFO","msg":"プレミアムを付与しました",...}` が出る

```bash
make revoke EMAIL=foo@example.com
```
Expected: `{"level":"INFO","msg":"プレミアムを取り消しました",...}` が出る

```bash
make grant EMAIL=nobody@example.com
```
Expected: `{"level":"ERROR","msg":"該当する利用者が居ません",...}` が出て終了コード 1

- [ ] **Step 5: コミット**

```bash
git add backend/cmd/grant/main.go Makefile
git commit -m "feat: プレミアムを付与・取消するCLIを追加"
```

---

### Task 10: 保存上限をプラン由来にする

**Files:**
- Modify: `backend/internal/service/saved_weekly.go`
- Modify: `backend/internal/handler/problem.go:81`
- Modify: `backend/internal/service/ports.go`（`Entitlements` ポート追加）
- Modify: `backend/cmd/server/main.go:116` 付近（配線）
- Test: `backend/internal/service/saved_weekly_test.go`

**Interfaces:**
- Consumes: `service.EntitlementService`（Task 7）、`domain.Entitlement`（Task 2）
- Produces: `service.Entitlements` インターフェース、`service.SavedWeeklyMenuLimitError{Limit int}`、`service.NewSavedWeeklyMenuService(SavedWeeklyMenuStore, Entitlements)`（**引数が1つ増える破壊的変更**）
- 変わらないもの: sentinel `service.ErrSavedWeeklyMenuLimitReached`、HTTPステータス 409、problem type `saved-weekly-menu-limit-reached`

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/service/saved_weekly_test.go` に追記（既存テストの `NewSavedWeeklyMenuService` 呼び出しは Step 4 でまとめて直す）:

```go
// fakeEntitlements は常に同じプランを返す。
type fakeEntitlements struct{ plan domain.Plan }

func (f fakeEntitlements) For(context.Context, string) (domain.Entitlement, error) {
	return domain.NewEntitlement(f.plan), nil
}

func TestSavedWeekly_freeは10件で断る(t *testing.T) {
	t.Parallel()

	store := &fakeSavedWeeklyStore{count: 10}
	svc := service.NewSavedWeeklyMenuService(store, fakeEntitlements{plan: domain.PlanFree})

	_, err := svc.Save(context.Background(), domain.NewUserID().String(), weekInput())
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuLimitReached)
	assert.Contains(t, err.Error(), "10件",
		"断る理由の件数はプラン由来で本文に出るべき")
	assert.Zero(t, store.saveCalls, "上限に達したら保存を呼ばない")
}

func TestSavedWeekly_premiumは10件でも保存できる(t *testing.T) {
	t.Parallel()

	store := &fakeSavedWeeklyStore{count: 10}
	svc := service.NewSavedWeeklyMenuService(store, fakeEntitlements{plan: domain.PlanPremium})

	_, err := svc.Save(context.Background(), domain.NewUserID().String(), weekInput())
	require.NoError(t, err, "premium の上限は50件なので10件目は通る")
	assert.Equal(t, 1, store.saveCalls)
}

func TestSavedWeekly_premiumは50件で断る(t *testing.T) {
	t.Parallel()

	store := &fakeSavedWeeklyStore{count: 50}
	svc := service.NewSavedWeeklyMenuService(store, fakeEntitlements{plan: domain.PlanPremium})

	_, err := svc.Save(context.Background(), domain.NewUserID().String(), weekInput())
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuLimitReached)
	assert.Contains(t, err.Error(), "50件")
}

// 既存の errors.Is による判定を壊していないことの担保。
// handler のエラー写像は sentinel との一致で動いている。
func TestSavedWeeklyMenuLimitError_sentinelと一致する(t *testing.T) {
	t.Parallel()

	err := &service.SavedWeeklyMenuLimitError{Limit: 50}
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuLimitReached)
}
```

既存の `fakeSavedWeeklyStore`（`count` フィールドを持つ）と `weekInput()` は
同ファイルの先頭に既にある。新たに作らずそのまま使う。

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/service/ -run TestSavedWeekly -v`
Expected: FAIL — `NewSavedWeeklyMenuService` の引数が合わない（コンパイルエラー）

- [ ] **Step 3: ポートを追加する**

`backend/internal/service/ports.go` の末尾に追記:

```go
// Entitlements は利用者が使える範囲の問い合わせを抽象化する。
// 実装は同じ service パッケージの EntitlementService。
//
// インターフェースを挟むのは、上限を使う側（保存や履歴）が
// 加入の保存方法を知らずに済むようにするため。
type Entitlements interface {
	// For は利用者のエンタイトルメントを返す。
	// userID が空（未認証）でもエラーにせず free を返す。
	For(ctx context.Context, userID string) (domain.Entitlement, error)
}
```

- [ ] **Step 4: `saved_weekly.go` を書き換える**

定数とエラーの宣言部（16〜20行目付近）を差し替える:

```go
// SavedWeeklyMenuLimitError は保存の上限に達していることを表す（409）。
//
// 上限はプランによって変わるため、固定文言の sentinel ではなく件数を持つ型にする。
// Is を実装しているので、既存の errors.Is(err, ErrSavedWeeklyMenuLimitReached) は
// そのまま通る。handler は Detail に err.Error() を入れるため、
// 利用者には自分のプランの件数が伝わる。
type SavedWeeklyMenuLimitError struct {
	Limit int
}

func (e *SavedWeeklyMenuLimitError) Error() string {
	return fmt.Sprintf("保存できる週間献立は%d件までです。古いものを削除してください", e.Limit)
}

// Is は sentinel との一致を成立させる。
func (e *SavedWeeklyMenuLimitError) Is(target error) bool {
	return target == ErrSavedWeeklyMenuLimitReached
}

// ErrSavedWeeklyMenuLimitReached は上限到達の sentinel。
// 判定にのみ使い、利用者に見せる文言は SavedWeeklyMenuLimitError が持つ。
var ErrSavedWeeklyMenuLimitReached = errors.New("保存できる週間献立の上限に達しました")
```

構造体とコンストラクタを差し替える:

```go
// SavedWeeklyMenuService は週間献立の保存を担う。
type SavedWeeklyMenuService struct {
	store        SavedWeeklyMenuStore
	entitlements Entitlements
}

// NewSavedWeeklyMenuService は SavedWeeklyMenuService を生成する。
func NewSavedWeeklyMenuService(
	store SavedWeeklyMenuStore, entitlements Entitlements,
) *SavedWeeklyMenuService {
	return &SavedWeeklyMenuService{store: store, entitlements: entitlements}
}
```

`Save` の上限判定部を差し替える（`count, err := s.store.Count(...)` の直前に挿入し、比較を書き換える）:

```go
	// 上限はプランによって変わる（設計 3.1）。
	// free は現行の10件を据え置き、premium が上回る。既存利用者の体験は削らない。
	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return domain.SavedWeeklyMenuID{}, err
	}
	limit := ent.SavedWeeklyMenuLimit()

	// 上限は保存の直前に見る（spec.md 2.8）。
	//
	// **数えてから書くまでの間に別のリクエストが割り込む余地は残る。**
	// 同一利用者が同時に保存を送ると上限+1件目が通りうるが、上限は
	// 「際限なく溜めない」ための目安であり、そこまでの厳密さは要らないと判断した。
	// DB制約で締めるには件数を数えるトリガか排他ロックが要り、割に合わない。
	count, err := s.store.Count(ctx, uid)
	if err != nil {
		return domain.SavedWeeklyMenuID{}, err
	}
	if count >= limit {
		return domain.SavedWeeklyMenuID{}, &SavedWeeklyMenuLimitError{Limit: limit}
	}
```

`List` のコメント「上限が10件と小さいので全件返す」を、上限が50件まで伸びた事実に合わせて直す:

```go
// List は認証済みユーザーの保存を新しい順に、中身の7日分も含めて返す。
//
// 上限は最大でも50件と小さいので全件返す。ページングは設けない。
```

- [ ] **Step 5: `problem.go` の title を件数非依存にする**

`backend/internal/handler/problem.go:81` 付近を差し替える:

```go
	// 保存した週間献立が上限に達している。履歴のように押し出さず断るため、
	// 入力の誤り（400）ではなく今の状態との競合（409）にする（spec.md 2.8）。
	//
	// 上限はプランによって変わるため title に件数を書かない。
	// 実際の件数は Detail（err.Error()）が持つ。
	{service.ErrSavedWeeklyMenuLimitReached, http.StatusConflict, "saved-weekly-menu-limit-reached", "保存できる週間献立の上限に達しました"},
```

- [ ] **Step 6: 配線を直す**

`backend/cmd/server/main.go` の `savedWeeklySvc` の行を差し替える:

```go
	subscriptionRepo := repository.NewSubscriptionRepository(pool)
	entitlementSvc := service.NewEntitlementService(subscriptionRepo, time.Now)

	savedWeeklyRepo := repository.NewSavedWeeklyMenuRepository(pool)
	savedWeeklySvc := service.NewSavedWeeklyMenuService(savedWeeklyRepo, entitlementSvc)
	savedWeeklyHandler := handler.NewSavedWeeklyMenuHandler(savedWeeklySvc, tokens)
```

`time` が未 import なら追加する。

- [ ] **Step 7: 既存テストの呼び出しを直す**

`grep -rn "NewSavedWeeklyMenuService" backend/` で全ての呼び出しを洗い出し、
第2引数に `fakeEntitlements{plan: domain.PlanFree}` を渡すよう直す。
既存テストは free の10件で書かれているため、free を渡せば期待値は変わらない。

- [ ] **Step 8: テストが通ることを確認する**

Run: `cd backend && go build ./... && go test ./... -count=1`
Expected: PASS（既存テストを含め全て。特に `internal/handler` のエラー写像テストが緑であること）

- [ ] **Step 9: コミット**

```bash
git add backend/internal/service/saved_weekly.go backend/internal/service/saved_weekly_test.go \
        backend/internal/service/ports.go backend/internal/handler/problem.go \
        backend/cmd/server/main.go
git commit -m "feat: 週間献立の保存上限をプラン由来にする"
```

---

### Task 11: `/auth/me` がプランを返す

**Files:**
- Modify: `api/openapi.yaml`（`User` スキーマ）
- Modify: `backend/internal/handler/auth.go`（`userDTO` / `toUserDTO` / `Me`）
- Modify: `backend/cmd/server/main.go`（`NewAuthHandler` の配線）
- Modify: `frontend/src/api/types.ts`（`make gen-api` で再生成）
- Test: `backend/internal/handler/auth_test.go`

**Interfaces:**
- Consumes: `service.Entitlements`（Task 10）、`domain.Entitlement`（Task 2）
- Produces: `GET /api/v1/auth/me` のレスポンス `user.plan` が `"free" | "premium"`、`handler.NewAuthHandler(AuthUseCase, *auth.JWT, GoogleOAuth, string, service.Entitlements)`（**引数が1つ増える破壊的変更**）、`handler.userDTO` に `Plan string \`json:"plan"\``

上限値そのものは返さない。フロントが件数を持つと二重管理になる。

- [ ] **Step 1: OpenAPI を更新する**

`api/openapi.yaml` の `User` スキーマを差し替える:

```yaml
    User:
      type: object
      required: [id, email, displayName, plan]
      properties:
        id:
          type: string
          format: uuid
        email:
          type: string
          format: email
        displayName:
          type: string
        plan:
          type: string
          enum: [free, premium]
          description: |
            契約プラン。上限そのものは返さない。フロントが件数を持つと
            サーバとの二重管理になるため、上限に達したことは 409 の本文で伝える。
```

- [ ] **Step 2: 失敗するテストを書く**

`backend/internal/handler/auth_test.go` に追記する。

まず fake を足す:

```go
// fakeEntitlements は常に同じプランを返す。
type fakeEntitlements struct{ plan domain.Plan }

func (f fakeEntitlements) For(context.Context, string) (domain.Entitlement, error) {
	return domain.NewEntitlement(f.plan), nil
}
```

既存の `newAuthApp` はプランを指定できる版に委譲させる。
**既存の呼び出しを1つも壊さないため、`newAuthApp` の引数は変えない。**

```go
func newAuthApp(t *testing.T, svc handler.AuthUseCase, opts ...auth.JWTOption) (*echo.Echo, *auth.JWT) {
	t.Helper()
	return newAuthAppWithPlan(t, svc, domain.PlanFree, opts...)
}

// newAuthAppWithPlan はプランを指定してアプリを組む。
func newAuthAppWithPlan(
	t *testing.T, svc handler.AuthUseCase, plan domain.Plan, opts ...auth.JWTOption,
) (*echo.Echo, *auth.JWT) {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret), opts...)
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewAuthHandler(svc, tokens, testGoogleOAuth(), testFrontendURL,
		fakeEntitlements{plan: plan}).RegisterRoutes(e)
	return e, tokens
}
```

テスト本体:

```go
func TestMe_プランを返す(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		plan domain.Plan
		want string
	}{
		{"free", domain.PlanFree, `"plan":"free"`},
		{"premium", domain.PlanPremium, `"plan":"premium"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := newTestUser(t, "plan@example.com")
			svc := &fakeAuthService{currentUser: user}
			e, tokens := newAuthAppWithPlan(t, svc, tt.plan)

			access, err := tokens.Issue(user.ID.String())
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.want)
		})
	}
}
```

- [ ] **Step 3: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/handler/ -run TestMe -v`
Expected: FAIL — レスポンスに `plan` が無い

- [ ] **Step 4: ハンドラを実装する**

`backend/internal/handler/auth.go`:

`userDTO` に追加:

```go
type userDTO struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Plan        string `json:"plan"`
}
```

`AuthHandler` に `entitlements service.Entitlements` フィールドを足し、
`NewAuthHandler` の引数に加える。

`toUserDTO` をプランを受け取る形に変える:

```go
func toUserDTO(u domain.User, plan domain.Plan) userDTO {
	return userDTO{
		ID:          u.ID.String(),
		Email:       u.Email.String(),
		DisplayName: u.DisplayName,
		Plan:        plan.String(),
	}
}
```

`Me` でエンタイトルメントを引く:

```go
	ent, err := h.entitlements.For(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, userResponse{User: toUserDTO(user, ent.Plan())})
```

**`toUserDTO` の他の呼び出し箇所（サインアップ・ログイン）も直す。**
`grep -n "toUserDTO" backend/internal/handler/auth.go` で洗い出す。
サインアップ直後は必ず free なので `domain.PlanFree` を渡してよい。
ログインは既存利用者がプレミアムでありうるため、`Me` と同じく `h.entitlements.For` を通す。

- [ ] **Step 5: 配線を直す**

`backend/cmd/server/main.go` の `authHandler` の行を、`entitlementSvc` を渡す形に直す。
`entitlementSvc` は Task 10 で `authHandler` より後に定義しているため、**定義順を入れ替える**
（`subscriptionRepo` / `entitlementSvc` を `userRepo` の直後に移す）。

- [ ] **Step 6: テストが通ることを確認する**

Run: `cd backend && go build ./... && go test ./... -count=1`
Expected: PASS（契約テスト `internal/handler/contract_test.go` を含む）

- [ ] **Step 7: TS の型を再生成する**

Run: `make gen-api`
Expected: `frontend/src/api/types.ts` に `plan` が入る（差分が出る）

Run: `cd frontend && npx tsc -b`
Expected: 型エラーなし

- [ ] **Step 8: コミット**

```bash
git add api/openapi.yaml backend/internal/handler/auth.go backend/internal/handler/auth_test.go \
        backend/cmd/server/main.go frontend/src/api/types.ts
git commit -m "feat: /auth/me がプランを返す"
```

---

### Task 12: フロントに premium バッジを出す

**Files:**
- Modify: `frontend/src/features/auth/AuthMenu.tsx`
- Test: `frontend/src/features/auth/AuthMenu.test.tsx`（無ければ新規）

**Interfaces:**
- Consumes: `useCurrentUser()` が返す `user.plan`（Task 11）
- Produces: premium のときだけ表示される要素（`aria-label="プレミアム会員"`）

**free には何も新しく表示しない。アップグレード導線も作らない。**
決済が存在しない状態で「プレミアムにする」ボタンを出すのは不誠実である。
バッジは勧誘ではなく状態の表示なので、この線引きに反しない。

- [ ] **Step 1: 失敗するテストを書く**

`frontend/src/features/auth/AuthMenu.test.tsx`:

```tsx
import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithProviders } from '../../test/render'
import { server } from '../../test/server'
import { AuthMenu } from './AuthMenu'

// respondMe は現在のユーザーの応答を仕込む。プランだけを差し替える。
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

describe('AuthMenu', () => {
  it('premium ならバッジを出す', async () => {
    respondMe('premium')
    renderWithProviders(<AuthMenu />)

    expect(await screen.findByLabelText('プレミアム会員')).toBeInTheDocument()
  })

  it('free ならバッジを出さない', async () => {
    respondMe('free')
    renderWithProviders(<AuthMenu />)

    // 「無いこと」を検査する前に描画の完了を待つ。待たずに検査すると、
    // まだ何も描かれていない状態でも通ってしまい、常に緑の偽の合格になる。
    expect(await screen.findByText('ユーザー')).toBeInTheDocument()
    expect(screen.queryByLabelText('プレミアム会員')).not.toBeInTheDocument()
  })
})
```

`queryBy` を使うのは、「無いこと」の検証に `findBy` を使うと必ず失敗するため。
`renderWithProviders` と `server` は既存の `LoginPage.test.tsx` が使っているものと同じ。

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd frontend && npx vitest run src/features/auth/AuthMenu.test.tsx`
Expected: FAIL — `プレミアム会員` のラベルが見つからない

- [ ] **Step 3: バッジを実装する**

`frontend/src/features/auth/AuthMenu.tsx` の認証済みの返り値を差し替える:

```tsx
  return (
    <span className="ml-auto flex items-center gap-3">
      {/*
        プレミアムであることの表示。free には何も出さない。
        決済が無い段階でアップグレード導線を出すのは不誠実なので、
        ここは勧誘ではなく状態の表示に留める。
      */}
      {user.plan === 'premium' && (
        <span
          aria-label="プレミアム会員"
          className="rounded-full bg-kon-leaf/20 px-2 py-0.5 text-xs text-kon-ink"
        >
          プレミアム
        </span>
      )}
      <span className="text-sm text-kon-ink/80">{user.displayName}</span>
      <button
        type="button"
        onClick={() => logout.mutate(undefined, { onSettled: () => setNotice(true) })}
        disabled={logout.isPending}
        className="whitespace-nowrap text-sm text-kon-ink/70 underline decoration-kon-leaf underline-offset-2 hover:text-kon-ink disabled:text-kon-ink/40"
      >
        ログアウト
      </button>
    </span>
  )
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd frontend && npx vitest run src/features/auth/AuthMenu.test.tsx`
Expected: PASS（2テスト）

- [ ] **Step 5: フロント全体の検査を通す**

Run: `make test-frontend`
Expected: `tsc -b` / `oxlint` / `vitest` が全て緑

- [ ] **Step 6: コミット**

```bash
git add frontend/src/features/auth/AuthMenu.tsx frontend/src/features/auth/AuthMenu.test.tsx
git commit -m "feat: プレミアム会員のバッジを出す"
```

---

### Task 13: E2E

**Files:**
- Create: `frontend/e2e/premium.spec.ts`

**Interfaces:**
- Consumes: CLI `cmd/grant`（Task 9）、バッジ（Task 12）
- Produces: なし

**上限の境界値はここで検証しない。** 11件保存するには週間献立を11回組む必要があり、
実行時間に見合わない。境界値は Task 10 の service テストで担保済み。

- [ ] **Step 1: E2E を書く**

`frontend/e2e/premium.spec.ts`:

```ts
import { execSync } from 'node:child_process'

import { expect, test } from '@playwright/test'

import { signUp, uniqueEmail } from './helpers'

test('プレミアムを付与するとバッジが出る', async ({ page }) => {
  const email = uniqueEmail('premium')
  await signUp(page, email)

  // 付与前は出ていない。ここを確かめておかないと、
  // 「元から出ていた」のか「付与で出た」のかを区別できない。
  await expect(page.getByLabel('プレミアム会員')).toBeHidden()

  // 決済が無いので付与はCLIで行う。既存E2Eが make up / make seed を
  // 前提にしているのと同じ流儀で、起動中のコンテナに対して実行する。
  execSync(`docker compose run --rm backend go run ./cmd/grant -email=${email} -months=1`, {
    cwd: '..',
    stdio: 'inherit',
  })

  // useCurrentUser は staleTime 5分でキャッシュするため、取り直しが要る。
  await page.reload()

  await expect(page.getByLabel('プレミアム会員')).toBeVisible()
})

test('無料の利用者にはバッジが出ない', async ({ page }) => {
  await signUp(page, uniqueEmail('free'))

  // signUp がログアウトボタンの出現まで待つので、ヘッダの描画は完了している。
  // それを確かめたうえで「無いこと」を検査する。
  await expect(page.getByRole('button', { name: 'ログアウト' })).toBeVisible()
  await expect(page.getByLabel('プレミアム会員')).toBeHidden()
})
```

`signUp` / `uniqueEmail` は既存の `frontend/e2e/helpers.ts` にある。
`docker compose` の作業ディレクトリはリポジトリ直下なので `cwd: '..'`。

- [ ] **Step 2: E2E を走らせる**

Run: `make up && make migrate && make seed && make test-e2e`
Expected: `premium.spec.ts` の2テストが PASS

- [ ] **Step 3: コミット**

```bash
git add frontend/e2e/premium.spec.ts
git commit -m "test: プレミアムのバッジのE2E"
```

---

### Task 14: `spec.md` を更新する

**Files:**
- Modify: `spec.md`

**Interfaces:**
- Consumes: 設計文書 `docs/superpowers/specs/2026-07-23-premium-entitlement-design.md`
- Produces: なし（仕様の正を実装に追随させる）

`spec.md` が仕様の正なので、実装した内容を反映する。

- [ ] **Step 1: 2.11 節を足す**

`### 2.10 献立の役割（フェーズ14）` の後ろに `### 2.11 プレミアムプラン（フェーズ15）` を新設し、
以下を書く。

- 有料のプレミアムプランを設ける。**この段階では決済を実装せず**、運用者がCLIで付与する
- 週間献立の保存上限: free = 10件（現行維持）、premium = 50件
- 上限は「上乗せ」であり「取り上げ」ではない。既存利用者の体験は削らない
- 加入の有効性は参照のたびに計算する（期限切れをバッチで書き戻さない）
- `GET /auth/me` が `plan` を返す。上限値は返さない
- アップグレード導線は決済フェーズまで作らない

- [ ] **Step 2: 4.2 節に `subscriptions` のテーブル定義を足す**

既存のテーブル定義の書式に合わせ、Task 4 の DDL と対応する表を書く。
`free はレコードを持たない` ことと、部分UNIQUE索引の理由を明記する。

- [ ] **Step 3: 15章「有料化の前提条件」を足す**

設計文書の12章をそのまま転記する。冒頭の注意書き（法律の専門家によるものではない旨）を必ず含める。

- [ ] **Step 4: 13章「未決事項」に販売主体と価格を足す**

- 販売主体（個人事業主／バーチャルオフィス／MoR）。決済事業者の選定より先に決める
- 価格（月額）

- [ ] **Step 5: 10章「実装フェーズ」にフェーズ15を足す**

フェーズ15「プレミアムプラン基盤」を、完了条件つきで足す。

- [ ] **Step 6: コミット**

```bash
git add spec.md
git commit -m "docs: プレミアムプランを仕様に反映"
```

---

## 完了後の手順

1. `make test` と `make test-e2e` が全て緑であることを確認する
2. **`/code-review` を走らせる**。指摘はその場で直し、直したもの・見送ったものを理由つきで報告する
3. 作業ブランチ → `feature/premium` のPRを出す。CIが緑なら自分でマージする
4. **`feature/premium` → `main` のPRを出す前に `000009_add_menu_role` が `main` に入っていることを確認する**（Global Constraints）。入っていなければ待つか採番し直す
5. `feature/premium` → `main` のPRを作成したら `/code-review:code-review` を走らせ、**ユーザーのマージを待つ**

## この計画に含まれないもの

決済事業者の接続・Webhook・カスタマーポータル、課金UIとアップグレード導線、
利用規約／特商法に基づく表示／プライバシーポリシーの文面作成、プラン変更と日割り、
トライアル、招待コード。いずれも決済フェーズの仕事で、
その着手前に設計文書12章（法務要件）を満たす必要がある。
