# 食材テキスト解決のレート制限 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `POST /api/v1/ingredients/resolve` の LLM 呼び出しに日次の上限を入れ、請求額に天井を作る。

**Architecture:** 日次カウンタを Postgres に持ち（Cloud Run の `min-instances=0` / `max-instances=2` ではメモリ上のカウンタが日次上限として当てにならないため）、IP／ユーザー／全体の3スコープを数える。数えるのは **LLM 呼び出しが発生したときだけ**。上限判定はミドルウェアで完結できない（LLM に行くかは service が①完全一致・②キャッシュを試すまで確定しない）ため、handler が残枠を読んで `ResolvePolicy` を service に渡し、service が③の直前で判断する。

**Tech Stack:** Go 1.x / Echo v4 / pgx v5 / golang-migrate / React 19 / TanStack Query / Vitest + MSW / Playwright

## Global Constraints

- 設計は `docs/superpowers/specs/2026-08-05-resolve-rate-limit-design.md`。値・文言・命名はすべてそこが正。
- 上限値の既定: 非ログイン **10回/日**、ログイン **30回/日**、全体 **300回/日**。
- 環境変数は `0` で無制限（既存 `RateLimiter` の約束に揃える）。Vite プロキシ配下の開発・E2E ではこれで切る。
- 日付の境界は **JST**。`time.LoadLocation` は使わず `time.FixedZone("JST", 9*60*60)`（コンテナに tzdata を要求しないため）。
- 判定の順序は **全体 → 利用者**。逆にすると全体が詰まっているときに「ログインすると増えます」と誤導する。
- カウンタが読めないときは **フェイルクローズ**（LLM をスキップ）。
- コメントは日本語。既存ファイルの密度・語り口に合わせる。「なぜ」を書き、「何を」は書かない。
- Makefile のレシピは ASCII のみ・1コマンド単位（Windows の GNU Make 3.81 対策。先頭コメント参照）。
- `api/openapi.yaml` が API 仕様の正。変えたら `make gen-api` で TS 型を再生成してコミットする（CI が再生成して差分が出たら落ちる）。
- テスト: `make test-backend`（`cd backend && go test ./... -cover`）、`make test-frontend`。
- DB を要する Go テストは testcontainers を使う。Docker が無ければ自動でスキップされる。

---

### Task 1: 日次カウンタのテーブル

**Files:**
- Create: `backend/db/migrations/000014_create_resolve_usage_counters.up.sql`
- Create: `backend/db/migrations/000014_create_resolve_usage_counters.down.sql`
- Test: `backend/internal/repository/resolve_usage_schema_test.go`

**Interfaces:**
- Consumes: なし
- Produces: テーブル `resolve_usage_counters(usage_date date, scope text, subject text, count int)`、主キー `(usage_date, scope, subject)`

- [ ] **Step 1: 失敗するスキーマテストを書く**

`backend/internal/repository/resolve_usage_schema_test.go`:

```go
package repository_test

import (
	"context"
	"testing"
)

// TestResolveUsageCountersSchema は日次カウンタのテーブル定義を確かめる。
//
// 他のテスト関数と日付を分けているのは、このパッケージが1つのDBを共有しており、
// 後始末の TRUNCATE がこの表を対象にしていないため。
func TestResolveUsageCountersSchema(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	t.Run("total は subject を空にして入る", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'total', '', 1)`)
		if err != nil {
			t.Fatalf("total 行を保存できませんでした: %v", err)
		}
	})

	t.Run("未知の scope は入らない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'session', 'abc', 1)`)
		if err == nil {
			t.Fatal("CHECK 違反になるはずが成功しました")
		}
	})

	t.Run("total 以外は subject が必須", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'ip', '', 1)`)
		if err == nil {
			t.Fatal("CHECK 違反になるはずが成功しました")
		}
	})

	t.Run("total に subject は付けられない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'total', 'abc', 1)`)
		if err == nil {
			t.Fatal("CHECK 違反になるはずが成功しました")
		}
	})

	t.Run("同じ日付・scope・subject は二重に入らない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'total', '', 1)`)
		if err == nil {
			t.Fatal("主キー違反になるはずが成功しました")
		}
	})
}
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd backend && go test ./internal/repository/ -run TestResolveUsageCountersSchema -v`
Expected: FAIL（`relation "resolve_usage_counters" does not exist`）

- [ ] **Step 3: マイグレーションを書く**

`backend/db/migrations/000014_create_resolve_usage_counters.up.sql`:

```sql
-- 「読み取る」（LLM 呼び出し）の日次カウンタ（設計 6章）。
--
-- メモリではなくDBに持つのは本番構成のため。Cloud Run は min-instances=0 で
-- アイドルのたびにプロセスが落ち、max-instances=2 で2つ並ぶ。メモリ上の
-- カウンタは日次上限として当てにならず、全体上限は原理的に守れない。
--
-- **数えるのは LLM 呼び出しが発生したときだけ。** 完全一致やキャッシュで
-- 解けたリクエストは料金が発生しないので枠を消さない（設計 4章）。
CREATE TABLE resolve_usage_counters (
    -- JST の日付。UTC で持つと日本の深夜に枠がリセットされる（設計 6.1）。
    usage_date date NOT NULL,
    -- 'ip'（非ログイン）/ 'user'（ログイン）/ 'total'（サービス全体）。
    scope      text NOT NULL,
    -- IPのHMAC / ユーザーID。total は持たない。
    -- **生のIPは入れない。** 元に戻せない値にすることで、
    -- プライバシーポリシーの改定なしに数を数えられる（設計 6.2）。
    subject    text NOT NULL,
    count      int  NOT NULL DEFAULT 0,

    PRIMARY KEY (usage_date, scope, subject),
    CONSTRAINT resolve_usage_counters_scope_valid
        CHECK (scope IN ('ip', 'user', 'total')),
    -- total は subject を持たない。他は必ず持つ。
    -- 空の subject を 'ip' で作れてしまうと、全非ログインが1行に集約されて
    -- 上限が誰にも当たらなくなる。
    CONSTRAINT resolve_usage_counters_subject_matches_scope
        CHECK ((scope = 'total' AND subject = '')
            OR (scope <> 'total' AND btrim(subject) <> ''))
);
```

`backend/db/migrations/000014_create_resolve_usage_counters.down.sql`:

```sql
DROP TABLE resolve_usage_counters;
```

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd backend && go test ./internal/repository/ -run TestResolveUsageCountersSchema -v`
Expected: PASS（5サブテストすべて）

- [ ] **Step 5: コミット**

```bash
git add backend/db/migrations/000014_create_resolve_usage_counters.up.sql backend/db/migrations/000014_create_resolve_usage_counters.down.sql backend/internal/repository/resolve_usage_schema_test.go
git commit -m "feat: 読み取りの日次カウンタのテーブルを足す"
```

---

### Task 2: ResolveUsageRepository

**Files:**
- Create: `backend/internal/repository/resolve_usage.go`
- Test: `backend/internal/repository/resolve_usage_test.go`

**Interfaces:**
- Consumes: Task 1 のテーブル
- Produces:
  - `repository.NewResolveUsageRepository(pool *pgxpool.Pool) *ResolveUsageRepository`
  - `(*ResolveUsageRepository) Counts(ctx context.Context, day, scope, subject string) (total, own int, err error)`
  - `(*ResolveUsageRepository) Increment(ctx context.Context, day, scope, subject string) error`
  - `(*ResolveUsageRepository) DeleteOlderThan(ctx context.Context, day string) (int64, error)`
  - `day` は `2006-01-02` 形式の文字列。`time.Time` を渡さないのは、`date` 列との対応を SQL の `$1::date` で明示するため。

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/repository/resolve_usage_test.go`:

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func TestResolveUsageRepository(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewResolveUsageRepository(pool)

	// 日付はこのテスト関数専用のものを使う。このパッケージは1つのDBを共有し、
	// 後始末の TRUNCATE はこの表を対象にしていない。
	const day = "2026-02-01"
	const other = "2026-02-02"

	t.Run("加算すると全体と利用者の両方が増える", func(t *testing.T) {
		if err := repo.Increment(ctx, day, "ip", "hash-a"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		total, own, err := repo.Counts(ctx, day, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if total != 1 || own != 1 {
			t.Errorf("total=1, own=1 を期待しましたが total=%d, own=%d でした", total, own)
		}
	})

	t.Run("同じ利用者を重ねると加算される", func(t *testing.T) {
		if err := repo.Increment(ctx, day, "ip", "hash-a"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		total, own, err := repo.Counts(ctx, day, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if total != 2 || own != 2 {
			t.Errorf("total=2, own=2 を期待しましたが total=%d, own=%d でした", total, own)
		}
	})

	t.Run("別の利用者は自分の枠を消さないが全体は増やす", func(t *testing.T) {
		if err := repo.Increment(ctx, day, "ip", "hash-b"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		total, own, err := repo.Counts(ctx, day, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if total != 3 {
			t.Errorf("全体は 3 のはずです: %d", total)
		}
		if own != 2 {
			t.Errorf("hash-a の枠は 2 のままのはずです: %d", own)
		}
	})

	t.Run("scope が違えば別に数える", func(t *testing.T) {
		if err := repo.Increment(ctx, day, "user", "hash-a"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		_, own, err := repo.Counts(ctx, day, "user", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		// subject の文字列が同じでも ip と user は混ざらない。
		if own != 1 {
			t.Errorf("user スコープは 1 のはずです: %d", own)
		}
	})

	t.Run("日付が違えば0から数える", func(t *testing.T) {
		total, own, err := repo.Counts(ctx, other, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if total != 0 || own != 0 {
			t.Errorf("翌日は 0 から始まるはずです: total=%d, own=%d", total, own)
		}
	})

	t.Run("問い合わせたことのない利用者は0", func(t *testing.T) {
		_, own, err := repo.Counts(ctx, day, "ip", "hash-unknown")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if own != 0 {
			t.Errorf("0 のはずです: %d", own)
		}
	})

	t.Run("指定日より前の行を消せる", func(t *testing.T) {
		if err := repo.Increment(ctx, "2026-01-20", "ip", "hash-old"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		n, err := repo.DeleteOlderThan(ctx, "2026-02-01")
		if err != nil {
			t.Fatalf("DeleteOlderThan が失敗しました: %v", err)
		}
		if n < 1 {
			t.Errorf("1件以上消えるはずです: %d", n)
		}
		_, own, err := repo.Counts(ctx, "2026-01-20", "ip", "hash-old")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if own != 0 {
			t.Errorf("古い行が残っています: %d", own)
		}
		// 当日ぶんは残る。
		_, todayOwn, err := repo.Counts(ctx, day, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if todayOwn != 2 {
			t.Errorf("当日の行まで消えています: %d", todayOwn)
		}
	})
}
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd backend && go test ./internal/repository/ -run TestResolveUsageRepository -v`
Expected: FAIL（`undefined: repository.NewResolveUsageRepository`）

