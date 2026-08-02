# サブスクリプション撤廃 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** プレミアム限定を撤廃し、週間献立・その保存・買い物リストの永続化を、ログインすれば誰でも使える状態にする。

**Architecture:** 権限判定は `domain.Entitlement` の3メソッドに集約されている。そこだけを常に「使える」に変え、`RequirePremium` ミドルウェアや service の権限チェックは呼び出し位置ごと残す（配管を残し、復活を1ファイルの復元で済ませる）。frontend の課金画面はルートごと削除する。

**Tech Stack:** Go 1.23 / Echo / testify、React 19 / TypeScript / TanStack Query / react-router / Vitest / MSW / Playwright、PostgreSQL

**設計:** `docs/superpowers/specs/2026-08-02-remove-subscription-design.md`

**ブランチ:** `feat/remove-subscription`（`main` = b9eea64 から分岐済み）

## Global Constraints

- 保存上限は**全員 50件**。`freeSavedWeeklyMenuLimit` / `premiumSavedWeeklyMenuLimit` の2定数は `savedWeeklyMenuLimit = 50` の1つに統合する。
- backend の課金配管は**削除しない**。`RequirePremium`、service の権限チェック、`handler/billing.go`、`service/billing.go`、`service/subscription.go`、`repository/subscription.go`、`repository/payment_stripe.go`、`cmd/grant`、migration `000010` / `000012`、`api/openapi.yaml`、`.env.example` の `STRIPE_*`、Makefile の `grant` / `revoke` ターゲットはすべて据え置く。
- `cmd/server/main.go` の Stripe 環境変数チェック（未設定なら起動失敗）も据え置く。
- `domain.Entitlement` の `Plan()` と非公開フィールド `plan` は残す。`/auth/me` が返し続けるため。
- 403 を検証していたテストは**削除せず「通る」に書き換える**。配管が全員を通すことを検証する価値があるため。
- **週間献立はログイン必須のまま。** `menu.go` の `suggest-weekly` / `reroll-day` は `RequireAuth` → `RequirePremium` の順で守られており、プレミアムのゲートを外しても `RequireAuth` は残る。よって frontend の `/weekly` は `RequireAuth` で包む（Task 2）。backend のルーティングは変更しない。
- コミットメッセージは日本語。本文には「なぜそうしたか」を書く。末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける。
- テストコマンド: `make test-backend` / `make test-frontend` / `make test` / `make lint`。

---

## File Structure

| ファイル | 責務 | 変更 |
| --- | --- | --- |
| `backend/internal/domain/entitlement.go` | 権限の唯一の判定点 | 修正（本計画で唯一のプロダクションコード変更） |
| `backend/internal/domain/entitlement_test.go` | 上記の検証 | 書き換え |
| `backend/internal/handler/{middleware,menu,saved_weekly,saved_shopping_list}_test.go` | 配管が全員を通すことの検証 | 書き換え |
| `backend/internal/service/{saved_weekly,saved_shopping_list}_test.go` | 上限とオーバーレイの検証 | 書き換え |
| `frontend/src/features/menu/WeeklyPage.tsx` | 週間献立の画面 | ゲート削除 |
| `frontend/src/features/menu/SavedWeeklyPage.tsx` | 保存一覧の画面 | ゲート削除 |
| `frontend/src/features/menu/ShoppingListPage.tsx` | 買い物リストの画面 | 永続化条件の変更、勧誘の削除 |
| `frontend/src/features/{billing,pricing,premium}/` | 課金画面 | ディレクトリごと削除 |
| `frontend/src/hooks/useOnceFlag.{ts,test.ts}` | 勧誘を1回だけ出すフック | 削除 |
| `frontend/src/app/App.tsx` | ルーティング | 課金・特商法のルート削除 |
| `frontend/src/components/Footer.tsx` | 常設フッター | 料金・特商法のリンク削除 |
| `frontend/src/features/auth/AuthMenu.tsx` | ヘッダの認証メニュー | プレミアムバッジと `/account` 削除 |
| `frontend/src/features/home/HomePage.tsx` | ホーム | 料金への常設導線を削除 |
| `frontend/src/test/handlers.ts` | MSW の既定ハンドラ | `/billing/plan` 削除 |
| `frontend/e2e/*.spec.ts` | 通しの検証 | premium 付与の削除、加入導線の spec 削除 |
| `frontend/src/features/legal/content/terms.md` | 利用規約の正文 | 第4条削除と条番号の繰り上げ |
| `spec.md` / `README.md` / `DEPLOY.md` / `docs/manual-e2e-payment.md` | ドキュメント | 無料前提に更新 |

---

## Task 1: backend のゲート解放

**Files:**
- Modify: `backend/internal/domain/entitlement.go`
- Test: `backend/internal/domain/entitlement_test.go`
- Test: `backend/internal/handler/middleware_test.go:52`
- Test: `backend/internal/handler/menu_test.go:1137,1187`
- Test: `backend/internal/handler/saved_weekly_test.go:415,429,443`
- Test: `backend/internal/handler/saved_shopping_list_test.go:207,233`
- Test: `backend/internal/service/saved_weekly_test.go:304,317,328`
- Test: `backend/internal/service/saved_shopping_list_test.go:80,156`

**Interfaces:**
- Consumes: なし（最初のタスク）
- Produces: `domain.Entitlement` の以下の振る舞い。frontend / e2e のタスクはこれを前提にする。
  - `SavedWeeklyMenuLimit() int` — プランによらず常に `50`
  - `CanPersistShoppingList() bool` — 常に `true`
  - `CanUseWeeklyPlanning() bool` — 常に `true`
  - `Plan() domain.Plan` — 変更なし（premium 以外は `PlanFree`）

このタスクは backend のテスト全体（`make test-backend`）が緑になるところまでを1単位とする。domain だけ先に変えると 403 を期待する handler / service のテストが落ち、途中のコミットが壊れた状態になるため。

- [ ] **Step 1: `entitlement_test.go` を書き換えて失敗させる**

`backend/internal/domain/entitlement_test.go` を以下で全置換する。

