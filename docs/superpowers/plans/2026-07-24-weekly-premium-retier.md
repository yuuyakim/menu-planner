# 週間まわりを premium にする（free/premium 再編）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 週間まわり一式（週間献立の提案・引き直し・保存/一覧/削除・保存済み週の買い物リスト）を premium 限定にし、free/未ログインには週間画面でロック付きプレビュー＋アップグレード導線を出す。単発フローは一切変えない。

**Architecture:** 認可は `RequirePremium` echo ミドルウェア1つに集約し（`RequireAuth` の後段で `EntitlementService.For` を引き `CanUseWeeklyPlanning()` を判定、false なら 403）、weekly 系の全ルートに前置する。フロントは共有 `PremiumLock` コンポーネントで、未ログイン→ログイン導線 / free→アップグレード導線を出し分ける。

**Tech Stack:** Go 1.25 / echo v4 / pgx v5 / React 19 / TanStack Query / Vitest + MSW / Playwright。

## Global Constraints

- **設計の正:** `docs/superpowers/specs/2026-07-24-weekly-premium-retier-design.md`。食い違いは設計を優先。
- **ブランチ:** base は現在の `main`（`feature/premium` は既に main へマージ済みで存在しない）。作業は `feat/weekly-premium` 上。1タスク=1PR=🔴+🟢、🔴と🟢は別コミット（`test:`→`feat:`）。
- **これは live な free 機能の取り上げ**（設計1.1）。`feat/weekly-premium`→`main` は本番境界で、**マージは人間**。デプロイ＝即、free から週間が消える。マイグレーション不要。
- **認可の位置は1箇所に集約する**: `RequirePremium` ミドルウェア。サービス内の既存 premium 判定（`SavedShoppingListService.ReplaceOverrides` の `CanPersistShoppingList`）は defense-in-depth として残す（消さない）。
- 未ログイン=401（`token-invalid`）、ログイン済み free=403（`service.ErrPremiumRequired`、写像済み・追加不要）。
- **単発フローは不変**: `GET /menus/suggest`、献立詳細・レシピ、`POST /shopping-list`、`POST /menus/search-by-ingredients`、履歴、お気に入り。**回帰させないこと。**
- **`GET /menus/suggest`（単発）は OptionalAuth のまま**。premium 化するのは `suggest-weekly`/`reroll-day` だけ。
- テスト実行: backend `make test-backend`（testcontainers）、frontend `make test-frontend`（tsc -b + lint + vitest）、E2E `make test-e2e`。Docker デーモンが要る（統合テストは testcontainers、E2E は compose）。
- 🔴/🟢 の commit は Co-Authored-By トレーラを付けない（周辺の作業コミットの流儀）。

---

## File Structure

**バックエンド（新規）**
- （なし。新ファイルは無く、既存への追加のみ）

**バックエンド（変更）**
- `backend/internal/domain/entitlement.go`（+test） — `CanUseWeeklyPlanning()`。
- `backend/internal/handler/middleware.go`（+test） — `RequirePremium` ミドルウェア。
- `backend/internal/handler/menu.go`（+test） — `suggest-weekly`/`reroll-day` を RequireAuth+RequirePremium に。`NewMenuHandler` に entitlements を足す。
- `backend/internal/handler/saved_weekly.go`（+test） — 3ルートに RequirePremium。constructor に entitlements。
- `backend/internal/handler/saved_shopping_list.go`（+test） — GET/PUT に RequirePremium。constructor に entitlements。
- `backend/cmd/server/main.go` — 各ハンドラ構築に `entitlementSvc` を渡す。

**フロント（新規）**
- `frontend/src/features/premium/PremiumLock.tsx`（+test） — 共有のロック付きプレビュー＋CTA。

**フロント（変更）**
- `frontend/src/features/menu/WeeklyPage.tsx`（+test） — 非 premium は `PremiumLock`。
- `frontend/src/features/menu/SavedWeeklyPage.tsx`（+test） — 非 premium は `PremiumLock`。

**責務の境界:**
- 認可は `RequirePremium` ミドルウェアに一元化。ハンドラ本体・サービスの分岐を増やさない。
- フロントは「開く前にプランで出し分け」を主経路にし、API 由来の 401/403 は保険。

---

### Task 1: `domain.Entitlement.CanUseWeeklyPlanning()`

