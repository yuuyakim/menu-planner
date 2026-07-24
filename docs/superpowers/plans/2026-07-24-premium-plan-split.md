# プレミアムの機能差分（買い物リストの永続化）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 保存済みの週間献立に紐づく買い物リストの差分（チェック状態・手動品目の追加/削除）を premium のときだけ永続化する。free / premium で画面は同じままにし、premium の2機能（保存上限50件・買い物リスト永続化）のうち後者を実装しきる。

**Architecture:** 既存のステートレスな買い物リスト（`POST /shopping-list`）はそのまま残し、**保存済みの週に対してのみ差分（overlay）を保存する新経路**を足す。リスト本体は毎回献立から導出し、そこに差分だけを重ねる（`spec.md` 2.8 の「二重に持たない」を維持）。差分は `(saved_weekly_menu_id, name)` を主キーにした1テーブルに持ち、API は品目単位ではなく overlay 全体の**一括置換**にする。エンタイトルメントは既存の `service.EntitlementService` を参照して premium 判定する。

**Tech Stack:** Go 1.25 / echo v4 / pgx v5 / golang-migrate（embed）/ React 19 / TanStack Query / openapi-typescript / Vitest + MSW / Playwright。

## Global Constraints

- **設計の正:** `docs/superpowers/specs/2026-07-23-premium-plan-split-design.md`（本計画の設計）と基盤設計 `docs/superpowers/specs/2026-07-23-premium-entitlement-design.md`。食い違いは設計を優先。
- **ブランチ:** base はすべて `feature/premium`（`main` ではない）。作業ブランチは feature から切る。1タスク = 1PR = 🔴テスト + 🟢実装。**🔴 と 🟢 は別コミット**（`test:` → `feat:`/`fix:`。git 履歴の流儀）。PR前に `/pre-pr-review`、CI緑で自分でマージ、次へ。
- **マイグレーション番号は `000011`。** `000009`(add_menu_role) / `000010`(create_subscriptions) は feature/premium に取り込み済み。`feature`→`main` の最終マージ前に `000009`/`000010` がその順で `main` に入っていることを確認する（設計 5.2）。
- **エンタイトルメントは上限をメソッドで導出する。** `Entitlement{}` のゼロ値が free に落ちること（安全側）をドメインで保証する（基盤設計5.2）。
- **API は overlay の一括置換。** 品目単位の部分更新はしない。後勝ちを許容する（設計 3.5）。
- **差分だけを保存する。** リスト本体は導出し続ける。行が無い = 「献立由来のまま・未チェック」（設計 3.4 / 5.1）。
- **新しい service/domain エラーは `handler/problem.go` の `problemMapping` に必ず足す。** さもないと handler に届いた瞬間 500 になり、`problem_coverage_test.go` が落ちる（フェーズ12/13で2度踏んだ）。
- **`origin` は CHECK 制約ではなくアプリ側で検証する**（既存 `menus.role` 等の流儀。設計 5.1）。
- **手動品目は1リスト100件を上限**とし、超過は 409（設計 7.2）。
- **buy一覧の並びはカテゴリ順→カナ順。** 既存 `aggregate` と揃える。
- **フロントは件数・プラン分岐の文言をサーバに寄せる。** localStorage は「案内を1回だけ」の記録にのみ使う（設計 9）。
- **チェックボックスは買い物リストに常に出す**（ユーザー確定）。永続化は premium × 保存済みの週のときだけ。未保存・free はその場限り。free が初めてチェックしたら案内を1回。
- テスト実行: バックエンド `make test-backend`（統合テストは docker のDBが要る）、フロント `make test-frontend`（`tsc -b` + lint + vitest）、E2E `make test-e2e`（`make up` + `make seed` 前提）。型再生成 `make gen-api`。

---

## File Structure

**バックエンド（新規）**
- `backend/internal/domain/shopping_list_override.go` — `Origin` 値オブジェクトと `ShoppingListOverride` 値オブジェクト。
- `backend/internal/repository/migrations/000011_create_shopping_list_overrides.up.sql` / `.down.sql`
- `backend/internal/repository/shopping_list_override.go` — `ShoppingListOverrideRepository`。
- `backend/internal/service/saved_shopping_list.go` — `SavedShoppingListService`（`For` / `ReplaceOverrides`）。
- `backend/internal/handler/saved_shopping_list.go` — `GET/PUT /weekly-menus/:id/shopping-list`。

**バックエンド（変更）**
- `backend/internal/domain/entitlement.go` — `CanPersistShoppingList()` を足す。
- `backend/internal/service/ports.go` — `ShoppingListOverrideStore` ポートを足す。
- `backend/internal/service/saved_weekly.go` — `SavedWeeklyMenuStore` に `Find` を足す。
- `backend/internal/repository/saved_weekly.go` — `Find` を実装。
- `backend/internal/handler/problem.go` — 403/409 の写像を足す。
- `backend/cmd/api/main.go`（結線） — repo / service / handler を組み立て、ルート登録。
- `api/openapi.yaml` — パスとスキーマを足す。

**フロント（新規）**
- `frontend/src/hooks/useOnceFlag.ts` — 「一度きり」を localStorage に持つ hook（案内用）。

**フロント（変更）**
- `frontend/src/api/types.ts` — 生成型の別名を足す。
- `frontend/src/features/menu/api.ts` — `fetchSavedShoppingList` / `saveShoppingListOverrides`。
- `frontend/src/features/menu/ShoppingListPage.tsx` — GET/PUT、チェックボックス、手動品目、案内。
- `frontend/src/features/menu/SavedWeeklyPage.tsx` — 「開く」で保存IDを持ち回す。
- `frontend/src/features/menu/WeeklyPage.tsx` — 週を作り直したら保存IDを捨てる。

**責務の境界:**
- 差分の**重ね合わせはサーバ**で行い、フロントには最終形だけ渡す（設計 8）。フロントに `POST` 経路とのマージを二重実装しない。
- 保存済み週の所有者検証は**既存の週間献立の取得と同じ経路**（`SavedWeeklyMenuStore.Find`）を通す。他人のリストには構造上到達できない。
- **設計 7.2 は `ShoppingListService` の拡張を書いているが、本計画は別の `SavedShoppingListService` に分ける。** 理由: 既存 `NewShoppingListService(m, i)` の引数を増やすと Build/MenuIngredients の既存テストと `main.go` を広く巻き込む。ステートレスな導出（`Build`）はそのまま残し、新サービスがそれを**合成**して差分を重ねる。導出ロジックは重複させない（`ShoppingListDeriver` インターフェースで `Build` を借りる）。

---

### Task 1: `domain.Entitlement.CanPersistShoppingList()`

premium が買い物リストを永続化できるか。ゼロ値が false に落ちること（安全側）を担保する。

**Files:**
- Modify: `backend/internal/domain/entitlement.go`
- Test: `backend/internal/domain/entitlement_test.go`

**Interfaces:**
- Consumes: `domain.Entitlement`（`plan` を持つ既存型）、`domain.NewEntitlement(Plan)`、`domain.PlanFree` / `domain.PlanPremium`。
- Produces: `func (e Entitlement) CanPersistShoppingList() bool` — premium:true / それ以外:false。

- [ ] **Step 1: 失敗するテストを書く**

`entitlement_test.go` に足す:

```go
func TestEntitlement_CanPersistShoppingList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ent  domain.Entitlement
		want bool
	}{
		{"premium は永続化できる", domain.NewEntitlement(domain.PlanPremium), true},
		{"free は永続化できない", domain.NewEntitlement(domain.PlanFree), false},
		// ゼロ値は取得し忘れを表す。free と同じく永続化できない（安全側）。
		{"ゼロ値は永続化できない", domain.Entitlement{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.ent.CanPersistShoppingList(); got != tt.want {
				t.Errorf("CanPersistShoppingList() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/domain/ -run TestEntitlement_CanPersistShoppingList`
Expected: FAIL（`ent.CanPersistShoppingList undefined`）

- [ ] **Step 3: 🔴 をコミットする**

```bash
git add backend/internal/domain/entitlement_test.go
git commit -m "test: CanPersistShoppingList の導出（ゼロ値は false）"
```

- [ ] **Step 4: 最小の実装を書く**

`entitlement.go` の `SavedWeeklyMenuLimit` の下に足す:

```go
// CanPersistShoppingList は買い物リストの差分を保存できるかを返す。
//
// premium だけが true。ゼロ値の Entitlement は Plan() が free に落ちるため
// false になり、取得し忘れても永続化の権限は漏れない（false 側が安全）。
func (e Entitlement) CanPersistShoppingList() bool {
	return e.Plan() == PlanPremium
}
```

- [ ] **Step 5: テストが通ることを確認する**

Run: `cd backend && go test ./internal/domain/ -run TestEntitlement_CanPersistShoppingList`
Expected: PASS

- [ ] **Step 6: 🟢 をコミットする**

```bash
git add backend/internal/domain/entitlement.go
git commit -m "feat: プレミアムが買い物リストを永続化できる権限を足す"
```

---

### Task 2: `domain.ShoppingListOverride` と `domain.Origin`

差分1行の値オブジェクト。`origin`（献立由来 / 手動）を型で持つ。

**Files:**
- Create: `backend/internal/domain/shopping_list_override.go`
- Test: `backend/internal/domain/shopping_list_override_test.go`

**Interfaces:**
- Consumes: `domain.SavedWeeklyMenuID`、`domain.IngredientCategory`（`.Valid()`）。
- Produces:
  - `type Origin string`、`OriginDerived Origin = "derived"`、`OriginManual Origin = "manual"`、`ParseOrigin(string) (Origin, error)`、`(Origin).Valid() bool`、`(Origin).String() string`、`var ErrInvalidOrigin`。
  - `type ShoppingListOverride struct { SavedWeeklyMenuID SavedWeeklyMenuID; Name string; Category IngredientCategory; Origin Origin; Checked bool; Hidden bool }`、`(ShoppingListOverride).Validate() error`、`var ErrInvalidOverride`。

- [ ] **Step 1: 失敗するテストを書く**

`shopping_list_override_test.go`:

```go
package domain_test

import (
	"errors"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestParseOrigin(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"derived", "manual"} {
		if _, err := domain.ParseOrigin(s); err != nil {
			t.Errorf("ParseOrigin(%q) は成功すべき: %v", s, err)
		}
	}
	// 表記ゆれ・空・未知は拒否（DBの値と乖離させない）。
	for _, s := range []string{"", "Derived", " manual", "other"} {
		if _, err := domain.ParseOrigin(s); !errors.Is(err, domain.ErrInvalidOrigin) {
			t.Errorf("ParseOrigin(%q) は ErrInvalidOrigin を返すべき: %v", s, err)
		}
	}
}

func validOverride() domain.ShoppingListOverride {
	return domain.ShoppingListOverride{
		SavedWeeklyMenuID: domain.NewSavedWeeklyMenuID(),
		Name:              "にんじん",
		Category:          domain.CategoryVegetable,
		Origin:            domain.OriginDerived,
		Checked:           true,
	}
}

func TestShoppingListOverride_Validate(t *testing.T) {
	t.Parallel()

	if err := validOverride().Validate(); err != nil {
		t.Fatalf("正当な差分は通るべき: %v", err)
	}

	tests := []struct {
		name  string
		mutate func(o *domain.ShoppingListOverride)
	}{
		{"IDが未設定", func(o *domain.ShoppingListOverride) { o.SavedWeeklyMenuID = domain.SavedWeeklyMenuID{} }},
		{"名前が空", func(o *domain.ShoppingListOverride) { o.Name = "  " }},
		{"カテゴリが不正", func(o *domain.ShoppingListOverride) { o.Category = "spice" }},
		{"originが不正", func(o *domain.ShoppingListOverride) { o.Origin = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := validOverride()
			tt.mutate(&o)
			if err := o.Validate(); !errors.Is(err, domain.ErrInvalidOverride) {
				t.Errorf("%s は ErrInvalidOverride を返すべき: %v", tt.name, err)
			}
		})
	}
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/domain/ -run 'TestParseOrigin|TestShoppingListOverride'`
Expected: FAIL（未定義）

- [ ] **Step 3: 🔴 をコミットする**

```bash
git add backend/internal/domain/shopping_list_override_test.go
git commit -m "test: 買い物リストの差分と origin の検証"
```

- [ ] **Step 4: 最小の実装を書く**

`shopping_list_override.go`:

```go
package domain

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidOrigin は文字列が既知の origin に一致しないことを表す。
	ErrInvalidOrigin = errors.New("不正な差分の由来です")
	// ErrInvalidOverride は差分の必須項目が満たされていないことを表す。
	ErrInvalidOverride = errors.New("不正な買い物リストの差分です")
)

// Origin は買い物リストの差分行の由来。
//
// derived は献立から導出された品目に対する差分（チェック・非表示）。
// manual は利用者が自分で足した品目。DBの origin カラムの値と一致する。
type Origin string

const (
	// OriginDerived は献立由来の品目への差分。
	OriginDerived Origin = "derived"
	// OriginManual は利用者が手で足した品目。
	OriginManual Origin = "manual"
)

// ParseOrigin は文字列を Origin に変換する。完全一致のみ受け付ける。
func ParseOrigin(s string) (Origin, error) {
	o := Origin(s)
	if !o.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidOrigin, s)
	}
	return o, nil
}

// Valid は定義済みの origin かどうかを返す。
func (o Origin) Valid() bool {
	switch o {
	case OriginDerived, OriginManual:
		return true
	default:
		return false
	}
}

// String は DB およびAPIで用いる文字列表現を返す。
func (o Origin) String() string { return string(o) }

// ShoppingListOverride は保存済みの週の買い物リストに重ねる差分1行（設計 5.1）。
//
// **これはリストの実体ではなく、献立から導出したリストからの「ズレ」だけを持つ。**
// 行が無いことは「献立由来のまま・未チェック」を意味する。
// 主キーは (SavedWeeklyMenuID, Name)。同じリストに同名の品目は作れない。
type ShoppingListOverride struct {
	SavedWeeklyMenuID SavedWeeklyMenuID
	Name              string
	Category          IngredientCategory
	Origin            Origin
	// Checked はチェック済みか。
	Checked bool
	// Hidden は「家にあるから消した」など、表示から外すか。
	Hidden bool
}

// Validate は必須項目が満たされているかを検証する。
func (o ShoppingListOverride) Validate() error {
	if o.SavedWeeklyMenuID.IsZero() {
		return fmt.Errorf("%w: 週間献立IDが未設定です", ErrInvalidOverride)
	}
	if strings.TrimSpace(o.Name) == "" {
		return fmt.Errorf("%w: 名前が空です", ErrInvalidOverride)
	}
	if !o.Category.Valid() {
		return fmt.Errorf("%w: 不正なカテゴリです: %q", ErrInvalidOverride, o.Category)
	}
	if !o.Origin.Valid() {
		return fmt.Errorf("%w: 不正な由来です: %q", ErrInvalidOverride, o.Origin)
	}
	return nil
}
```