```go
package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// サブスク撤廃後、上限はプランによらず一律。
// プランごとに分けていた頃の名残（free=10）が残っていないことを、
// free / premium / ゼロ値のすべてで確かめる。
func TestEntitlement_保存上限はプランによらず50(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ent  domain.Entitlement
	}{
		{"free", domain.NewEntitlement(domain.PlanFree)},
		{"premium", domain.NewEntitlement(domain.PlanPremium)},
		{"未知のプラン", domain.NewEntitlement(domain.Plan("pro"))},
		// ゼロ値は取得し忘れを表す。撤廃後は誰も締め出さないため、
		// ここも 50 でなければならない（0 だと1件も保存できなくなる）。
		{"ゼロ値", domain.Entitlement{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, 50, tt.ent.SavedWeeklyMenuLimit())
		})
	}
}

func TestEntitlement_買い物リストの永続化は誰でもできる(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ent  domain.Entitlement
	}{
		{"free", domain.NewEntitlement(domain.PlanFree)},
		{"premium", domain.NewEntitlement(domain.PlanPremium)},
		{"ゼロ値", domain.Entitlement{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.True(t, tt.ent.CanPersistShoppingList())
		})
	}
}

func TestEntitlement_週間献立は誰でも使える(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ent  domain.Entitlement
	}{
		{"free", domain.NewEntitlement(domain.PlanFree)},
		{"premium", domain.NewEntitlement(domain.PlanPremium)},
		{"ゼロ値", domain.Entitlement{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.True(t, tt.ent.CanUseWeeklyPlanning())
		})
	}
}

// Plan は撤廃後も残す。/auth/me が返し続けるフィールドであり、
// DB の subscriptions.plan をそのまま表す役割は変わらない。
func TestEntitlement_Planを返す(t *testing.T) {
	t.Parallel()

	require.Equal(t, domain.PlanPremium,
		domain.NewEntitlement(domain.PlanPremium).Plan())
	require.Equal(t, domain.PlanFree,
		domain.NewEntitlement(domain.PlanFree).Plan())
	require.Equal(t, domain.PlanFree,
		domain.NewEntitlement(domain.Plan("pro")).Plan(),
		"未知のプランは free に落ちる")

	var zero domain.Entitlement
	require.Equal(t, domain.PlanFree, zero.Plan())
}
```

- [ ] **Step 2: 落ちることを確認する**

```bash
docker compose run --rm backend go test ./internal/domain/ -run TestEntitlement -v
```

Expected: FAIL。`保存上限はプランによらず50/free` が `10 != 50`、`買い物リストの永続化は誰でもできる/free` と `週間献立は誰でも使える/free` が `false != true` で落ちる。

- [ ] **Step 3: `entitlement.go` を書き換える**

`backend/internal/domain/entitlement.go` を以下で全置換する。

```go
package domain

// 週間献立の保存上限。プランによらず一律（spec.md 2.8）。
//
// 上限値は仕様であってデータなので、DBではなくコードに置く。
// DBに置くと変更のたびにマイグレーションが要り、テストもDBの状態に依存する。
//
// サブスク撤廃前は free=10 / premium=50 に分かれていた。撤廃にあたり、
// 加入者が損をしない側（50）に寄せて1つにまとめた。
const savedWeeklyMenuLimit = 50

// Entitlement は「今この利用者が何をどれだけ使えるか」を表す。
//
// サブスク撤廃により、機能の可否はプランに依存しなくなった。それでも型と
// メソッドは残す。判定点をここ1箇所に保っておけば、将来サブスクを再開する
// ときにこのファイルを戻すだけで済み、RequirePremium や service の
// 権限チェック（呼び出し位置）を探し直さずに済むため。
type Entitlement struct {
	plan Plan
}

// NewEntitlement はプランから Entitlement を組み立てる。
func NewEntitlement(p Plan) Entitlement {
	return Entitlement{plan: p}
}

// Plan は契約プランを返す。
// premium 以外は全て free として扱う（ゼロ値・DBの想定外の値を含む）。
//
// 機能の可否には使われなくなったが、/auth/me が返すため残す。
func (e Entitlement) Plan() Plan {
	if e.plan == PlanPremium {
		return PlanPremium
	}
	return PlanFree
}

// SavedWeeklyMenuLimit は保存できる週間献立の件数を返す。
func (e Entitlement) SavedWeeklyMenuLimit() int {
	return savedWeeklyMenuLimit
}

// CanPersistShoppingList は買い物リストの差分を保存できるかを返す。
// サブスク撤廃により誰でも保存できる。
func (e Entitlement) CanPersistShoppingList() bool {
	return true
}

// CanUseWeeklyPlanning は週間献立の計画一式（提案・保存・週間の買い物リスト）を
// 使えるかを返す。サブスク撤廃により誰でも使える。
func (e Entitlement) CanUseWeeklyPlanning() bool {
	return true
}
```

- [ ] **Step 4: domain のテストが通ることを確認する**

```bash
docker compose run --rm backend go test ./internal/domain/ -run TestEntitlement -v
```

Expected: PASS（4関数すべて）。

- [ ] **Step 5: backend 全体を走らせ、落ちるテストを洗い出す**

```bash
make test-backend
```

Expected: FAIL。403 と旧上限を期待している以下が落ちる。次のステップで書き換える。

- [ ] **Step 6: handler のテストを「通る」に書き換える**

いずれも「free でも成功する」ことを検証する形に変える。テスト関数名も実態に合わせて改名する。

`backend/internal/handler/middleware_test.go` — 52-66行の `TestRequirePremium_freeは403` を次で置き換える。

```go
// サブスク撤廃後、RequirePremium は誰も止めない。ミドルウェアは復活に備えて
// 配線ごと残してあるため、「free も通す」ことを検証し続ける。
func TestRequirePremium_freeも通す(t *testing.T) {
	t.Parallel()

	e, tokens, called := premiumRoute(t, fakeEntitlements{plan: domain.PlanFree})
	access, err := tokens.Issue("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/premium-only", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, *called)
}
```

`TestRequirePremium_プレミアムは通す` / `_userID無しは401` / `_エンタイトルメント引き当て失敗はそのままエラーを返す` の3つは**変更しない**。認証と障害時の振る舞いは撤廃と無関係のため。

`backend/internal/handler/menu_test.go` — `TestSuggestWeekly_freeは403` → `TestSuggestWeekly_freeも200`、`TestRerollDay_freeは403` → `TestRerollDay_freeも200`。`http.StatusForbidden` を `http.StatusOK` に変え、既存の `_premiumは200` 版と同じレスポンス検証を行う。

`backend/internal/handler/saved_weekly_test.go` — `TestSavedWeeklyMenus_Save_freeは403` / `_List_freeは403` / `_Delete_freeは403` を `_freeも成功する` に改名し、期待ステータスをそれぞれ既存の premium 版と揃える。

`backend/internal/handler/saved_shopping_list_test.go` — `TestSavedShoppingListHandler_Get_freeは403` / `_Put_freeは403` を `_freeも成功する` に改名し、期待ステータスを premium 版と揃える。

各ファイルで、書き換えたテストの直上に理由をコメントで残す（「サブスク撤廃により free も通る。配管は残しているため検証も残す」）。

- [ ] **Step 7: service のテストを書き換える**

`backend/internal/service/saved_weekly_test.go`
- `TestSavedWeekly_freeは10件で断る` → `TestSavedWeekly_freeも50件までは保存できる`。free の Entitlement で10件保存済みの状態から11件目が成功することを検証する。
- `TestSavedWeekly_premiumは10件でも保存できる` は上と重複するため削除する。
- `TestSavedWeekly_premiumは50件で断る` → `TestSavedWeekly_50件で断る`。プランに触れない名前にし、free の Entitlement でも50件で `ErrSavedWeeklyMenuLimitReached` が返ることを検証する。