- [ ] **Step 3: リポジトリを実装する**

`backend/internal/repository/resolve_usage.go`:

```go
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ResolveUsageRepository は「読み取る」の日次カウンタへのアクセスを提供する。
//
// day は 2006-01-02 形式の文字列で受ける。time.Time を渡すと、
// timestamptz として送られて date 列との比較がタイムゾーン依存になりうる。
// 文字列 + $1::date なら、どの日を指しているかがコードと SQL の両方で読める。
type ResolveUsageRepository struct {
	pool *pgxpool.Pool
}

// NewResolveUsageRepository は ResolveUsageRepository を生成する。
func NewResolveUsageRepository(pool *pgxpool.Pool) *ResolveUsageRepository {
	return &ResolveUsageRepository{pool: pool}
}

// Counts は指定日の「サービス全体」と「その利用者」の回数を返す。
//
// 1往復で両方を引く。上限判定は必ず両方を見るため、分けても往復が増えるだけ。
func (r *ResolveUsageRepository) Counts(
	ctx context.Context, day, scope, subject string,
) (total, own int, err error) {
	rows, err := r.pool.Query(ctx,
		`SELECT scope, count
		   FROM resolve_usage_counters
		  WHERE usage_date = $1::date
		    AND ((scope = 'total' AND subject = '')
		      OR (scope = $2 AND subject = $3))`, day, scope, subject)
	if err != nil {
		return 0, 0, fmt.Errorf("読み取りカウンタの取得に失敗しました: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return 0, 0, fmt.Errorf("読み取りカウンタの読み取りに失敗しました: %w", err)
		}
		if s == "total" {
			total = n
			continue
		}
		own = n
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("読み取りカウンタの取得に失敗しました: %w", err)
	}
	return total, own, nil
}

// Increment は「サービス全体」と「その利用者」の回数を1つずつ加算する。
//
// 2行を1文で撃つ。別々に撃つと、片方だけ成功したときに全体と利用者の
// 帳尻が合わなくなる。
func (r *ResolveUsageRepository) Increment(
	ctx context.Context, day, scope, subject string,
) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
		 VALUES ($1::date, 'total', '', 1), ($1::date, $2, $3, 1)
		 ON CONFLICT (usage_date, scope, subject)
		 DO UPDATE SET count = resolve_usage_counters.count + 1`,
		day, scope, subject)
	if err != nil {
		return fmt.Errorf("読み取りカウンタの加算に失敗しました: %w", err)
	}
	return nil
}

// DeleteOlderThan は day より前の行を消す（設計 6.3）。
// 運用コマンドから流す。日付ごとに行が増え続けるのを止めるためだけの経路。
func (r *ResolveUsageRepository) DeleteOlderThan(
	ctx context.Context, day string,
) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM resolve_usage_counters WHERE usage_date < $1::date`, day)
	if err != nil {
		return 0, fmt.Errorf("古い読み取りカウンタの削除に失敗しました: %w", err)
	}
	return tag.RowsAffected(), nil
}
```

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd backend && go test ./internal/repository/ -run TestResolveUsage -v`
Expected: PASS（Task 1 のスキーマテストも含めて通る）

- [ ] **Step 5: コミット**

```bash
git add backend/internal/repository/resolve_usage.go backend/internal/repository/resolve_usage_test.go
git commit -m "feat: 読み取りの日次カウンタのリポジトリを足す"
```

---

### Task 3: 縮退の理由を結果に載せる

既存の `degraded`（bool）だけでは「LLM が落ちた」と「上限に達した」を区別できない。
理由の型を先に入れ、既存の LLM 失敗経路に値を入れる。上限の判定はまだ入れない。

**Files:**
- Modify: `backend/internal/service/ingredient_resolve.go`
- Modify: `backend/internal/handler/ingredient_resolve.go`
- Test: `backend/internal/service/ingredient_resolve_test.go`（追記）
- Test: `backend/internal/handler/ingredient_resolve_test.go`（追記）

**Interfaces:**
- Consumes: なし
- Produces:
  - `service.DegradedReason`（`string` の別名型）
  - 定数 `service.ReasonLLMError` / `ReasonCounterUnavailable` / `ReasonAnonDailyLimit` / `ReasonUserDailyLimit` / `ReasonServiceDailyLimit`
  - `service.ResolveResult.Reason DegradedReason`
  - JSON の `degradedReason`（`degraded` が false のときは省略）

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/service/ingredient_resolve_test.go` の末尾に追記:

```go
func TestResolve_GatewayErrorSetsReason(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{err: errors.New("LLMが落ちました")}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw)

	got, err := svc.Resolve(ctx, "マツタケ")
	if err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}
	if !got.Degraded {
		t.Fatal("degraded が立っていません")
	}
	if got.Reason != service.ReasonLLMError {
		t.Errorf("理由が llm_error ではありません: %q", got.Reason)
	}
}
```

`backend/internal/handler/ingredient_resolve_test.go` の `TestResolveHandler` に追記:

```go
	t.Run("縮退の理由をdegradedReasonで返す", func(t *testing.T) {
		uc := &stubResolveUseCase{result: service.ResolveResult{
			Resolved:   []service.ResolvedWord{},
			Unresolved: []string{"マツタケ"},
			Degraded:   true,
			Reason:     service.ReasonLLMError,
		}}
		rec := postResolve(t, uc, `{"text":"マツタケ"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("200 を期待しましたが %d でした: %s", rec.Code, rec.Body.String())
		}
		var got struct {
			Degraded       bool   `json:"degraded"`
			DegradedReason string `json:"degradedReason"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("レスポンスを解釈できませんでした: %v", err)
		}
		if !got.Degraded || got.DegradedReason != "llm_error" {
			t.Errorf("degraded=true, degradedReason=llm_error を期待しました: %+v", got)
		}
	})

	t.Run("縮退していなければdegradedReasonを出さない", func(t *testing.T) {
		uc := &stubResolveUseCase{result: service.ResolveResult{
			Resolved:   []service.ResolvedWord{},
			Unresolved: []string{},
			Degraded:   false,
		}}
		rec := postResolve(t, uc, `{"text":"玉ねぎ"}`)

		if strings.Contains(rec.Body.String(), "degradedReason") {
			t.Errorf("縮退していないのに degradedReason が出ています: %s", rec.Body.String())
		}
	})
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd backend && go test ./internal/service/ ./internal/handler/ -run Resolve -v`
Expected: FAIL（`undefined: service.ReasonLLMError` / `unknown field Reason`）

- [ ] **Step 3: 理由の型と値を足す**

`backend/internal/service/ingredient_resolve.go` の `ResolveResult` の直前に追加:

```go
// DegradedReason は LLM への問い合わせをスキップした理由（設計 8章）。
//
// bool ひとつでは「LLM が落ちた」と「上限に達した」を区別できず、
// 画面が出す文言を選べない。値と文言は1対1ではない
// （counter_unavailable は llm_error と同じ文言を出す）。
type DegradedReason string

const (
	// ReasonLLMError は LLM への問い合わせが失敗したことを表す。
	ReasonLLMError DegradedReason = "llm_error"
	// ReasonCounterUnavailable は日次カウンタが読めずフェイルクローズしたことを表す（設計 9.1）。
	ReasonCounterUnavailable DegradedReason = "counter_unavailable"
	// ReasonAnonDailyLimit は非ログインの日次上限に達したことを表す。
	ReasonAnonDailyLimit DegradedReason = "anon_daily_limit"
	// ReasonUserDailyLimit はログインユーザーの日次上限に達したことを表す。
	ReasonUserDailyLimit DegradedReason = "user_daily_limit"
	// ReasonServiceDailyLimit はサービス全体の日次上限に達したことを表す。
	ReasonServiceDailyLimit DegradedReason = "service_daily_limit"
)
```

`ResolveResult` に追加:

```go
	// Reason は Degraded が立った理由。Degraded が false なら空。
	Reason DegradedReason
```

`resolveByGateway` のエラー分岐（`result.Degraded = true` の直後）に追加:

```go
		result.Reason = ReasonLLMError
```

- [ ] **Step 4: ハンドラのレスポンスに足す**

`backend/internal/handler/ingredient_resolve.go` の `resolveResponse` に追加:

```go
	// DegradedReason は Degraded が立った理由。立っていなければ出さない。
	// 画面はこれで文言を選ぶ（設計 10章）。
	DegradedReason string `json:"degradedReason,omitempty"`
```

`c.JSON` の呼び出しを差し替え:

```go
	return c.JSON(http.StatusOK, resolveResponse{
		Resolved: resolved, Unresolved: unresolved, Degraded: result.Degraded,
		DegradedReason: string(result.Reason),
	})
```

ハンドラのテストで `strings` を使うため、`backend/internal/handler/ingredient_resolve_test.go` の import に `"strings"` が無ければ足す（既存の `postResolve` が `strings.NewReader` を使っているので既にあるはず）。

- [ ] **Step 5: テストが通ることを確かめる**

Run: `cd backend && go test ./internal/service/ ./internal/handler/ -run Resolve -v`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add backend/internal/service/ingredient_resolve.go backend/internal/service/ingredient_resolve_test.go backend/internal/handler/ingredient_resolve.go backend/internal/handler/ingredient_resolve_test.go
git commit -m "feat: 縮退の理由を degradedReason で返す"
```

---

### Task 4: ResolveQuota（上限の判定と記録）

**Files:**
- Create: `backend/internal/service/resolve_quota.go`
- Test: `backend/internal/service/resolve_quota_test.go`

**Interfaces:**
- Consumes: Task 2 の `Counts` / `Increment`（インターフェース越し）、Task 3 の `DegradedReason`
- Produces:
  - `service.ScopeIP = "ip"` / `service.ScopeUser = "user"`
  - `service.ResolveSubject{Scope, Subject string}`
  - `service.ResolveQuotaLimits{Anon, User, Total int}`
  - `service.ResolveUsageCounter` インターフェース（`Counts` / `Increment`）
  - `service.NewResolveQuota(c ResolveUsageCounter, l ResolveQuotaLimits, now func() time.Time) *ResolveQuota`
  - `(*ResolveQuota) Check(ctx context.Context, s ResolveSubject) (bool, DegradedReason)`
  - `(*ResolveQuota) Record(ctx context.Context, s ResolveSubject) error`

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/service/resolve_quota_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeUsageCounter は日次カウンタの最小のインメモリ実装。
type fakeUsageCounter struct {
	// total は日付ごとの全体の回数。
	total map[string]int
	// own は "日付|scope|subject" ごとの回数。
	own map[string]int
	err error
	// days は Counts / Increment に渡された日付。JST の境界を検証する。
	days []string
}

func newFakeUsageCounter() *fakeUsageCounter {
	return &fakeUsageCounter{total: map[string]int{}, own: map[string]int{}}
}

func (c *fakeUsageCounter) key(day, scope, subject string) string {
	return day + "|" + scope + "|" + subject
}

func (c *fakeUsageCounter) Counts(
	_ context.Context, day, scope, subject string,
) (int, int, error) {
	c.days = append(c.days, day)
	if c.err != nil {
		return 0, 0, c.err
	}
	return c.total[day], c.own[c.key(day, scope, subject)], nil
}

func (c *fakeUsageCounter) Increment(
	_ context.Context, day, scope, subject string,
) error {
	c.days = append(c.days, day)
	if c.err != nil {
		return c.err
	}
	c.total[day]++
	c.own[c.key(day, scope, subject)]++
	return nil
}

// fixedNow は時刻を固定する。日付境界を確かめるために使う。
func fixedNow(s string) func() time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("テストの時刻が不正です: " + err.Error())
	}
	return func() time.Time { return t }
}

var testLimits = service.ResolveQuotaLimits{Anon: 10, User: 30, Total: 300}

func anonSubject() service.ResolveSubject {
	return service.ResolveSubject{Scope: service.ScopeIP, Subject: "hash-a"}
}

func userSubject() service.ResolveSubject {
	return service.ResolveSubject{Scope: service.ScopeUser, Subject: "user-1"}
}

func TestResolveQuota_AllowsWithinLimit(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	// 9回まで使っている状態。10回目は通る。
	c.own[c.key("2026-08-05", "ip", "hash-a")] = 9
	q := service.NewResolveQuota(c, testLimits, fixedNow("2026-08-05T01:00:00Z"))

	allow, reason := q.Check(ctx, anonSubject())
	if !allow {
		t.Errorf("上限内なので通るはずです: reason=%q", reason)
	}
}

func TestResolveQuota_BlocksAtAnonLimit(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.own[c.key("2026-08-05", "ip", "hash-a")] = 10
	q := service.NewResolveQuota(c, testLimits, fixedNow("2026-08-05T01:00:00Z"))

	allow, reason := q.Check(ctx, anonSubject())
	if allow {
		t.Fatal("上限に達しているので止まるはずです")
	}
	if reason != service.ReasonAnonDailyLimit {
		t.Errorf("理由が anon_daily_limit ではありません: %q", reason)
	}
}

func TestResolveQuota_BlocksAtUserLimit(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.own[c.key("2026-08-05", "user", "user-1")] = 30
	q := service.NewResolveQuota(c, testLimits, fixedNow("2026-08-05T01:00:00Z"))

	allow, reason := q.Check(ctx, userSubject())
	if allow {
		t.Fatal("上限に達しているので止まるはずです")
	}
	if reason != service.ReasonUserDailyLimit {
		t.Errorf("理由が user_daily_limit ではありません: %q", reason)
	}
}

func TestResolveQuota_TotalWinsOverSubject(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	// 全体も本人も同時に上限。
	c.total["2026-08-05"] = 300
	c.own[c.key("2026-08-05", "ip", "hash-a")] = 10
	q := service.NewResolveQuota(c, testLimits, fixedNow("2026-08-05T01:00:00Z"))

	_, reason := q.Check(ctx, anonSubject())
	// 全体が詰まっているときに「ログインすると増えます」と出すと誤導になる（設計 5.1）。
	if reason != service.ReasonServiceDailyLimit {
		t.Errorf("理由が service_daily_limit ではありません: %q", reason)
	}
}

func TestResolveQuota_ZeroMeansUnlimited(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.total["2026-08-05"] = 100000
	c.own[c.key("2026-08-05", "ip", "hash-a")] = 100000
	q := service.NewResolveQuota(c,
		service.ResolveQuotaLimits{Anon: 0, User: 0, Total: 0},
		fixedNow("2026-08-05T01:00:00Z"))

	allow, _ := q.Check(ctx, anonSubject())
	if !allow {
		t.Fatal("無制限なので通るはずです")
	}
	// 無制限なら数を読む必要が無い。開発・E2Eで無駄な往復を出さない。
	if len(c.days) != 0 {
		t.Errorf("カウンタを読んでいます: %v", c.days)
	}
}

func TestResolveQuota_FailsClosedWhenCounterUnavailable(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.err = errors.New("DBが落ちました")
	q := service.NewResolveQuota(c, testLimits, fixedNow("2026-08-05T01:00:00Z"))

	allow, reason := q.Check(ctx, anonSubject())
	// カウンタが読めない状況ではキャッシュも読めておらず、全語がLLMに行く
	// ＝最も高い状態。素通しはコスト保護そのものを裏切る（設計 9.1）。
	if allow {
		t.Fatal("カウンタが読めないときは止まるはずです")
	}
	if reason != service.ReasonCounterUnavailable {
		t.Errorf("理由が counter_unavailable ではありません: %q", reason)
	}
}

func TestResolveQuota_UsesJSTDate(t *testing.T) {
	ctx := context.Background()

	t.Run("UTC14:59はまだJSTの当日", func(t *testing.T) {
		c := newFakeUsageCounter()
		q := service.NewResolveQuota(c, testLimits, fixedNow("2026-08-05T14:59:59Z"))
		q.Check(ctx, anonSubject())
		if c.days[0] != "2026-08-05" {
			t.Errorf("2026-08-05 を期待しましたが %q でした", c.days[0])
		}
	})

	t.Run("UTC15:00でJSTの翌日になる", func(t *testing.T) {
		c := newFakeUsageCounter()
		q := service.NewResolveQuota(c, testLimits, fixedNow("2026-08-05T15:00:00Z"))
		q.Check(ctx, anonSubject())
		if c.days[0] != "2026-08-06" {
			t.Errorf("2026-08-06 を期待しましたが %q でした", c.days[0])
		}
	})
}

func TestResolveQuota_RecordIncrements(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	q := service.NewResolveQuota(c, testLimits, fixedNow("2026-08-05T01:00:00Z"))

	if err := q.Record(ctx, anonSubject()); err != nil {
		t.Fatalf("Record が失敗しました: %v", err)
	}
	if c.total["2026-08-05"] != 1 {
		t.Errorf("全体が加算されていません: %d", c.total["2026-08-05"])
	}
	if c.own[c.key("2026-08-05", "ip", "hash-a")] != 1 {
		t.Error("利用者の枠が加算されていません")
	}
}

func TestResolveQuota_RecordReturnsError(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.err = errors.New("DBが落ちました")
	q := service.NewResolveQuota(c, testLimits, fixedNow("2026-08-05T01:00:00Z"))

	if err := q.Record(ctx, anonSubject()); err == nil {
		t.Fatal("加算に失敗したらエラーを返すはずです")
	}
}
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd backend && go test ./internal/service/ -run TestResolveQuota -v`
Expected: FAIL（`undefined: service.NewResolveQuota`）

- [ ] **Step 3: ResolveQuota を実装する**

`backend/internal/service/resolve_quota.go`:

```go
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// jst は日次の境界を決めるタイムゾーン（設計 6.1）。
//
// time.LoadLocation("Asia/Tokyo") はコンテナに tzdata を要求する。
// 日本にサマータイムは無いため、固定オフセットで足りる。
var jst = time.FixedZone("JST", 9*60*60)

// スコープの値。DB の CHECK 制約と揃える。
const (
	// ScopeIP は非ログイン。subject は IP のHMAC。
	ScopeIP = "ip"
	// ScopeUser はログインユーザー。subject はユーザーID。
	ScopeUser = "user"
)

// ResolveUsageCounter は日次カウンタの読み書き。
type ResolveUsageCounter interface {
	// Counts は指定日の「全体」と「その利用者」の回数を返す。
	Counts(ctx context.Context, day, scope, subject string) (total, own int, err error)
	// Increment は「全体」と「その利用者」を1つずつ加算する。
	Increment(ctx context.Context, day, scope, subject string) error
}

// ResolveSubject は誰のぶんとして数えるかを表す。
type ResolveSubject struct {
	// Scope は ScopeIP または ScopeUser。
	Scope string
	// Subject は IP のHMAC またはユーザーID。**生のIPは入れない**（設計 6.2）。
	Subject string
}

// ResolveQuotaLimits は日次の上限。**0以下は無制限**（設計 7章）。
//
// 0 を無制限にするのは既存 RateLimiter と同じ約束。Vite プロキシ配下では
// 全リクエストが単一IPに集約されるため、開発と E2E ではこれで切る。
type ResolveQuotaLimits struct {
	Anon  int
	User  int
	Total int
}

// ResolveQuota は「読み取る」の日次上限を判定し、実績を記録する。
//
// 判定（Check）と記録（Record）が別のメソッドなのは、LLM を呼ぶかどうかが
// ①完全一致・②キャッシュを試すまで決まらないため。handler が Check し、
// service が③の直前で使い、呼んだら Record する（設計 5章）。
type ResolveQuota struct {
	counter ResolveUsageCounter
	limits  ResolveQuotaLimits
	now     func() time.Time
}

// NewResolveQuota は ResolveQuota を生成する。
func NewResolveQuota(
	c ResolveUsageCounter, l ResolveQuotaLimits, now func() time.Time,
) *ResolveQuota {
	return &ResolveQuota{counter: c, limits: l, now: now}
}

// Check は今日の残枠を読む。allow=false のとき、2つ目の戻り値に理由が入る。
//
// **判定は「前回までの実績」を見る。** 同時に走るリクエストは互いを見ないため、
// max-instances=2 のもとで数件ぶん超過しうる。金額にして数円なので受け入れる。
func (q *ResolveQuota) Check(ctx context.Context, s ResolveSubject) (bool, DegradedReason) {
	own := q.subjectLimit(s)
	if q.limits.Total <= 0 && own <= 0 {
		// 全体も利用者も無制限。数を読む意味が無い。
		return true, ""
	}

	day := q.day()
	total, used, err := q.counter.Counts(ctx, day, s.Scope, s.Subject)
	if err != nil {
		// フェイルクローズ（設計 9.1）。カウンタが読めない状況では解決キャッシュも
		// 読めておらず、全語がLLMに行く＝最も高い状態になっている。
		slog.WarnContext(ctx, "読み取りカウンタを読めませんでした。LLMをスキップします",
			"error", err)
		return false, ReasonCounterUnavailable
	}

	// 全体 → 利用者 の順（設計 5.1）。逆にすると、全体が詰まっているときに
	// 非ログインへ「ログインすると増えます」と出てしまい誤導になる。
	if q.limits.Total > 0 && total >= q.limits.Total {
		slog.WarnContext(ctx, "読み取りの全体上限に達しました",
			"day", day, "count", total, "limit", q.limits.Total)
		return false, ReasonServiceDailyLimit
	}
	if own > 0 && used >= own {
		reason := ReasonAnonDailyLimit
		if s.Scope == ScopeUser {
			reason = ReasonUserDailyLimit
		}
		slog.WarnContext(ctx, "読み取りの日次上限に達しました",
			"day", day, "scope", s.Scope, "count", used, "limit", own)
		return false, reason
	}
	return true, ""
}

// Record は LLM を呼んだ実績を1つ数える。
//
// 上限を切っていても数える。使用量を後から振り返れるようにするため。
func (q *ResolveQuota) Record(ctx context.Context, s ResolveSubject) error {
	if err := q.counter.Increment(ctx, q.day(), s.Scope, s.Subject); err != nil {
		return fmt.Errorf("読み取りカウンタの加算に失敗しました: %w", err)
	}
	return nil
}

// subjectLimit はその利用者に当てる上限を返す。
func (q *ResolveQuota) subjectLimit(s ResolveSubject) int {
	if s.Scope == ScopeUser {
		return q.limits.User
	}
	return q.limits.Anon
}

// day は JST での今日を 2006-01-02 で返す。
func (q *ResolveQuota) day() string {
	return q.now().In(jst).Format("2006-01-02")
}
```

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd backend && go test ./internal/service/ -run TestResolveQuota -v`
Expected: PASS（9つのテスト関数すべて）

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/resolve_quota.go backend/internal/service/resolve_quota_test.go
git commit -m "feat: 読み取りの日次上限を判定する ResolveQuota を足す"
```

---

### Task 5: service が ResolvePolicy を受ける

**Files:**
- Modify: `backend/internal/service/ingredient_resolve.go`
- Modify: `backend/internal/handler/ingredient_resolve.go`
- Test: `backend/internal/service/ingredient_resolve_test.go`
- Test: `backend/internal/handler/ingredient_resolve_test.go`

**Interfaces:**
- Consumes: Task 4 の `ResolveSubject`、Task 3 の `DegradedReason`
- Produces:
  - `service.ResolvePolicy{AllowLLM bool, DenyReason DegradedReason, Subject ResolveSubject}`
  - `service.ResolveRecorder` インターフェース（`Record(ctx, ResolveSubject) error`）
  - `service.NewIngredientResolveService(ingredients IngredientRepository, cache ResolutionRepository, gw IngredientResolveGateway, rec ResolveRecorder)` ← 引数が4つになる
  - `(*IngredientResolveService) Resolve(ctx context.Context, text string, policy ResolvePolicy) (ResolveResult, error)`
  - `handler.IngredientResolveUseCase` の `Resolve` も同じ形になる

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/service/ingredient_resolve_test.go` の先頭付近（`testCatalog` の下）に共通のヘルパーを足す:

```go
// fakeRecorder は LLM 呼び出しの実績を数えるスタブ。
type fakeRecorder struct {
	calls    int
	subjects []service.ResolveSubject
	err      error
}

func (r *fakeRecorder) Record(_ context.Context, s service.ResolveSubject) error {
	r.calls++
	r.subjects = append(r.subjects, s)
	return r.err
}

// allowAll は LLM を許可する既定のポリシー。上限そのものを見ないテストで使う。
func allowAll() service.ResolvePolicy {
	return service.ResolvePolicy{
		AllowLLM: true,
		Subject:  service.ResolveSubject{Scope: service.ScopeIP, Subject: "hash-test"},
	}
}
```

同ファイルの末尾に新しいテストを足す:

```go
func TestResolve_DeniedPolicySkipsGateway(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{}
	rec := &fakeRecorder{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, rec)

	got, err := svc.Resolve(ctx, "玉ねぎ、マツタケ", service.ResolvePolicy{
		AllowLLM:   false,
		DenyReason: service.ReasonAnonDailyLimit,
		Subject:    service.ResolveSubject{Scope: service.ScopeIP, Subject: "hash-a"},
	})
	if err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}

	if gw.calls != 0 {
		t.Errorf("上限に達しているのに Gateway が呼ばれています: %d回", gw.calls)
	}
	if rec.calls != 0 {
		t.Errorf("呼んでいないのに実績が数えられています: %d回", rec.calls)
	}
	// ①で解けた分は返す。機能全体を落とさない。
	if len(got.Resolved) != 1 || got.Resolved[0].Word != "玉ねぎ" {
		t.Errorf("完全一致の結果が返っていません: %+v", got.Resolved)
	}
	if !got.Degraded || got.Reason != service.ReasonAnonDailyLimit {
		t.Errorf("理由が渡っていません: degraded=%v reason=%q", got.Degraded, got.Reason)
	}
	if len(got.Unresolved) != 1 || got.Unresolved[0] != "マツタケ" {
		t.Errorf("未解決語が返っていません: %+v", got.Unresolved)
	}
}