- [ ] **Step 5: 写像漏れ検査に対応する（同じPR内で。CIを緑に保つため）**

新しい公開エラーを domain に定義したので、`problem_coverage_test.go` が「写像 or 意図的除外」を要求する。定義しただけで対応しないと handler パッケージのテストが即落ちる。

`handler/problem.go` の `problemMapping` に足す（`ErrInvalidSavedWeeklyMenuID` の並びの後、400 の仲間として）:

```go
	// 買い物リストの差分（PUT のボディ）が不正。Validate が返す。
	{domain.ErrInvalidOverride, http.StatusBadRequest, "invalid-shopping-list-override", "不正な買い物リストの指定です"},
```

`handler/problem_coverage_test.go` の `intentionallyUnmapped` に足す:

```go
	// リクエスト経路では ShoppingListOverride.Validate が ErrInvalidOverride に
	// 丸めるため、この生のエラーは handler まで届かない（ParseOrigin は入力検証に使わない）。
	"domain.ErrInvalidOrigin": "リクエスト経路では ErrInvalidOverride に丸める",
```

- [ ] **Step 6: テストが通ることを確認する**

Run: `cd backend && go test ./internal/domain/ -run 'TestParseOrigin|TestShoppingListOverride' && go test ./internal/handler/ -run TestProblemMapping`
Expected: どちらも PASS

- [ ] **Step 7: 🟢 をコミットする**

```bash
git add backend/internal/domain/shopping_list_override.go \
        backend/internal/handler/problem.go \
        backend/internal/handler/problem_coverage_test.go
git commit -m "feat: 買い物リストの差分の値オブジェクトを足す"
```

---

### Task 3: マイグレーション `000011_create_shopping_list_overrides`

差分テーブル。複合主キー `(saved_weekly_menu_id, name)`、週が消えれば差分も消える（CASCADE）。

**Files:**
- Create: `backend/internal/repository/migrations/000011_create_shopping_list_overrides.up.sql`
- Create: `backend/internal/repository/migrations/000011_create_shopping_list_overrides.down.sql`
- Test: `backend/internal/repository/shopping_list_overrides_schema_test.go`

**Interfaces:**
- Consumes: 既存の統合テストヘルパ（同一 `repository_test` パッケージ）: `newTestPool(t)`、`createUser(t, pool, email)`、`insertSavedWeek(t, pool, userID)`（`saved_weekly_schema_test.go` で定義済み）。
- Produces: テーブル `shopping_list_overrides`。

- [ ] **Step 1: 失敗するテストを書く**

`shopping_list_overrides_schema_test.go`:

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// insertOverride は差分を1行入れる（生SQL。スキーマそのものを検証するため）。
func insertOverride(ctx context.Context, t *testing.T, week, name string) error {
	t.Helper()
	pool := newTestPool(t)
	_, err := pool.Exec(ctx,
		`INSERT INTO shopping_list_overrides
		   (saved_weekly_menu_id, name, category, origin, checked)
		 VALUES ($1, $2, 'vegetable', 'derived', true)`, week, name)
	return err
}

func TestShoppingListOverridesSchema_同じ週に同名は入らない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	u := createUser(t, pool, "slo-dup@example.com")
	week := insertSavedWeek(t, pool, u.ID)

	_, err := pool.Exec(ctx,
		`INSERT INTO shopping_list_overrides
		   (saved_weekly_menu_id, name, category, origin) VALUES ($1, 'にんじん', 'vegetable', 'derived')`, week)
	require.NoError(t, err)

	// 複合主キー (saved_weekly_menu_id, name) により2件目は入らない。
	_, err = pool.Exec(ctx,
		`INSERT INTO shopping_list_overrides
		   (saved_weekly_menu_id, name, category, origin) VALUES ($1, 'にんじん', 'meat', 'manual')`, week)
	require.Error(t, err, "同じ週に同名の差分は入ってはいけない")
}

func TestShoppingListOverridesSchema_既定値(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	u := createUser(t, pool, "slo-default@example.com")
	week := insertSavedWeek(t, pool, u.ID)

	// checked / hidden を省くと false、時刻は now() が入る。
	_, err := pool.Exec(ctx,
		`INSERT INTO shopping_list_overrides
		   (saved_weekly_menu_id, name, category, origin) VALUES ($1, 'たまねぎ', 'vegetable', 'derived')`, week)
	require.NoError(t, err)

	var checked, hidden bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT checked, hidden FROM shopping_list_overrides
		  WHERE saved_weekly_menu_id=$1 AND name='たまねぎ'`, week).Scan(&checked, &hidden))
	require.False(t, checked)
	require.False(t, hidden)
}

func TestShoppingListOverridesSchema_週を消すと差分も消える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	u := createUser(t, pool, "slo-cascade@example.com")
	week := insertSavedWeek(t, pool, u.ID)
	_, err := pool.Exec(ctx,
		`INSERT INTO shopping_list_overrides
		   (saved_weekly_menu_id, name, category, origin) VALUES ($1, 'にんじん', 'vegetable', 'derived')`, week)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM saved_weekly_menus WHERE id=$1`, week)
	require.NoError(t, err)

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM shopping_list_overrides WHERE saved_weekly_menu_id=$1`, week).Scan(&n))
	require.Zero(t, n, "週を消したら差分も消えるべき（CASCADE）")
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/repository/ -run TestShoppingListOverridesSchema`
Expected: FAIL（`relation "shopping_list_overrides" does not exist`）

- [ ] **Step 3: 🔴 をコミットする**

```bash
git add backend/internal/repository/shopping_list_overrides_schema_test.go
git commit -m "test: 買い物リスト差分テーブルのスキーマ検査"
```

- [ ] **Step 4: マイグレーションを書く**

`000011_create_shopping_list_overrides.up.sql`:

```sql
-- 保存済みの週間献立に紐づく買い物リストの差分（設計 5.1）。
-- リスト本体は献立から毎回導出し、ここには導出結果からの「ズレ」だけを持つ。
-- 行が無いことは「献立由来のまま・未チェック」を意味する。
CREATE TABLE shopping_list_overrides (
    saved_weekly_menu_id uuid        NOT NULL REFERENCES saved_weekly_menus(id) ON DELETE CASCADE,
    name                 text        NOT NULL,
    category             text        NOT NULL,
    -- origin は 'derived' / 'manual'。CHECK ではなくアプリで検証する
    -- （既存 menus.role 等の流儀。将来の値を DDL 変更なしに受けられるように）。
    origin               text        NOT NULL,
    checked              boolean     NOT NULL DEFAULT false,
    hidden               boolean     NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    -- 同じリストに同名の品目は作れない。献立由来と同名の手動品目が並ぶのを防ぐ。
    PRIMARY KEY (saved_weekly_menu_id, name)
);
```

`000011_create_shopping_list_overrides.down.sql`:

```sql
DROP TABLE shopping_list_overrides;
```

- [ ] **Step 5: テストが通ることを確認する（up が当たること）**

Run: `cd backend && go test ./internal/repository/ -run TestShoppingListOverridesSchema`
Expected: PASS（`newTestPool` がマイグレーションを適用する）

- [ ] **Step 6: down が通ることを確認する**

Run（ローカルDBに対して手で確認）:
```bash
make up
docker compose exec backend go run ./cmd/migrate up
docker compose exec backend go run ./cmd/migrate down 1
docker compose exec backend go run ./cmd/migrate up
```
Expected: いずれもエラーなく完了（`000011` の up→down→up が通る）

- [ ] **Step 7: 🟢 をコミットする**

```bash
git add backend/internal/repository/migrations/000011_create_shopping_list_overrides.up.sql \
        backend/internal/repository/migrations/000011_create_shopping_list_overrides.down.sql
git commit -m "feat: 買い物リスト差分テーブルを追加する"
```

---

### Task 4: `ShoppingListOverrideStore` ポートと `ShoppingListOverrideRepository`

差分の読み出しと一括置換。`Replace` は当該週の差分を丸ごと置き換える（設計 3.5 / 7.1）。

**Files:**
- Modify: `backend/internal/service/ports.go`（ポートを足す）
- Create: `backend/internal/repository/shopping_list_override.go`
- Test: `backend/internal/repository/shopping_list_override_test.go`

**Interfaces:**
- Consumes: `domain.SavedWeeklyMenuID`、`domain.ShoppingListOverride`、`domain.Origin`、`domain.IngredientCategory`。
- Produces:
  - ポート `service.ShoppingListOverrideStore`:
    - `FindBySavedWeeklyMenu(ctx, domain.SavedWeeklyMenuID) ([]domain.ShoppingListOverride, error)`
    - `Replace(ctx, domain.SavedWeeklyMenuID, []domain.ShoppingListOverride) error`
  - `repository.NewShoppingListOverrideRepository(pool) *ShoppingListOverrideRepository` がこれを満たす。

- [ ] **Step 1: 失敗するテストを書く**

`shopping_list_override_test.go`:

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func TestShoppingListOverrideRepository_置換と取得(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewShoppingListOverrideRepository(pool)

	u := createUser(t, pool, "slo-repo@example.com")
	weekStr := insertSavedWeek(t, pool, u.ID)
	week, err := domain.ParseSavedWeeklyMenuID(weekStr)
	require.NoError(t, err)

	// 最初は空。
	got, err := repo.FindBySavedWeeklyMenu(ctx, week)
	require.NoError(t, err)
	require.Empty(t, got)

	// 2件入れる。
	overrides := []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: week, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
		{SavedWeeklyMenuID: week, Name: "牛乳", Category: domain.CategoryDairyEgg, Origin: domain.OriginManual, Checked: false},
	}
	require.NoError(t, repo.Replace(ctx, week, overrides))

	got, err = repo.FindBySavedWeeklyMenu(ctx, week)
	require.NoError(t, err)
	require.Len(t, got, 2)
	// name 順で返る。
	require.Equal(t, "にんじん", got[0].Name)
	require.True(t, got[0].Checked)
	require.Equal(t, domain.OriginManual, got[1].Origin)
	require.Equal(t, week, got[0].SavedWeeklyMenuID)
}

func TestShoppingListOverrideRepository_置換は丸ごと入れ替える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewShoppingListOverrideRepository(pool)

	u := createUser(t, pool, "slo-replace@example.com")
	weekStr := insertSavedWeek(t, pool, u.ID)
	week, _ := domain.ParseSavedWeeklyMenuID(weekStr)

	require.NoError(t, repo.Replace(ctx, week, []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: week, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
	}))
	// 2回目は前の差分を消して新しいものだけにする。
	require.NoError(t, repo.Replace(ctx, week, []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: week, Name: "たまねぎ", Category: domain.CategoryVegetable, Origin: domain.OriginManual, Checked: false},
	}))

	got, err := repo.FindBySavedWeeklyMenu(ctx, week)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "たまねぎ", got[0].Name, "前の差分は消えているべき")
}

func TestShoppingListOverrideRepository_空で置換すると全部消える(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewShoppingListOverrideRepository(pool)

	u := createUser(t, pool, "slo-clear@example.com")
	weekStr := insertSavedWeek(t, pool, u.ID)
	week, _ := domain.ParseSavedWeeklyMenuID(weekStr)

	require.NoError(t, repo.Replace(ctx, week, []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: week, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
	}))
	require.NoError(t, repo.Replace(ctx, week, nil))

	got, err := repo.FindBySavedWeeklyMenu(ctx, week)
	require.NoError(t, err)
	require.Empty(t, got, "空で置換したら全部消えるべき")
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/repository/ -run TestShoppingListOverrideRepository`
Expected: FAIL（`repository.NewShoppingListOverrideRepository undefined`）

- [ ] **Step 3: 🔴 をコミットする**

```bash
git add backend/internal/repository/shopping_list_override_test.go
git commit -m "test: 買い物リスト差分リポジトリの置換と取得"
```

- [ ] **Step 4: ポートを足す**

`service/ports.go` の末尾（`Entitlements` の下）に足す:

```go
// ShoppingListOverrideStore は保存済みの週の買い物リストの差分（overlay）の
// 永続化を抽象化する。実装は internal/repository にある。
//
// リストの実体は持たず「差分」だけを扱う（設計 3.4）。更新は品目単位の部分更新では
// なく overlay 全体の一括置換にする（設計 3.5）。冪等で実装が単純になり、
// 1端末1利用の実態に競合制御は要らない。
type ShoppingListOverrideStore interface {
	// FindBySavedWeeklyMenu は当該週の差分を name 順で返す。無ければ空スライス。
	FindBySavedWeeklyMenu(ctx context.Context, id domain.SavedWeeklyMenuID) ([]domain.ShoppingListOverride, error)

	// Replace は当該週の差分を丸ごと置き換える。削除と挿入を1トランザクションで行う。
	Replace(ctx context.Context, id domain.SavedWeeklyMenuID, overrides []domain.ShoppingListOverride) error
}
```

- [ ] **Step 5: リポジトリを実装する**

`repository/shopping_list_override.go`:

```go
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ShoppingListOverrideRepository は買い物リストの差分を Postgres に保存する。
type ShoppingListOverrideRepository struct {
	pool *pgxpool.Pool
}

// NewShoppingListOverrideRepository は ShoppingListOverrideRepository を生成する。
func NewShoppingListOverrideRepository(pool *pgxpool.Pool) *ShoppingListOverrideRepository {
	return &ShoppingListOverrideRepository{pool: pool}
}