`backend/internal/service/saved_shopping_list_test.go`
- `TestSavedShoppingListService_For_freeは導出そのまま` → `TestSavedShoppingListService_For_freeも差分を重ねる`。free でもオーバーレイが適用されることを検証する（`_premiumは差分を重ねる` と同じ期待値）。
- `TestReplaceOverrides_freeは403` → `TestReplaceOverrides_freeも保存する`。`ErrForbidden` ではなく保存が成功することを検証する。

`backend/internal/service/entitlement_test.go` は**変更しない**。`EntitlementService.For` が返す `Plan` の判定（期限切れ・canceled・trialing）は撤廃後も同じ振る舞いのため。

- [ ] **Step 8: backend 全体が通ることを確認する**

```bash
make test-backend
```

Expected: PASS。

```bash
make lint
```

Expected: PASS。未使用シンボルの警告が出たら、それは配管として残す方針のものか確認する。`Entitlement` のメソッドが引数レシーバを使わなくなるため `unused-receiver` 系の指摘が出る場合は、`golangci-lint` の設定を変えるのではなくレシーバ名を `_` にはせず現状維持でよいか確認したうえで対応する。

- [ ] **Step 9: コミット**

```bash
git add backend/internal/domain/entitlement.go backend/internal/domain/entitlement_test.go \
  backend/internal/handler/middleware_test.go backend/internal/handler/menu_test.go \
  backend/internal/handler/saved_weekly_test.go backend/internal/handler/saved_shopping_list_test.go \
  backend/internal/service/saved_weekly_test.go backend/internal/service/saved_shopping_list_test.go
git commit -F - <<'EOF'
feat: プレミアム限定を外し全機能を開放する

Entitlement の3メソッドを常に「使える」に変え、保存上限は加入者が損を
しない 50 に一本化した。判定はこの型に集約されているため、変更は
domain/entitlement.go 1ファイルで足りる。

RequirePremium や service の権限チェックは呼び出し位置ごと残す。
呼び出し側から削除すると変更が handler と service に散り、将来サブスクを
再開するときに差し込み位置を探し直すことになるため。

403 を検証していたテストは削除せず「free でも通る」に書き換えた。
配管が全員を通すことは、配管が残っている限り検証する価値がある。

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 2: 週間献立と保存一覧のゲート解除

**Files:**
- Modify: `frontend/src/features/menu/WeeklyPage.tsx:11,116-127`
- Modify: `frontend/src/features/menu/SavedWeeklyPage.tsx:10,64-65,84-93`
- Modify: `frontend/src/app/App.tsx:96,102-103`（`/weekly` を `RequireAuth` で包む）
- Test: `frontend/src/features/menu/WeeklyPage.test.tsx`
- Test: `frontend/src/features/menu/SavedWeeklyPage.test.tsx`
- Test: `frontend/src/app/App.auth.test.tsx`

**Interfaces:**
- Consumes: Task 1 の backend 挙動（free でも `suggest-weekly` / 保存 / 一覧が 200）
- Produces: `WeeklyPage` と `SavedWeeklyPage` が `PremiumLock` を import しない状態。Task 4 が `features/premium/` を削除できる前提になる。

**このタスクの要点:** `suggest-weekly` / `reroll-day` は backend で `RequireAuth` に守られており、プレミアムのゲートを外しても未ログインでは 401 になる。`WeeklyPage` のゲートを外すだけだと、未ログインの人に入力フォームが見えて送信で初めて失敗する。だから `/weekly` を `RequireAuth` で包む。

- [ ] **Step 1: WeeklyPage のテストを書き換えて失敗させる**

`frontend/src/features/menu/WeeklyPage.test.tsx` の「free ならロックが出る」趣旨のケースを、次の趣旨に置き換える。

同ファイルには `respondMe(plan?: 'free' | 'premium')` という既存ヘルパがある（`server.use` で `/api/v1/auth/me` を差し替える。引数なしなら未ログイン＝既定の401）。これを使う。

```tsx
it('free でも週間献立の画面が出る', async () => {
  // 撤廃前は PremiumLock が出ていた位置に、本文が出る。
  respondMe('free')
  renderWithProviders(<WeeklyPage />)

  expect(
    await screen.findByRole('heading', { name: '1週間の献立' }),
  ).toBeInTheDocument()
  expect(screen.queryByText('1週間まとめて計画')).not.toBeInTheDocument()
})

```

同ファイルの `respondMe('premium')` を使っているケースは、すべて `respondMe('free')` に変える。プランで挙動が変わらないことを示すため。

**未ログインのケースは `WeeklyPage` 単体では検証しない。** 未ログインを弾くのは `/weekly` を包む `RequireAuth` の仕事であり、Step 10 の `App.auth.test.tsx` で検証する。同ファイルに `respondMe()` を呼ばない（＝未ログイン）ケースが既にあれば削除する。

- [ ] **Step 2: 落ちることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/features/menu/WeeklyPage.test.tsx
```

Expected: FAIL。見出しが見つからず `1週間まとめて計画`（PremiumLock の title）が出ている。

- [ ] **Step 3: WeeklyPage からゲートを削除する**

`frontend/src/features/menu/WeeklyPage.tsx` から次を削除する。

- 11行目の `import { PremiumLock } from '../premium/PremiumLock'`
- 116-127行の `if (user?.plan !== 'premium') { return <PremiumLock ... /> }` ブロックとその上のコメント

`useCurrentUser()` の呼び出しが他で使われていなければ、その行と import も削除する。使われていれば残す。

- [ ] **Step 4: 通ることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/features/menu/WeeklyPage.test.tsx
```

Expected: PASS。

- [ ] **Step 5: SavedWeeklyPage のテストを書き換えて失敗させる**

`frontend/src/features/menu/SavedWeeklyPage.test.tsx` にも `respondMe(plan: 'free' | 'premium')` ヘルパが44行目にある。93行の `respondMe('free')` を使っているケース（ロックが出ることの検証）を次で置き換える。

```tsx
it('free でも保存した週間献立の一覧が出る', async () => {
  respondMe('free')
  renderWithProviders(<SavedWeeklyPage />)

  expect(
    await screen.findByRole('heading', { name: '保存した週間献立' }),
  ).toBeInTheDocument()
  // PremiumLock の title が出ていないこと。
  expect(screen.queryByText('プレミアムにアップグレード')).not.toBeInTheDocument()
})
```

110行以降の `respondMe('premium')` を使っているケースは、すべて `respondMe('free')` に変える。

- [ ] **Step 6: 落ちることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/features/menu/SavedWeeklyPage.test.tsx
```

Expected: FAIL。

- [ ] **Step 7: SavedWeeklyPage からゲートを削除する**

`frontend/src/features/menu/SavedWeeklyPage.tsx`:

- 10行目の `import { PremiumLock } from '../premium/PremiumLock'` を削除
- 84-93行の `if (user?.plan !== 'premium') { ... }` ブロックとその上のコメントを削除
- 64-65行のクエリの `enabled` を、プランではなくログイン有無で判定する形に変える。