**Files:**
- Modify: `backend/internal/domain/entitlement.go`
- Test: `backend/internal/domain/entitlement_test.go`

**Interfaces:**
- Produces: `func (e Entitlement) CanUseWeeklyPlanning() bool` — premium:true / それ以外:false。

- [ ] **Step 1: 失敗するテストを書く**

`entitlement_test.go` に足す:

```go
func TestEntitlement_CanUseWeeklyPlanning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ent  domain.Entitlement
		want bool
	}{
		{"premium は週間を使える", domain.NewEntitlement(domain.PlanPremium), true},
		{"free は使えない", domain.NewEntitlement(domain.PlanFree), false},
		{"ゼロ値は使えない", domain.Entitlement{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.ent.CanUseWeeklyPlanning(); got != tt.want {
				t.Errorf("CanUseWeeklyPlanning() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 失敗を確認** — `cd backend && go test ./internal/domain/ -run TestEntitlement_CanUseWeeklyPlanning`（未定義でFAIL）
- [ ] **Step 3: 🔴 コミット** — `git commit -m "test: CanUseWeeklyPlanning の導出（ゼロ値は false）"`
- [ ] **Step 4: 実装** — `entitlement.go` の `CanPersistShoppingList` の隣に:

```go
// CanUseWeeklyPlanning は週間献立の計画一式（提案・保存・週間の買い物リスト）を
// 使えるかを返す。premium だけ true。ゼロ値は free に落ちるため false（安全側）。
func (e Entitlement) CanUseWeeklyPlanning() bool {
	return e.Plan() == PlanPremium
}
```

- [ ] **Step 5: 通過を確認** — 同上コマンドで PASS
- [ ] **Step 6: 🟢 コミット** — `git commit -m "feat: 週間献立の計画一式の権限を足す"`

---

### Task 2: `RequirePremium` ミドルウェア

**Files:**
- Modify: `backend/internal/handler/middleware.go`
- Test: `backend/internal/handler/middleware_test.go`（無ければ新規。既存の middleware テストの流儀に合わせる）

**Interfaces:**
- Consumes: `service.Entitlements`（`For(ctx, userID string) (domain.Entitlement, error)`・既存）、`UserIDFromContext`、`auth.ErrTokenInvalid`、`service.ErrPremiumRequired`。
- Produces: `func RequirePremium(entitlements service.Entitlements) echo.MiddlewareFunc`。**`RequireAuth` の後段に置く前提**（userID がコンテキストにある）。

- [ ] **Step 1: 失敗するテストを書く**

`middleware_test.go` に足す（fake entitlements と echo コンテキストで検証。既存の RequireAuth テストの組み立てを参考に）:

```go
type fakeEntitlements struct {
	ent domain.Entitlement
	err error
}

func (f fakeEntitlements) For(context.Context, string) (domain.Entitlement, error) {
	return f.ent, f.err
}