func TestResolve_RecordsWhenGatewayCalled(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{mapping: map[string]string{"まつたけ": ""}}
	rec := &fakeRecorder{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, rec)

	subject := service.ResolveSubject{Scope: service.ScopeUser, Subject: "user-1"}
	if _, err := svc.Resolve(ctx, "マツタケ", service.ResolvePolicy{
		AllowLLM: true, Subject: subject,
	}); err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}

	if rec.calls != 1 {
		t.Fatalf("1回数えるはずです: %d回", rec.calls)
	}
	if rec.subjects[0] != subject {
		t.Errorf("キーが違います: %+v", rec.subjects[0])
	}
}

func TestResolve_RecordsEvenWhenGatewayFails(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{err: errors.New("LLMが落ちました")}
	rec := &fakeRecorder{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, rec)

	if _, err := svc.Resolve(ctx, "マツタケ", allowAll()); err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}
	// 失敗してもトークンは消費されている（設計 4章）。
	if rec.calls != 1 {
		t.Errorf("失敗した呼び出しも数えるはずです: %d回", rec.calls)
	}
}

func TestResolve_ExactMatchDoesNotRecord(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	rec := &fakeRecorder{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, &countingResolver{}, rec)

	if _, err := svc.Resolve(ctx, "玉ねぎ、卵", allowAll()); err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}
	// 料金が発生しない解決で枠を消さない（設計 4章）。
	if rec.calls != 0 {
		t.Errorf("完全一致だけなら数えないはずです: %d回", rec.calls)
	}
}