```tsx
  } = useQuery({
    queryKey: savedWeeklyMenusQueryKey,
    queryFn: fetchSavedWeeklyMenus,
    // 未ログインでは 401 になるため、無駄打ちを避けて取得しない。
    // （この画面は RequireAuth の内側だが、判定が付くまでの一瞬がある）
    enabled: user != null,
  })
```

- [ ] **Step 8: 通ることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/features/menu/SavedWeeklyPage.test.tsx
```

Expected: PASS。

- [ ] **Step 9: `/weekly` が未ログインを弾くテストを書いて失敗させる**

`frontend/src/app/App.auth.test.tsx` に次を足す。文言と組み立ては、同ファイルにある `/histories` や `/favorites` の `RequireAuth` を検証している既存ケースに揃える。

```tsx
it('未ログインで /weekly を開くとログイン画面へ送られる', async () => {
  // suggest-weekly は backend で RequireAuth に守られており、
  // フォームを見せても送信で 401 になる。先にログインへ送る。
  renderWithProviders(<App />, { route: '/weekly' })

  expect(
    await screen.findByRole('heading', { name: 'ログイン' }),
  ).toBeInTheDocument()
})
```

- [ ] **Step 10: 落ちることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/app/App.auth.test.tsx
```

Expected: FAIL。ログイン画面ではなく週間献立の画面が出る。

- [ ] **Step 11: `/weekly` を RequireAuth で包む**

`frontend/src/app/App.tsx` の96行を次に置き換える。

```tsx
            {/* 週間献立は backend の suggest-weekly / reroll-day が RequireAuth で
                守られているため、未ログインでは使えない。フォームを見せてから
                401 で断るより、先にログインへ送る。 */}
            <Route
              path="/weekly"
              element={
                <RequireAuth>
                  <WeeklyPage />
                </RequireAuth>
              }
            />
```

あわせて102-103行のコメント「検索と週間献立は未認証でも使える（spec.md 1.3）」を「検索は未認証でも使える（spec.md 1.3）」に直す。プレミアム化以前の記述が残っていたもの。

- [ ] **Step 12: 通ることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/app/App.auth.test.tsx
make test-frontend
```

Expected: 両方 PASS。

- [ ] **Step 13: コミット**

```bash
git add frontend/src/features/menu/WeeklyPage.tsx frontend/src/features/menu/WeeklyPage.test.tsx \
  frontend/src/features/menu/SavedWeeklyPage.tsx frontend/src/features/menu/SavedWeeklyPage.test.tsx \
  frontend/src/app/App.tsx frontend/src/app/App.auth.test.tsx
git commit -F - <<'EOF'
feat: 週間献立と保存一覧のプレミアム限定を外す

PremiumLock による差し替えをやめ、プランを見ずに本文を描画する。
保存一覧の取得条件はプランからログイン有無に変えた。この画面は
RequireAuth の内側にあるが、判定が付くまでの一瞬に未認証で取りに行くと
401 を無駄に踏むため。

/weekly は RequireAuth で包んだ。suggest-weekly と reroll-day は backend で
RequireAuth に守られており、プレミアム限定を外しても未ログインでは 401 になる。
包まずにゲートだけ外すと、未ログインの人にフォームが見えて送信で初めて
失敗する画面になるため。

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 3: 買い物リストの永続化を開放し勧誘を削除する

**Files:**
- Modify: `frontend/src/features/menu/ShoppingListPage.tsx:15,109-112,181-184,206-212,380-397`
- Delete: `frontend/src/hooks/useOnceFlag.ts`
- Delete: `frontend/src/hooks/useOnceFlag.test.ts`
- Test: `frontend/src/features/menu/ShoppingListPage.test.tsx`

**Interfaces:**
- Consumes: Task 1 の backend 挙動（free でも `PUT /saved-weekly-menus/:id/shopping-list` が 200）
- Produces: `ShoppingListPage` が `/checkout` へのリンクと `useOnceFlag` を持たない状態。Task 4 が `features/billing/` を削除できる前提になる。

`useOnceFlag` は `ShoppingListPage` の `useOnceFlag('premium-shopping')` からしか使われていない。プレミアム勧誘を1回だけ出すために導入されたフックであり、勧誘と同時に役目を終えるため削除する。

- [ ] **Step 1: テストを書き換えて失敗させる**

`frontend/src/features/menu/ShoppingListPage.test.tsx` の「free がチェックすると勧誘が出る」ケースを削除し、次を足す。

同ファイルには `respondMe(plan)`（13行）と `withSavedId(id)`（76行、`sessionStorage` に `menu-planner:weekly.savedId` を書く）のヘルパがある。永続化の API は `/api/v1/weekly-menus/:id/shopping-list`（`saved-weekly-menus` ではない）。230行以降の「free は PUT しない」趣旨のケースを、次で置き換える。

```tsx
it('free でも保存済みの週ならチェックがサーバに送られる', async () => {
  const savedId = '11111111-1111-1111-1111-111111111111'
  withSavedId(savedId)
  respondMe('free')

  let putCalled = false
  server.use(
    http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
      HttpResponse.json({ items: [] }),
    ),
    http.put(`/api/v1/weekly-menus/${savedId}/shopping-list`, () => {
      putCalled = true
      return new HttpResponse(null, { status: 204 })
    }),
  )

  renderWithProviders(<ShoppingListPage />)
  await userEvent.click(await screen.findByRole('checkbox', { name: /たまねぎ/ }))

  await waitFor(() => expect(putCalled).toBe(true))
})

it('チェックしてもプレミアムの勧誘は出ない', async () => {
  respondMe('free')
  renderWithProviders(<ShoppingListPage />)

  await userEvent.click(await screen.findByRole('checkbox', { name: /たまねぎ/ }))

  expect(
    screen.queryByText(/プレミアムプランなら、チェックした買い物リスト/),
  ).not.toBeInTheDocument()
})
```

GET のレスポンス形と品目名（`たまねぎ`）は、同ファイル141行以降の既存ケースが組み立てている献立・食材に合わせる。既存が使っている食材名が違う場合はそちらに寄せる。

- [ ] **Step 2: 落ちることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/features/menu/ShoppingListPage.test.tsx
```

Expected: FAIL。free では PUT が飛ばず、勧誘が出る。

- [ ] **Step 3: ShoppingListPage を修正する**

109-112行を次に変える。

```tsx
  // 保存済みの週のときだけ、チェックはサーバに残る。
  // 未保存の週はその場限りで、画面を離れると消える。
  //
  // savedId は保存時か保存済みの週を開いたときにだけ入り、ログアウトで
  // clearSessionState() により消える。したがって savedId != null は
  // ログイン済みを含意する。
  const { user } = useCurrentUser()
  const canPersist = savedId != null
```

`user` が他で使われていなければ、`useCurrentUser` の呼び出しと import も削除する。

15行目の `import { useOnceFlag } from '../../hooks/useOnceFlag'` を削除する。

181-184行の勧誘の state を削除する。

```tsx
  // 削除する:
  // const [guidanceDone, markGuidance] = useOnceFlag('premium-shopping')
  // const [showGuidance, setShowGuidance] = useState(false)