func TestRequirePremium(t *testing.T) {
	// premium は通す / free は 403 / userID 無しは 401 を確認する。
	// RequireAuth が先に userID をコンテキストに載せる前提なので、
	// テストではハンドラ手前で c.Set(userIDContextKey, ...) 相当を用意する
	// （既存 UserIDFromContext が読むキーに合わせる。middleware.go を参照）。

	newCtx := func(userID string, hasUser bool) (echo.Context, *httptest.ResponseRecorder) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if hasUser {
			// UserIDFromContext が読むのと同じ方法で userID を載せる。
			c.Set(userIDContextKey, userID) // ← 実際のキー名は middleware.go に合わせる
		}
		return c, rec
	}

	called := false
	next := func(c echo.Context) error { called = true; return nil }

	t.Run("premium は通す", func(t *testing.T) {
		called = false
		mw := handler.RequirePremium(fakeEntitlements{ent: domain.NewEntitlement(domain.PlanPremium)})
		c, _ := newCtx("11111111-1111-1111-1111-111111111111", true)
		require.NoError(t, mw(next)(c))
		require.True(t, called)
	})
	t.Run("free は 403", func(t *testing.T) {
		called = false
		mw := handler.RequirePremium(fakeEntitlements{ent: domain.NewEntitlement(domain.PlanFree)})
		c, _ := newCtx("11111111-1111-1111-1111-111111111111", true)
		err := mw(next)(c)
		require.ErrorIs(t, err, service.ErrPremiumRequired)
		require.False(t, called)
	})
	t.Run("userID 無しは 401", func(t *testing.T) {
		called = false
		mw := handler.RequirePremium(fakeEntitlements{ent: domain.NewEntitlement(domain.PlanPremium)})
		c, _ := newCtx("", false)
		err := mw(next)(c)
		require.ErrorIs(t, err, auth.ErrTokenInvalid)
		require.False(t, called)
	})
}
```

> `userIDContextKey` は `middleware.go` が `UserIDFromContext`/`RequireAuth` で使っている実際のキー。**まず middleware.go を読み、同じキー/方法で載せる**こと（`c.Get(...)` の裏返し）。テスト内で直接載せられない設計（unexported）なら、`RequireAuth` を前段に通す形（有効な JWT Cookie＋`RequireAuth(tokens)` → `RequirePremium`）でテストを組む。既存 handler テストの認証セットアップを流用する。

- [ ] **Step 2: 失敗を確認** — `cd backend && go test ./internal/handler/ -run TestRequirePremium`
- [ ] **Step 3: 🔴 コミット** — `git commit -m "test: RequirePremium ミドルウェア（premium通過/free403/未認証401）"`
- [ ] **Step 4: 実装** — `middleware.go` に足す（`RequireAuth` の下）:

```go
// RequirePremium は premium 加入者だけを通すミドルウェア。RequireAuth の後段に置く。
//
// userID が無ければ 401（RequireAuth が先に通っていない配線ミス、または未認証）。
// premium でなければ 403（ErrPremiumRequired）。エンタイトルメントの引き当てに
// 失敗したら、その err をそのまま返す（500 系）。
func RequirePremium(entitlements service.Entitlements) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID, ok := UserIDFromContext(c)
			if !ok {
				return auth.ErrTokenInvalid
			}
			ent, err := entitlements.For(c.Request().Context(), userID)
			if err != nil {
				return err
			}
			if !ent.CanUseWeeklyPlanning() {
				return service.ErrPremiumRequired
			}
			return next(c)
		}
	}
}
```

> import に `service` が必要（handler は既に service を import している）。`service.Entitlements` は `For(ctx, userID string) (domain.Entitlement, error)` を持つ既存インターフェース。

- [ ] **Step 5: 通過を確認** — `cd backend && go build ./... && go test ./internal/handler/ -run TestRequirePremium`
- [ ] **Step 6: 🟢 コミット** — `git commit -m "feat: premium 限定ミドルウェア RequirePremium を足す"`

---

### Task 3: `suggest-weekly` / `reroll-day` を premium 限定に

**Files:**
- Modify: `backend/internal/handler/menu.go`（constructor + RegisterRoutes）
- Modify: `backend/internal/handler/menu_test.go` / `menu_history_test.go`（該当テスト）
- Modify: `backend/cmd/server/main.go`（`NewMenuHandler` に entitlements を渡す）
- Modify: `backend/internal/handler/contract_test.go` / `routing_test.go`（公開前提のテストがあれば更新）

**Interfaces:**
- Consumes: `RequirePremium`（Task 2）、`RequireAuth`、`service.Entitlements`。
- Produces: `NewMenuHandler(s MenuUseCase, history MenuHistory, tokens *auth.JWT, entitlements service.Entitlements)`。`suggest-weekly`/`reroll-day` は **要ログイン＋premium**。`suggest`（単発）は OptionalAuth のまま。

- [ ] **Step 1: 失敗するテストを書く**

`menu_test.go`（または該当ファイル）に、weekly の認可を固定するテストを足す:

```go
func TestSuggestWeekly_未ログインは401(t *testing.T) {
	// entitlements は呼ばれる前に RequireAuth で弾かれる。
	h := newMenuHandlerForTest(t, /* fake svc */, fakeEntitlements{ent: domain.NewEntitlement(domain.PlanPremium)})
	rec := doPost(t, h, "/api/v1/menus/suggest-weekly", `{}`) // Cookie 無し
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestSuggestWeekly_freeは403(t *testing.T) {
	h := newMenuHandlerForTest(t, /* fake svc */, fakeEntitlements{ent: domain.NewEntitlement(domain.PlanFree)})
	rec := doAuthedPost(t, h, "/api/v1/menus/suggest-weekly", `{}`) // 有効Cookie・free
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSuggestWeekly_premiumは200(t *testing.T) {
	h := newMenuHandlerForTest(t, /* fake svc returning a week */, fakeEntitlements{ent: domain.NewEntitlement(domain.PlanPremium)})
	rec := doAuthedPost(t, h, "/api/v1/menus/suggest-weekly", `{}`)
	require.Equal(t, http.StatusOK, rec.Code)
}
// reroll-day も同型で 401/403/200 を1本ずつ。
```

> `newMenuHandlerForTest` / `doPost` / `doAuthedPost` は既存の menu handler テストのセットアップに合わせて用意/流用する（現状の menu_test.go の echo 組み立て・認証 Cookie の積み方を読んで踏襲）。`fakeEntitlements` は Task 2 で定義済みのものを使う。**既存の「suggest-weekly が未認証で 200/422 を返す」前提のテストは、認可が変わったので 401 期待に更新する**（回帰ではなく仕様変更）。単発 `suggest` のテストは変えない。

- [ ] **Step 2: 失敗を確認** — `cd backend && go test ./internal/handler/ -run 'TestSuggestWeekly|TestRerollDay'`
- [ ] **Step 3: 🔴 コミット** — `git commit -m "test: 週間献立の提案/引き直しを premium 限定にする"`
- [ ] **Step 4: 実装**

`menu.go` の constructor に entitlements を足す:

```go
func NewMenuHandler(s MenuUseCase, history MenuHistory, tokens *auth.JWT, entitlements service.Entitlements) *MenuHandler {
	return &MenuHandler{svc: s, history: history, tokens: tokens, entitlements: entitlements}
}
```

（`MenuHandler` 構造体に `entitlements service.Entitlements` フィールドを追加。）

`RegisterRoutes` を変更（`suggest` は optional 据え置き、weekly 2本を auth+premium に）:

```go
func (h *MenuHandler) RegisterRoutes(e *echo.Echo, mw ...echo.MiddlewareFunc) {
	g := e.Group(APIBasePath, mw...)
	optional := OptionalAuth(h.tokens)
	requireAuth := RequireAuth(h.tokens)
	premium := RequirePremium(h.entitlements)

	g.GET("/menus/suggest", h.Suggest, optional)          // 単発: 据え置き
	g.POST("/menus/suggest-weekly", h.SuggestWeekly, requireAuth, premium) // 週間: premium
	g.POST("/menus/reroll-day", h.RerollDay, requireAuth, premium)         // 週間: premium
	g.GET("/menus/:id", h.Get)
	g.GET("/menus/:id/recipes", h.Recipes)
}
```

> 履歴記録: `SuggestWeekly`/`RerollDay` ハンドラ内で userID を使う履歴記録は、RequireAuth で userID が必ず入るため従来どおり動く（premium 利用者の経路でのみ発生）。ハンドラ本体のロジックは変えない。

`main.go` の `menuHandler := handler.NewMenuHandler(menuSvc, historySvc, tokens)` を:

```go
menuHandler := handler.NewMenuHandler(menuSvc, historySvc, tokens, entitlementSvc)
```

- [ ] **Step 5: 通過を確認** — `cd backend && go build ./... && go test ./internal/handler/ && go test ./...`
- [ ] **Step 6: 🟢 コミット** — `git commit -m "feat: 週間献立の提案/引き直しを premium 限定にする"`

---

### Task 4: `weekly-menus`（保存/一覧/削除）を premium 限定に

**Files:**
- Modify: `backend/internal/handler/saved_weekly.go`（constructor + RegisterRoutes）
- Modify: `backend/internal/handler/saved_weekly_test.go`
- Modify: `backend/cmd/server/main.go`（`NewSavedWeeklyMenuHandler` に entitlements）

**Interfaces:**
- Produces: `NewSavedWeeklyMenuHandler(svc SavedWeeklyMenuUseCase, tokens *auth.JWT, entitlements service.Entitlements)`。`GET/POST /weekly-menus`・`DELETE /weekly-menus/:id` は RequireAuth+RequirePremium。

- [ ] **Step 1: 失敗するテストを書く** — 保存/一覧/削除それぞれで free403・premium200 を固定（未ログイン401 は既存の RequireAuth テストがあれば流用/追加）:

```go
func TestSavedWeekly_List_freeは403(t *testing.T) {
	e, tokens := savedWeeklyApp(t, &fakeSavedWeeklyService{}, fakeEntitlements{ent: domain.NewEntitlement(domain.PlanFree)})
	rec := doAuthedGet(t, e, tokens, "/api/v1/weekly-menus")
	require.Equal(t, http.StatusForbidden, rec.Code)
}
// Save(POST)・Delete も free403 を1本ずつ。premium200/204 も各1本。
```

> `savedWeeklyApp` は既存ヘルパ。**entitlements を受け取れるようシグネチャを拡張**する（`savedWeeklyApp(t, svc, entitlements)`）。既存テストの呼び出しは premium 既定（`fakeEntitlements{ent: premium}`）に更新して従来の 200/204 を保つ。

- [ ] **Step 2: 失敗を確認** — `cd backend && go test ./internal/handler/ -run TestSavedWeekly`
- [ ] **Step 3: 🔴 コミット** — `git commit -m "test: 週間献立の保存/一覧/削除を premium 限定にする"`
- [ ] **Step 4: 実装**

`saved_weekly.go` の constructor に entitlements を足し、`RegisterRoutes` の3ルートに `RequirePremium(h.entitlements)` を `requireAuth` の後に足す:

```go
func (h *SavedWeeklyMenuHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	requireAuth := RequireAuth(h.tokens)
	premium := RequirePremium(h.entitlements)
	g.GET("/weekly-menus", h.List, requireAuth, premium)
	g.POST("/weekly-menus", h.Save, requireAuth, premium)
	g.DELETE("/weekly-menus/:id", h.Delete, requireAuth, premium)
}
```

`main.go`: `handler.NewSavedWeeklyMenuHandler(savedWeeklySvc, tokens, entitlementSvc)`。

- [ ] **Step 5: 通過を確認** — `cd backend && go build ./... && go test ./internal/handler/ ./...`
- [ ] **Step 6: 🟢 コミット** — `git commit -m "feat: 週間献立の保存/一覧/削除を premium 限定にする"`

---

### Task 5: `GET/PUT /weekly-menus/:id/shopping-list` を premium 限定に

**Files:**
- Modify: `backend/internal/handler/saved_shopping_list.go`（constructor + RegisterRoutes）
- Modify: `backend/internal/handler/saved_shopping_list_test.go`
- Modify: `backend/cmd/server/main.go`

**Interfaces:**
- Produces: `NewSavedShoppingListHandler(svc SavedShoppingListUseCase, tokens *auth.JWT, entitlements service.Entitlements)`。GET/PUT ともに RequireAuth+RequirePremium。

- [ ] **Step 1: 失敗するテストを書く** — GET の free403 を足す（既存の Get_200 は premium 既定に更新）:

```go
func TestSavedShoppingListHandler_Get_freeは403(t *testing.T) {
	e, tokens := savedShoppingListApp(t, &fakeSavedShoppingList{}, fakeEntitlements{ent: domain.NewEntitlement(domain.PlanFree)})
	rec := doAuthedGet(t, e, tokens, "/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list")
	require.Equal(t, http.StatusForbidden, rec.Code)
}
```

> `savedShoppingListApp` に entitlements 引数を足す。既存の Get_200/Put_204 テストは `fakeEntitlements{premium}` に更新して従来の期待を保つ。PUT の free403 は既存（サービスが `ErrPremiumRequired`）だが、ミドルウェアでも 403 になる（二重の防御。テストは 403 を確認できればよい）。

- [ ] **Step 2: 失敗を確認** — `cd backend && go test ./internal/handler/ -run TestSavedShoppingListHandler`
- [ ] **Step 3: 🔴 コミット** — `git commit -m "test: 保存済み週の買い物リストを premium 限定にする"`
- [ ] **Step 4: 実装** — constructor に entitlements、`RegisterRoutes` の GET/PUT に `RequirePremium` を前置。`main.go`: `handler.NewSavedShoppingListHandler(savedShoppingListSvc, tokens, entitlementSvc)`。
- [ ] **Step 5: 通過を確認** — `cd backend && go build ./... && go test ./...`
- [ ] **Step 6: 🟢 コミット** — `git commit -m "feat: 保存済み週の買い物リストを premium 限定にする"`

---

### Task 6: フロント — 共有 `PremiumLock` コンポーネント

**Files:**
- Create: `frontend/src/features/premium/PremiumLock.tsx`
- Test: `frontend/src/features/premium/PremiumLock.test.tsx`

**Interfaces:**
- Consumes: `useCurrentUser`（`user`/`isLoading`/`isUnauthenticated`）、`MascotStatus`、`Link`。
- Produces: `export function PremiumLock({ title, description }: { title: string; description: string }): JSX.Element` — ローディング中は `<MascotStatus>`、未ログインはログイン導線、ログイン済み free はアップグレード導線を出す。

- [ ] **Step 1: 失敗するテストを書く**

```tsx
test('未ログインはログイン導線を出す', async () => {
  respondMe(undefined) // /auth/me 401
  renderWithProviders(<PremiumLock title="1週間まとめて計画" description="..." />)
  expect(await screen.findByRole('link', { name: /ログイン/ })).toBeInTheDocument()
})

test('ログイン済み free はアップグレード導線を出す', async () => {
  respondMe('free')
  renderWithProviders(<PremiumLock title="1週間まとめて計画" description="..." />)
  expect(await screen.findByText(/プレミアム/)).toBeInTheDocument()
})
```

> `respondMe` は既存テストの `/auth/me` 差し替えヘルパ（AuthMenu.test.tsx 等）。無ければ MSW で `http.get('/api/v1/auth/me', ...)` を返す小ヘルパをこのファイルに定義。

- [ ] **Step 2: 失敗を確認 → 🔴 コミット** — `npm test -- PremiumLock`（FAIL）→ `git commit -m "test: PremiumLock のログイン/アップグレード出し分け"`
- [ ] **Step 3: 実装**

```tsx
import { Link } from 'react-router'
import { MascotStatus } from '../../components/MascotStatus'
import { useCurrentUser } from '../auth/useCurrentUser'

export function PremiumLock({ title, description }: { title: string; description: string }) {
  const { user, isLoading } = useCurrentUser()
  if (isLoading) return <MascotStatus />
  return (
    <div className="mx-auto max-w-md rounded-2xl border border-kon-leaf-soft bg-white p-6 text-center">
      <p className="text-lg font-bold text-kon-ink">{title}</p>
      <p className="mt-2 text-sm text-kon-ink/70">{description}</p>
      {user ? (
        // ログイン済み free: アップグレード導線。決済導線は未実装のため当面は案内文言。
        <p className="mt-4 rounded-lg bg-kon-leaf/10 p-3 text-sm text-kon-ink">
          プレミアムプランでご利用いただけます。
        </p>
      ) : (
        <Link to="/login" className="mt-4 inline-block rounded-lg bg-kon-leaf px-4 py-2 font-medium text-white">
          ログインする
        </Link>
      )}
    </div>
  )
}
```

> **決済フローは未実装**（設計スコープ外）。ログイン済み free の「アップグレード」導線は当面は案内文言に留める（決済導入時に差し替え）。文言・ビジュアルは frontend-design で調整可（本タスクは最小の骨格）。

- [ ] **Step 4: 通過を確認 → 🟢 コミット** — `npm test -- PremiumLock` + `npx tsc -b` + `npm run lint` → `git commit -m "feat: 共有の PremiumLock コンポーネントを足す"`

---

### Task 7: フロント — WeeklyPage を premium ゲート

**Files:**
- Modify: `frontend/src/features/menu/WeeklyPage.tsx`
- Test: `frontend/src/features/menu/WeeklyPage.test.tsx`

**Interfaces:**
- Consumes: `PremiumLock`（Task 6）、`useCurrentUser`（既に import 済み）。

- [ ] **Step 1: 失敗するテストを書く**

```tsx
test('free は週間の生成UIを出さずロックを出す', async () => {
  respondMe('free')
  renderWithProviders(<WeeklyPage />)
  expect(await screen.findByText(/1週間まとめて/)).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /1週間分を作る/ })).not.toBeInTheDocument()
})