func TestResolve_RecordFailureDoesNotBreakResult(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	// mapping のキーは正規化後の語。NormalizeIngredientWord はカタカナを
	// ひらがなにするだけなので、「豚こま」はそのまま「豚こま」で引ける。
	gw := &countingResolver{mapping: map[string]string{"豚こま": "豚肉"}}
	rec := &fakeRecorder{err: errors.New("DBが落ちました")}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{all: items}, &fakeResolutionRepo{}, gw, rec)

	got, err := svc.Resolve(ctx, "豚こま", allowAll())
	if err != nil {
		t.Fatalf("加算の失敗で機能を止めてはいけません: %v", err)
	}
	// 呼び出しはもう済んでいる。数え漏れは許容する（設計 9.2）。
	if len(got.Resolved) != 1 {
		t.Errorf("解決結果が返っていません: %+v", got.Resolved)
	}
	if got.Degraded {
		t.Error("加算の失敗で縮退させてはいけません")
	}
}
```

- [ ] **Step 2: 既存のテスト呼び出しを新しいシグネチャに直す**

`backend/internal/service/ingredient_resolve_test.go` の既存の呼び出しをすべて機械的に直す:

- `service.NewIngredientResolveService(a, b, c)` → `service.NewIngredientResolveService(a, b, c, &fakeRecorder{})`
- `svc.Resolve(ctx, X)` → `svc.Resolve(ctx, X, allowAll())`

`backend/internal/handler/ingredient_resolve_test.go` の `stubResolveUseCase` を直す:

```go
type stubResolveUseCase struct {
	result service.ResolveResult
	err    error
	calls  int
	// policy は最後に渡されたポリシー。上限の判定が service まで届くことを確かめる。
	policy service.ResolvePolicy
}

func (s *stubResolveUseCase) Resolve(
	_ context.Context, _ string, p service.ResolvePolicy,
) (service.ResolveResult, error) {
	s.calls++
	s.policy = p
	return s.result, s.err
}
```

- [ ] **Step 3: テストが落ちることを確かめる**

Run: `cd backend && go test ./internal/service/ ./internal/handler/ -run Resolve -v`
Expected: FAIL（`too many arguments` / `undefined: service.ResolvePolicy`）

- [ ] **Step 4: service を実装する**

`backend/internal/service/ingredient_resolve.go` に追加（`ResolveResult` の下）:

```go
// ResolveRecorder は LLM を呼んだ実績を数える。
//
// 判定（ResolveQuota.Check）は handler が行い、記録だけを service が持つ。
// LLM を呼んだかどうかは③まで来て初めて分かるため（設計 5章）。
type ResolveRecorder interface {
	Record(ctx context.Context, s ResolveSubject) error
}