```

186行のコメント「追加フォームの入力値。premium×保存済みのときだけ表示する。」を「追加フォームの入力値。保存済みの週のときだけ表示する。」に直す。

119行のコメント「manual は手動追加した品目（premium×保存済みのみ）」を「manual は手動追加した品目（保存済みの週のみ）」に直す。

206-212行の `toggle` の分岐から勧誘の枝を落とす。

```tsx
    setChecked(next)
    if (canPersist) {
      persist.mutate(buildOverlay(next, hidden, manual))
    }
```

`adding` 変数が他で使われていなければ、198行の宣言も削除する。

380-397行の `{showGuidance && ( ... )}` ブロックを丸ごと削除する。これにより `Link` の import が未使用になれば、それも削除する。

- [ ] **Step 4: 通ることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/features/menu/ShoppingListPage.test.tsx
```

Expected: PASS。

- [ ] **Step 5: useOnceFlag を削除する**

```bash
git rm frontend/src/hooks/useOnceFlag.ts frontend/src/hooks/useOnceFlag.test.ts
```

- [ ] **Step 6: 型チェックと Lint を含めて frontend 全体を確認する**

```bash
make test-frontend
```

Expected: このタスクの範囲では PASS。まだ Task 4 の対象（`App.checkout.test.tsx` など）は触っていないが、それらは削除される画面のテストであり現時点では通っている。落ちた場合は原因が本タスクの変更かを確認する。

- [ ] **Step 7: コミット**

```bash
git add frontend/src/features/menu/ShoppingListPage.tsx frontend/src/features/menu/ShoppingListPage.test.tsx
git commit -F - <<'EOF'
feat: 買い物リストの永続化を全員に開放し勧誘を削除する

チェックの永続化条件からプランを外し、保存済みの週かどうかだけで
決めるようにした。savedId はログアウト時に clearSessionState() で
消えるため、savedId があることはログイン済みを含意する。

プレミアム勧誘のバナーと、それを1回だけ出すための useOnceFlag を
削除した。useOnceFlag は 'premium-shopping' からしか使われておらず、
勧誘と同時に役目を終えるため。

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 4: 課金画面とその導線の削除

**Files:**
- Delete: `frontend/src/features/billing/`（`AccountPage.tsx` / `AccountPage.test.tsx` / `CheckoutPage.tsx` / `CheckoutPage.test.tsx` / `CheckoutCompletePage.tsx` / `CheckoutCompletePage.test.tsx` / `api.ts`）
- Delete: `frontend/src/features/pricing/`（`PricingPage.tsx` / `PricingPage.test.tsx`）
- Delete: `frontend/src/features/premium/`（`PremiumLock.tsx` / `PremiumLock.test.tsx`）
- Delete: `frontend/src/app/App.checkout.test.tsx`
- Modify: `frontend/src/app/App.tsx:9-11,24,129-157`
- Modify: `frontend/src/components/Footer.tsx:3-10`
- Modify: `frontend/src/features/auth/AuthMenu.tsx:55-74`
- Modify: `frontend/src/features/home/HomePage.tsx:141-157`
- Modify: `frontend/src/test/handlers.ts:41-45`
- Test: `frontend/src/app/App.auth.test.tsx`
- Test: `frontend/src/features/home/HomePage.test.tsx`
- Test: `frontend/src/features/auth/AuthMenu.test.tsx`

**Interfaces:**
- Consumes: Task 2 と Task 3（`PremiumLock` と `/checkout` リンクの参照がすべて消えていること）
- Produces: `/checkout` `/checkout/complete` `/account` `/pricing` が 404 になる状態。Task 5 の e2e がこれを前提にする。

`/legal/tokushoho` のルートと導線は Task 6（法務）で扱う。ここでは触らない。

- [ ] **Step 1: ルートが 404 になるテストを書いて失敗させる**

`frontend/src/app/App.auth.test.tsx` に次を足す（既存の課金導線を検証しているケースは削除する）。

```tsx
it.each(['/checkout', '/checkout/complete', '/account', '/pricing'])(
  '%s は撤廃済みで 404 になる',
  async (path) => {
    renderWithProviders(<App />, { route: path })

    expect(await screen.findByText('ページが見つかりません')).toBeInTheDocument()
  },
)
```

`renderWithProviders` の route 指定方法と 404 画面の文言は `frontend/src/components/NotFoundPage.tsx` と既存テストに合わせる。

- [ ] **Step 2: 落ちることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/app/App.auth.test.tsx
```

Expected: FAIL。各パスで課金画面が描画され 404 にならない。

- [ ] **Step 3: 課金画面のディレクトリを削除する**

```bash
git rm -r frontend/src/features/billing frontend/src/features/pricing frontend/src/features/premium
git rm frontend/src/app/App.checkout.test.tsx
```

- [ ] **Step 4: App.tsx からルートと import を削除する**

削除する import（9-11行、24行）:

```tsx
import { AccountPage } from '../features/billing/AccountPage'
import { CheckoutCompletePage } from '../features/billing/CheckoutCompletePage'
import { CheckoutPage } from '../features/billing/CheckoutPage'
import { PricingPage } from '../features/pricing/PricingPage'
```

削除する `<Route>`（129-157行のうち課金分）: `/checkout`、`/checkout/complete`、`/account`、`/pricing` の4つと、それぞれの上のコメント。

`/legal/tokushoho` は Task 6 まで残す。

- [ ] **Step 5: Footer から料金プランのリンクを削除する**

`frontend/src/components/Footer.tsx` の `footerLinks` から `/pricing` の行を削除し、コメントを直す。

```tsx
// footerLinks は常設の導線。法務ページは表示義務があり、
// どの画面からでも辿れる必要がある。
const footerLinks = [
  { to: '/legal/tokushoho', label: '特定商取引法に基づく表記' },
  { to: '/legal/terms', label: '利用規約' },
  { to: '/legal/privacy', label: 'プライバシーポリシー' },
] as const
```

`/legal/tokushoho` の行は Task 6 で消す。

- [ ] **Step 6: AuthMenu からバッジと /account を削除する**

`frontend/src/features/auth/AuthMenu.tsx` の55-74行を次に置き換える。

```tsx
  return (
    <span className="ml-auto flex items-center gap-3">
      <span className="text-sm text-kon-ink/80">{user.displayName}</span>
      <button
```

プレミアムバッジのブロック（55-67行）と `/account` への `Link`（69-74行）を削除する。`Link` が「ログイン」導線（45行付近）で使われているため、import は残す。

- [ ] **Step 7: HomePage から料金への常設導線を削除する**

`frontend/src/features/home/HomePage.tsx` の141-157行（コメントを含む `{!isLoading && user?.plan !== 'premium' && ( ... )}` ブロック）を丸ごと削除する。`isLoading` と `user` はエントリの `requiresAuth` 表示（130行）で使い続けるため残す。`Link` も同様に残す。