test('premium は生成UIを出す', async () => {
  respondMe('premium')
  renderWithProviders(<WeeklyPage />)
  expect(await screen.findByRole('button', { name: /1週間分を作る/ })).toBeInTheDocument()
})

test('未ログインはロック（ログイン導線）', async () => {
  respondMe(undefined)
  renderWithProviders(<WeeklyPage />)
  expect(await screen.findByRole('link', { name: /ログイン/ })).toBeInTheDocument()
})
```

> WeeklyPage の既存テストで「未ログインでも生成できる」前提のものは、仕様変更なので premium 前提に更新する（`respondMe('premium')` を足す）。既存の生成/引き直し/保存のテストは premium コンテキストで従来どおり通ること。

- [ ] **Step 2: 失敗を確認 → 🔴 コミット** — `npm test -- WeeklyPage`（FAIL）→ `git commit -m "test: WeeklyPage を premium ゲートする"`
- [ ] **Step 3: 実装** — WeeklyPage の描画の入口で出し分け:

```tsx
const { user, isLoading } = useCurrentUser()
// ...既存の hooks はそのまま（フックは早期 return より前で呼ぶ）...
if (isLoading) return <MascotStatus />
if (user?.plan !== 'premium') {
  return (
    <PremiumLock
      title="1週間まとめて計画"
      description="プレミアムプランなら、1週間分の献立をまとめて作って保存し、買い物リストも週単位で使えます。"
    />
  )
}
// 以降は既存の生成UI（premium のみ到達）
```

> **フックはすべて早期 return より前で呼ぶ**（条件付きフック禁止）。`useCurrentUser` は既に上部で呼ばれているので、その戻り値で分岐するだけ。単発フローには一切触れない。

- [ ] **Step 4: 通過を確認 → 🟢 コミット** — `npm test -- WeeklyPage` + `npx tsc -b` + `npm run lint` → `git commit -m "feat: WeeklyPage を premium ゲートする"`

---

### Task 8: フロント — SavedWeeklyPage を premium ゲート

**Files:**
- Modify: `frontend/src/features/menu/SavedWeeklyPage.tsx`
- Test: `frontend/src/features/menu/SavedWeeklyPage.test.tsx`

**Interfaces:**
- Consumes: `PremiumLock`、`useCurrentUser`。`/saved-weekly` は既に `RequireAuth` 配下（未ログインは /login リダイレクト）なので、**ここでは free→ロックのみ**扱う。

- [ ] **Step 1: 失敗するテストを書く**

```tsx
test('free はロックを出し保存一覧を取得しない', async () => {
  respondMe('free')
  let listed = false
  server.use(http.get('/api/v1/weekly-menus', () => { listed = true; return HttpResponse.json({ weeklyMenus: [] }) }))
  renderWithProviders(<SavedWeeklyPage />)
  expect(await screen.findByText(/プレミアム/)).toBeInTheDocument()
  expect(listed).toBe(false) // free では一覧 API を叩かない
})