// ResolvePolicy は1リクエストぶんの LLM 利用の可否（設計 5章）。
type ResolvePolicy struct {
	// AllowLLM が false なら③をスキップする。
	AllowLLM bool
	// DenyReason は AllowLLM が false のときの理由。結果の Reason にそのまま載る。
	DenyReason DegradedReason
	// Subject は実績を数えるときのキー。
	Subject ResolveSubject
}
```

`IngredientResolveService` に `recorder ResolveRecorder` フィールドを足し、コンストラクタを差し替える:

```go
// NewIngredientResolveService は IngredientResolveService を生成する。
func NewIngredientResolveService(
	ingredients IngredientRepository,
	cache ResolutionRepository,
	gw IngredientResolveGateway,
	rec ResolveRecorder,
) *IngredientResolveService {
	return &IngredientResolveService{
		ingredients: ingredients, cache: cache, gateway: gw, recorder: rec,
	}
}
```

`Resolve` のシグネチャと③の直前を差し替える:

```go
func (s *IngredientResolveService) Resolve(
	ctx context.Context, text string, policy ResolvePolicy,
) (ResolveResult, error) {
```

`s.resolveByGateway(...)` の呼び出しを次に差し替える:

```go
	if !policy.AllowLLM {
		// 上限に達している、またはカウンタが読めない。③をスキップする。
		// ①②で解けた分は返す。よくある食材はここまでで通るため、
		// 機能が丸ごと死んだようには見えない（設計 5章）。
		result.Degraded = true
		result.Reason = policy.DenyReason
		for _, e := range pending {
			result.Unresolved = append(result.Unresolved, e.original)
		}
		return result, nil
	}

	s.resolveByGateway(ctx, pending, byName, items, seen, &result, policy.Subject)
	return result, nil
```

`resolveByGateway` に `subject ResolveSubject` 引数を足し、gateway 呼び出しの直後に記録を入れる:

```go
func (s *IngredientResolveService) resolveByGateway(
	ctx context.Context,
	pending []resolveEntry,
	byName map[string]domain.Ingredient,
	items []domain.Ingredient,
	seen map[domain.IngredientID]bool,
	result *ResolveResult,
	subject ResolveSubject,
) {
	words := make([]string, 0, len(pending))
	for _, e := range pending {
		words = append(words, e.normalized)
	}

	answers, err := s.gateway.Resolve(ctx, words, catalogNames(items))

	// **呼んだ時点で数える。** 失敗してもトークンは消費されている（設計 4章）。
	if rerr := s.recorder.Record(ctx, subject); rerr != nil {
		// 呼び出しはもう済んでいる。ここで機能を止めても払った金は戻らない。
		// 数え漏れは許容する（設計 9.2）。
		slog.WarnContext(ctx, "読み取りカウンタの加算に失敗しました", "error", rerr)
	}

	if err != nil {
```

（`if err != nil {` から下の既存の本体はそのまま。）

- [ ] **Step 5: handler を新しいシグネチャに合わせる**

`backend/internal/handler/ingredient_resolve.go` の `IngredientResolveUseCase` を差し替える:

```go
// IngredientResolveUseCase は手持ちの食材テキストを食材に解決する。
type IngredientResolveUseCase interface {
	Resolve(ctx context.Context, text string, policy service.ResolvePolicy) (service.ResolveResult, error)
}
```

`Resolve` メソッド内の呼び出しを差し替える（上限の判定は Task 6 で入れる）:

```go
	result, err := h.svc.Resolve(c.Request().Context(), req.Text,
		service.ResolvePolicy{AllowLLM: true})
```

- [ ] **Step 6: テストが通ることを確かめる**

Run: `cd backend && go test ./internal/service/ ./internal/handler/ -v`
Expected: PASS

- [ ] **Step 7: コミット**

```bash
git add backend/internal/service/ingredient_resolve.go backend/internal/service/ingredient_resolve_test.go backend/internal/handler/ingredient_resolve.go backend/internal/handler/ingredient_resolve_test.go
git commit -m "feat: 解決サービスが LLM 利用の可否をポリシーで受け取る"
```

---

### Task 6: handler がキーを決めて上限を通す

**Files:**
- Modify: `backend/internal/handler/ingredient_resolve.go`
- Test: `backend/internal/handler/ingredient_resolve_test.go`

**Interfaces:**
- Consumes: Task 4 の `ResolveQuota.Check`、Task 5 の `ResolvePolicy`
- Produces:
  - `handler.ResolveQuotaChecker` インターフェース（`Check(ctx, service.ResolveSubject) (bool, service.DegradedReason)`）
  - `handler.NewIngredientResolveHandler(svc IngredientResolveUseCase, quota ResolveQuotaChecker, ipHashSecret string, tokens *auth.JWT) *IngredientResolveHandler` ← 引数が4つになる

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/handler/ingredient_resolve_test.go` の `newResolveServer` / `postResolve` を差し替え、テストを足す:

```go
// stubQuota は上限判定のスタブ。
type stubQuota struct {
	allow    bool
	reason   service.DegradedReason
	subjects []service.ResolveSubject
}

func (q *stubQuota) Check(
	_ context.Context, s service.ResolveSubject,
) (bool, service.DegradedReason) {
	q.subjects = append(q.subjects, s)
	return q.allow, q.reason
}

const resolveTestHashSecret = "test-secret"

func newResolveServer(uc handler.IngredientResolveUseCase, q handler.ResolveQuotaChecker) *echo.Echo {
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	if err != nil {
		panic("テスト用JWTの生成に失敗しました: " + err.Error())
	}
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewIngredientResolveHandler(uc, q, resolveTestHashSecret, tokens).RegisterRoutes(e)
	return e
}

func postResolve(t *testing.T, uc handler.IngredientResolveUseCase, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postResolveWith(t, uc, &stubQuota{allow: true}, body, "")
}

// postResolveWith は上限のスタブとアクセストークンを指定して叩く。
// access が空文字なら非ログインとして送る。
func postResolveWith(
	t *testing.T,
	uc handler.IngredientResolveUseCase,
	q handler.ResolveQuotaChecker,
	body string,
	access string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingredients/resolve", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if access != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	}
	newResolveServer(uc, q).ServeHTTP(rec, req)
	return rec
}

func TestResolveHandler_Quota(t *testing.T) {
	t.Run("非ログインはIPをハッシュ化して数える", func(t *testing.T) {
		uc := &stubResolveUseCase{}
		q := &stubQuota{allow: true}
		postResolveWith(t, uc, q, `{"text":"玉ねぎ"}`, "")

		if len(q.subjects) != 1 {
			t.Fatalf("1回判定するはずです: %d回", len(q.subjects))
		}
		got := q.subjects[0]
		if got.Scope != service.ScopeIP {
			t.Errorf("scope が ip ではありません: %q", got.Scope)
		}
		// 生のIPを保存しないことがこの機能の前提（設計 6.2）。
		if strings.Contains(got.Subject, "192.0.2.1") {
			t.Errorf("生のIPがキーに入っています: %q", got.Subject)
		}
		if len(got.Subject) != 64 {
			t.Errorf("HMAC-SHA256 の hex は64文字のはずです: %q", got.Subject)
		}
	})

	t.Run("ログイン中はユーザーIDで数える", func(t *testing.T) {
		tokens, err := auth.NewJWT([]byte(authTestSecret))
		if err != nil {
			t.Fatalf("JWTの生成に失敗しました: %v", err)
		}
		access, err := tokens.Issue("user-1")
		if err != nil {
			t.Fatalf("アクセストークンの発行に失敗しました: %v", err)
		}

		uc := &stubResolveUseCase{}
		q := &stubQuota{allow: true}
		postResolveWith(t, uc, q, `{"text":"玉ねぎ"}`, access)

		if len(q.subjects) != 1 {
			t.Fatalf("1回判定するはずです: %d回", len(q.subjects))
		}
		if q.subjects[0].Scope != service.ScopeUser || q.subjects[0].Subject != "user-1" {
			t.Errorf("ユーザーIDで数えていません: %+v", q.subjects[0])
		}
	})

	t.Run("上限に達していたら理由をserviceに渡す", func(t *testing.T) {
		uc := &stubResolveUseCase{}
		q := &stubQuota{allow: false, reason: service.ReasonAnonDailyLimit}
		postResolveWith(t, uc, q, `{"text":"玉ねぎ"}`, "")

		if uc.policy.AllowLLM {
			t.Error("AllowLLM が false で渡るはずです")
		}
		if uc.policy.DenyReason != service.ReasonAnonDailyLimit {
			t.Errorf("理由が渡っていません: %q", uc.policy.DenyReason)
		}
		if uc.policy.Subject.Scope != service.ScopeIP {
			t.Errorf("キーが渡っていません: %+v", uc.policy.Subject)
		}
	})

	t.Run("上限に達していても400の検証が先に効く", func(t *testing.T) {
		uc := &stubResolveUseCase{}
		q := &stubQuota{allow: false, reason: service.ReasonAnonDailyLimit}
		rec := postResolveWith(t, uc, q, `{"text":"`+strings.Repeat("あ", 201)+`"}`, "")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("400 を期待しましたが %d でした", rec.Code)
		}
		if uc.calls != 0 {
			t.Error("検証で落ちたのに service が呼ばれています")
		}
	})
}
```

import に `"github.com/yuuyakim/menu-planner/backend/internal/auth"` を足す。

> `authTestSecret` は `backend/internal/handler` のテストで既に定義されている共有の定数。
> アクセストークンの発行は `(*auth.JWT).Issue(userID)`（リフレッシュは `IssueRefresh`）。

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd backend && go test ./internal/handler/ -run TestResolveHandler -v`
Expected: FAIL（`too many arguments in call to handler.NewIngredientResolveHandler`）

- [ ] **Step 3: handler を実装する**

`backend/internal/handler/ingredient_resolve.go` の import に足す:

```go
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
```

型とコンストラクタを差し替える:

```go
// ResolveQuotaChecker は今日の残枠を読む。
type ResolveQuotaChecker interface {
	Check(ctx context.Context, s service.ResolveSubject) (bool, service.DegradedReason)
}

// IngredientResolveHandler は食材テキスト解決のHTTP境界。
type IngredientResolveHandler struct {
	svc   IngredientResolveUseCase
	quota ResolveQuotaChecker
	// ipHashSecret は IP を復元できない値にするための鍵（設計 6.2）。
	ipHashSecret []byte
	tokens       *auth.JWT
}

// NewIngredientResolveHandler は IngredientResolveHandler を生成する。
func NewIngredientResolveHandler(
	svc IngredientResolveUseCase,
	quota ResolveQuotaChecker,
	ipHashSecret string,
	tokens *auth.JWT,
) *IngredientResolveHandler {
	return &IngredientResolveHandler{
		svc: svc, quota: quota, ipHashSecret: []byte(ipHashSecret), tokens: tokens,
	}
}
```

`RegisterRoutes` に `OptionalAuth` を足す:

```go
func (h *IngredientResolveHandler) RegisterRoutes(e *echo.Echo, mw ...echo.MiddlewareFunc) {
	g := e.Group(APIBasePath, mw...)
	// OptionalAuth を付けるのは、ログイン中の利用者を IP ではなくユーザーIDで
	// 数えるため。未認証でも通る（拒否はしない）。
	g.POST("/ingredients/resolve", h.Resolve, OptionalAuth(h.tokens))
}
```

`Resolve` の service 呼び出しを差し替える（長さ検証の後）:

```go
	subject := h.subjectFor(c)
	allow, reason := h.quota.Check(c.Request().Context(), subject)

	result, err := h.svc.Resolve(c.Request().Context(), req.Text, service.ResolvePolicy{
		AllowLLM: allow, DenyReason: reason, Subject: subject,
	})
```

ファイル末尾に足す:

```go
// subjectFor は数え上げのキーを決める。
//
// ログイン中はユーザーID、非ログインは IP のHMAC（設計 6.2）。
// ブラウザ保存で数えないのは、シークレットウィンドウで無限にリセットできるため。
func (h *IngredientResolveHandler) subjectFor(c echo.Context) service.ResolveSubject {
	if userID, ok := UserIDFromContext(c); ok {
		return service.ResolveSubject{Scope: service.ScopeUser, Subject: userID}
	}
	return service.ResolveSubject{
		Scope:   service.ScopeIP,
		Subject: hashIP(h.ipHashSecret, c.RealIP()),
	}
}

// hashIP は IP を元に戻せない文字列にする。
//
// 数を数えるにはこれで足り、生のIPを保存しないため
// プライバシーポリシーの改定が要らない（設計 6.2）。
func hashIP(secret []byte, ip string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}
```

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd backend && go test ./internal/handler/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add backend/internal/handler/ingredient_resolve.go backend/internal/handler/ingredient_resolve_test.go
git commit -m "feat: 読み取りの上限をIP/ユーザー単位で判定する"
```

---

### Task 7: main.go の結線と環境変数

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `.env.example`
- Modify: `docker-compose.yml`
- Modify: `DEPLOY.md`

**Interfaces:**
- Consumes: Task 2 / 4 / 6 のコンストラクタ
- Produces: 環境変数 `RESOLVE_DAILY_LIMIT_ANON` / `RESOLVE_DAILY_LIMIT_USER` / `RESOLVE_DAILY_LIMIT_TOTAL` / `RESOLVE_IP_HASH_SECRET`

- [ ] **Step 1: main.go を結線する**

`backend/cmd/server/main.go` の `resolveSvc := ...` / `resolveHandler := ...` の2行を差し替える:

```go
	// 読み取り（LLM）の日次上限（設計 3章）。分単位のバースト制御は searchLimit が
	// 担い、その上に日次の層を重ねる。0 で無制限にできるのは、Vite プロキシ配下の
	// 開発・E2E で全リクエストが単一IPに集約されるため。
	resolveUsageRepo := repository.NewResolveUsageRepository(pool)
	resolveQuota := service.NewResolveQuota(resolveUsageRepo, service.ResolveQuotaLimits{
		Anon:  envInt("RESOLVE_DAILY_LIMIT_ANON", 10),
		User:  envInt("RESOLVE_DAILY_LIMIT_USER", 30),
		Total: envInt("RESOLVE_DAILY_LIMIT_TOTAL", 300),
	}, time.Now)

	// 未設定で起動させない。既定値で動かすと生IPと同じ強度の値が全環境で共有され、
	// ハッシュ化の意味が無くなる（設計 6.2）。
	resolveIPHashSecret := os.Getenv("RESOLVE_IP_HASH_SECRET")
	if resolveIPHashSecret == "" {
		slog.Error("RESOLVE_IP_HASH_SECRET が未設定です")
		os.Exit(1)
	}

	resolveSvc := service.NewIngredientResolveService(
		ingredientRepo, resolutionRepo, resolveGateway, resolveQuota)
	resolveHandler := handler.NewIngredientResolveHandler(
		resolveSvc, resolveQuota, resolveIPHashSecret, tokens)
```

- [ ] **Step 2: ビルドが通ることを確かめる**

Run: `cd backend && go build ./... && go vet ./...`
Expected: エラーなし

- [ ] **Step 3: .env.example に足す**

`.env.example` の「レート制限」の節の下に足す:

```
# --- 読み取り（LLM）の日次上限（0で無制限） ---
# 「読み取る」がLLMを呼んだ回数だけを数える。完全一致とキャッシュで解けた分は数えない。
# 分単位のバースト制御は SEARCH_RATE_LIMIT_PER_MIN が担う。
RESOLVE_DAILY_LIMIT_ANON=0
RESOLVE_DAILY_LIMIT_USER=0
RESOLVE_DAILY_LIMIT_TOTAL=0

# IPを数えるためのハッシュ鍵。openssl rand -base64 32 で生成した値に置き換えること。
# **空にすると起動に失敗する**（生IPと同じ強度の既定値が共有されるのを防ぐため）。
RESOLVE_IP_HASH_SECRET=dev-only-secret-do-not-use-in-production
```

> 開発の既定を `0` にするのは、Vite プロキシ配下では全リクエストが単一IPに集約され、
> 正当な操作でも即座に上限へ達するため。本番の値（10 / 30 / 300）は DEPLOY.md に書く。

- [ ] **Step 4: docker-compose.yml に足す**

`SEARCH_RATE_LIMIT_PER_MIN` の行の下に足す:

```yaml
      # 読み取り（LLM）の日次上限。開発・E2Eでは 0（無制限）で切る。
      # 本番は spec 値（10 / 30 / 300）を設定する。
      RESOLVE_DAILY_LIMIT_ANON: ${RESOLVE_DAILY_LIMIT_ANON:-0}
      RESOLVE_DAILY_LIMIT_USER: ${RESOLVE_DAILY_LIMIT_USER:-0}
      RESOLVE_DAILY_LIMIT_TOTAL: ${RESOLVE_DAILY_LIMIT_TOTAL:-0}
      RESOLVE_IP_HASH_SECRET: ${RESOLVE_IP_HASH_SECRET:-dev-only-secret-do-not-use-in-production}
```

- [ ] **Step 5: DEPLOY.md に足す**

`DEPLOY.md` で `INGREDIENT_RESOLVER_*` の環境変数を説明している箇所の直後に、同じ書式で次を足す:

- `RESOLVE_DAILY_LIMIT_ANON`（既定 10）: 非ログインの1日あたりの読み取り回数。IP単位。
- `RESOLVE_DAILY_LIMIT_USER`（既定 30）: ログインユーザーの1日あたりの読み取り回数。
- `RESOLVE_DAILY_LIMIT_TOTAL`（既定 300）: サービス全体の1日あたりの読み取り回数。**請求額の天井はこれで決まる**（最悪 約210円/日）。
- `RESOLVE_IP_HASH_SECRET`（必須）: IPを数えるためのHMAC鍵。`openssl rand -base64 32` で生成する。**未設定だと起動に失敗する。**
- 併せて、`make purge-counters` を月1回程度流して古いカウンタ行を消すこと（Task 8 で追加）。

- [ ] **Step 6: 起動を確かめる**

Run: `make up` のあと `make health`
Expected: `{"status":"ok"}`（起動時に `RESOLVE_IP_HASH_SECRET` で落ちないこと）

- [ ] **Step 7: コミット**

```bash
git add backend/cmd/server/main.go .env.example docker-compose.yml DEPLOY.md
git commit -m "feat: 読み取りの日次上限を結線し設定を公開する"
```

---

### Task 8: 古いカウンタを消す運用コマンド

**Files:**
- Modify: `backend/cmd/resolutions/main.go`
- Modify: `Makefile`

**Interfaces:**
- Consumes: Task 2 の `DeleteOlderThan`
- Produces: `go run ./cmd/resolutions prune-counters`、`make purge-counters`

- [ ] **Step 1: コマンドにサブコマンドを足す**

`backend/cmd/resolutions/main.go` を差し替える:

```go
// Command resolutions は食材の解決まわりを運用するためのコマンド。
//
//	go run ./cmd/resolutions purge-unresolved
//	go run ./cmd/resolutions prune-counters
//
// purge-unresolved は食材マスタを更新したあとに流す。「マスタに無い」と
// 保存された語を消し、次回のアクセスで LLM に聞き直させる（設計 9章）。
//
// prune-counters は読み取りの日次カウンタのうち古い行を消す
// （レート制限の設計 6.3）。30日ぶんは残し、使用量を後から振り返れるようにする。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

// counterRetentionDays はカウンタを残す日数。
const counterRetentionDays = 30

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("DBに接続できませんでした", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	switch os.Args[1] {
	case "purge-unresolved":
		n, err := repository.NewResolutionRepository(pool).DeleteUnresolved(ctx)
		if err != nil {
			slog.Error("未解決キャッシュの削除に失敗しました", "error", err)
			os.Exit(1)
		}
		slog.Info("未解決キャッシュを削除しました", "deleted", n)

	case "prune-counters":
		// 境界は JST。サーバが UTC で動くため、日付の意味を揃える。
		jst := time.FixedZone("JST", 9*60*60)
		cutoff := time.Now().In(jst).AddDate(0, 0, -counterRetentionDays).Format("2006-01-02")
		n, err := repository.NewResolveUsageRepository(pool).DeleteOlderThan(ctx, cutoff)
		if err != nil {
			slog.Error("古い読み取りカウンタの削除に失敗しました", "error", err)
			os.Exit(1)
		}
		slog.Info("古い読み取りカウンタを削除しました", "before", cutoff, "deleted", n)

	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: resolutions purge-unresolved|prune-counters")
	os.Exit(2)
}
```

- [ ] **Step 2: ビルドが通ることを確かめる**

Run: `cd backend && go build ./... && go vet ./...`
Expected: エラーなし

- [ ] **Step 3: Makefile にターゲットを足す**

`.PHONY` の行に `prune-counters` を足す（既存の `purge-unresolved` も抜けているので併せて足す）。

`purge-unresolved` ターゲットの下に足す:

```make
# 読み取りの日次カウンタは日付ごとに行が増える。30日より古い行を消す。
# 月1回程度でよい。消しても上限の判定には影響しない（当日ぶんは残る）。
prune-counters: ## 古い読み取りカウンタを消す
	docker compose run --rm backend go run ./cmd/resolutions prune-counters
```

`help` ターゲットの `@echo` 一覧にも1行足す:

```make
	@echo "  prune-counters delete old resolve counters"
```

- [ ] **Step 4: 実際に動かして確かめる**

Run: `make up` のあと `make prune-counters`
Expected: `古い読み取りカウンタを削除しました before=... deleted=0`

- [ ] **Step 5: コミット**

```bash
git add backend/cmd/resolutions/main.go Makefile
git commit -m "feat: 古い読み取りカウンタを消す運用コマンドを足す"
```

---

### Task 9: OpenAPI に degradedReason を足す

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `frontend/src/api/schema.d.ts`（`make gen-api` が生成する。手で書かない）
- Modify: `frontend/src/api/types.ts`

**Interfaces:**
- Consumes: Task 3 の JSON フィールド
- Produces: `Schemas['ResolveResult']['degradedReason']`、`types.DegradedReason`

- [ ] **Step 1: OpenAPI を更新する**

`api/openapi.yaml` の `ResolveResult` スキーマの `degraded` の下に足す:

```yaml
        degradedReason:
          type: string
          description: |
            縮退した理由。degraded が false なら出さない。
            画面はこれで文言を選ぶ。llm_error と counter_unavailable は
            利用者から見れば同じ「今うまく読めない」なので同じ文言を出す。
          enum:
            - llm_error
            - counter_unavailable
            - anon_daily_limit
            - user_daily_limit
            - service_daily_limit
```

同じファイルの `/api/v1/ingredients/resolve` の `description` の末尾に足す:

```
        「読み取る」は LLM を呼ぶため日次の上限がある（非ログインはIP単位）。
        上限に達しても 502 にはせず、①②で解けた分を 200 で返して
        degraded と degradedReason を立てる。チェックボックスから選ぶ経路は
        上限の対象外で、いつでも使える。
```

- [ ] **Step 2: TS の型を再生成する**

Run: `make gen-api`
Expected: `frontend/src/api/schema.d.ts` に `degradedReason?: "llm_error" | ...` が入る

- [ ] **Step 3: types.ts に別名を足す**

`frontend/src/api/types.ts` の `ResolveResult` の下に足す:

```ts
/** DegradedReason は読み取りが縮退した理由（設計 8章）。 */
export type DegradedReason = NonNullable<ResolveResult['degradedReason']>
```

- [ ] **Step 4: 型チェックが通ることを確かめる**

Run: `cd frontend && npx tsc -b`
Expected: エラーなし

- [ ] **Step 5: コミット**

```bash
git add api/openapi.yaml frontend/src/api/schema.d.ts frontend/src/api/types.ts
git commit -m "docs: 解決APIに degradedReason を足す"
```

---

### Task 10: 画面の出し分け

**Files:**
- Modify: `frontend/src/features/menu/ResolveResultPanel.tsx`
- Modify: `frontend/src/features/menu/SearchByIngredientsPage.tsx`
- Test: `frontend/src/features/menu/SearchByIngredientsPage.test.tsx`

**Interfaces:**
- Consumes: Task 9 の `DegradedReason`
- Produces: `ResolveResultPanel` の props に `reason?: DegradedReason`

- [ ] **Step 1: 失敗するテストを書く**

`frontend/src/features/menu/SearchByIngredientsPage.test.tsx` の `respondResolve` の型に `degradedReason` を足す:

```tsx
function respondResolve(result: {
  resolved: { word: string; ingredient: Ingredient }[]
  unresolved: string[]
  degraded: boolean
  degradedReason?: string
}) {
```

同ファイルの末尾に足す:

```tsx
describe('読み取りの上限', () => {
  it('非ログインの上限ではログインへ誘導する', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondResolve({
      resolved: [],
      unresolved: ['マツタケ'],
      degraded: true,
      degradedReason: 'anon_daily_limit',
    })
    renderWithProviders(<SearchByIngredientsPage />)

    await readAloud(user, 'マツタケ')

    expect(
      await screen.findByText(/今日の読み取り上限に達しました/),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'ログイン' })).toHaveAttribute(
      'href',
      '/login',
    )
  })

  it('ログイン済みの上限ではログインへ誘導しない', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondResolve({
      resolved: [],
      unresolved: ['マツタケ'],
      degraded: true,
      degradedReason: 'user_daily_limit',
    })
    renderWithProviders(<SearchByIngredientsPage />)

    await readAloud(user, 'マツタケ')

    expect(await screen.findByText(/明日また使えます/)).toBeInTheDocument()
    // ログインしても増えないので導線を出さない。
    expect(screen.queryByRole('link', { name: 'ログイン' })).not.toBeInTheDocument()
  })

  it('全体の上限では混み合っていると伝える', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondResolve({
      resolved: [],
      unresolved: ['マツタケ'],
      degraded: true,
      degradedReason: 'service_daily_limit',
    })
    renderWithProviders(<SearchByIngredientsPage />)

    await readAloud(user, 'マツタケ')

    expect(
      await screen.findByText(/ただいま読み取りが混み合っています/),
    ).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'ログイン' })).not.toBeInTheDocument()
  })

  it('カウンタが読めないときはLLM障害と同じ文言を出す', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondResolve({
      resolved: [],
      unresolved: ['マツタケ'],
      degraded: true,
      degradedReason: 'counter_unavailable',
    })
    renderWithProviders(<SearchByIngredientsPage />)

    await readAloud(user, 'マツタケ')

    // 利用者から見れば「今うまく読めない」で同じ。区別はログの側だけに残す。
    expect(await screen.findByText(/一部だけ読み取れました/)).toBeInTheDocument()
  })

  it('理由が無い縮退は従来の文言のまま', async () => {
    const user = userEvent.setup()
    respondIngredients()
    respondResolve({
      resolved: [],
      unresolved: ['マツタケ'],
      degraded: true,
      degradedReason: 'llm_error',
    })
    renderWithProviders(<SearchByIngredientsPage />)

    await readAloud(user, 'マツタケ')

    expect(await screen.findByText(/一部だけ読み取れました/)).toBeInTheDocument()
  })
})
```

（このファイルは冒頭で `import { describe, expect, it } from 'vitest'` 済みなので import の追加は要らない。）

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd frontend && npm test -- SearchByIngredientsPage`
Expected: FAIL（「今日の読み取り上限に達しました」が見つからない）

- [ ] **Step 3: ResolveResultPanel を実装する**

`frontend/src/features/menu/ResolveResultPanel.tsx` を差し替える:

```tsx
import { Link } from 'react-router'

import type { DegradedReason } from '../../api/types'

type Props = {
  unresolved: string[]
  degraded: boolean
  reason?: DegradedReason
}

// degradedMessages は縮退の理由ごとの文言（設計 10章）。
//
// **値は5つ、文言は4つ。** counter_unavailable は llm_error と同じ文言を出す。
// 利用者から見ればどちらも「今うまく読めない」で、区別が要るのは運用の側だけ。
const partialMessage = '一部だけ読み取れました。残りは下から選んでください。'

const degradedMessages: Record<DegradedReason, string> = {
  llm_error: partialMessage,
  counter_unavailable: partialMessage,
  anon_daily_limit:
    '今日の読み取り上限に達しました。ログインすると回数が増えます。',
  user_daily_limit: '今日の読み取り上限に達しました。明日また使えます。',
  service_daily_limit:
    'ただいま読み取りが混み合っています。時間をおいてお試しください。',
}

// limitReasons は「上限に達した」系の理由。障害系（llm_error /
// counter_unavailable）と違い、チェックボックスの経路がまだ使えることを添える。
const limitReasons: DegradedReason[] = [
  'anon_daily_limit',
  'user_daily_limit',
  'service_daily_limit',
]

// ResolveResultPanel は読み取りの結果のうち、チェックに現れないものを伝える。
//
// **ピッカーの上に置く。** IngredientPicker は max-h-[55vh] のスクロール領域を
// 持つため、入ったチェックが画面外になりうる。変化に気付ける位置に出す（設計 6.2）。
export function ResolveResultPanel({ unresolved, degraded, reason }: Props) {
  if (unresolved.length === 0 && !degraded) return null

  // 理由が無い縮退は、上限ではなく LLM 側の失敗として扱う。
  const message = degradedMessages[reason ?? 'llm_error']
  // ログインしても増えないケースで導線を出すと誤導になる。
  const showLogin = reason === 'anon_daily_limit'
  const isLimit = reason !== undefined && limitReasons.includes(reason)

  return (
    // aria-label を付けるのは、IngredientPicker の選択数表示も role="status" を
    // 持つため。名前が無いと支援技術（とテスト）がどちらの通知か区別できない。
    <div className="space-y-2" role="status" aria-label="読み取りの結果">
      {degraded && (
        <p className="rounded-2xl bg-kon-cream px-5 py-3 text-sm text-kon-ink/80">
          {message}
          {showLogin && (
            <Link
              to="/login"
              className="mt-2 inline-block rounded-full border border-kon-leaf-soft bg-white px-4 py-1.5 text-sm font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream"
            >
              ログイン
            </Link>
          )}
          {isLimit && (
            <span className="mt-1 block text-kon-ink/60">
              下のリストから選んで探すことはできます。
            </span>
          )}
        </p>
      )}
      {unresolved.length > 0 && (
        <p className="rounded-2xl bg-kon-cream px-5 py-3 text-sm text-kon-ink/80">
          登録がありませんでした: {unresolved.join('・')}
          <span className="mt-1 block text-kon-ink/60">
            この{unresolved.length}件は検索に使われません。
          </span>
        </p>
      )}
    </div>
  )
}
```

- [ ] **Step 4: SearchByIngredientsPage に理由を持たせる**

`frontend/src/features/menu/SearchByIngredientsPage.tsx` を3か所直す。

import に足す:

```tsx
import type { DegradedReason, MenuMatch } from '../../api/types'
```

（既存の `import type { MenuMatch } from '../../api/types'` を置き換える。）

state を足す:

```tsx
  const [degradedReason, setDegradedReason] = useState<DegradedReason | undefined>(
    undefined,
  )
```

`resolve` の `onSuccess` に足す:

```tsx
      setDegraded(result.degraded)
      setDegradedReason(result.degradedReason)
```

`clear` に足す:

```tsx
    setDegraded(false)
    setDegradedReason(undefined)
```

パネルの呼び出しを差し替える:

```tsx
          <ResolveResultPanel
            unresolved={unresolved}
            degraded={degraded}
            reason={degradedReason}
          />
```

- [ ] **Step 5: テストが通ることを確かめる**

Run: `cd frontend && npm test -- SearchByIngredientsPage`
Expected: PASS（既存のテストも含めて）

- [ ] **Step 6: 全体の確認**

Run: `make test`
Expected: バックエンド・フロントエンドともに PASS

Run: `make lint`
Expected: エラーなし

- [ ] **Step 7: コミット**

```bash
git add frontend/src/features/menu/ResolveResultPanel.tsx frontend/src/features/menu/SearchByIngredientsPage.tsx frontend/src/features/menu/SearchByIngredientsPage.test.tsx
git commit -m "feat: 読み取りの上限に達したときの文言を出し分ける"
```

---

## 実装後の確認

- [ ] `make test` が通る
- [ ] `make lint` が通る
- [ ] `make up && make seed && make test-e2e` が通る（既存の `from-fridge.spec.ts` が上限で落ちないこと。開発の既定は `0` = 無制限）
- [ ] `make migrate-down` → `make migrate` で 000014 が往復できる
- [ ] `RESOLVE_IP_HASH_SECRET` を空にすると起動が失敗する