- [ ] **Step 8: MSW の既定ハンドラから /billing/plan を削除する**

`frontend/src/test/handlers.ts` の41-45行（`http.get('/api/v1/billing/plan', ...)` とその上のコメント）を削除する。

- [ ] **Step 9: 関連する既存テストから課金のケースを落とす**

- `frontend/src/features/home/HomePage.test.tsx` — 「プランを見る」導線を検証しているケースを削除し、代わりに `expect(screen.queryByRole('link', { name: 'プランを見る' })).not.toBeInTheDocument()` を確かめるケースを1つ置く。
- `frontend/src/features/auth/AuthMenu.test.tsx` — 「プレミアム」バッジと「アカウント設定」リンクを検証しているケースを削除する。
- `frontend/src/app/App.favorite-prompt.test.tsx` — 24行のコメント「週間献立は premium 限定＝Task 7 で未認証では使えなくなったため」が事実と合わなくなるため、コメントを現状に合わせて直す。テストの中身は変えない。
- `frontend/src/app/App.account-switch.test.tsx` は**触らない**。アカウント切替の検証で premium とは無関係のため。

- [ ] **Step 10: frontend 全体が通ることを確認する**

```bash
make test-frontend
```

Expected: PASS（型チェック・Lint・テストすべて）。未使用 import が残っていれば Lint が指摘する。

- [ ] **Step 11: コミット**

```bash
git add -A frontend/src
git commit -F - <<'EOF'
feat: 課金画面とその導線を削除する

/checkout、/checkout/complete、/account、/pricing のルートを外し、
features/billing・pricing・premium をディレクトリごと削除した。
ルートを消せばこれらは到達不能になり、残してもテストだけが宙に浮く。
逆にルートを残すと導線が無くても URL 直打ちで加入でき、撤廃と矛盾する。

フッターの「料金プラン」、ヘッダのプレミアムバッジと「アカウント設定」、
ホームの料金への常設導線、MSW の /billing/plan ハンドラも合わせて削除。

backend の課金配管は残してあるため、復活時に書き直すのは画面だけで済む。

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 5: e2e の書き換え

**Files:**
- Delete: `frontend/e2e/premium.spec.ts`
- Delete: `frontend/e2e/weekly-premium.spec.ts`
- Modify: `frontend/e2e/weekly.spec.ts:11-12,52`
- Modify: `frontend/e2e/saved-weekly.spec.ts:7,10-15,23,79-90,101,126,141`
- Modify: `frontend/e2e/shopping-list.spec.ts:11-12`
- Modify: `frontend/e2e/shopping-list-persist.spec.ts:7,13,21`
- Modify: `frontend/e2e/menu-role.spec.ts:97-98`

**Interfaces:**
- Consumes: Task 1〜4 のすべて（backend が誰でも通し、frontend に課金導線が無い状態）
- Produces: premium 付与に依存しない e2e 一式

`grantPremium` は共通ヘルパではなく、各 spec が個別に定義している（`docker compose run --rm backend go run ./cmd/grant -email=... -months=1` を `execSync` で叩く関数）。`frontend/e2e/helpers.ts` には無いため、削除は spec ごとに行う。

`backend/cmd/grant` と Makefile の `grant` / `revoke` ターゲットは**残す**。課金配管の一部であり、配管を残す方針と揃える。

- [ ] **Step 1: 加入導線の spec を削除する**

```bash
git rm frontend/e2e/premium.spec.ts frontend/e2e/weekly-premium.spec.ts
```

- [ ] **Step 2: 各 spec から premium 付与を落とす**

対象ファイルごとに、次の3つを削除する。

1. ファイル冒頭の `function grantPremium(email: string): void { ... }` 定義
2. テスト本文中の `grantPremium(email)` 呼び出し
3. `execSync` の import が未使用になればその import 行

あわせて、「premium 限定」と説明しているコメントを実態に合わせて直す。例（`weekly.spec.ts:11-12`）:

```ts
  // 週間献立の作成・引き直しはログインすれば誰でも使える。
```

`shopping-list-persist.spec.ts` は次の2点も直す。
- 7行目のテスト名 `'premium は保存済み週の買い物リストのチェックがリロード後も残る'` → `'保存済み週の買い物リストのチェックはリロード後も残る'`
- 21行目の `await expect(page.getByLabel('プレミアム会員')).toBeVisible()` を削除（バッジは Task 4 で消えている）

`saved-weekly.spec.ts` は79-90行の「未ログインには加入導線が出る」ケースを削除する。未ログインで `/saved-weekly` を開いた場合は `RequireAuth` によりログイン画面へ送られる、という趣旨のケースに置き換える。

```ts
test('未ログインで保存一覧を開くとログイン画面へ送られる', async ({ page }) => {
  await page.goto('/saved-weekly')

  await expect(page.getByRole('heading', { name: 'ログイン' })).toBeVisible()
})
```

見出しの文言は `frontend/src/features/auth/LoginPage.tsx` の実装に合わせる。

- [ ] **Step 3: e2e を走らせる**

```bash
docker compose run --rm frontend npx playwright test
```

Expected: PASS。落ちた場合は、削除した導線をまだ参照している spec が無いか確認する。

- [ ] **Step 4: 残存参照が無いことを確認する**

```bash
grep -rn "grantPremium\|プレミアム\|/checkout\|/pricing\|/account" frontend/e2e/
```

Expected: 出力なし。出た場合はその行を処理してから次へ進む。

- [ ] **Step 5: コミット**

```bash
git add -A frontend/e2e
git commit -F - <<'EOF'
test: e2e から premium 付与と加入導線を外す

加入導線の通し（premium.spec.ts）とロック表示（weekly-premium.spec.ts）は
検証対象そのものが無くなったため削除した。他の spec は CLI による
premium 付与をやめ、素のログインに戻した。

未ログインで保存一覧を開いたときの期待は、加入導線の表示から
RequireAuth によるログイン画面への遷移に変えた。

backend/cmd/grant と make grant / revoke は残す。課金配管の一部であり、
配管を残す方針と揃えるため。

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 6: 法務ページの整理

**Files:**
- Modify: `frontend/src/features/legal/content/terms.md:14,26-38,62,73`
- Modify: `frontend/src/app/App.tsx`（`/legal/tokushoho` のルート）
- Modify: `frontend/src/components/Footer.tsx`（特商法のリンク）
- Modify: `frontend/e2e/legal-pages.spec.ts`
- Test: `frontend/src/features/legal/` の既存テストがあれば追随させる

**Interfaces:**
- Consumes: Task 4（Footer と App.tsx が既に整理されていること）
- Produces: `/legal/tokushoho` が 404 になる状態

`content/tokushoho.md` と `TokushohoPage.tsx` は**ファイルとして残す**。有料の役務提供が無くなり表示義務も無くなるが、復活時にそのまま使えるため。`docs/legal/` 配下も触らない。

- [ ] **Step 1: 特商法ページが 404 になるテストを書いて失敗させる**