test('premium は保存一覧を出す', async () => {
  respondMe('premium')
  server.use(http.get('/api/v1/weekly-menus', () => HttpResponse.json({ weeklyMenus: [] })))
  renderWithProviders(<SavedWeeklyPage />)
  // 空一覧の見出し等、既存の premium 表示を確認
})
```

- [ ] **Step 2: 失敗を確認 → 🔴 コミット** — `git commit -m "test: SavedWeeklyPage を premium ゲートする"`
- [ ] **Step 3: 実装** — SavedWeeklyPage の入口で `user?.plan !== 'premium'` なら `PremiumLock` を返す。**一覧取得の useQuery は `enabled: user?.plan === 'premium'` にして free では叩かない**（free は 403 になるため無駄打ち回避）。フックは早期 return 前に呼ぶ。
- [ ] **Step 4: 通過を確認 → 🟢 コミット** — `npm test -- SavedWeeklyPage` + `tsc -b` + lint → `git commit -m "feat: SavedWeeklyPage を premium ゲートする"`

---

### Task 9: E2E — free はロック / premium は週間が使える

**Files:**
- Create: `frontend/e2e/weekly-premium.spec.ts`

- [ ] **Step 1: E2E を書く**

```ts
import { execSync } from 'node:child_process'
import { test, expect } from '@playwright/test'
import { uniqueEmail, signUp } from './helpers'