// FindBySavedWeeklyMenu は当該週の差分を name 順で返す。
func (r *ShoppingListOverrideRepository) FindBySavedWeeklyMenu(
	ctx context.Context, id domain.SavedWeeklyMenuID,
) ([]domain.ShoppingListOverride, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT name, category, origin, checked, hidden
		   FROM shopping_list_overrides
		  WHERE saved_weekly_menu_id = $1
		  ORDER BY name`, id.String())
	if err != nil {
		return nil, fmt.Errorf("買い物リストの差分の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	out := make([]domain.ShoppingListOverride, 0)
	for rows.Next() {
		var (
			name, category, origin string
			checked, hidden        bool
		)
		if err := rows.Scan(&name, &category, &origin, &checked, &hidden); err != nil {
			return nil, fmt.Errorf("買い物リストの差分の読み取りに失敗しました: %w", err)
		}
		// DBの値はアプリが書いたものなので、そのまま型に載せる（検証は書き込み側）。
		out = append(out, domain.ShoppingListOverride{
			SavedWeeklyMenuID: id,
			Name:              name,
			Category:          domain.IngredientCategory(category),
			Origin:            domain.Origin(origin),
			Checked:           checked,
			Hidden:            hidden,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("買い物リストの差分の読み取りに失敗しました: %w", err)
	}
	return out, nil
}

// Replace は当該週の差分を丸ごと置き換える。削除と挿入を1トランザクションで行う。
//
// 部分更新にせず全消し＋全入れにするのは、overlay を冪等な1リソースとして
// 扱うため（設計 3.5）。品目数は高々100件程度で、全入れ替えの負荷は問題にならない。
func (r *ShoppingListOverrideRepository) Replace(
	ctx context.Context, id domain.SavedWeeklyMenuID, overrides []domain.ShoppingListOverride,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("差分の置換を開始できませんでした: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`DELETE FROM shopping_list_overrides WHERE saved_weekly_menu_id = $1`, id.String()); err != nil {
		return fmt.Errorf("差分の削除に失敗しました: %w", err)
	}

	for _, o := range overrides {
		if _, err := tx.Exec(ctx,
			`INSERT INTO shopping_list_overrides
			   (saved_weekly_menu_id, name, category, origin, checked, hidden)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			id.String(), o.Name, o.Category.String(), o.Origin.String(), o.Checked, o.Hidden); err != nil {
			return fmt.Errorf("差分の保存に失敗しました: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("差分の置換を確定できませんでした: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: テストが通ることを確認する**

Run: `cd backend && go test ./internal/repository/ -run TestShoppingListOverrideRepository`
Expected: PASS

- [ ] **Step 7: インターフェース適合を確認する**

`repository/interface_test.go` に足す（既存の適合チェックの流儀に合わせる）:

```go
var _ service.ShoppingListOverrideStore = (*repository.ShoppingListOverrideRepository)(nil)
```

Run: `cd backend && go build ./... && go test ./internal/repository/ -run TestShoppingListOverrideRepository`
Expected: PASS

- [ ] **Step 8: 🟢 をコミットする**

```bash
git add backend/internal/service/ports.go \
        backend/internal/repository/shopping_list_override.go \
        backend/internal/repository/interface_test.go
git commit -m "feat: 買い物リスト差分のポートとリポジトリを足す"
```

---

### Task 5: `SavedWeeklyMenuStore.Find`（所有者スコープの単一取得）

保存済みの週を**本人のものだけ**1件取得する。買い物リストの導出と差分の置換で、所有者検証と献立IDの取得を兼ねる。

**Files:**
- Modify: `backend/internal/service/saved_weekly.go`（`SavedWeeklyMenuStore` に `Find` を足す）
- Modify: `backend/internal/repository/saved_weekly.go`（`Find` を実装）
- Test: `backend/internal/repository/saved_weekly_test.go`（統合テストを足す）
- Modify: `backend/internal/service/saved_weekly_test.go`（既存 fake `fakeSavedWeeklyStore` に `Find` を足す）

**Interfaces:**
- Consumes: `domain.UserID`、`domain.SavedWeeklyMenuID`、`domain.SavedWeeklyMenu`、`service.ErrSavedWeeklyMenuNotFound`（既存）、`r.fillDays`（既存の private ヘルパ）。
- Produces: `SavedWeeklyMenuStore.Find(ctx, userID domain.UserID, id domain.SavedWeeklyMenuID) (domain.SavedWeeklyMenu, error)` — 他人のもの・存在しないものは `ErrSavedWeeklyMenuNotFound`。中身の7日分も詰めて返す。

- [ ] **Step 1: 失敗するテストを書く**

`repository/saved_weekly_test.go` に足す（既存の保存ヘルパの流儀に合わせる。保存の作り方は同ファイルの既存テストを参照）:

```go
func TestSavedWeeklyRepository_Find_本人の週を中身つきで返す(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "sw-find@example.com")
	days := seedSevenDays(t, pool) // 既存テストが使う7日分の DayMenu を作るヘルパ
	id, err := repo.Save(ctx, u.ID, days)
	require.NoError(t, err)

	got, err := repo.Find(ctx, u.ID, id)
	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	require.Len(t, got.Days, 7, "中身の7日分が詰まっているべき")
}

func TestSavedWeeklyRepository_Find_他人の週は見つからない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	owner := createUser(t, pool, "sw-find-owner@example.com")
	other := createUser(t, pool, "sw-find-other@example.com")
	id, err := repo.Save(ctx, owner.ID, seedSevenDays(t, pool))
	require.NoError(t, err)

	_, err = repo.Find(ctx, other.ID, id)
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuNotFound, "他人の週は存在を明かさず not found")
}

func TestSavedWeeklyRepository_Find_無い週は見つからない(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewSavedWeeklyMenuRepository(pool)

	u := createUser(t, pool, "sw-find-missing@example.com")
	_, err := repo.Find(ctx, u.ID, domain.NewSavedWeeklyMenuID())
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuNotFound)
}
```

> `seedSevenDays` は既存の `saved_weekly_test.go` に該当ヘルパがあればそれを使う。無ければ既存の `Save` テストが7日分を組み立てている箇所を関数に抽出する（`insertMenu` を7回呼び `[]domain.DayMenu` を返すだけ）。抽出も同じ 🔴 コミットに含める。

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/repository/ -run TestSavedWeeklyRepository_Find`
Expected: FAIL（`repo.Find undefined`）

- [ ] **Step 3: 🔴 をコミットする**

```bash
git add backend/internal/repository/saved_weekly_test.go
git commit -m "test: 保存した週間献立の所有者スコープ取得"
```

- [ ] **Step 4: ポートに足す**

`service/saved_weekly.go` の `SavedWeeklyMenuStore` インターフェースに足す:

```go
	// Find は本人の保存を1件、中身の7日分も含めて返す。
	// 他人のもの・存在しないものは ErrSavedWeeklyMenuNotFound を返す（存在を明かさない）。
	Find(ctx context.Context, userID domain.UserID, id domain.SavedWeeklyMenuID) (domain.SavedWeeklyMenu, error)
```

- [ ] **Step 5: リポジトリに実装する**

`repository/saved_weekly.go` に足す（既存の `fillDays` を再利用する）:

```go
// Find は本人の保存を1件、中身の7日分も含めて返す。
//
// user_id でも絞るため、他人の行には構造上たどり着けない。
// 見つからなければ「無い」と「他人のもの」を区別せず ErrSavedWeeklyMenuNotFound
// を返す（区別すると他人の保存の存在を明かす。Delete と同じ扱い）。
func (r *SavedWeeklyMenuRepository) Find(
	ctx context.Context, userID domain.UserID, id domain.SavedWeeklyMenuID,
) (domain.SavedWeeklyMenu, error) {
	var createdAt time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT created_at FROM saved_weekly_menus WHERE id = $1 AND user_id = $2`,
		id.String(), userID.String()).Scan(&createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.SavedWeeklyMenu{}, service.ErrSavedWeeklyMenuNotFound
		}
		return domain.SavedWeeklyMenu{}, fmt.Errorf("保存した週間献立の取得に失敗しました: %w", err)
	}

	week := domain.SavedWeeklyMenu{
		ID:        id,
		Days:      make([]domain.DayMenu, 0, domain.WeekLength),
		CreatedAt: createdAt,
	}
	// fillDays は複数週用だが、1件でもそのまま使える。
	idStr := id.String()
	saved := []domain.SavedWeeklyMenu{week}
	if err := r.fillDays(ctx, []string{idStr}, map[string]int{idStr: 0}, saved); err != nil {
		return domain.SavedWeeklyMenu{}, err
	}
	return saved[0], nil
}
```

- [ ] **Step 6: 既存 fake に `Find` を足す**

`service/saved_weekly_test.go` の `fakeSavedWeeklyStore` に足す（保存済みを map で持つ流儀に合わせる。既存フィールドを使い、無ければ `saved map[...]domain.SavedWeeklyMenu` を追加）:

```go
func (s *fakeSavedWeeklyStore) Find(
	_ context.Context, userID domain.UserID, id domain.SavedWeeklyMenuID,
) (domain.SavedWeeklyMenu, error) {
	w, ok := s.byID[id] // fake が保持する週。owner が一致しなければ not found
	if !ok || w.owner != userID {
		return domain.SavedWeeklyMenu{}, service.ErrSavedWeeklyMenuNotFound
	}
	return w.menu, nil
}
```

> 既存 `fakeSavedWeeklyStore` の内部表現に合わせて調整すること（`byID`/`owner`/`menu` はこの実装が持っていなければ足す）。`Save` で採番したIDと owner を覚え、`Find` で引けるようにするのが要点。既存の `Save`/`List`/`Count`/`Delete` の挙動は変えない。

- [ ] **Step 7: テストが通ることを確認する**

Run: `cd backend && go build ./... && go test ./internal/repository/ -run TestSavedWeeklyRepository_Find && go test ./internal/service/`
Expected: PASS（service の既存テストも fake が新メソッドを実装したことで通り続ける）

- [ ] **Step 8: 🟢 をコミットする**

```bash
git add backend/internal/service/saved_weekly.go \
        backend/internal/repository/saved_weekly.go \
        backend/internal/service/saved_weekly_test.go
git commit -m "feat: 保存した週間献立を所有者スコープで1件取得する"
```

---

### Task 6: `SavedShoppingListService.For`（保存済み週の買い物リスト＝差分の重ね合わせ）

保存済みの週から買い物リストを導出し、premium のときだけ差分を重ねて返す。free は導出結果そのまま。

**Files:**
- Create: `backend/internal/service/saved_shopping_list.go`
- Test: `backend/internal/service/saved_shopping_list_test.go`

**Interfaces:**
- Consumes: `SavedWeeklyMenuStore`（`Find`）、`ShoppingListOverrideStore`（`FindBySavedWeeklyMenu`）、`Entitlements`（`For`）、`ShoppingItem`（既存）、`domain.MenuID`、`domain.Origin`、`domain.IngredientCategory`。
- Produces:
  - `type ShoppingListDeriver interface { Build(ctx context.Context, ids []domain.MenuID) ([]ShoppingItem, error) }`（`*ShoppingListService` が満たす）。
  - `type SavedShoppingItem struct { Name string; NameKana string; Category domain.IngredientCategory; Origin domain.Origin; Checked bool; UsedIn []domain.Menu }`
  - `NewSavedShoppingListService(deriver ShoppingListDeriver, saved SavedWeeklyMenuStore, overrides ShoppingListOverrideStore, entitlements Entitlements) *SavedShoppingListService`
  - `(*SavedShoppingListService).For(ctx, userID string, savedWeeklyMenuID string) ([]SavedShoppingItem, error)`

- [ ] **Step 1: fake の差分ストアを書く**

`saved_shopping_list_test.go`（テスト用の差分ストアと deriver。既存の `fakeSavedWeeklyStore` / `fakeEntitlements` は同パッケージにあるので再利用する）:

```go
package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeOverrideStore は差分をメモリに持つ。
type fakeOverrideStore struct {
	byWeek map[domain.SavedWeeklyMenuID][]domain.ShoppingListOverride
	err    error
}

func newFakeOverrideStore() *fakeOverrideStore {
	return &fakeOverrideStore{byWeek: map[domain.SavedWeeklyMenuID][]domain.ShoppingListOverride{}}
}

func (s *fakeOverrideStore) FindBySavedWeeklyMenu(
	_ context.Context, id domain.SavedWeeklyMenuID,
) ([]domain.ShoppingListOverride, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.byWeek[id], nil
}

func (s *fakeOverrideStore) Replace(
	_ context.Context, id domain.SavedWeeklyMenuID, overrides []domain.ShoppingListOverride,
) error {
	if s.err != nil {
		return s.err
	}
	s.byWeek[id] = overrides
	return nil
}

// fakeDeriver は導出結果を固定で返す。導出そのものは ShoppingListService のテストで検証済み。
type fakeDeriver struct {
	items []service.ShoppingItem
	err   error
}

func (d fakeDeriver) Build(_ context.Context, _ []domain.MenuID) ([]service.ShoppingItem, error) {
	return d.items, d.err
}

// ing は導出結果1件を組み立てる。
func ing(name, kana string, cat domain.IngredientCategory) service.ShoppingItem {
	return service.ShoppingItem{
		Ingredient: domain.Ingredient{ID: domain.NewIngredientID(), Name: name, NameKana: kana, Category: cat},
	}
}
```

- [ ] **Step 2: 失敗するテストを書く**

同ファイルに足す:

```go
// setupForTest は For を呼ぶための一式を用意し、保存済み週のIDを返す。
func setupForTest(t *testing.T, plan domain.Plan, derived []service.ShoppingItem) (
	*service.SavedShoppingListService, *fakeOverrideStore, string, string,
) {
	t.Helper()
	saved := newFakeSavedWeeklyStore(t) // 既存 fakeSavedWeeklyStore を作るヘルパ（無ければ &fakeSavedWeeklyStore{...} を直接）
	userID := domain.NewUserID().String()
	weekID := saved.putWeek(t, userID) // 本人の週を1件登録し、その id 文字列を返す（fake に足すヘルパ）
	overrides := newFakeOverrideStore()
	svc := service.NewSavedShoppingListService(
		fakeDeriver{items: derived}, saved, overrides, fakeEntitlements{plan: plan})
	return svc, overrides, userID, weekID
}

func TestSavedShoppingListService_For_freeは導出そのまま(t *testing.T) {
	t.Parallel()
	derived := []service.ShoppingItem{
		ing("にんじん", "にんじん", domain.CategoryVegetable),
		ing("豚肉", "ぶたにく", domain.CategoryMeat),
	}
	svc, overrides, userID, weekID := setupForTest(t, domain.PlanFree, derived)
	// free でも差分行があっても無視される。
	wid, _ := domain.ParseSavedWeeklyMenuID(weekID)
	overrides.byWeek[wid] = []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: wid, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
	}

	items, err := svc.For(context.Background(), userID, weekID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	for _, it := range items {
		require.False(t, it.Checked, "free は差分を重ねないので全て未チェック")
	}
}

func TestSavedShoppingListService_For_premiumは差分を重ねる(t *testing.T) {
	t.Parallel()
	derived := []service.ShoppingItem{
		ing("にんじん", "にんじん", domain.CategoryVegetable),
		ing("豚肉", "ぶたにく", domain.CategoryMeat),
		ing("たまねぎ", "たまねぎ", domain.CategoryVegetable),
	}
	svc, overrides, userID, weekID := setupForTest(t, domain.PlanPremium, derived)
	wid, _ := domain.ParseSavedWeeklyMenuID(weekID)
	overrides.byWeek[wid] = []domain.ShoppingListOverride{
		{SavedWeeklyMenuID: wid, Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true},
		{SavedWeeklyMenuID: wid, Name: "たまねぎ", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Hidden: true},
		{SavedWeeklyMenuID: wid, Name: "牛乳", Category: domain.CategoryDairyEgg, Origin: domain.OriginManual, Checked: false},
	}

	items, err := svc.For(context.Background(), userID, weekID)
	require.NoError(t, err)

	byName := map[string]service.SavedShoppingItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	require.True(t, byName["にんじん"].Checked, "チェックが重なる")
	require.NotContains(t, byName, "たまねぎ", "hidden は表示から外れる")
	require.Contains(t, byName, "牛乳", "手動品目が足される")
	require.Equal(t, domain.OriginManual, byName["牛乳"].Origin)
	require.Contains(t, byName, "豚肉", "差分の無い導出品目は残る")
}

func TestSavedShoppingListService_For_他人の週は404(t *testing.T) {
	t.Parallel()
	svc, _, _, weekID := setupForTest(t, domain.PlanPremium, nil)
	// 別ユーザーで引く。
	_, err := svc.For(context.Background(), domain.NewUserID().String(), weekID)
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuNotFound)
}

func TestSavedShoppingListService_For_並びはカテゴリ順カナ順(t *testing.T) {
	t.Parallel()
	derived := []service.ShoppingItem{
		ing("豚肉", "ぶたにく", domain.CategoryMeat),
		ing("にんじん", "にんじん", domain.CategoryVegetable),
	}
	svc, _, userID, weekID := setupForTest(t, domain.PlanPremium, derived)
	items, err := svc.For(context.Background(), userID, weekID)
	require.NoError(t, err)
	require.Equal(t, "にんじん", items[0].Name, "野菜が肉より先")
	require.Equal(t, "豚肉", items[1].Name)
}
```

> `newFakeSavedWeeklyStore` / `putWeek` は Task 5 で `fakeSavedWeeklyStore` に足した内部表現に合わせたヘルパ。無ければ Task 5 で足した `byID` map に本人の週（`Days` は空でよい。deriver が導出を担うため）を直接入れる小さなヘルパをこのファイルに置く。`For` は週の `Days` から献立IDを取り出して deriver に渡すだけなので、テストでは `Days` に適当な `DayMenu{Menu: domain.Menu{ID: domain.NewMenuID()}}` を1つ入れておけばよい（deriver は ID を見ない）。

- [ ] **Step 3: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/service/ -run TestSavedShoppingListService_For`
Expected: FAIL（`service.NewSavedShoppingListService undefined`）

- [ ] **Step 4: 🔴 をコミットする**

```bash
git add backend/internal/service/saved_shopping_list_test.go
git commit -m "test: 保存済み週の買い物リストの差分重ね合わせ"
```

- [ ] **Step 5: 実装する**

`saved_shopping_list.go`:

```go
package service

import (
	"context"
	"sort"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ShoppingListDeriver は献立の集合から買い物リストを導出する。
// *ShoppingListService が満たす。差分の重ね合わせはこの導出結果に対して行う。
type ShoppingListDeriver interface {
	Build(ctx context.Context, ids []domain.MenuID) ([]ShoppingItem, error)
}

// SavedShoppingItem は保存済み週の買い物リストの1項目（差分適用後）。
//
// 導出品目（origin=derived）は Ingredient 由来の名前・カナ・カテゴリを持ち、
// 手動品目（origin=manual）は利用者が付けた名前とカテゴリを持つ（カナは名前で代用）。
type SavedShoppingItem struct {
	Name     string
	NameKana string
	Category domain.IngredientCategory
	Origin   domain.Origin
	Checked  bool
	// UsedIn はその食材を使う献立。手動品目では空。
	UsedIn []domain.Menu
}

// SavedShoppingListService は保存済み週の買い物リストを、差分を重ねて返す。
type SavedShoppingListService struct {
	deriver      ShoppingListDeriver
	saved        SavedWeeklyMenuStore
	overrides    ShoppingListOverrideStore
	entitlements Entitlements
}

// NewSavedShoppingListService は SavedShoppingListService を生成する。
func NewSavedShoppingListService(
	deriver ShoppingListDeriver, saved SavedWeeklyMenuStore,
	overrides ShoppingListOverrideStore, entitlements Entitlements,
) *SavedShoppingListService {
	return &SavedShoppingListService{
		deriver: deriver, saved: saved, overrides: overrides, entitlements: entitlements,
	}
}

// For は保存済み週の買い物リストを返す。
//
// 導出は毎回行い、premium のときだけ差分を重ねる。free では差分を無視するため
// 従来の買い物リストと同じ結果になる（設計 8.2）。
// 他人の週・存在しない週は ErrSavedWeeklyMenuNotFound（404）。
func (s *SavedShoppingListService) For(
	ctx context.Context, userID, savedWeeklyMenuID string,
) ([]SavedShoppingItem, error) {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	sid, err := domain.ParseSavedWeeklyMenuID(savedWeeklyMenuID)
	if err != nil {
		return nil, err
	}

	// 所有者検証も兼ねる。他人の週なら not found。
	week, err := s.saved.Find(ctx, uid, sid)
	if err != nil {
		return nil, err
	}

	ids := make([]domain.MenuID, 0, len(week.Days))
	for _, d := range week.Days {
		ids = append(ids, d.Menu.ID)
	}
	derived, err := s.deriver.Build(ctx, ids)
	if err != nil {
		return nil, err
	}

	base := make([]SavedShoppingItem, 0, len(derived))
	for _, it := range derived {
		base = append(base, SavedShoppingItem{
			Name:     it.Ingredient.Name,
			NameKana: it.Ingredient.NameKana,
			Category: it.Ingredient.Category,
			Origin:   domain.OriginDerived,
			UsedIn:   it.UsedIn,
		})
	}

	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !ent.CanPersistShoppingList() {
		sortSavedItems(base)
		return base, nil
	}

	overrides, err := s.overrides.FindBySavedWeeklyMenu(ctx, sid)
	if err != nil {
		return nil, err
	}
	return mergeOverlay(base, overrides), nil
}

// mergeOverlay は導出結果に差分を重ねる。
//
// 名前を項目の同一性とする。導出品目に同名の差分があればチェック/非表示を適用し、
// 導出に無い名前の手動品目を足す。hidden は表示から外す。
func mergeOverlay(base []SavedShoppingItem, overrides []domain.ShoppingListOverride) []SavedShoppingItem {
	byName := make(map[string]domain.ShoppingListOverride, len(overrides))
	for _, o := range overrides {
		byName[o.Name] = o
	}
	baseNames := make(map[string]bool, len(base))

	out := make([]SavedShoppingItem, 0, len(base)+len(overrides))
	for _, it := range base {
		baseNames[it.Name] = true
		if o, ok := byName[it.Name]; ok {
			if o.Hidden {
				continue // 消された導出品目は出さない
			}
			it.Checked = o.Checked
		}
		out = append(out, it)
	}
	for _, o := range overrides {
		if o.Origin != domain.OriginManual || o.Hidden || baseNames[o.Name] {
			continue
		}
		out = append(out, SavedShoppingItem{
			Name:     o.Name,
			NameKana: o.Name, // 手動品目はカナを持たないので名前で並べる
			Category: o.Category,
			Origin:   domain.OriginManual,
			Checked:  o.Checked,
		})
	}
	sortSavedItems(out)
	return out
}

// sortSavedItems はカテゴリ順→カナ順に並べる（既存 aggregate と同じ規則）。
func sortSavedItems(items []SavedShoppingItem) {
	sort.SliceStable(items, func(a, b int) bool {
		ca, cb := items[a].Category.Order(), items[b].Category.Order()
		if ca != cb {
			return ca < cb
		}
		return items[a].NameKana < items[b].NameKana
	})
}
```

- [ ] **Step 6: `*ShoppingListService` が deriver を満たすことを確認する**

`shopping_list.go` は既に `Build(ctx, []domain.MenuID) ([]ShoppingItem, error)` を持つ。適合チェックを `saved_shopping_list.go` に足す:

```go
var _ ShoppingListDeriver = (*ShoppingListService)(nil)
```

- [ ] **Step 7: テストが通ることを確認する**

Run: `cd backend && go build ./... && go test ./internal/service/ -run TestSavedShoppingListService_For`
Expected: PASS

- [ ] **Step 8: 🟢 をコミットする**

```bash
git add backend/internal/service/saved_shopping_list.go
git commit -m "feat: 保存済み週の買い物リストに差分を重ねて返す"
```

---

### Task 7: `SavedShoppingListService.ReplaceOverrides`（差分の一括置換・premium 限定）

差分を丸ごと置き換える。free は 403、手動品目100件超は 409。

**Files:**
- Modify: `backend/internal/service/saved_shopping_list.go`（メソッドと `OverrideInput` / エラーを足す）
- Modify: `backend/internal/service/saved_shopping_list_test.go`
- Modify: `backend/internal/handler/problem.go`（403/409 の写像）
- Modify: `backend/internal/handler/problem_coverage_test.go` は不要（エラーを写像するため）

**Interfaces:**
- Consumes: 上記の依存に加え `domain.ShoppingListOverride.Validate`、`domain.IngredientCategory`、`domain.Origin`。
- Produces:
  - `type OverrideInput struct { Name string; Category string; Origin string; Checked bool; Hidden bool }`
  - `var ErrPremiumRequired`、`var ErrShoppingListItemLimitReached`
  - `(*SavedShoppingListService).ReplaceOverrides(ctx, userID string, savedWeeklyMenuID string, inputs []OverrideInput) error`

- [ ] **Step 1: 失敗するテストを書く**

`saved_shopping_list_test.go` に足す:

```go
func manualInput(name string) service.OverrideInput {
	return service.OverrideInput{Name: name, Category: "other", Origin: "manual"}
}

func TestReplaceOverrides_freeは403(t *testing.T) {
	t.Parallel()
	svc, _, userID, weekID := setupForTest(t, domain.PlanFree, nil)
	err := svc.ReplaceOverrides(context.Background(), userID, weekID,
		[]service.OverrideInput{manualInput("牛乳")})
	require.ErrorIs(t, err, service.ErrPremiumRequired)
}

func TestReplaceOverrides_premiumは保存する(t *testing.T) {
	t.Parallel()
	svc, overrides, userID, weekID := setupForTest(t, domain.PlanPremium, nil)
	err := svc.ReplaceOverrides(context.Background(), userID, weekID, []service.OverrideInput{
		{Name: "にんじん", Category: "vegetable", Origin: "derived", Checked: true},
		manualInput("牛乳"),
	})
	require.NoError(t, err)
	wid, _ := domain.ParseSavedWeeklyMenuID(weekID)
	require.Len(t, overrides.byWeek[wid], 2)
}

func TestReplaceOverrides_他人の週は404(t *testing.T) {
	t.Parallel()
	svc, _, _, weekID := setupForTest(t, domain.PlanPremium, nil)
	err := svc.ReplaceOverrides(context.Background(), domain.NewUserID().String(), weekID,
		[]service.OverrideInput{manualInput("牛乳")})
	require.ErrorIs(t, err, service.ErrSavedWeeklyMenuNotFound)
}

func TestReplaceOverrides_手動品目100件超は409(t *testing.T) {
	t.Parallel()
	svc, _, userID, weekID := setupForTest(t, domain.PlanPremium, nil)
	inputs := make([]service.OverrideInput, 0, 101)
	for i := 0; i < 101; i++ {
		inputs = append(inputs, manualInput("品目"+itoa(i)))
	}
	err := svc.ReplaceOverrides(context.Background(), userID, weekID, inputs)
	require.ErrorIs(t, err, service.ErrShoppingListItemLimitReached)
}

func TestReplaceOverrides_不正なカテゴリは400(t *testing.T) {
	t.Parallel()
	svc, _, userID, weekID := setupForTest(t, domain.PlanPremium, nil)
	err := svc.ReplaceOverrides(context.Background(), userID, weekID,
		[]service.OverrideInput{{Name: "牛乳", Category: "spice", Origin: "manual"}})
	require.ErrorIs(t, err, domain.ErrInvalidOverride)
}

func TestReplaceOverrides_名前の重複は400(t *testing.T) {
	t.Parallel()
	svc, _, userID, weekID := setupForTest(t, domain.PlanPremium, nil)
	err := svc.ReplaceOverrides(context.Background(), userID, weekID, []service.OverrideInput{
		manualInput("牛乳"), manualInput("牛乳"),
	})
	require.ErrorIs(t, err, domain.ErrInvalidOverride)
}
```

> `itoa` は `strconv.Itoa`。テストファイル冒頭で import する。

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/service/ -run TestReplaceOverrides`
Expected: FAIL（`ReplaceOverrides undefined`）

- [ ] **Step 3: 🔴 をコミットする**

```bash
git add backend/internal/service/saved_shopping_list_test.go
git commit -m "test: 差分の一括置換（403/409/400）"
```

- [ ] **Step 4: 実装する**

`saved_shopping_list.go` に足す（import に `errors`, `fmt`, `strings` を追加）:

```go
// maxManualShoppingItems は1つの買い物リストに手で足せる品目の上限（設計 7.2）。
// 上限そのものに意味はないが、無制限の利用者由来レコードが Neon の無料枠を
// 圧迫しないための歯止め。
const maxManualShoppingItems = 100

var (
	// ErrPremiumRequired はプレミアムプランでのみ使える操作を free が試みたことを表す（403）。
	ErrPremiumRequired = errors.New("プレミアムプランが必要です")

	// ErrShoppingListItemLimitReached は手動品目が上限に達したことを表す（409）。
	ErrShoppingListItemLimitReached = errors.New("追加できる品目の上限に達しました")
)

// OverrideInput は差分1件の生の指定。APIの値をそのまま受ける（SavedDayInput と同じ流儀）。
type OverrideInput struct {
	Name     string
	Category string
	Origin   string
	Checked  bool
	Hidden   bool
}

// ReplaceOverrides は保存済み週の差分を丸ごと置き換える（設計 3.5）。
//
// free は ErrPremiumRequired（403）。他人の週は ErrSavedWeeklyMenuNotFound（404）。
// 手動品目が上限超なら ErrShoppingListItemLimitReached（409）。
// 名前の重複・不正なカテゴリ/由来は domain.ErrInvalidOverride（400）。
func (s *SavedShoppingListService) ReplaceOverrides(
	ctx context.Context, userID, savedWeeklyMenuID string, inputs []OverrideInput,
) error {
	uid, err := domain.ParseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	sid, err := domain.ParseSavedWeeklyMenuID(savedWeeklyMenuID)
	if err != nil {
		return err
	}

	// 権限を先に見る。free には保存経路を一切開かない。
	ent, err := s.entitlements.For(ctx, userID)
	if err != nil {
		return err
	}
	if !ent.CanPersistShoppingList() {
		return ErrPremiumRequired
	}

	// 所有者検証。他人の週なら 404。
	if _, err := s.saved.Find(ctx, uid, sid); err != nil {
		return err
	}

	overrides := make([]domain.ShoppingListOverride, 0, len(inputs))
	seen := make(map[string]bool, len(inputs))
	manual := 0
	for _, in := range inputs {
		o := domain.ShoppingListOverride{
			SavedWeeklyMenuID: sid,
			Name:              strings.TrimSpace(in.Name),
			// Parse は使わず、Validate に不正値を弾かせる（不正は 400=ErrInvalidOverride に寄せる。
			// ParseIngredientCategory の ErrInvalidIngredientCategory は DB 読み取り用で 500 扱いのため）。
			Category: domain.IngredientCategory(strings.TrimSpace(in.Category)),
			Origin:   domain.Origin(strings.TrimSpace(in.Origin)),
			Checked:  in.Checked,
			Hidden:   in.Hidden,
		}
		if err := o.Validate(); err != nil {
			return err
		}
		if seen[o.Name] {
			return fmt.Errorf("%w: 品目名が重複しています: %q", domain.ErrInvalidOverride, o.Name)
		}
		seen[o.Name] = true
		if o.Origin == domain.OriginManual {
			manual++
		}
		overrides = append(overrides, o)
	}
	if manual > maxManualShoppingItems {
		return fmt.Errorf("%w: 追加できる品目は%d件までです", ErrShoppingListItemLimitReached, maxManualShoppingItems)
	}

	return s.overrides.Replace(ctx, sid, overrides)
}
```

- [ ] **Step 5: 写像を足す（同じPR内で。CIを緑に保つため）**

`handler/problem.go` の `problemMapping` に足す（409 の仲間の並びに）:

```go
	// プレミアム専用の操作を free が試みた。403。
	{service.ErrPremiumRequired, http.StatusForbidden, "premium-required", "プレミアムプランが必要です"},
	// 手動品目が上限に達した。既存の保存上限と同じく今の状態との競合なので 409。
	{service.ErrShoppingListItemLimitReached, http.StatusConflict, "shopping-list-item-limit-reached", "追加できる品目の上限に達しました"},
```

- [ ] **Step 6: テストが通ることを確認する**

Run: `cd backend && go build ./... && go test ./internal/service/ -run TestReplaceOverrides && go test ./internal/handler/ -run TestProblemMapping`
Expected: PASS

- [ ] **Step 7: 🟢 をコミットする**

```bash
git add backend/internal/service/saved_shopping_list.go \
        backend/internal/handler/problem.go
git commit -m "feat: 買い物リストの差分を一括置換する（premium限定）"
```

---

### Task 8: `GET /weekly-menus/:id/shopping-list`（保存済み週の買い物リスト）

差分適用後のリストを返す。free でも呼べる（差分が重ならないだけ、形は同じ）。認証必須。

**Files:**
- Modify: `api/openapi.yaml`（パスとスキーマ）
- Create: `backend/internal/handler/saved_shopping_list.go`
- Test: `backend/internal/handler/saved_shopping_list_test.go`
- Modify: `backend/internal/handler/contract_test.go`（契約テスト）
- Modify: `backend/cmd/api/main.go`（結線）
- Modify: `frontend/src/api/schema.d.ts`（`make gen-api` で再生成）

**Interfaces:**
- Consumes: `service.SavedShoppingListService.For`、`service.SavedShoppingItem`、`auth.JWT`、`RequireAuth`、`UserIDFromContext`、`APIBasePath`。
- Produces:
  - `SavedShoppingListUseCase` インターフェース（`For` を含む。PUT は Task 9 で足す）
  - `SavedShoppingListHandler` と `GET /weekly-menus/:id/shopping-list`
  - openapi: `SavedShoppingItem` / `SavedShoppingListResponse` / `Origin` スキーマ、`GET` パス。

- [ ] **Step 1: openapi にパスとスキーマを足す**

`api/openapi.yaml` の `/api/v1/weekly-menus/{id}` の後に足す:

```yaml
  /api/v1/weekly-menus/{id}/shopping-list:
    get:
      tags: [weekly-menus]
      summary: 保存済みの週の買い物リストを取得する
      description: |
        保存済みの週間献立から買い物リストを導出し、差分（チェック・手動品目・非表示）を
        重ねて返す。差分の重ね合わせはサーバで行い、フロントには最終形だけを渡す。
        free でも呼べる（差分が重ならないだけで形は同じ）。他人の週は 404。
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '200':
          description: 差分適用後の買い物リスト（カテゴリ順→カナ順）
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SavedShoppingListResponse'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '404':
          $ref: '#/components/responses/NotFound'
```

`components/schemas` に足す（`ShoppingListResponse` の後）:

```yaml
    Origin:
      type: string
      description: 差分行の由来。derived=献立由来 / manual=手動追加。
      enum: [derived, manual]

    SavedShoppingItem:
      type: object
      required: [name, category, origin, checked, usedIn]
      properties:
        name:
          type: string
        category:
          $ref: '#/components/schemas/IngredientCategory'
        origin:
          $ref: '#/components/schemas/Origin'
        checked:
          type: boolean
        usedIn:
          type: array
          description: その食材を使う献立。手動品目では空。
          items:
            type: object
            required: [id, name]
            properties:
              id:
                type: string
                format: uuid
              name:
                type: string

    SavedShoppingListResponse:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items:
            $ref: '#/components/schemas/SavedShoppingItem'
```

> `IngredientCategory` スキーマが未定義なら、`Ingredient` の `category` が使っている enum 定義を参照する（既存 `Ingredient` スキーマの `category` プロパティを見て、共通の `IngredientCategory` enum に切り出すか、その enum を inline で複製する。既存に合わせること）。

- [ ] **Step 2: 型を再生成し、差分を確認する**

Run: `make gen-api`
Expected: `frontend/src/api/schema.d.ts` に `SavedShoppingItem` / `SavedShoppingListResponse` / `Origin` と新パスが追加される。`git diff` で確認。

- [ ] **Step 3: 🔴 のテストを書く（ハンドラ + 契約）**

`handler/saved_shopping_list_test.go`:

```go
package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeSavedShoppingList は SavedShoppingListUseCase の fake。
type fakeSavedShoppingList struct {
	items []service.SavedShoppingItem
	err   error
	// Replace 用（Task 9 で使う）
	replaceErr error
	replaced   []service.OverrideInput
}

func (f *fakeSavedShoppingList) For(_ context.Context, _, _ string) ([]service.SavedShoppingItem, error) {
	return f.items, f.err
}
func (f *fakeSavedShoppingList) ReplaceOverrides(_ context.Context, _, _ string, in []service.OverrideInput) error {
	f.replaced = in
	return f.replaceErr
}

func TestSavedShoppingListHandler_Get_200(t *testing.T) {
	svc := &fakeSavedShoppingList{items: []service.SavedShoppingItem{
		{Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived, Checked: true,
			UsedIn: []domain.Menu{{ID: domain.NewMenuID(), Name: "肉じゃが"}}},
		{Name: "牛乳", Category: domain.CategoryDairyEgg, Origin: domain.OriginManual},
	}}
	rec := doAuthedGet(t, svc, "/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"items":[
		{"name":"にんじん","category":"vegetable","origin":"derived","checked":true,"usedIn":[{"id":"`+
		"...", rec.Body.String()) // 実際は構造体でデコードして field を検証する（下記参照）
}

func TestSavedShoppingListHandler_Get_404(t *testing.T) {
	svc := &fakeSavedShoppingList{err: service.ErrSavedWeeklyMenuNotFound}
	rec := doAuthedGet(t, svc, "/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSavedShoppingListHandler_Get_未認証は401(t *testing.T) {
	svc := &fakeSavedShoppingList{}
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list", nil)
	rec := httptest.NewRecorder()
	newSavedShoppingListEcho(t, svc).ServeHTTP(rec, req) // Cookie 無し
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

> `doAuthedGet` / `newSavedShoppingListEcho` は既存のハンドラテストが使う「echo を組んで認証Cookie付きリクエストを投げる」流儀のヘルパ。既存 `saved_weekly_test.go` のテストが同型のヘルパ（認証済みリクエストの組み立て）を持つので、それに倣って本ファイル用の小さなセットアップ関数を書く。`RequireAuth` を通すため有効な JWT Cookie を積む（既存テストの `issueAccessCookie` 等を再利用）。**JSON の検証は生文字列比較ではなく、レスポンスを DTO にデコードして field を assert する**（`にんじん` が checked、`牛乳` が origin=manual など）。上の `JSONEq` はイメージであり、実装時は構造体デコードに置き換えること。

契約テスト `handler/contract_test.go` に、GET のレスポンスが openapi の `SavedShoppingListResponse` に一致することを既存の契約テストの流儀（レスポンスを openapi 由来の型でデコードできること）で足す。

- [ ] **Step 4: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/handler/ -run TestSavedShoppingListHandler_Get`
Expected: FAIL（未実装）

- [ ] **Step 5: 🔴 をコミットする**

```bash
git add api/openapi.yaml frontend/src/api/schema.d.ts \
        backend/internal/handler/saved_shopping_list_test.go \
        backend/internal/handler/contract_test.go
git commit -m "test: 保存済み週の買い物リスト取得API"
```

- [ ] **Step 6: ハンドラを実装する**

`handler/saved_shopping_list.go`:

```go
package handler

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// SavedShoppingListUseCase は保存済み週の買い物リストのAPIが必要とする操作。
// 実装は service.SavedShoppingListService。
type SavedShoppingListUseCase interface {
	For(ctx context.Context, userID, savedWeeklyMenuID string) ([]service.SavedShoppingItem, error)
	ReplaceOverrides(ctx context.Context, userID, savedWeeklyMenuID string, inputs []service.OverrideInput) error
}

// SavedShoppingListHandler は保存済み週の買い物リストAPIの受け口。
type SavedShoppingListHandler struct {
	svc    SavedShoppingListUseCase
	tokens *auth.JWT
}

// NewSavedShoppingListHandler は SavedShoppingListHandler を生成する。
func NewSavedShoppingListHandler(svc SavedShoppingListUseCase, tokens *auth.JWT) *SavedShoppingListHandler {
	return &SavedShoppingListHandler{svc: svc, tokens: tokens}
}

// RegisterRoutes はルーティングを登録する。保存は本人のものだけを扱うため認証必須。
func (h *SavedShoppingListHandler) RegisterRoutes(e *echo.Echo) {
	g := e.Group(APIBasePath)
	requireAuth := RequireAuth(h.tokens)
	g.GET("/weekly-menus/:id/shopping-list", h.Get, requireAuth)
	g.PUT("/weekly-menus/:id/shopping-list", h.Replace, requireAuth) // 実体は Task 9
}

// savedShoppingUsedInDTO はその食材を使う献立。
type savedShoppingUsedInDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// savedShoppingItemDTO は差分適用後の買い物リストの1項目。
type savedShoppingItemDTO struct {
	Name     string                   `json:"name"`
	Category string                   `json:"category"`
	Origin   string                   `json:"origin"`
	Checked  bool                     `json:"checked"`
	UsedIn   []savedShoppingUsedInDTO `json:"usedIn"`
}

type savedShoppingListResponse struct {
	Items []savedShoppingItemDTO `json:"items"`
}

// Get は保存済み週の買い物リストを差分適用後の形で返す。
//
//	GET /api/v1/weekly-menus/:id/shopping-list
func (h *SavedShoppingListHandler) Get(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}
	items, err := h.svc.For(c.Request().Context(), userID, c.Param("id"))
	if err != nil {
		return err
	}

	out := make([]savedShoppingItemDTO, 0, len(items))
	for _, it := range items {
		usedIn := make([]savedShoppingUsedInDTO, 0, len(it.UsedIn))
		for _, m := range it.UsedIn {
			usedIn = append(usedIn, savedShoppingUsedInDTO{ID: m.ID.String(), Name: m.Name})
		}
		out = append(out, savedShoppingItemDTO{
			Name:     it.Name,
			Category: it.Category.String(),
			Origin:   it.Origin.String(),
			Checked:  it.Checked,
			UsedIn:   usedIn,
		})
	}
	return c.JSON(http.StatusOK, savedShoppingListResponse{Items: out})
}
```

> `Replace` メソッドは Task 9 で実装する。本タスクでは PUT ルートは登録しつつ、`Replace` は `echo.ErrMethodNotAllowed` を返す最小のスタブにしておくか、Task 9 まで PUT の登録行をコメントアウトしておく。**CIを緑に保つため、スタブにするなら PUT のテストは Task 9 まで書かない。** 推奨: 本タスクでは GET ルートだけ登録し、`RegisterRoutes` の PUT 行は Task 9 で足す。

- [ ] **Step 7: main.go に結線する**

`backend/cmd/api/main.go` に足す（既存の repo/service/handler 組み立ての並びに合わせる）:

```go
overrideRepo := repository.NewShoppingListOverrideRepository(pool)
savedShoppingListSvc := service.NewSavedShoppingListService(
	shoppingListSvc, savedWeeklyRepo, overrideRepo, entitlementSvc)
savedShoppingListHandler := handler.NewSavedShoppingListHandler(savedShoppingListSvc, jwt)
savedShoppingListHandler.RegisterRoutes(e)
```

> `shoppingListSvc`（既存の `*ShoppingListService`）を deriver として渡す。`savedWeeklyRepo` / `entitlementSvc` / `jwt` は既存の変数名に合わせる（main.go の現状を読んで正確に）。

- [ ] **Step 8: テストが通ることを確認する**

Run: `cd backend && go build ./... && go test ./internal/handler/ -run 'TestSavedShoppingListHandler_Get|TestProblemMapping' && go test ./...`
Expected: PASS

- [ ] **Step 9: 実機確認**

```bash
make up && make migrate && make seed
# 適当なユーザーでログイン→週を保存→その id で GET
```
Expected: 保存した週の買い物リストが 200 で返る（free なので checked は全て false）。他人の id で 404、未ログインで 401。

- [ ] **Step 10: 🟢 をコミットする**

```bash
git add backend/internal/handler/saved_shopping_list.go backend/cmd/api/main.go
git commit -m "feat: 保存済み週の買い物リスト取得APIを足す"
```

---

### Task 9: `PUT /weekly-menus/:id/shopping-list`（差分の一括置換）

差分を保存する。free は 403、手動品目100件超は 409。成功は 204。

**Files:**
- Modify: `api/openapi.yaml`（PUT を足す）
- Modify: `backend/internal/handler/saved_shopping_list.go`（`Replace` を実装、PUT ルート登録）
- Test: `backend/internal/handler/saved_shopping_list_test.go`
- Modify: `frontend/src/api/schema.d.ts`（`make gen-api`）

**Interfaces:**
- Consumes: `SavedShoppingListUseCase.ReplaceOverrides`、`service.OverrideInput`、`service.ErrPremiumRequired`、`service.ErrShoppingListItemLimitReached`。
- Produces: `PUT /weekly-menus/:id/shopping-list`（204）、openapi の request スキーマ `ShoppingListOverridesRequest`。

- [ ] **Step 1: openapi に PUT を足す**

`/api/v1/weekly-menus/{id}/shopping-list` に `put` を足す:

```yaml
    put:
      tags: [weekly-menus]
      summary: 保存済みの週の買い物リストの差分を置き換える
      description: |
        チェック状態・手動品目・非表示を overlay 全体として一括置換する（設計 3.5）。
        品目単位の部分更新はしない。free は 403。手動品目が上限を超えると 409。
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ShoppingListOverridesRequest'
      responses:
        '204':
          description: 置き換えた
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '403':
          description: プレミアムプランが必要
          content:
            application/problem+json:
              schema:
                $ref: '#/components/schemas/Problem'
        '404':
          $ref: '#/components/responses/NotFound'
        '409':
          description: 手動品目が上限に達している
          content:
            application/problem+json:
              schema:
                $ref: '#/components/schemas/Problem'
```

`components/schemas` に足す:

```yaml
    ShoppingListOverridesRequest:
      type: object
      required: [items]
      properties:
        items:
          type: array
          description: overlay 全体。導出品目のチェック/非表示と、手動品目を並べる。
          items:
            type: object
            required: [name, category, origin, checked, hidden]
            properties:
              name:
                type: string
              category:
                $ref: '#/components/schemas/IngredientCategory'
              origin:
                $ref: '#/components/schemas/Origin'
              checked:
                type: boolean
              hidden:
                type: boolean
```

- [ ] **Step 2: 型を再生成する**

Run: `make gen-api`
Expected: `schema.d.ts` に PUT と `ShoppingListOverridesRequest` が入る。

- [ ] **Step 3: 🔴 のテストを書く**

`handler/saved_shopping_list_test.go` に足す（`fakeSavedShoppingList` は Task 8 で `ReplaceOverrides` を実装済み）:

```go
func TestSavedShoppingListHandler_Put_204(t *testing.T) {
	svc := &fakeSavedShoppingList{}
	body := `{"items":[{"name":"にんじん","category":"vegetable","origin":"derived","checked":true,"hidden":false}]}`
	rec := doAuthedPut(t, svc, "/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list", body)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, svc.replaced, 1)
	require.Equal(t, "にんじん", svc.replaced[0].Name)
}

func TestSavedShoppingListHandler_Put_freeは403(t *testing.T) {
	svc := &fakeSavedShoppingList{replaceErr: service.ErrPremiumRequired}
	rec := doAuthedPut(t, svc, "/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list",
		`{"items":[]}`)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSavedShoppingListHandler_Put_上限で409(t *testing.T) {
	svc := &fakeSavedShoppingList{replaceErr: service.ErrShoppingListItemLimitReached}
	rec := doAuthedPut(t, svc, "/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list",
		`{"items":[]}`)
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestSavedShoppingListHandler_Put_壊れたボディは400(t *testing.T) {
	svc := &fakeSavedShoppingList{}
	rec := doAuthedPut(t, svc, "/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list",
		`{`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
```

> `doAuthedPut` は `doAuthedGet` と同型のヘルパ（本文と Content-Type: application/json を積む）。

- [ ] **Step 4: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/handler/ -run TestSavedShoppingListHandler_Put`
Expected: FAIL

- [ ] **Step 5: 🔴 をコミットする**

```bash
git add api/openapi.yaml frontend/src/api/schema.d.ts \
        backend/internal/handler/saved_shopping_list_test.go
git commit -m "test: 保存済み週の買い物リスト差分の置換API"
```

- [ ] **Step 6: `Replace` を実装する**

`handler/saved_shopping_list.go` に足す（Task 8 でスタブ/未登録にしていた PUT を有効化）:

```go
// putShoppingListOverridesRequest は PUT のリクエストボディ。
type putShoppingListOverridesRequest struct {
	Items []struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Origin   string `json:"origin"`
		Checked  bool   `json:"checked"`
		Hidden   bool   `json:"hidden"`
	} `json:"items"`
}

// Replace は保存済み週の買い物リストの差分を一括置換する。
//
//	PUT /api/v1/weekly-menus/:id/shopping-list
func (h *SavedShoppingListHandler) Replace(c echo.Context) error {
	userID, ok := UserIDFromContext(c)
	if !ok {
		return auth.ErrTokenInvalid
	}
	var req putShoppingListOverridesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "リクエストの形式が不正です")
	}

	inputs := make([]service.OverrideInput, 0, len(req.Items))
	for _, i := range req.Items {
		inputs = append(inputs, service.OverrideInput{
			Name: i.Name, Category: i.Category, Origin: i.Origin, Checked: i.Checked, Hidden: i.Hidden,
		})
	}

	if err := h.svc.ReplaceOverrides(c.Request().Context(), userID, c.Param("id"), inputs); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
```

（Task 8 で PUT 行をコメントアウトしていた場合は `RegisterRoutes` の `g.PUT(...)` を有効化する。）

- [ ] **Step 7: テストが通ることを確認する**

Run: `cd backend && go build ./... && go test ./internal/handler/ && go test ./...`
Expected: PASS

- [ ] **Step 8: 実機確認**

```bash
# premium ユーザーを付与して PUT→GET でチェックが残ることを確認
docker compose run --rm backend go run ./cmd/grant -email=<自分> -months=1
# ログイン → 週を保存 → PUT で にんじん を checked → GET で checked=true
# free ユーザーで PUT → 403
```

- [ ] **Step 9: 🟢 をコミットする**

```bash
git add api/openapi.yaml backend/internal/handler/saved_shopping_list.go
git commit -m "feat: 保存済み週の買い物リスト差分の置換APIを足す"
```

---

### Task 10: フロント — 保存済み週の買い物リストを GET で開く

保存済みの週を開いたときは新しい GET を使い、未保存の週は従来の `POST /shopping-list` のまま。表示の形は同じに揃える（チェックUIは Task 11）。

**Files:**
- Modify: `frontend/src/api/types.ts`（型の別名）
- Modify: `frontend/src/features/menu/api.ts`（`fetchSavedShoppingList`）
- Modify: `frontend/src/features/menu/ShoppingListPage.tsx`（savedId で分岐、表示形を統一）
- Modify: `frontend/src/features/menu/SavedWeeklyPage.tsx`（「開く」で savedId を持つ）
- Modify: `frontend/src/features/menu/WeeklyPage.tsx`（週を作り直したら savedId を捨てる、保存成功で savedId を持つ）
- Test: `frontend/src/features/menu/ShoppingListPage.test.tsx`

**Interfaces:**
- Consumes: 生成型 `components['schemas']['SavedShoppingItem']`、既存 `apiGet`、`useSessionState`、`useCurrentUser`。
- Produces:
  - types: `export type SavedShoppingItem = Schemas['SavedShoppingItem']`、`export type Origin = Schemas['Origin']`
  - api: `savedShoppingListQueryKey(savedId)`、`fetchSavedShoppingList(savedId): Promise<SavedShoppingItem[]>`
  - sessionStorage キー `weekly.savedId`（`useSessionState<string | null>('weekly.savedId', null)`）
  - `ShoppingListPage` 内の共通ビューモデル `ViewItem { key; name; category; usedIn; checked; origin }`

- [ ] **Step 1: 型と api を足す**

`api/types.ts` に足す:

```ts
export type SavedShoppingItem = Schemas['SavedShoppingItem']
export type Origin = Schemas['Origin']
```

`features/menu/api.ts` に足す:

```ts
export function savedShoppingListQueryKey(savedId: string) {
  return ['saved-shopping-list', savedId] as const
}

// fetchSavedShoppingList は保存済み週の買い物リストを差分適用後の形で取る。
export async function fetchSavedShoppingList(savedId: string): Promise<SavedShoppingItem[]> {
  const res = await apiGet<{ items: SavedShoppingItem[] }>(`/weekly-menus/${savedId}/shopping-list`)
  return res.items
}
```

（`import type { SavedShoppingItem } from '../../api/types'` を足す。）

- [ ] **Step 2: 失敗するテストを書く**

`ShoppingListPage.test.tsx` に足す（既存の `withWeek` / `respondShoppingList` の流儀を踏襲。保存IDの設定は `withSavedId`）:

```ts
function withSavedId(id: string) {
  sessionStorage.setItem('menu-planner:weekly.savedId', JSON.stringify(id))
}

test('保存済みの週を開くと GET /weekly-menus/:id/shopping-list を使う', async () => {
  const savedId = '11111111-1111-1111-1111-111111111111'
  withWeek([menu('m1', '肉じゃが')])
  withSavedId(savedId)

  let hit = false
  server.use(
    http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () => {
      hit = true
      return HttpResponse.json({
        items: [{ name: 'にんじん', category: 'vegetable', origin: 'derived', checked: false, usedIn: [{ id: 'm1', name: '肉じゃが' }] }],
      })
    }),
  )

  renderWithProviders(<ShoppingListPage />)
  expect(await screen.findByText('にんじん')).toBeInTheDocument()
  expect(hit).toBe(true)
})

test('未保存の週は従来どおり POST /shopping-list を使う', async () => {
  withWeek([menu('m1', '肉じゃが')]) // savedId は設定しない
  const bodies = respondShoppingList([item('たまねぎ', 'vegetable', ['肉じゃが'])])

  renderWithProviders(<ShoppingListPage />)
  expect(await screen.findByText('たまねぎ')).toBeInTheDocument()
  expect(bodies[0]).toEqual({ menuIds: ['m1'] })
})
```

- [ ] **Step 3: テストが失敗することを確認する**

Run: `cd frontend && npm test -- ShoppingListPage`
Expected: FAIL（GET のハンドラが呼ばれない＝未実装）

- [ ] **Step 4: 🔴 をコミットする**

```bash
git add frontend/src/api/types.ts frontend/src/features/menu/api.ts \
        frontend/src/features/menu/ShoppingListPage.test.tsx
git commit -m "test: 保存済み週の買い物リストをGETで開く"
```

- [ ] **Step 5: `ShoppingListPage` を分岐させる**

`ShoppingListPage.tsx` を次の骨格に変える（既存の `groupByCategory` 表示はそのまま使えるよう、両経路を共通の `ViewItem[]` に正規化する）:

```tsx
type ViewItem = {
  key: string
  name: string
  category: IngredientCategory
  usedIn: { id: string; name: string }[]
  checked: boolean
  origin: 'derived' | 'manual'
}

export function ShoppingListPage() {
  const [week] = useSessionState<DayMenu[] | null>('weekly.week', null)
  const [savedId] = useSessionState<string | null>('weekly.savedId', null)
  const menuIds = week?.map((d) => d.menu.id) ?? []

  // 保存済みの週: GET（差分適用後）。未保存: 従来の POST（ステートレス）。
  const saved = useQuery({
    queryKey: savedShoppingListQueryKey(savedId ?? ''),
    queryFn: () => fetchSavedShoppingList(savedId as string),
    enabled: savedId != null,
  })
  const derived = useQuery({
    queryKey: ['shopping-list', menuIds],
    queryFn: () => fetchShoppingList(menuIds),
    enabled: savedId == null && menuIds.length > 0,
  })

  const active = savedId != null ? saved : derived
  const items: ViewItem[] = savedId != null
    ? (saved.data ?? []).map((it) => ({
        key: it.name, name: it.name, category: it.category,
        usedIn: it.usedIn, checked: it.checked, origin: it.origin,
      }))
    : (derived.data ?? []).map((it) => ({
        key: it.ingredient.id, name: it.ingredient.name, category: it.ingredient.category,
        usedIn: it.usedIn, checked: false, origin: 'derived' as const,
      }))

  if (menuIds.length === 0) return /* 既存の空案内 + <Link to="/weekly"> をそのまま */
  if (active.isPending) return <MascotStatus />
  if (active.error) return <ErrorMessage error={active.error} />

  // 既存の groupByCategory(items) の表示に ViewItem を渡す。
  // （groupByCategory は it.ingredient.category を見ていたので、ViewItem.category を見るよう直す）
  return /* 既存のグルーピング表示。チェックボックスは Task 11 で足す */
}
```

> 既存 `groupByCategory` と `<li>` の描画は `ViewItem` を受けるよう最小限だけ直す（`it.ingredient.name` → `it.name`、`it.ingredient.category` → `it.category`、`it.usedIn` は同じ）。表示の見た目は変えない。

- [ ] **Step 6: 「開く」で savedId を持つ / 作り直しで捨てる**

`SavedWeeklyPage.tsx` の `open(week)`:

```tsx
const [, setSavedId] = useSessionState<string | null>('weekly.savedId', null)
// open の中で:
setWeek(week.days)
setFilter(emptyMenuFilter)
setSavedId(week.id)      // ← 足す。買い物リストがこの保存IDで永続化経路を使う
navigate('/weekly')
```

`WeeklyPage.tsx`:
- 新しい週を提案・1日引き直しする箇所（`setWeek(...)` を呼ぶ経路）の直後で `setSavedId(null)` を呼ぶ。未保存の週にしてしまうと、古い保存IDにひも付いた差分を誤って重ねてしまうため。
- 保存成功時（`save` の `onSuccess`）に `setSavedId(newId)` を呼ぶ。保存直後からその週を永続化経路にする。`saveWeeklyMenu` が返す id を使う。

- [ ] **Step 7: テストが通ることを確認する**

Run: `cd frontend && npx tsc -b && npm test -- ShoppingListPage SavedWeeklyPage WeeklyPage`
Expected: PASS（既存テストも壊れていないこと。`groupByCategory` の型変更に既存テストが追随しているか確認）

- [ ] **Step 8: 🟢 をコミットする**

```bash
git add frontend/src/features/menu/ShoppingListPage.tsx \
        frontend/src/features/menu/SavedWeeklyPage.tsx \
        frontend/src/features/menu/WeeklyPage.tsx
git commit -m "feat: 保存済み週の買い物リストをGET経路で開く"
```

---

### Task 11: フロント — チェックボックスを常に出し、premium×保存済みで永続化

チェックボックスを全リストに出す。premium かつ保存済みの週のときだけ PUT で永続化。それ以外はその場限り。

**Files:**
- Modify: `frontend/src/api/client.ts`（`apiPut` を足す）
- Modify: `frontend/src/features/menu/api.ts`（`saveShoppingListOverrides`）
- Modify: `frontend/src/features/menu/ShoppingListPage.tsx`（チェックボックス、トグル、PUT）
- Test: `frontend/src/features/menu/ShoppingListPage.test.tsx`

**Interfaces:**
- Consumes: `useCurrentUser`（`user.plan`）、`useMutation`、`useQueryClient`、`apiPut`。
- Produces:
  - `apiPut<T = void>(path: string, body?: unknown): Promise<T>`（`apiPost` と同型で method だけ PUT）
  - `saveShoppingListOverrides(savedId, items: ShoppingListOverride[]): Promise<void>`
  - `ShoppingListPage` にチェック状態のローカル管理と、premium×保存済み時の PUT。

- [ ] **Step 1: `apiPut` を足す（client.ts）**

`client.ts` の `apiPost` の隣に、同じ内部 `request` を使って:

```ts
export function apiPut<T = void>(path: string, body?: unknown): Promise<T> {
  return request<T>('PUT', path, body)
}
```

> 既存 `apiPost` が内部の共通 `request`（method 引数を取る）を呼ぶ形なら同じものを PUT で呼ぶ。`apiPost` が method を内包している実装なら、`apiPost` の実装を参考に PUT 版を作る。204 応答を `undefined` に落とす既存挙動をそのまま使う。

- [ ] **Step 2: 失敗するテストを書く**

`ShoppingListPage.test.tsx` に足す（`respondMe(plan)` は `AuthMenu.test.tsx` の流儀で `/auth/me` を差し替えるヘルパ。無ければローカルに定義）:

```ts
test('premium が保存済みの週でチェックすると PUT で永続化する', async () => {
  const savedId = '11111111-1111-1111-1111-111111111111'
  withWeek([menu('m1', '肉じゃが')]); withSavedId(savedId)
  respondMe('premium')
  server.use(http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
    HttpResponse.json({ items: [{ name: 'にんじん', category: 'vegetable', origin: 'derived', checked: false, usedIn: [] }] })))

  const puts: unknown[] = []
  server.use(http.put(`/api/v1/weekly-menus/${savedId}/shopping-list`, async ({ request }) => {
    puts.push(await request.json())
    return new HttpResponse(null, { status: 204 })
  }))

  renderWithProviders(<ShoppingListPage />)
  const box = await screen.findByRole('checkbox', { name: /にんじん/ })
  await userEvent.click(box)

  await waitFor(() => expect(puts.length).toBe(1))
  expect(puts[0]).toEqual({ items: [{ name: 'にんじん', category: 'vegetable', origin: 'derived', checked: true, hidden: false }] })
})

test('free はチェックしても PUT を投げない（その場限り）', async () => {
  const savedId = '11111111-1111-1111-1111-111111111111'
  withWeek([menu('m1', '肉じゃが')]); withSavedId(savedId)
  respondMe('free')
  server.use(http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
    HttpResponse.json({ items: [{ name: 'にんじん', category: 'vegetable', origin: 'derived', checked: false, usedIn: [] }] })))
  let put = false
  server.use(http.put(`/api/v1/weekly-menus/${savedId}/shopping-list`, () => { put = true; return new HttpResponse(null, { status: 204 }) }))

  renderWithProviders(<ShoppingListPage />)
  await userEvent.click(await screen.findByRole('checkbox', { name: /にんじん/ }))
  expect(await screen.findByRole('checkbox', { name: /にんじん/ })).toBeChecked() // 画面上は反映
  expect(put).toBe(false)
})
```

- [ ] **Step 3: テストが失敗することを確認する → 🔴 コミット**

Run: `cd frontend && npm test -- ShoppingListPage`（FAIL を確認）

```bash
git add frontend/src/api/client.ts frontend/src/features/menu/api.ts \
        frontend/src/features/menu/ShoppingListPage.test.tsx
git commit -m "test: 買い物リストのチェック永続化（premium×保存済み）"
```

- [ ] **Step 4: 実装する**

`api.ts` に足す:

```ts
import type { ShoppingListOverride } from '../../api/types' // = Schemas['ShoppingListOverridesRequest']['items'][number]

export async function saveShoppingListOverrides(savedId: string, items: ShoppingListOverride[]): Promise<void> {
  await apiPut(`/weekly-menus/${savedId}/shopping-list`, { items })
}
```

`types.ts` に足す:

```ts
export type ShoppingListOverride = Schemas['ShoppingListOverridesRequest']['items'][number]
```

`ShoppingListPage.tsx`（チェック状態のローカル管理と PUT）:

```tsx
const { user } = useCurrentUser()
const canPersist = savedId != null && user?.plan === 'premium'
const queryClient = useQueryClient()

// 表示中のチェック集合。GET/POST の結果が来たら初期化する。
const [checked, setChecked] = useState<Set<string>>(new Set())
useEffect(() => {
  setChecked(new Set(items.filter((it) => it.checked).map((it) => it.key)))
  // items の元データ（saved.data / derived.data）が変わったときだけ初期化
}, [saved.data, derived.data]) // eslint 上は items ではなくソースを見る

const persist = useMutation({
  mutationFn: (next: Set<string>) => {
    const overrides: ShoppingListOverride[] = items
      .filter((it) => next.has(it.key)) // Task 11 では checked=true の導出品目だけ送る
      .map((it) => ({ name: it.name, category: it.category, origin: 'derived', checked: true, hidden: false }))
    return saveShoppingListOverrides(savedId as string, overrides)
  },
  onSuccess: () => queryClient.invalidateQueries({ queryKey: savedShoppingListQueryKey(savedId as string) }),
})

function toggle(key: string) {
  setChecked((prev) => {
    const next = new Set(prev)
    next.has(key) ? next.delete(key) : next.add(key)
    if (canPersist) persist.mutate(next)
    return next
  })
}
```

チェックボックスの描画（各 `<li>` に足す。`name`/`aria` に品目名を入れて支援技術とテストが区別できるように）:

```tsx
<label className="flex items-center gap-2">
  <input
    type="checkbox"
    checked={checked.has(it.key)}
    onChange={() => toggle(it.key)}
    aria-label={it.name}
  />
  <span className={checked.has(it.key) ? 'line-through text-kon-ink/50' : ''}>{it.name}</span>
</label>
```

> **PUT は overlay 全体の一括置換**なので、`persist.mutate(next)` は「今チェックされている導出品目の全集合」を送る。1件ずつの差分ではない（設計 3.5）。手動品目・非表示は Task 13 で overlay に加える。楽観更新はローカル state（`checked`）が担い、`onSuccess` の invalidate でサーバの正と突き合わせる（既存 FavoriteButton と同じ思想）。

- [ ] **Step 5: テストが通ることを確認する → 🟢 コミット**

Run: `cd frontend && npx tsc -b && npm test -- ShoppingListPage`（PASS）

```bash
git add frontend/src/api/client.ts frontend/src/features/menu/api.ts \
        frontend/src/api/types.ts frontend/src/features/menu/ShoppingListPage.tsx
git commit -m "feat: 買い物リストのチェックを常に表示しpremiumで永続化する"
```

---

### Task 12: フロント — free が初めてチェックしたら案内を1回だけ

free の利用者が買い物リストのチェックを付けた最初の1回だけ、プレミアムの案内を出す。localStorage に記録し再表示しない。

**Files:**
- Create: `frontend/src/hooks/useOnceFlag.ts`
- Modify: `frontend/src/features/menu/ShoppingListPage.tsx`
- Test: `frontend/src/hooks/useOnceFlag.test.ts`、`frontend/src/features/menu/ShoppingListPage.test.tsx`

**Interfaces:**
- Produces: `useOnceFlag(key: string): [done: boolean, mark: () => void]`（localStorage 恒久記録）。

- [ ] **Step 1: hook の失敗テストを書く**

`useOnceFlag.test.ts`:

```ts
import { renderHook, act } from '@testing-library/react'
import { useOnceFlag } from './useOnceFlag'

afterEach(() => localStorage.clear())

test('mark すると done になり、次回のマウントでも done', () => {
  const { result, unmount } = renderHook(() => useOnceFlag('premium-shopping'))
  expect(result.current[0]).toBe(false)
  act(() => result.current[1]())
  expect(result.current[0]).toBe(true)
  unmount()
  const again = renderHook(() => useOnceFlag('premium-shopping'))
  expect(again.result.current[0]).toBe(true)
})
```

- [ ] **Step 2: FAIL 確認 → 🔴 コミット**

Run: `cd frontend && npm test -- useOnceFlag`（FAIL）
```bash
git add frontend/src/hooks/useOnceFlag.test.ts
git commit -m "test: 一度きりフラグの hook"
```

- [ ] **Step 3: hook を実装する**

`useOnceFlag.ts`:

```ts
import { useCallback, useState } from 'react'

const prefix = 'menu-planner:once:'

// useOnceFlag は「一度きり」をブラウザに恒久的に記録する。
// 一時状態の sessionStorage（タブで消える）と違い、案内は端末で一度出れば十分なので
// localStorage を使う。private モードで失敗しても画面は動く（消えても「もう一度出る」だけ）。
export function useOnceFlag(key: string): [boolean, () => void] {
  const storageKey = prefix + key
  const [done, setDone] = useState(() => {
    try {
      return localStorage.getItem(storageKey) === '1'
    } catch {
      return false
    }
  })
  const mark = useCallback(() => {
    setDone(true)
    try {
      localStorage.setItem(storageKey, '1')
    } catch {
      /* private モード: 記録できなくても実害は「次も出る」だけ */
    }
  }, [storageKey])
  return [done, mark]
}
```

- [ ] **Step 4: PASS 確認 → 🟢 コミット**

```bash
git add frontend/src/hooks/useOnceFlag.ts
git commit -m "feat: 一度きりフラグの hook を足す"
```

- [ ] **Step 5: 案内の失敗テストを書く（ShoppingListPage）**

```ts
test('free が初めてチェックすると案内が1回だけ出る', async () => {
  const savedId = '11111111-1111-1111-1111-111111111111'
  withWeek([menu('m1', '肉じゃが')]); withSavedId(savedId)
  respondMe('free')
  server.use(http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
    HttpResponse.json({ items: [{ name: 'にんじん', category: 'vegetable', origin: 'derived', checked: false, usedIn: [] }] })))

  renderWithProviders(<ShoppingListPage />)
  await userEvent.click(await screen.findByRole('checkbox', { name: /にんじん/ }))
  expect(await screen.findByText(/プレミアム/)).toBeInTheDocument() // 案内が出る

  // 閉じてもう一度チェックしても出ない
  await userEvent.click(screen.getByRole('button', { name: /閉じる/ }))
  await userEvent.click(screen.getByRole('checkbox', { name: /にんじん/ }))
  expect(screen.queryByText(/プレミアム/)).not.toBeInTheDocument()
})

test('premium にはチェックしても案内を出さない', async () => {
  const savedId = '11111111-1111-1111-1111-111111111111'
  withWeek([menu('m1', '肉じゃが')]); withSavedId(savedId)
  respondMe('premium')
  server.use(
    http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
      HttpResponse.json({ items: [{ name: 'にんじん', category: 'vegetable', origin: 'derived', checked: false, usedIn: [] }] })),
    http.put(`/api/v1/weekly-menus/${savedId}/shopping-list`, () => new HttpResponse(null, { status: 204 })),
  )
  renderWithProviders(<ShoppingListPage />)
  await userEvent.click(await screen.findByRole('checkbox', { name: /にんじん/ }))
  expect(screen.queryByText(/プレミアム/)).not.toBeInTheDocument()
})
```

> localStorage はテスト間で残るため、`ShoppingListPage.test.tsx` の `afterEach` で `localStorage.clear()` を追加する（既存の setup が sessionStorage しか消していないため）。

- [ ] **Step 6: FAIL 確認 → 🔴 コミット**

```bash
git add frontend/src/features/menu/ShoppingListPage.test.tsx
git commit -m "test: free の初回チェックで案内を1回だけ出す"
```

- [ ] **Step 7: 案内を実装する**

`ShoppingListPage.tsx` の `toggle` に、free の初回チェック時の案内を足す:

```tsx
const [guidanceDone, markGuidance] = useOnceFlag('premium-shopping')
const [showGuidance, setShowGuidance] = useState(false)

function toggle(key: string) {
  setChecked((prev) => {
    const next = new Set(prev)
    const adding = !next.has(key)
    next.has(key) ? next.delete(key) : next.add(key)
    if (canPersist) {
      persist.mutate(next)
    } else if (adding && user?.plan !== 'premium' && !guidanceDone) {
      // free（未認証含む）が初めてチェックを付けたとき、案内を1回だけ。
      setShowGuidance(true)
      markGuidance()
    }
    return next
  })
}
```

案内の描画（控えめなカード。常設バナーにはしない。設計 3.6）:

```tsx
{showGuidance && (
  <div role="status" className="rounded-lg bg-kon-leaf/10 p-3 text-sm">
    <p>プレミアムプランなら、チェックした買い物リストがそのまま残ります。</p>
    <button type="button" onClick={() => setShowGuidance(false)}>閉じる</button>
  </div>
)}
```

> `user?.plan !== 'premium'` を条件にするのは、未認証（`user` が undefined）も free 扱いにするため。案内文言はサーバではなくフロントに持つ（プランの件数のようなサーバ真実ではなく、単なる勧誘のため）。

- [ ] **Step 8: PASS 確認 → 🟢 コミット**

Run: `cd frontend && npx tsc -b && npm test -- ShoppingListPage useOnceFlag`（PASS）
```bash
git add frontend/src/features/menu/ShoppingListPage.tsx
git commit -m "feat: freeの初回チェックでプレミアムの案内を1回出す"
```

---

### Task 13: フロント — premium×保存済みで品目の手動追加・削除

premium かつ保存済みの週のとき、品目の追加（`manual`）と非表示（`hidden`）を PUT に反映する。

**Files:**
- Modify: `frontend/src/features/menu/ShoppingListPage.tsx`
- Test: `frontend/src/features/menu/ShoppingListPage.test.tsx`

**Interfaces:**
- Consumes: 既存の overlay 構築ロジック（Task 11 の `persist`）。
- Produces: 追加フォーム（名前 + カテゴリ）と、各品目の非表示（削除）操作。overlay に `manual` / `hidden` を含めて PUT。

- [ ] **Step 1: 失敗するテストを書く**

```ts
test('premium が品目を手で足すと manual として PUT に載る', async () => {
  const savedId = '11111111-1111-1111-1111-111111111111'
  withWeek([menu('m1', '肉じゃが')]); withSavedId(savedId)
  respondMe('premium')
  server.use(http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
    HttpResponse.json({ items: [] })))
  const puts: any[] = []
  server.use(http.put(`/api/v1/weekly-menus/${savedId}/shopping-list`, async ({ request }) => {
    puts.push(await request.json()); return new HttpResponse(null, { status: 204 })
  }))

  renderWithProviders(<ShoppingListPage />)
  await userEvent.type(await screen.findByLabelText(/品目を追加/), '牛乳')
  await userEvent.selectOptions(screen.getByLabelText(/カテゴリ/), 'dairy_egg')
  await userEvent.click(screen.getByRole('button', { name: /追加/ }))

  await waitFor(() => expect(puts.length).toBe(1))
  expect(puts[0].items).toContainEqual({ name: '牛乳', category: 'dairy_egg', origin: 'manual', checked: false, hidden: false })
})

test('premium が導出品目を消すと hidden として PUT に載る', async () => {
  const savedId = '11111111-1111-1111-1111-111111111111'
  withWeek([menu('m1', '肉じゃが')]); withSavedId(savedId)
  respondMe('premium')
  server.use(http.get(`/api/v1/weekly-menus/${savedId}/shopping-list`, () =>
    HttpResponse.json({ items: [{ name: 'にんじん', category: 'vegetable', origin: 'derived', checked: false, usedIn: [] }] })))
  const puts: any[] = []
  server.use(http.put(`/api/v1/weekly-menus/${savedId}/shopping-list`, async ({ request }) => {
    puts.push(await request.json()); return new HttpResponse(null, { status: 204 })
  }))

  renderWithProviders(<ShoppingListPage />)
  await userEvent.click(await screen.findByRole('button', { name: /にんじんを消す/ }))
  await waitFor(() => expect(puts.length).toBe(1))
  expect(puts[0].items).toContainEqual({ name: 'にんじん', category: 'vegetable', origin: 'derived', checked: false, hidden: true })
})
```

- [ ] **Step 2: FAIL 確認 → 🔴 コミット**

```bash
git add frontend/src/features/menu/ShoppingListPage.test.tsx
git commit -m "test: premium×保存済みで品目の手動追加・削除"
```

- [ ] **Step 3: 実装する**

`ShoppingListPage.tsx`。overlay 構築を「チェックだけ」から「チェック + 手動 + 非表示」に広げ、追加/削除 UI（premium×保存済みのときだけ）を出す:

```tsx
// 手動品目と非表示をローカルに持つ。
const [manual, setManual] = useState<{ name: string; category: IngredientCategory }[]>([])
const [hidden, setHidden] = useState<Set<string>>(new Set())

// overlay を「今の画面状態」から丸ごと組み立てる（一括置換）。
function buildOverlay(nextChecked: Set<string>, nextHidden: Set<string>, nextManual: typeof manual): ShoppingListOverride[] {
  const derivedOverrides = items
    .filter((it) => it.origin === 'derived')
    .filter((it) => nextChecked.has(it.key) || nextHidden.has(it.key))
    .map((it) => ({ name: it.name, category: it.category, origin: 'derived' as const,
      checked: nextChecked.has(it.key), hidden: nextHidden.has(it.key) }))
  const manualOverrides = nextManual.map((m) => ({
    name: m.name, category: m.category, origin: 'manual' as const,
    checked: nextChecked.has(m.name), hidden: false }))
  return [...derivedOverrides, ...manualOverrides]
}
```

`persist.mutationFn` を `buildOverlay(...)` を使う形に差し替え、`toggle` / 追加 / 削除の各操作が `persist.mutate(...)`（canPersist のときだけ）を呼ぶようにする。

追加フォームと削除ボタン（premium×保存済み時のみ表示）:

```tsx
{canPersist && (
  <form onSubmit={(e) => { e.preventDefault(); addManual() }} className="mt-4 flex gap-2">
    <input aria-label="品目を追加" value={draftName} onChange={(e) => setDraftName(e.target.value)} />
    <select aria-label="カテゴリ" value={draftCat} onChange={(e) => setDraftCat(e.target.value as IngredientCategory)}>
      {categoryOrder.map((c) => <option key={c} value={c}>{categoryLabels[c]}</option>)}
    </select>
    <button type="submit">追加</button>
  </form>
)}
```

各 `<li>` に（canPersist のとき）`<button aria-label={`${it.name}を消す`} onClick={() => remove(it.key, it.name)}>×</button>` を足す。`remove` は導出品目なら `hidden` に足し、手動品目なら `manual` から除く。いずれも `persist.mutate` で反映。

> **表示は「差分適用後」をサーバが返す形**（Task 8 の GET）なので、手動追加/削除の結果は PUT → invalidate → GET 再取得で画面に反映される。ローカル state（manual/hidden/checked）は楽観表示に使い、サーバの再取得で正に揃える。追加した手動品目名が既存の導出品目名と衝突する場合はサーバが 400（名前重複）を返すので、`persist.error` を `<ErrorMessage>` で見せる。

- [ ] **Step 4: PASS 確認 → 🟢 コミット**

Run: `cd frontend && npx tsc -b && npm test -- ShoppingListPage`（PASS）
```bash
git add frontend/src/features/menu/ShoppingListPage.tsx
git commit -m "feat: premium×保存済みで品目の手動追加・削除を足す"
```

---

### Task 14: E2E — premium で保存済み週のチェックがリロード後も残る

この機能の本体（永続化）を実環境で1本だけ確かめる。品目追加などは単体で足りる（設計 10）。

**Files:**
- Create: `frontend/e2e/shopping-list-persist.spec.ts`

**Interfaces:**
- Consumes: `helpers.ts`（`uniqueEmail`, `signUp`, `testPassword`）、`premium.spec.ts` の付与手順（`docker compose run --rm backend go run ./cmd/grant`）。

- [ ] **Step 1: E2E を書く**

`frontend/e2e/shopping-list-persist.spec.ts`:

```ts
import { execSync } from 'node:child_process'
import { test, expect } from '@playwright/test'
import { uniqueEmail, signUp } from './helpers'

test('premium は保存済み週の買い物リストのチェックがリロード後も残る', async ({ page }) => {
  const email = uniqueEmail('slo-persist')
  await signUp(page, email)

  // premium を付与（決済が無いため CLI で）。
  execSync(`docker compose run --rm backend go run ./cmd/grant -email=${email} -months=1`,
    { cwd: '..', stdio: 'inherit' })
  await page.reload()

  // 週を作って保存 → 保存一覧から開く → 買い物リストへ
  await page.goto('/weekly')
  await page.getByRole('button', { name: '1週間分を作る' }).click()
  await expect(page.getByRole('listitem')).toHaveCount(7)
  await page.getByRole('button', { name: 'この週を保存する' }).click()
  await page.getByRole('link', { name: '買い物リストを見る' }).click()

  // 最初のチェックボックスを付ける
  const first = page.getByRole('checkbox').first()
  await first.check()
  await expect(first).toBeChecked()

  // リロードしても残る（＝サーバに永続化された）
  await page.reload()
  await expect(page.getByRole('checkbox').first()).toBeChecked()
})
```

> ランダム献立のため品目名は固定できない。「最初のチェックボックスを付けてリロード後も付いている」に絞る。付与→`page.reload()` は `useCurrentUser` の staleTime を無効化するため（`premium.spec.ts` と同じ）。保存→開く導線は `saved-weekly.spec.ts` と同じ経路。

- [ ] **Step 2: 実行して通す**

Run:
```bash
make up && make migrate && make seed
make test-e2e -- shopping-list-persist   # または npx playwright test shopping-list-persist
```
Expected: PASS。落ちる場合は `make seed` 済みか、`grant` CLI がそのメールを引けているかを確認。

- [ ] **Step 3: コミット**

```bash
git add frontend/e2e/shopping-list-persist.spec.ts
git commit -m "test: 買い物リストのチェックがリロード後も残るE2E"
```

---

### Task 15: `spec.md` を更新する

実装が仕様に追いつくよう、2.7 / 2.8 とプレミアムプランの記述を直す（設計 11 の最後）。

**Files:**
- Modify: `menu-planner/spec.md`

- [ ] **Step 1: spec.md を直す**

以下を反映する（章番号は現状の spec.md に合わせる。**プレミアムの章の置き場所は 2章に新節を足す**。設計 12 の未決だったが、既存 2.11 がプレミアムの上限を扱っているため、その並びが自然）:

- **2.8（買い物リスト）**: 「買い物リストは保存しない」を、**「未保存の週は毎回作り直す。保存済みの週に限り、premium はチェック状態・手動品目・非表示の差分を保存できる。リスト本体は保存せず差分だけを持つ」**に更新。
- **2.11 の並び（プレミアム）**: premium の機能を2つ明記する — ①週間献立の保存上限 50件（既存）②買い物リストの永続化（保存済みの週）。free/premium の対応表（設計 4 の表）を載せる。
- **利用者レビューの A/C/D/E は free** である旨（設計 3.2）を、該当箇所（後続タスクの節）に1行追記。
- API 一覧（5章）に `GET/PUT /weekly-menus/{id}/shopping-list` を足す。

具体の文言は設計ドキュメント 3〜4章と 8章をそのまま要約して移す。**推測で新しい仕様を足さない**（設計に無いことは書かない）。

- [ ] **Step 2: コミット**

```bash
git add menu-planner/spec.md
git commit -m "docs: 買い物リストの永続化とプレミアムの線引きを仕様に反映"
```

---

## Self-Review

計画を書き終えたので、設計ドキュメントと突き合わせて自己点検した結果。

**1. 仕様カバレッジ（設計の各節 → タスク）:**
- 3.1 premium 2機能 → 保存上限は基盤で実装済み、永続化は Task 1〜13。✅
- 3.2 A/C/D/E は free → Task 15（spec.md への明記）。実装対象外なのでコードは無し。✅
- 3.3 履歴は据え置き → 触らない（本計画に履歴の変更なし）。✅
- 3.4 差分だけ保存 → Task 3（テーブル）/ Task 6（重ね合わせ）。✅
- 3.5 一括置換 → Task 4（Replace）/ Task 9（PUT）/ Task 11（overlay 構築）。✅
- 3.6 導線は文脈のときだけ・案内1回 → Task 12（free 初回案内）。**保存上限のときの導線は基盤で実装済み**（WeeklyPage の 409）。✅
- 3.7 専用画面を作らない → 同じ ShoppingListPage を使う（Task 10〜13）。✅
- 4 free/premium 表 → Task 15。永続化対象が保存済み週に限る点は Task 10 の savedId 分岐で担保。✅
- 5.1 テーブル定義 → Task 3。✅ / 5.2 番号 000011 → Global Constraints。✅
- 6.1 CanPersistShoppingList → Task 1。SavedWeeklyMenuLimit は実装済み。✅ / 6.2 値オブジェクト → Task 2。✅
- 7.1 ポート → Task 4。✅ / 7.2 For / ReplaceOverrides / 100件上限 / 所有者検証 → Task 5〜7（※ ShoppingListService 拡張ではなく SavedShoppingListService に分けた。File Structure に理由記載）。✅
- 8.1 POST は変えない → Task 10 で未保存経路として残す。✅ / 8.2 GET/PUT・403・409・gen-api・契約テスト → Task 8/9。✅
- 9 フロント → Task 10〜13。✅
- 10 テスト戦略（層ごと）→ 各タスクのテストで網羅。E2E は「リロード後も残る」に絞る → Task 14。✅

**2. プレースホルダ点検:** コードを伴うステップは実コードを載せた。フロントの一部（既存 `groupByCategory` 描画への差し込み、main.go の既存変数名、`fakeSavedWeeklyStore` の内部表現）は**既存コードに依存するため「既存に合わせる」と明記**した。これは曖昧さではなく、実装時に現物を読んで合わせる指示。**着手時に必ず現物を確認すること。**

**3. 型の一貫性（タスクをまたぐ名前）:**
- `domain.Origin` / `OriginDerived` / `OriginManual`（Task 2 → 4/6/7/8/9）一致。✅
- `domain.ShoppingListOverride`（フィールド名 `SavedWeeklyMenuID/Name/Category/Origin/Checked/Hidden`）(Task 2 → 4/7)一致。✅
- `service.ShoppingListOverrideStore`（`FindBySavedWeeklyMenu` / `Replace`）(Task 4 → 6/7)一致。✅
- `SavedWeeklyMenuStore.Find(ctx, userID, id)`（Task 5 → 6/7）一致。✅
- `service.SavedShoppingItem`（`Name/NameKana/Category/Origin/Checked/UsedIn`）(Task 6 → 8)一致。✅
- `service.OverrideInput`（`Name/Category/Origin/Checked/Hidden` 全て string/bool）(Task 7 → 9)一致。✅
- `ErrPremiumRequired`(403) / `ErrShoppingListItemLimitReached`(409) / `ErrInvalidOverride`(400)(Task 2/7 → problem.go)一致。✅
- API: `GET/PUT /weekly-menus/{id}/shopping-list`、`SavedShoppingListResponse` / `ShoppingListOverridesRequest` / `Origin`（Task 8/9 → フロント Task 10/11）一致。✅
- フロント: `savedShoppingListQueryKey` / `fetchSavedShoppingList` / `saveShoppingListOverrides` / sessionStorage `weekly.savedId` / `useOnceFlag`（Task 10〜13）一致。✅

**要確認（実装時の既知の未確定・現物合わせ）:**
- `api/openapi.yaml` に `IngredientCategory` の共通 enum があるか。無ければ Task 8 で `Ingredient.category` の enum を共通スキーマに切り出す。
- `client.ts` の内部が method 引数を取る共通 `request` かどうか（`apiPut` の足し方が変わる。Task 11 Step 1）。
- `main.go` の既存変数名（`shoppingListSvc` / `savedWeeklyRepo` / `entitlementSvc` / `jwt` / echo の `e`）。
- `fakeSavedWeeklyStore` の内部表現（Task 5/6 のヘルパはこれに合わせる）。

---

## 実行の引き継ぎ

計画は `docs/superpowers/plans/2026-07-24-premium-plan-split.md` に保存済み。base は `feature/premium`。実行方法は2択:

**1. Subagent-Driven（推奨）** — タスクごとに新しいサブエージェントを立て、タスク間でレビューする。速い反復。

**2. Inline Execution** — このセッションでタスクを順に実行し、チェックポイントでレビューする。

どちらで進めるか。