`frontend/src/app/App.auth.test.tsx` の Task 4 で追加した `it.each` に `/legal/tokushoho` を足す。

```tsx
it.each([
  '/checkout',
  '/checkout/complete',
  '/account',
  '/pricing',
  '/legal/tokushoho',
])('%s は撤廃済みで 404 になる', async (path) => {
```

- [ ] **Step 2: 落ちることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/app/App.auth.test.tsx
```

Expected: FAIL。`/legal/tokushoho` だけ 404 にならない。

- [ ] **Step 3: ルートと導線を外す**

`frontend/src/app/App.tsx` から `<Route path="/legal/tokushoho" element={<TokushohoPage />} />` と `import { TokushohoPage } from '../features/legal/TokushohoPage'` を削除し、159-160行のコメント「法務3ページは…」を「法務2ページは…」に直す。

`frontend/src/components/Footer.tsx` の `footerLinks` から `/legal/tokushoho` の行を削除する。

- [ ] **Step 4: 通ることを確認する**

```bash
docker compose run --rm frontend npx vitest run src/app/App.auth.test.tsx
```

Expected: PASS。

- [ ] **Step 5: 利用規約から課金条項を削除する**

`frontend/src/features/legal/content/terms.md` を次のとおり直す。

1. **14行目**（第2条3項）「「プレミアムプラン」とは、第4条に定める有料の継続的役務提供サービスをいいます。」を削除し、以降の項番号を繰り上げる。
2. **26-38行の第4条（プレミアムプラン）を全削除**する。
3. **第5条以降の条番号を1つずつ繰り上げる**（第5条→第4条 … 第13条→第12条）。
4. **他条からの相互参照を直す**。特に旧第6条2項の「（第6条2項）」のような参照が本文中にあれば、繰り上げ後の番号に合わせる。`grep -n "第[0-9]*条" terms.md` で全参照を洗い出して突き合わせる。
5. **旧62行（第6条2項）** サービス停止時に利用料金を日割返金する旨を、無料サービス前提に書き換える。

```markdown
2. 前項による停止または中断により利用者が本サービスを利用できなかった場合であっても、
   本サービスは無償で提供されるため、当方は利用料金の返金その他の金銭的な補償を行いません。
```

6. **旧73行（第8条2項）** の賠償上限を、利用料金に依存しない書き方に直す。

```markdown
2. 当方は、本サービスに起因して利用者に生じた損害について賠償する責任を負います。
   ただし、当方に故意または重大な過失がある場合を除き、本サービスが無償で提供されることに鑑み、
   その賠償額は**3,000円を上限**とします。
```

7. 規約の冒頭に最終改定日を記す欄があれば、`2026-08-02` に更新する。

- [ ] **Step 6: 条番号の整合を確認する**

```bash
grep -n "^## 第\|第[0-9]\+条" frontend/src/features/legal/content/terms.md
```

Expected: 見出しが第1条から第12条まで欠番・重複なく並び、本文中の相互参照がすべて存在する条を指している。

- [ ] **Step 7: e2e から特商法のケースを落とす**

`frontend/e2e/legal-pages.spec.ts` から特商法ページを開くケースを削除する。規約とプライバシーの2ページを検証する形に変える。規約の本文を検証しているケースが「プレミアムプラン」の文言に依存していれば、残っている見出しに差し替える。

- [ ] **Step 8: frontend 全体と e2e を確認する**

```bash
make test-frontend
docker compose run --rm frontend npx playwright test frontend/e2e/legal-pages.spec.ts
```

Expected: 両方 PASS。

- [ ] **Step 9: コミット**

```bash
git add -A frontend/src frontend/e2e
git commit -F - <<'EOF'
docs: 利用規約から課金条項を外し特商法ページの導線を落とす

有料の役務提供が無くなったため、第4条（プレミアムプラン）を削除して
以降の条番号を繰り上げた。サービス停止時の日割返金と賠償額の上限は、
利用料金を前提にした書き方だったため無償提供を前提に直した。

特商法ページはファイルを残したままルートとフッター導線だけ外す。
表示義務は無くなったが、復活時にそのまま使えるため。

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 7: ドキュメントの更新

**Files:**
- Modify: `spec.md`（1.1 / 1.2 / 2.2 / 2.7 / 2.8 / 2.11 / 5.7 / 15）
- Modify: `README.md`
- Modify: `DEPLOY.md`
- Modify: `docs/manual-e2e-payment.md`

**Interfaces:**
- Consumes: Task 1〜6 の実装結果
- Produces: なし（最終タスク）

- [ ] **Step 1: spec.md を更新する**

| 節 | 変更 |
| --- | --- |
| 1.1 確定した設計判断 | プレミアム前提の判断があれば「サブスクは 2026-08-02 に撤廃」と追記する |
| 1.2 MVPスコープ | 課金がスコープに含まれていれば外す |
| 2.2 献立検索（1週間分） | premium 限定の記述を削除し、ログインすれば誰でも使える旨に直す |
| 2.7 必要食材と買い物リスト | チェックの永続化が premium 限定である記述を削除する |
| 2.8 週間献立の保存 | 保存上限を「全員50件」に直す（free 10 / premium 50 の表を1行にする） |
| 2.11 プレミアムプラン | **節ごと削除しない。**冒頭に「**2026-08-02 に撤廃。**以下は当時の仕様で、backend の配管は残っている（復活時の出発点）」と注記し、本文は残す |
| 5.7 課金（フェーズ15、2.11） | 同上。API は実在し続けるため記述を残し、UI から呼ばれない旨を注記する |
| 15 有料化の前提条件 | 同上。撤廃済みである旨を冒頭に注記する |

2.11 / 5.7 / 15 を削除ではなく注記に留めるのは、backend の配管が残っており、仕様書から消すと実在する API とエンドポイントの説明が失われるため。

- [ ] **Step 2: README.md を更新する**

プレミアムプラン・課金・料金に触れている箇所を、撤廃済みである旨に直す。機能一覧に「プレミアム限定」の但し書きがあれば削除する。

- [ ] **Step 3: DEPLOY.md に Stripe 環境変数の扱いを明記する**

環境変数の節に次を足す。