test('free は週間がロックされ、premium 付与後に使える', async ({ page }) => {
  const email = uniqueEmail('weekly-premium')
  await signUp(page, email)

  // free: /weekly はロック
  await page.goto('/weekly')
  await expect(page.getByText(/1週間まとめて|プレミアム/)).toBeVisible()
  await expect(page.getByRole('button', { name: '1週間分を作る' })).toHaveCount(0)

  // premium 付与 → リロード
  execSync(`docker compose run --rm backend go run ./cmd/grant -email=${email} -months=1`, { cwd: '..', stdio: 'inherit' })
  await page.reload()

  // premium: 生成UIが出て作れる
  await page.getByRole('button', { name: '1週間分を作る' }).click()
  await expect(page.getByRole('listitem')).toHaveCount(7)
})
```

- [ ] **Step 2: 実行して通す** — `make up && make migrate && make seed` の後 `cd frontend && npx playwright test e2e/weekly-premium.spec.ts`。落ちたらセレクタ/タイミングを直す（既存 spec の流儀に合わせる）。スタックが上がらない環境なら BLOCKED を報告（テストは書いてコミット）。
- [ ] **Step 3: コミット** — `git commit -m "test: 週間の premium ゲートのE2E"`

---

### Task 10: `spec.md` を更新する

**Files:**
- Modify: `spec.md`

- [ ] **Step 1: spec.md を直す**（旧「永続化だけ premium」を上書き）:
  - 週間献立の章: **premium 限定**であることを明記（提案・引き直し・保存・週間の買い物リスト）。未ログイン401/free403。free/未ログインはロック付きプレビュー。
  - 単発（1食分）は free で不変であることを明記。
  - プレミアムの節（2.11 付近）の free/premium 表を本計画 §4 の線引きに更新。
  - API 一覧（5章）: `suggest-weekly`/`reroll-day`/`weekly-menus`系/`weekly-menus/:id/shopping-list` の認証・認可（要ログイン＋premium）を反映。
  - 設計ドキュメント `2026-07-24-weekly-premium-retier-design.md` を参照として明記。**推測で仕様を足さない**。
- [ ] **Step 2: コミット** — `git commit -m "docs: 週間まわりを premium 限定にする線引きを仕様に反映"`

---

## Self-Review

**1. 仕様カバレッジ（設計 → タスク）:**
- §3.1 深さで課金（単発free/週間premium）→ Task 3〜8 全体。単発不変は Global Constraints で明示。✅
- §3.2 ロック付きプレビュー → Task 6/7/8。✅
- §3.3 未ログイン401/free403 → Task 2（middleware）+ 各 handler タスクのテスト。✅
- §3.4 grandfather しない → 追加実装なし（free は 403 で一覧見えない・データ保持）。Task 4 の挙動がこれを満たす。✅
- §3.5 単発買い物リスト/手持ち食材 free 据え置き → 触らない（Global Constraints）。✅
- §5 CanUseWeeklyPlanning → Task 1。✅ / §6 enforcement 各エンドポイント → Task 3/4/5。✅
- §7 フロント → Task 6/7/8。✅ / §8 テスト → 各タスク＋Task 9。✅ / §10 spec → Task 10。✅

**2. プレースホルダ点検:** コード付きステップは実コードを載せた。テストヘルパ（`newMenuHandlerForTest`/`savedWeeklyApp`/`doAuthedGet`/`respondMe` 等）は**既存のものに合わせる**と明記（現物を読んで踏襲）。middleware テストの userID 注入方法は middleware.go の実キーに合わせる指示。

**3. 型の一貫性:**
- `service.Entitlements`（`For`）を middleware・3ハンドラ constructor で一貫使用。✅
- `RequirePremium(entitlements service.Entitlements)`（Task 2 → 3/4/5 で適用）一致。✅
- `service.ErrPremiumRequired`(403) / `auth.ErrTokenInvalid`(401) 既存写像を使用（追加不要）。✅
- `Entitlement.CanUseWeeklyPlanning()`（Task 1 → middleware）一致。✅
- フロント `PremiumLock`（Task 6 → 7/8）、`user?.plan === 'premium'` 判定一致。✅

**要確認（実装時の現物合わせ）:**
- `middleware.go` の userID コンテキストキー名（`UserIDFromContext` の裏）。Task 2 のテストの注入方法。
- 各 handler テストの既存セットアップ（echo 組み立て・認証 Cookie・`savedWeeklyApp`/`savedShoppingListApp` のシグネチャ拡張）。
- `suggest-weekly`/`reroll-day` の**既存テストで「公開前提」のもの**（未認証で200/422 を期待している箇所）を 401 期待に更新（仕様変更）。
- 履歴記録が weekly の auth 化後も動くこと（userID 常在で従来どおり）。

---

## 実行の引き継ぎ

計画は `docs/superpowers/plans/2026-07-24-weekly-premium-retier.md`。base は現在の `main`、作業は `feat/weekly-premium`。`feat/weekly-premium`→`main` は本番境界（人間がマージ）。

**1. Subagent-Driven（推奨）** / **2. Inline Execution** のどちらで実装するか。