```markdown
> **サブスク撤廃後も `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` / `STRIPE_PRICE_ID` は必要。**
> 2026-08-02 にサブスクを撤廃したが、backend の課金配管（`/billing/*`・webhook・
> `cmd/grant`）は復活に備えて残してある。`cmd/server/main.go` はこの3つが未設定だと
> 起動時に落ちるため、設定は消せない。設定が欠けたまま起動できる方が危険という判断。
```

- [ ] **Step 4: docs/manual-e2e-payment.md に注記する**

冒頭に次を足す。

```markdown
> **2026-08-02 にサブスクは撤廃済み。**この手順は現在実施しない。
> backend の課金配管は残してあるため、サブスクを復活させるときの出発点として保存している。
```

- [ ] **Step 5: 撤廃漏れを洗い出す**

```bash
grep -rn "プレミアム\|premium\|Premium" --include=*.tsx --include=*.ts frontend/src frontend/e2e
```

Expected: 出力なし、または `plan` 型（`'free' | 'premium'`）由来の `schema.d.ts` のみ。それ以外が出た場合はその箇所を処理する。

```bash
grep -rn "プレミアム限定\|premium 限定" backend/ spec.md README.md
```

Expected: backend のコメントは配管の説明として残ってよい。spec.md / README.md に「限定」が残っていれば直す。

- [ ] **Step 6: 全体を通して検証する**

```bash
make test
make lint
docker compose run --rm frontend npx playwright test
```

Expected: すべて PASS。

- [ ] **Step 7: 手で動線を確認する**

```bash
make up
```

1. 未ログインで `/weekly` を開くとログイン画面へ送られる
2. ログインして週間献立が提案でき、保存でき、`/saved-weekly` に出る
3. 買い物リストのチェックがリロード後も残る
4. フッターと画面のどこにも料金・加入への導線が無い
5. `/pricing` `/checkout` `/account` `/legal/tokushoho` が 404 になる

- [ ] **Step 8: コミット**

```bash
git add spec.md README.md DEPLOY.md docs/manual-e2e-payment.md
git commit -F - <<'EOF'
docs: サブスク撤廃を仕様とデプロイ手順に反映する

週間献立・保存・買い物リストの節から premium 限定の記述を外し、
保存上限を全員50件に直した。

プレミアムプランの節（2.11 / 5.7 / 15）は削除せず、撤廃済みである旨を
冒頭に注記して本文を残した。backend の配管が残っており、仕様書から
消すと実在する API とエンドポイントの説明が失われるため。

STRIPE_* が撤廃後も必要である理由を DEPLOY.md に明記した。
main.go が未設定で起動を拒むのは撤廃後も変えていない。

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 8: プライバシーポリシーと禁止事項から課金前提の記述を外す

**Files:**
- Modify: `frontend/src/features/legal/content/privacy.md:14,23,26,43,55-59,63,95`
- Modify: `frontend/src/features/legal/content/terms.md:35`

**Interfaces:**
- Consumes: Task 6（利用規約の再編は完了済み）
- Produces: なし（最終タスク）

**このタスクを足した理由:** 設計 §5（法務）が `terms.md` と特商法ページしか対象にしておらず、`privacy.md` が抜けていた。プライバシーポリシーは「あなたのデータをこう扱っています」と現在形で説明する文書であり、課金しないのに「決済情報を取得し米国の Stripe へ第三者提供している」と書かれた状態は事実と異なる。Task 6 のレビューが計画の抜けとして指摘し、対応することにした。

### privacy.md

1. **1節の表から「決済情報」の行を削除**（14行目）。
2. **2節の利用目的から「4. プレミアムプランの利用料金の請求および契約の管理」を削除**し、以降を1つずつ繰り上げる（5→4 … 8→7）。
3. **繰り上げ後の6番「サービスに関する重要なお知らせ」**（旧7番、26行目）から課金に関する例示を落とす。

```markdown
6. サービスに関する重要なお知らせ（障害、仕様変更等）の通知
```

4. **4節の委託先の表から `Stripe, Inc.` の行を削除**（43行目）。
5. **5節（外部送信）の表から `Stripe, Inc.` の行を削除**（57行目）。行を消すと表が空になるため、5節は表をやめて次の一文に置き換える。

```markdown
## 5. 利用者情報の外部送信について（電気通信事業法）

現在、本サービスから利用者の端末情報を外部の事業者へ送信する仕組みは導入していません。
導入する場合は本項に追記します。
```

6. **6節の Cookie の説明**（63行目）から「プレミアム機能」を落とす。

```markdown
ブラウザの設定により Cookie を無効にすることができますが、その場合、ログインを要する機能（履歴、お気に入り、週間献立の保存）はご利用いただけません。
```

7. 末尾の「制定日：2026年7月25日」の下に「改定日：2026年8月2日」を足す。

### terms.md

8. **禁止事項から「複数のアカウントを作成して無料お試し期間を反復して利用する行為」を削除**し（35行目）、以降の号番号を繰り上げる。無料お試し期間は第4条ごと削除済みで、存在しない機能についての禁止規定になっているため。

### 手順

- [ ] **Step 1: privacy.md の7箇所を直す**

上記1〜7を適用する。項番号の繰り上げ漏れが出やすいので、2節は最後に通しで番号を確認する。

- [ ] **Step 2: terms.md の禁止事項を直す**

上記8を適用する。号番号が欠番・重複しないことを確認する。

- [ ] **Step 3: 残存を確認する**

```bash
grep -n "決済\|Stripe\|プレミアム\|サブスクリプション\|課金\|利用料金\|無料お試し" frontend/src/features/legal/content/privacy.md frontend/src/features/legal/content/terms.md
```

Expected: 出力なし。

- [ ] **Step 4: 番号の整合を確認する**

```bash
grep -n "^[0-9]\+\. \|^## " frontend/src/features/legal/content/privacy.md
grep -n "^## 第\|^[0-9]\+\. " frontend/src/features/legal/content/terms.md
```

Expected: privacy.md の2節が1〜7、terms.md の禁止事項の号が欠番・重複なく並ぶ。

- [ ] **Step 5: テストを回す**

```bash
make test-frontend
```

Expected: PASS。法務ページのテストが本文の文言に依存していれば追随させる。

- [ ] **Step 6: コミット**

```bash
git add frontend/src/features/legal/content/privacy.md frontend/src/features/legal/content/terms.md
git commit -F - <<'EOF'
docs: プライバシーポリシーと禁止事項から課金前提の記述を外す

決済情報の取得、Stripe への委託と外部送信、プレミアムプランの課金管理、
無料お試し期間に関する記述を削除した。プライバシーポリシーは現在形で
データの取扱いを説明する文書であり、課金が無いのに決済情報を取得し
米国へ提供していると書かれた状態は事実と異なるため。

backend の Stripe 配管は復活に備えて残しているが、実際に決済は発生せず
決済ページも存在しないので、利用者への説明としては削除が正しい。
復活させるときは、この記述を戻すことが加入導線を作る前提になる。

利用規約の禁止事項からは「無料お試し期間を反復して利用する行為」を
落とした。無料お試し期間は第4条ごと削除済みで、存在しない機能について
の禁止規定になっていたため。

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

## 完了条件

- [ ] `make test` が通る
- [ ] `make lint` が通る
- [ ] `docker compose run --rm frontend npx playwright test` が通る
- [ ] `/pricing` `/checkout` `/account` `/legal/tokushoho` が 404
- [ ] 未ログインで `/weekly` を開くとログイン画面へ送られる
- [ ] ログインすればプランによらず週間献立の提案・保存・買い物リストの永続化ができる
- [ ] 画面のどこにも料金・加入への導線が無い
- [ ] backend の課金配管（`/billing/*`・`cmd/grant`・`subscriptions` テーブル）は残っている
