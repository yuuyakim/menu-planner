# 手持ちの食材のテキスト入力（LLMで表記揺れ吸収）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 冷蔵庫の中身を「豚こま、玉ねぎ、にんじん」と自由記述で書けるようにし、LLM で表記揺れを吸収して既存のチェックボックスに反映する。

**Architecture:** 新エンドポイント `POST /api/v1/ingredients/resolve` を足す。解決は ①食材マスタとの完全一致 → ②解決キャッシュ（`ingredient_resolutions`）→ ③LLM の3段。①②で解ければ LLM を呼ばない。LLM には UUID ではなく食材名だけを渡し、食材名を返させてアプリ側でIDに引き直す。LLM が落ちても①②の結果を 200 で返す（部分成功）。既存の `POST /menus/search-by-ingredients`・`IngredientPicker.tsx`・`IngredientRepository` には手を入れない。

**Tech Stack:** Go 1.x / echo / pgx v5 / PostgreSQL（Neon）/ React 19 + TypeScript + Vite / TanStack Query / Tailwind / Anthropic Go SDK

**設計書:** `docs/superpowers/specs/2026-08-02-ingredient-text-resolve-design.md`

## Global Constraints

- モジュールパス: `github.com/yuuyakim/menu-planner/backend`
- 依存の向きは `handler → service → repository`（spec.md 3.2）。インターフェースは **service パッケージ側**で定義する（依存関係逆転）。
- コメントは日本語で「なぜそうしたか」を書く。参照した仕様は `spec.md 2.9` のように節番号を添える。
- **既存ファイルの変更は最小限。** `IngredientRepository`・`IngredientService`・`POST /menus/search-by-ingredients`・`IngredientPicker.tsx` のロジックは変更しない（コメント更新を除く）。
- エラーはセンチネル `ErrXxx` を service に置き、handler が HTTP に変換する。
- マイグレーション番号は **000013 から**（既存の最大は 000012）。
- 新エンドポイントは**認証不要**、既存のレート制限ミドルウェア `searchLimit` を適用する。
- **実 LLM を叩くテストは CI に入れない。** eval のみ `-tags=eval`。
- テーブル・列の CHECK 制約は既存の流儀に合わせ、値の妥当性は**アプリ側で検証**する。

---

## File Structure

| ファイル | 責務 |
| --- | --- |
| `backend/internal/domain/ingredient_word.go` | 入力テキストの分割と正規化（純関数） |
| `backend/db/migrations/000013_create_ingredient_resolutions.{up,down}.sql` | 解決キャッシュのテーブル |
| `backend/internal/repository/ingredient_resolution.go` | 解決キャッシュの読み書き |
| `backend/internal/service/ingredient_resolve.go` | 3段解決・部分成功の組み立て・インターフェース定義 |
| `backend/internal/gateway/resolver_stub.go` | LLM のスタブ（テスト・CI・E2E） |
| `backend/internal/gateway/resolver_claude.go` | Claude Haiku 4.5 実装 |
| `backend/internal/gateway/resolver_deepseek.go` | DeepSeek V4 Flash 実装 |
| `backend/internal/gateway/resolver_factory.go` | 環境変数から実装を選ぶ |
| `backend/internal/handler/ingredient_resolve.go` | HTTP境界・入力長の検証 |
| `backend/internal/gateway/eval/` | eval ハーネス（`-tags=eval`） |
| `frontend/src/features/menu/IngredientTextInput.tsx` | textarea と「読み取る」 |
| `frontend/src/features/menu/ResolveResultPanel.tsx` | 未解決語・縮退の表示 |

---

## Task 1: 入力テキストの分割と正規化（domain）

**Files:**
- Create: `backend/internal/domain/ingredient_word.go`
- Test: `backend/internal/domain/ingredient_word_test.go`

**Interfaces:**
- Consumes: なし
- Produces:
  - `func SplitIngredientWords(text string) []string` — 分割のみ（正規化しない）。空語は捨てる。
  - `func NormalizeIngredientWord(s string) string` — 1語を正規化する。

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/domain/ingredient_word_test.go`:

```go
package domain_test

import (
	"reflect"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestSplitIngredientWords(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"読点", "豚こま、玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"半角カンマ", "豚こま,玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"全角カンマ", "豚こま，玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"改行", "豚こま\n玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"半角空白", "豚こま 玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"全角空白", "豚こま　玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"中黒", "豚こま・玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"区切りの連続は1つ", "豚こま、、 玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"前後の区切りは捨てる", "、豚こま、", []string{"豚こま"}},
		{"空文字", "", []string{}},
		{"区切りだけ", "、、、", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.SplitIngredientWords(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitIngredientWords(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeIngredientWord(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"前後の空白を除く", "  玉ねぎ  ", "玉ねぎ"},
		{"漢字かなはそのまま", "玉ねぎ", "玉ねぎ"},
		{"カタカナはひらがなに", "タマネギ", "たまねぎ"},
		{"ひらがなはそのまま", "たまねぎ", "たまねぎ"},
		{"半角カナは全角経由でひらがなに", "ﾀﾏﾈｷﾞ", "たまねぎ"},
		// 小文字化はしない方針なので、NFKC 後の大文字がそのまま残る（設計 3.4）。
		{"全角英数は半角に", "Ａ１", "A1"},
		{"長音は保つ", "ベーコン", "べーこん"},
		{"混在", "　ブタこま　", "ぶたこま"},
		{"空文字", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.NormalizeIngredientWord(tt.in); got != tt.want {
				t.Errorf("NormalizeIngredientWord(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/domain/ -run TestSplitIngredientWords -v
```
Expected: FAIL（`undefined: domain.SplitIngredientWords`）

- [ ] **Step 3: 最小の実装を書く**

`backend/internal/domain/ingredient_word.go`:

```go
package domain

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// ingredientWordSeparators は手持ちの食材テキストの区切り文字（設計 3.4）。
//
// 「・」を入れるのは、買い物メモの書き方として一般的なため。既存の画面も
// 「使える食材: A・B・C」と中黒で並べている。区切り文字を含む食材名は
// マスタに存在しないため、誤分割の恐れはない。
const ingredientWordSeparators = "、,，\n\r\t 　・"

// SplitIngredientWords は入力テキストを語に分割する。
//
// 連続した区切りは1つとして扱い、空になった語は捨てる。
// **正規化はここではしない**（呼び出し側が NormalizeIngredientWord を通す）。
// 分割と正規化を分けておくと、利用者が書いた元の語を保持したまま
// 正規化後の値で突き合わせられる（設計 4.1 の `word` は元の語を返す）。
func SplitIngredientWords(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return strings.ContainsRune(ingredientWordSeparators, r)
	})

	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if trimmed := strings.TrimSpace(f); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// NormalizeIngredientWord は1語を突き合わせ用の形に正規化する（設計 3.4）。
//
//	1. 前後の空白を除去
//	2. NFKC 正規化（全角英数→半角、半角カナ→全角カナ）
//	3. カタカナ → ひらがな
//
// 3を入れるのは ingredients.name_kana に当たるようにするため。
// 「タマネギ」→「たまねぎ」で name_kana と一致し、LLM を呼ばずに解決できる。
//
// **小文字化はしない。** 日本語主体のため効果が薄く、食材名に含まれる
// 英字（例: Ｌサイズ）の見た目を変える副作用の方が大きい。
//
// **この関数は ①完全一致 / ②キャッシュ / ③LLM の結果引き当ての
// すべてが通る。** 別々に正規化すると、①で引けなかった語が②でも
// 引けないという穴が開く。
func NormalizeIngredientWord(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = norm.NFKC.String(s)
	return katakanaToHiragana(s)
}

// katakanaToHiragana は全角カタカナをひらがなに変換する。
//
// 長音「ー」や中黒はカタカナの範囲外なのでそのまま残る。
// ヴ（U+30F4）などの濁音付きも符号位置の差が同じなので同じ式で変換できる。
func katakanaToHiragana(s string) string {
	return strings.Map(func(r rune) rune {
		// 'ァ'(U+30A1) 〜 'ヶ'(U+30F6) を 'ぁ'(U+3041) 〜 に寄せる。
		if r >= 'ァ' && r <= 'ヶ' {
			return r - 0x60
		}
		return r
	}, s)
}
```

- [ ] **Step 4: 依存を追加してテストが通ることを確認**

```bash
cd backend && go get golang.org/x/text/unicode/norm && go mod tidy
go test ./internal/domain/ -run 'TestSplitIngredientWords|TestNormalizeIngredientWord' -v
```
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add backend/internal/domain/ingredient_word.go backend/internal/domain/ingredient_word_test.go backend/go.mod backend/go.sum
git commit -m "feat: 手持ちの食材テキストの分割と正規化を足す"
```

---

## Task 2: 解決キャッシュのテーブル（マイグレーション）

**Files:**
- Create: `backend/db/migrations/000013_create_ingredient_resolutions.up.sql`
- Create: `backend/db/migrations/000013_create_ingredient_resolutions.down.sql`
- Test: `backend/internal/repository/ingredient_resolution_schema_test.go`

**Interfaces:**
- Consumes: なし
- Produces: テーブル `ingredient_resolutions(input_word text PK, ingredient_id uuid NULL, resolved_at timestamptz)`

- [ ] **Step 1: 失敗するテストを書く**

既存の `ingredients_schema_test.go` と同じ流儀（実DBに当てる）。

`backend/internal/repository/ingredient_resolution_schema_test.go`:

```go
package repository_test

import (
	"context"
	"testing"
)

// TestIngredientResolutionsSchema は解決キャッシュのテーブル定義を確かめる。
func TestIngredientResolutionsSchema(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	t.Run("未解決はNULLで保存できる", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO ingredient_resolutions (input_word, ingredient_id)
			 VALUES ('まつたけ', NULL)
			 ON CONFLICT (input_word) DO NOTHING`)
		if err != nil {
			t.Fatalf("NULL の解決を保存できませんでした: %v", err)
		}
	})

	t.Run("同じ語は二重に入らない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO ingredient_resolutions (input_word, ingredient_id)
			 VALUES ('まつたけ', NULL)`)
		if err == nil {
			t.Fatal("主キー違反になるはずが成功しました")
		}
	})

	t.Run("存在しない食材IDは入らない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO ingredient_resolutions (input_word, ingredient_id)
			 VALUES ('ありえない', '00000000-0000-0000-0000-000000000001')`)
		if err == nil {
			t.Fatal("外部キー違反になるはずが成功しました")
		}
	})
}
```

> `newTestPool` は既存のテストヘルパー。`backend/internal/repository/` 配下の既存 `*_schema_test.go` で使われている実際の関数名を確認し、違う名前ならそれに合わせること。

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/repository/ -run TestIngredientResolutionsSchema -v
```
Expected: FAIL（`relation "ingredient_resolutions" does not exist`）

- [ ] **Step 3: マイグレーションを書く**

`backend/db/migrations/000013_create_ingredient_resolutions.up.sql`:

```sql
-- 入力語から食材への解決キャッシュ（設計 5章）。
--
-- 「豚こま」→ 豚肉 のような表記揺れの対応づけを、一度 LLM に聞いたら保存しておく。
-- 同じ語を二度 LLM に問い合わせないための仕組みであり、運用が続くほど
-- LLM 呼び出しは逓減する。spec.md 2.9 が却下した「別名辞書」を、人手ではなく
-- LLM に育てさせる形にあたる。
--
-- **利用者に紐づかない。** 誰が書いた語であっても対応づけの結果は同じなので、
-- 未認証も含めて全員で共有する。
CREATE TABLE ingredient_resolutions (
    -- domain.NormalizeIngredientWord を通した後の語。
    -- 正規化前の語を入れると「タマネギ」と「たまねぎ」が別行になり、
    -- キャッシュが効かなくなる。
    input_word    text        PRIMARY KEY,
    -- **NULL は「マスタに無い」を意味する。**
    -- 未解決もキャッシュしないと、マスタに無い語だけが毎回 LLM を通ることになり、
    -- いちばん無駄な呼び出しが残り続ける。
    ingredient_id uuid        REFERENCES ingredients (id) ON DELETE CASCADE,
    resolved_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT ingredient_resolutions_input_word_not_blank
        CHECK (btrim(input_word) <> '')
);

-- 食材マスタを更新したとき、NULL 行だけを消す運用のための経路（設計 9章）。
-- 新しい食材が足されると、過去に NULL で保存した語が解決可能になる。
CREATE INDEX ingredient_resolutions_unresolved_idx
    ON ingredient_resolutions (input_word)
    WHERE ingredient_id IS NULL;
```

`backend/db/migrations/000013_create_ingredient_resolutions.down.sql`:

```sql
DROP TABLE IF EXISTS ingredient_resolutions;
```

- [ ] **Step 4: テストが通ることを確認**

```bash
cd backend && go test ./internal/repository/ -run TestIngredientResolutionsSchema -v
```
Expected: PASS

> **マイグレーションを手で流す必要はない。** `internal/repository` のテストは
> `TestMain`（`testhelper_test.go`）が testcontainers で Postgres を起動し、
> `db.Migrate(sharedDSN, db.MigrateUp)` を自動で実行する。新しい
> `000013_*.up.sql` はそこで一緒に適用される。

> ⛔ **`go run ./cmd/migrate down` を実行しないこと。** `.env` の
> `DATABASE_URL` は**本番の Neon** を指している。手元で down を流すと
> 本番のテーブルが落ちる。down の検証は Step 5 の方法で行う。

- [ ] **Step 5: down SQL が正しいことを確認する**

`000013_create_ingredient_resolutions.down.sql` が `up` で作ったものを
過不足なく落とすかを**目視で確認**する。このマイグレーションで作るのは
テーブル1つと、その上の部分インデックス1つだけで、インデックスは
`DROP TABLE` に含まれるため `DROP TABLE IF EXISTS ingredient_resolutions;`
の1行で足りる。

本番に流す前の実地確認は、ローカルの docker-compose DB に対して
利用者が行う（デプロイ手順に含める）。**このタスクでは実行しない。**

- [ ] **Step 6: コミット**

```bash
git add backend/db/migrations/000013_create_ingredient_resolutions.up.sql \
        backend/db/migrations/000013_create_ingredient_resolutions.down.sql \
        backend/internal/repository/ingredient_resolution_schema_test.go
git commit -m "feat: 入力語から食材への解決キャッシュのテーブルを足す"
```

---

## Task 3: 解決キャッシュのリポジトリ

**Files:**
- Create: `backend/internal/repository/ingredient_resolution.go`
- Test: `backend/internal/repository/ingredient_resolution_test.go`

**Interfaces:**
- Consumes: Task 2 のテーブル
- Produces:
  - `type ResolutionRepository struct { pool *pgxpool.Pool }`
  - `func NewResolutionRepository(pool *pgxpool.Pool) *ResolutionRepository`
  - `func (r *ResolutionRepository) FindByWords(ctx context.Context, words []string) (map[string]*domain.IngredientID, error)` — キーは正規化済みの語。値が `nil` は「マスタに無いと確定済み」。**見つからなかった語はキーごと入らない。**
  - `func (r *ResolutionRepository) Save(ctx context.Context, word string, id *domain.IngredientID) error`

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/repository/ingredient_resolution_test.go`:

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func TestResolutionRepository(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewResolutionRepository(pool)

	// **既存行を借りない。** テスト用DBの ingredients は
	// TestMain がシードするものではなく、他のテストが insertIngredient で
	// 足したものが溜まっているだけ。`SELECT ... LIMIT 1` に頼ると
	// 実行順に依存し、-run で単独実行したときに落ちる。自分で1件作る。
	//
	// insertIngredient は既存のヘルパー（ingredients_schema_test.go）で、
	// 生のUUID文字列を返す。
	rawID := insertIngredient(t, pool, "テスト用豚肉", "てすとようぶたにく", "meat")
	id, err := domain.ParseIngredientID(rawID)
	if err != nil {
		t.Fatalf("食材IDを解釈できませんでした: %v", err)
	}

	t.Run("保存した解決を引ける", func(t *testing.T) {
		if err := repo.Save(ctx, "ぶたこま", &id); err != nil {
			t.Fatalf("Save が失敗しました: %v", err)
		}
		got, err := repo.FindByWords(ctx, []string{"ぶたこま"})
		if err != nil {
			t.Fatalf("FindByWords が失敗しました: %v", err)
		}
		v, ok := got["ぶたこま"]
		if !ok {
			t.Fatal("保存したはずの語が引けませんでした")
		}
		if v == nil || *v != id {
			t.Errorf("食材IDが一致しません: got %v, want %v", v, id)
		}
	})

	t.Run("未解決(NULL)も引ける", func(t *testing.T) {
		if err := repo.Save(ctx, "まつたけ2", nil); err != nil {
			t.Fatalf("Save が失敗しました: %v", err)
		}
		got, err := repo.FindByWords(ctx, []string{"まつたけ2"})
		if err != nil {
			t.Fatalf("FindByWords が失敗しました: %v", err)
		}
		v, ok := got["まつたけ2"]
		if !ok {
			t.Fatal("未解決として保存した語が引けませんでした")
		}
		if v != nil {
			t.Errorf("未解決は nil であるべきです: got %v", v)
		}
	})

	t.Run("未知の語はキーごと入らない", func(t *testing.T) {
		got, err := repo.FindByWords(ctx, []string{"まだきいてない"})
		if err != nil {
			t.Fatalf("FindByWords が失敗しました: %v", err)
		}
		if _, ok := got["まだきいてない"]; ok {
			t.Error("問い合わせたことのない語がキーに入っています")
		}
	})

	t.Run("同じ語の再保存は上書きされる", func(t *testing.T) {
		if err := repo.Save(ctx, "うわがき", nil); err != nil {
			t.Fatalf("1回目の Save が失敗しました: %v", err)
		}
		if err := repo.Save(ctx, "うわがき", &id); err != nil {
			t.Fatalf("2回目の Save が失敗しました: %v", err)
		}
		got, _ := repo.FindByWords(ctx, []string{"うわがき"})
		if got["うわがき"] == nil {
			t.Error("上書きされていません")
		}
	})

	t.Run("0件の問い合わせは空を返す", func(t *testing.T) {
		got, err := repo.FindByWords(ctx, nil)
		if err != nil {
			t.Fatalf("FindByWords が失敗しました: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("空であるべきです: got %v", got)
		}
	})
}
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/repository/ -run TestResolutionRepository -v
```
Expected: FAIL（`undefined: repository.NewResolutionRepository`）

- [ ] **Step 3: 実装を書く**

`backend/internal/repository/ingredient_resolution.go`:

```go
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ResolutionRepository は入力語から食材への解決キャッシュへのアクセスを提供する。
//
// このテーブルは利用者に紐づかない（設計 5章）。誰が書いた語でも
// 対応づけの結果は同じなので、未認証も含めて全員で共有する。
type ResolutionRepository struct {
	pool *pgxpool.Pool
}

// NewResolutionRepository は ResolutionRepository を生成する。
func NewResolutionRepository(pool *pgxpool.Pool) *ResolutionRepository {
	return &ResolutionRepository{pool: pool}
}

// FindByWords は正規化済みの語に対する解決済みの対応づけを引く。
//
// 戻り値のキーは正規化済みの語。値が nil は「マスタに無いと確定済み」を表し、
// **キーが存在しないことは「まだ問い合わせていない」を表す。** この2つを
// 区別できないと、マスタに無い語を毎回 LLM に聞き直すことになる。
func (r *ResolutionRepository) FindByWords(
	ctx context.Context, words []string,
) (map[string]*domain.IngredientID, error) {
	out := make(map[string]*domain.IngredientID, len(words))
	if len(words) == 0 {
		// 空配列を投げても0件が返るだけだが、無駄な往復を省く。
		return out, nil
	}

	rows, err := r.pool.Query(ctx,
		`SELECT input_word, ingredient_id
		   FROM ingredient_resolutions
		  WHERE input_word = ANY($1::text[])`, words)
	if err != nil {
		return nil, fmt.Errorf("解決キャッシュの取得に失敗しました: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var word string
		var raw *string
		if err := rows.Scan(&word, &raw); err != nil {
			return nil, fmt.Errorf("解決キャッシュの読み取りに失敗しました: %w", err)
		}
		if raw == nil {
			out[word] = nil
			continue
		}
		id, err := domain.ParseIngredientID(*raw)
		if err != nil {
			// DBの外部キーが効いているのでここに来るのは異常。握りつぶさない。
			return nil, fmt.Errorf("解決キャッシュの食材IDが不正です(%q): %w", word, err)
		}
		out[word] = &id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("解決キャッシュの取得に失敗しました: %w", err)
	}
	return out, nil
}

// Save は1語ぶんの解決結果を保存する。id が nil なら「マスタに無い」として残す。
//
// 同じ語を再度保存したときは上書きする。食材マスタが更新されて
// 解決可能になった語を、後から正しい値に置き換えられるようにするため。
func (r *ResolutionRepository) Save(
	ctx context.Context, word string, id *domain.IngredientID,
) error {
	var raw *string
	if id != nil {
		s := id.String()
		raw = &s
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO ingredient_resolutions (input_word, ingredient_id)
		 VALUES ($1, $2)
		 ON CONFLICT (input_word)
		 DO UPDATE SET ingredient_id = EXCLUDED.ingredient_id, resolved_at = now()`,
		word, raw)
	if err != nil {
		return fmt.Errorf("解決キャッシュの保存に失敗しました(%q): %w", word, err)
	}
	return nil
}
```

- [ ] **Step 4: テストが通ることを確認**

```bash
cd backend && go test ./internal/repository/ -run TestResolutionRepository -v
```
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add backend/internal/repository/ingredient_resolution.go backend/internal/repository/ingredient_resolution_test.go
git commit -m "feat: 解決キャッシュのリポジトリを足す"
```

---

## Task 4: Gateway インターフェースとスタブ

**Files:**
- Create: `backend/internal/service/ingredient_resolve.go`（インターフェース定義のみ。サービス本体は Task 5）
- Create: `backend/internal/gateway/resolver_stub.go`
- Test: `backend/internal/gateway/resolver_stub_test.go`

**Interfaces:**
- Consumes: なし
- Produces:
  - `type GatewayResolution struct { Word string; Name string }`（service パッケージ）
  - `type IngredientResolveGateway interface { Resolve(ctx context.Context, words []string, catalog []string) ([]GatewayResolution, error) }`（service パッケージ）
  - `type StubResolver struct { ... }`（gateway パッケージ）と `func NewStubResolver(mapping map[string]string) StubResolver`

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/gateway/resolver_stub_test.go`:

```go
package gateway_test

import (
	"context"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
)

func TestStubResolver(t *testing.T) {
	ctx := context.Background()
	r := gateway.NewStubResolver(map[string]string{"ぶたこま": "豚肉"})

	t.Run("対応づけのある語は食材名を返す", func(t *testing.T) {
		got, err := r.Resolve(ctx, []string{"ぶたこま"}, []string{"豚肉", "玉ねぎ"})
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got) != 1 || got[0].Word != "ぶたこま" || got[0].Name != "豚肉" {
			t.Errorf("想定と違います: %+v", got)
		}
	})

	t.Run("対応づけの無い語は空文字を返す", func(t *testing.T) {
		got, err := r.Resolve(ctx, []string{"まつたけ"}, []string{"豚肉"})
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got) != 1 || got[0].Name != "" {
			t.Errorf("該当なしは空文字であるべきです: %+v", got)
		}
	})

	t.Run("問い合わせた語の数だけ返る", func(t *testing.T) {
		got, err := r.Resolve(ctx, []string{"ぶたこま", "まつたけ"}, []string{"豚肉"})
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("2件返るべきです: got %d", len(got))
		}
	})
}
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/gateway/ -run TestStubResolver -v
```
Expected: FAIL（`undefined: gateway.NewStubResolver`）

- [ ] **Step 3: インターフェースとスタブを書く**

`backend/internal/service/ingredient_resolve.go`（この時点ではインターフェースのみ）:

```go
package service

import "context"

// GatewayResolution は1語ぶんの対応づけ結果。
type GatewayResolution struct {
	// Word は問い合わせた語（正規化済み）。
	Word string
	// Name は対応づいた食材名。**空文字は「該当なし」を表す。**
	Name string
}

// IngredientResolveGateway は未解決語を食材名に対応づける外部サービス。
//
// RecipeSearchGateway（spec.md 3.3）と同じく、実装差し替えのみで
// プロバイダを切り替えられる状態にする。
//
// **食材IDではなく食材名をやり取りする**（設計 3.5）。UUID は36文字あり
// 166件で約3000トークンに達するうえ、1文字の取り違えが解決失敗になる。
// 名前で受けてアプリ側でIDに引き直せば、マスタに無い名前が返っても
// 「該当なし」に落ちるだけで済む。
type IngredientResolveGateway interface {
	// Resolve は words を catalog のいずれかの食材名に対応づける。
	// 戻り値は words と同じ件数・同じ順序であることを期待する。
	Resolve(ctx context.Context, words []string, catalog []string) ([]GatewayResolution, error)
}
```

`backend/internal/gateway/resolver_stub.go`:

```go
package gateway

import (
	"context"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// StubResolver は外部APIを呼ばずに、あらかじめ渡された対応表で解決する。
//
// APIキーが無くても全機能を動かせる状態を保つために使う（開発・CI・E2E）。
// Stub（レシピ検索）と同じ役割で、こちらは対応表を差し替えられるようにして
// テストから期待する解決結果を与えられるようにしている。
//
// 状態を変えないため、複数の goroutine から同時に使ってよい。
type StubResolver struct {
	mapping map[string]string
}

// NewStubResolver は対応表を持つスタブを返す。
// mapping のキーは正規化済みの語、値は食材名。
func NewStubResolver(mapping map[string]string) StubResolver {
	return StubResolver{mapping: mapping}
}

// Resolve は対応表を引く。**catalog は参照しない。**
// スタブの目的は決定的な結果を返すことであり、マスタとの整合は
// 呼び出し側（service）が名前→IDの引き当てで確かめる。
func (s StubResolver) Resolve(
	_ context.Context, words []string, _ []string,
) ([]service.GatewayResolution, error) {
	out := make([]service.GatewayResolution, 0, len(words))
	for _, w := range words {
		// 対応表に無ければ空文字＝該当なし。
		out = append(out, service.GatewayResolution{Word: w, Name: s.mapping[w]})
	}
	return out, nil
}
```

- [ ] **Step 4: テストが通ることを確認**

```bash
cd backend && go test ./internal/gateway/ -run TestStubResolver -v && go build ./...
```
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/ingredient_resolve.go backend/internal/gateway/resolver_stub.go backend/internal/gateway/resolver_stub_test.go
git commit -m "feat: 食材解決のGatewayインターフェースとスタブを足す"
```

---

## Task 5: 解決サービス ①完全一致のみ

**Files:**
- Modify: `backend/internal/service/ingredient_resolve.go`
- Test: `backend/internal/service/ingredient_resolve_test.go`

**Interfaces:**
- Consumes: `domain.SplitIngredientWords` / `domain.NormalizeIngredientWord`（Task 1）、`IngredientResolveGateway`（Task 4）
- Produces:
  - `type ResolvedWord struct { Word string; Ingredient domain.Ingredient }`
  - `type ResolveResult struct { Resolved []ResolvedWord; Unresolved []string; Degraded bool }`
  - `type IngredientResolveService struct { ... }`
  - `func NewIngredientResolveService(ingredients IngredientRepository, cache ResolutionRepository, gw IngredientResolveGateway) *IngredientResolveService`
  - `func (s *IngredientResolveService) Resolve(ctx context.Context, text string) (ResolveResult, error)`
  - `var ErrEmptyResolveText = errors.New("食材のテキストが空です")`
  - `type ResolutionRepository interface { FindByWords(...); Save(...) }`（service 側で定義）

このタスクでは **Gateway を一切呼ばない**。完全一致で解けなかった語はすべて `Unresolved` に落とす。

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/service/ingredient_resolve_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// countingResolver は「何回呼ばれたか」を数えるスタブ。
// ①②で解けたときに Gateway が呼ばれないことを検証するために使う。
type countingResolver struct {
	calls   int
	mapping map[string]string
	err     error
}

func (r *countingResolver) Resolve(
	_ context.Context, words []string, _ []string,
) ([]service.GatewayResolution, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	out := make([]service.GatewayResolution, 0, len(words))
	for _, w := range words {
		out = append(out, service.GatewayResolution{Word: w, Name: r.mapping[w]})
	}
	return out, nil
}

// fakeIngredientRepo は食材マスタの最小のインメモリ実装。
type fakeIngredientRepo struct {
	items []domain.Ingredient
}

func (r *fakeIngredientRepo) FindAll(_ context.Context) ([]domain.Ingredient, error) {
	return r.items, nil
}

// fakeResolutionRepo は解決キャッシュの最小のインメモリ実装。
type fakeResolutionRepo struct {
	data  map[string]*domain.IngredientID
	saved []string
}

func (r *fakeResolutionRepo) FindByWords(
	_ context.Context, words []string,
) (map[string]*domain.IngredientID, error) {
	out := map[string]*domain.IngredientID{}
	for _, w := range words {
		if v, ok := r.data[w]; ok {
			out[w] = v
		}
	}
	return out, nil
}

func (r *fakeResolutionRepo) Save(
	_ context.Context, word string, id *domain.IngredientID,
) error {
	if r.data == nil {
		r.data = map[string]*domain.IngredientID{}
	}
	r.data[word] = id
	r.saved = append(r.saved, word)
	return nil
}

// testCatalog はテスト用の食材マスタ。
func testCatalog(t *testing.T) []domain.Ingredient {
	t.Helper()
	mk := func(name, kana string, c domain.IngredientCategory) domain.Ingredient {
		return domain.Ingredient{
			ID: domain.NewIngredientID(), Name: name, NameKana: kana, Category: c,
		}
	}
	return []domain.Ingredient{
		mk("玉ねぎ", "たまねぎ", domain.CategoryVegetable),
		mk("豚肉", "ぶたにく", domain.CategoryMeat),
		mk("卵", "たまご", domain.CategoryDairyEgg),
	}
}

func TestResolve_ExactMatchOnly(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	gw := &countingResolver{}
	svc := service.NewIngredientResolveService(
		&fakeIngredientRepo{items: items}, &fakeResolutionRepo{}, gw)

	t.Run("全語が完全一致ならGatewayを呼ばない", func(t *testing.T) {
		got, err := svc.Resolve(ctx, "玉ねぎ、卵")
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if gw.calls != 0 {
			t.Errorf("Gateway が呼ばれています: %d回", gw.calls)
		}
		if len(got.Resolved) != 2 {
			t.Fatalf("2件解決するべきです: %+v", got.Resolved)
		}
		if len(got.Unresolved) != 0 {
			t.Errorf("未解決は0件であるべきです: %v", got.Unresolved)
		}
		if got.Degraded {
			t.Error("縮退していないのに Degraded が立っています")
		}
	})

	t.Run("カタカナ表記も完全一致で解ける", func(t *testing.T) {
		got, err := svc.Resolve(ctx, "タマネギ")
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got.Resolved) != 1 || got.Resolved[0].Ingredient.Name != "玉ねぎ" {
			t.Errorf("カナ一致で解けていません: %+v", got.Resolved)
		}
	})

	t.Run("元の語をそのまま返す", func(t *testing.T) {
		got, _ := svc.Resolve(ctx, "タマネギ")
		if got.Resolved[0].Word != "タマネギ" {
			t.Errorf("利用者が書いた語を返すべきです: %q", got.Resolved[0].Word)
		}
	})

	t.Run("重複した語は1件にまとめる", func(t *testing.T) {
		got, _ := svc.Resolve(ctx, "玉ねぎ、たまねぎ")
		if len(got.Resolved) != 1 {
			t.Errorf("同じ食材は1件にまとめるべきです: %+v", got.Resolved)
		}
	})

	t.Run("空テキストはエラー", func(t *testing.T) {
		_, err := svc.Resolve(ctx, "  、 ")
		if !errors.Is(err, service.ErrEmptyResolveText) {
			t.Errorf("ErrEmptyResolveText を返すべきです: %v", err)
		}
	})
}
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/service/ -run TestResolve_ExactMatchOnly -v
```
Expected: FAIL（`undefined: service.NewIngredientResolveService`）

- [ ] **Step 3: 実装を書く（Gateway はまだ呼ばない）**

`backend/internal/service/ingredient_resolve.go` に追記:

```go
import (
	"context"
	"errors"
	"fmt"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrEmptyResolveText は解決すべき語が1つも無いことを表す（400）。
var ErrEmptyResolveText = errors.New("食材のテキストが空です")

// ResolutionRepository は入力語から食材への解決キャッシュ。
//
// 値が nil は「マスタに無いと確定済み」、キーが無いことは
// 「まだ問い合わせていない」を表す。
type ResolutionRepository interface {
	FindByWords(ctx context.Context, words []string) (map[string]*domain.IngredientID, error)
	Save(ctx context.Context, word string, id *domain.IngredientID) error
}

// ResolvedWord は1語ぶんの解決結果。
type ResolvedWord struct {
	// Word は利用者が書いた**元の語**（正規化前）。
	// 画面で「マツタケ は登録がありません」と出すとき、書いた通りの
	// 文字列の方が伝わるため。
	Word string
	// Ingredient は対応づいた食材。
	Ingredient domain.Ingredient
}

// ResolveResult はテキスト1件ぶんの解決結果。
type ResolveResult struct {
	// Resolved は食材に対応づいた語。
	Resolved []ResolvedWord
	// Unresolved はマスタに無かった語（元の語）。検索には使われない。
	Unresolved []string
	// Degraded は LLM への問い合わせをスキップしたことを表す（設計 3.6）。
	// 立っていても①②で解けた分は Resolved に入っている。
	Degraded bool
}

// IngredientResolveService は手持ちの食材テキストを食材IDに解決する。
//
// IngredientService に相乗りさせていないのは、あちらが「食材マスタそのもの」と
// 「食材からの献立検索」を担っており、こちらは外部I/Oを2つ（キャッシュ・LLM）
// 新たに抱えるため。同居させると既存機能のテストに LLM スタブが必要になる。
type IngredientResolveService struct {
	ingredients IngredientRepository
	cache       ResolutionRepository
	gateway     IngredientResolveGateway
}

// NewIngredientResolveService は IngredientResolveService を生成する。
func NewIngredientResolveService(
	ingredients IngredientRepository,
	cache ResolutionRepository,
	gw IngredientResolveGateway,
) *IngredientResolveService {
	return &IngredientResolveService{ingredients: ingredients, cache: cache, gateway: gw}
}

// resolveEntry は解決の途中経過。元の語と正規化語の対応を保つ。
type resolveEntry struct {
	original   string
	normalized string
}

// Resolve はテキストを食材に対応づける（設計 3.3）。
//
//	① 食材マスタとの完全一致
//	② 解決キャッシュ
//	③ LLM
//
// **全語が①で解ければ LLM を一度も呼ばない。**
func (s *IngredientResolveService) Resolve(
	ctx context.Context, text string,
) (ResolveResult, error) {
	entries := buildResolveEntries(text)
	if len(entries) == 0 {
		return ResolveResult{}, fmt.Errorf("%w: 1語以上を指定してください", ErrEmptyResolveText)
	}

	items, err := s.ingredients.FindAll(ctx)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("食材マスタの取得に失敗しました: %w", err)
	}
	byName := buildNameIndex(items)

	result := ResolveResult{Resolved: []ResolvedWord{}, Unresolved: []string{}}
	seen := map[domain.IngredientID]bool{}

	for _, e := range entries {
		if ing, ok := byName[e.normalized]; ok {
			// ① 完全一致。同じ食材に落ちる語が複数あっても1件にまとめる。
			if !seen[ing.ID] {
				seen[ing.ID] = true
				result.Resolved = append(result.Resolved, ResolvedWord{Word: e.original, Ingredient: ing})
			}
			continue
		}
		result.Unresolved = append(result.Unresolved, e.original)
	}
	return result, nil
}

// buildResolveEntries はテキストを分割し、正規化して空語を捨てる。
// 正規化後に同じになる語（「玉ねぎ」と「玉ねぎ 」）は1件にまとめる。
func buildResolveEntries(text string) []resolveEntry {
	words := domain.SplitIngredientWords(text)
	entries := make([]resolveEntry, 0, len(words))
	seen := map[string]bool{}
	for _, w := range words {
		n := domain.NormalizeIngredientWord(w)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		entries = append(entries, resolveEntry{original: w, normalized: n})
	}
	return entries
}

// buildNameIndex は正規化した name / name_kana の両方から食材を引ける索引を作る。
//
// **domain.NormalizeIngredientWord を通す。** 入力側と同じ関数を通さないと、
// ①で引けなかった語が②でも引けないという穴が開く（設計 3.4）。
//
// 166件で固定的なため、毎リクエスト組み直しても実測できるコストにならない。
func buildNameIndex(items []domain.Ingredient) map[string]domain.Ingredient {
	idx := make(map[string]domain.Ingredient, len(items)*2)
	for _, i := range items {
		idx[domain.NormalizeIngredientWord(i.Name)] = i
		// name が優先。同じキーになったら name 側を残す。
		if k := domain.NormalizeIngredientWord(i.NameKana); k != "" {
			if _, exists := idx[k]; !exists {
				idx[k] = i
			}
		}
	}
	return idx
}
```

> `IngredientRepository` は既存の service パッケージのインターフェース。`FindAll` を含んでいるか確認し、含まない場合は既存定義に触らず本ファイルで最小の別インターフェースを定義すること（既存を変更しない方針）。

- [ ] **Step 4: テストが通ることを確認**

```bash
cd backend && go test ./internal/service/ -run TestResolve_ExactMatchOnly -v
```
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/ingredient_resolve.go backend/internal/service/ingredient_resolve_test.go
git commit -m "feat: 食材テキストを完全一致で解決するサービスを足す"
```

---

## Task 6: 解決サービス ③LLM連携と部分成功

**Files:**
- Modify: `backend/internal/service/ingredient_resolve.go`
- Modify: `backend/internal/service/ingredient_resolve_test.go`

**Interfaces:**
- Consumes: Task 5 の `IngredientResolveService`
- Produces: 変更なし（`Resolve` の振る舞いが増えるだけ）

- [ ] **Step 1: 失敗するテストを追加する**

`ingredient_resolve_test.go` に追記:

```go
func TestResolve_Gateway(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)

	newSvc := func(gw *countingResolver, cache *fakeResolutionRepo) *service.IngredientResolveService {
		return service.NewIngredientResolveService(&fakeIngredientRepo{items: items}, cache, gw)
	}

	t.Run("未解決語だけがGatewayに渡る", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"ぶたこま": "豚肉"}}
		got, err := newSvc(gw, &fakeResolutionRepo{}).Resolve(ctx, "玉ねぎ、豚こま")
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if gw.calls != 1 {
			t.Errorf("Gateway は1回だけ呼ばれるべきです: %d回", gw.calls)
		}
		if len(got.Resolved) != 2 {
			t.Errorf("2件解決するべきです: %+v", got.Resolved)
		}
	})

	t.Run("解決結果はキャッシュに保存される", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"ぶたこま": "豚肉"}}
		cache := &fakeResolutionRepo{}
		if _, err := newSvc(gw, cache).Resolve(ctx, "豚こま"); err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(cache.saved) != 1 || cache.saved[0] != "ぶたこま" {
			t.Errorf("正規化済みの語で保存するべきです: %v", cache.saved)
		}
	})

	t.Run("未解決(該当なし)もキャッシュに保存される", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{}}
		cache := &fakeResolutionRepo{}
		got, _ := newSvc(gw, cache).Resolve(ctx, "マツタケ")
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "マツタケ" {
			t.Errorf("未解決に落ちるべきです: %+v", got)
		}
		if len(cache.saved) != 1 {
			t.Error("該当なしもキャッシュしないと毎回LLMを通ってしまいます")
		}
	})

	t.Run("マスタに無い名前が返っても未解決に落ちる", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"まつたけ": "存在しない食材"}}
		got, _ := newSvc(gw, &fakeResolutionRepo{}).Resolve(ctx, "マツタケ")
		if len(got.Unresolved) != 1 {
			t.Errorf("マスタに無い名前は未解決に落とすべきです: %+v", got)
		}
		if got.Degraded {
			t.Error("ハルシネーションは障害ではないので Degraded は立てない")
		}
	})

	t.Run("Gatewayのエラーは部分成功になる", func(t *testing.T) {
		gw := &countingResolver{err: errors.New("timeout")}
		got, err := newSvc(gw, &fakeResolutionRepo{}).Resolve(ctx, "玉ねぎ、豚こま")
		if err != nil {
			t.Fatalf("部分成功にするべきです: %v", err)
		}
		if !got.Degraded {
			t.Error("Degraded が立つべきです")
		}
		if len(got.Resolved) != 1 || got.Resolved[0].Ingredient.Name != "玉ねぎ" {
			t.Errorf("完全一致で解けた分は返すべきです: %+v", got.Resolved)
		}
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "豚こま" {
			t.Errorf("解けなかった語は未解決に入れるべきです: %+v", got.Unresolved)
		}
	})

	t.Run("Gatewayが落ちてもキャッシュには保存しない", func(t *testing.T) {
		gw := &countingResolver{err: errors.New("timeout")}
		cache := &fakeResolutionRepo{}
		if _, err := newSvc(gw, cache).Resolve(ctx, "豚こま"); err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(cache.saved) != 0 {
			t.Error("失敗した問い合わせを未解決として焼き付けてはいけません")
		}
	})
}
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/service/ -run TestResolve_Gateway -v
```
Expected: FAIL（Gateway が呼ばれない）

- [ ] **Step 3: 実装を書く**

`Resolve` の未解決語の扱いを、`Unresolved` に直行させる代わりに③へ渡す形に変える。

```go
// Resolve の①ループを次のように書き換える。
	pending := make([]resolveEntry, 0, len(entries))
	for _, e := range entries {
		if ing, ok := byName[e.normalized]; ok {
			if !seen[ing.ID] {
				seen[ing.ID] = true
				result.Resolved = append(result.Resolved, ResolvedWord{Word: e.original, Ingredient: ing})
			}
			continue
		}
		pending = append(pending, e)
	}

	if len(pending) == 0 {
		// 全語が①で解けた。LLM を呼ばずに返す。
		return result, nil
	}

	s.resolveByGateway(ctx, pending, byName, items, seen, &result)
	return result, nil
}

// resolveByGateway は未解決語を LLM に問い合わせ、結果を result に足す。
//
// **エラーは返さない。** LLM の失敗で機能全体を落とさず、①②で解けた分を
// 返すため（設計 3.6）。失敗したことは result.Degraded で伝える。
func (s *IngredientResolveService) resolveByGateway(
	ctx context.Context,
	pending []resolveEntry,
	byName map[string]domain.Ingredient,
	items []domain.Ingredient,
	seen map[domain.IngredientID]bool,
	result *ResolveResult,
) {
	words := make([]string, 0, len(pending))
	for _, e := range pending {
		words = append(words, e.normalized)
	}

	answers, err := s.gateway.Resolve(ctx, words, catalogNames(items))
	if err != nil {
		// 縮退。未解決語はすべて Unresolved に落とす。
		// **キャッシュには保存しない。** 失敗した問い合わせを
		// 「マスタに無い」として焼き付けると、復旧後も誤りが残る。
		result.Degraded = true
		for _, e := range pending {
			result.Unresolved = append(result.Unresolved, e.original)
		}
		return
	}

	byWord := make(map[string]string, len(answers))
	for _, a := range answers {
		byWord[a.Word] = a.Name
	}

	for _, e := range pending {
		name := domain.NormalizeIngredientWord(byWord[e.normalized])
		ing, ok := byName[name]
		if !ok {
			// 空文字（該当なし）と、マスタに無い名前（ハルシネーション）が
			// ここに来る。どちらも「マスタに無い」として同じ扱いにする。
			result.Unresolved = append(result.Unresolved, e.original)
			s.saveResolution(ctx, e.normalized, nil)
			continue
		}
		id := ing.ID
		s.saveResolution(ctx, e.normalized, &id)
		if seen[id] {
			continue
		}
		seen[id] = true
		result.Resolved = append(result.Resolved, ResolvedWord{Word: e.original, Ingredient: ing})
	}
}

// catalogNames は LLM に渡す食材名の一覧を作る。
// **name のみ。** カナは①の完全一致で既に効いており、③に届く語は
// カナ一致で拾えなかったものに限られるため、渡してもトークンが倍になるだけ（設計 4.2）。
func catalogNames(items []domain.Ingredient) []string {
	names := make([]string, 0, len(items))
	for _, i := range items {
		names = append(names, i.Name)
	}
	return names
}

// saveResolution はキャッシュへの保存を試みる。
// **失敗しても解決自体は成功として扱う。** キャッシュはコスト削減のための
// 仕組みであり、書けなかったことを利用者に見せる意味がない。
func (s *IngredientResolveService) saveResolution(
	ctx context.Context, word string, id *domain.IngredientID,
) {
	if err := s.cache.Save(ctx, word, id); err != nil {
		slog.WarnContext(ctx, "解決キャッシュの保存に失敗しました", "word", word, "error", err)
	}
}
```

`log/slog` の import を足すこと。

- [ ] **Step 4: テストが通ることを確認**

```bash
cd backend && go test ./internal/service/ -run 'TestResolve_' -v
```
Expected: PASS（Task 5 のテストも引き続き通ること）

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/ingredient_resolve.go backend/internal/service/ingredient_resolve_test.go
git commit -m "feat: 未解決語をLLMに問い合わせ、失敗時は部分成功で返す"
```

---

## Task 7: 解決サービス ②キャッシュ参照

**Files:**
- Modify: `backend/internal/service/ingredient_resolve.go`
- Modify: `backend/internal/service/ingredient_resolve_test.go`

**Interfaces:** 変更なし

- [ ] **Step 1: 失敗するテストを追加する**

```go
func TestResolve_Cache(t *testing.T) {
	ctx := context.Background()
	items := testCatalog(t)
	porkID := items[1].ID // 豚肉

	t.Run("キャッシュにあればGatewayを呼ばない", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"ぶたこま": "豚肉"}}
		cache := &fakeResolutionRepo{data: map[string]*domain.IngredientID{"ぶたこま": &porkID}}
		svc := service.NewIngredientResolveService(&fakeIngredientRepo{items: items}, cache, gw)

		got, err := svc.Resolve(ctx, "豚こま")
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if gw.calls != 0 {
			t.Errorf("Gateway が呼ばれています: %d回", gw.calls)
		}
		if len(got.Resolved) != 1 || got.Resolved[0].Ingredient.Name != "豚肉" {
			t.Errorf("キャッシュから解決するべきです: %+v", got.Resolved)
		}
	})

	t.Run("未解決と確定済みならGatewayを呼ばない", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{}}
		cache := &fakeResolutionRepo{data: map[string]*domain.IngredientID{"まつたけ": nil}}
		svc := service.NewIngredientResolveService(&fakeIngredientRepo{items: items}, cache, gw)

		got, _ := svc.Resolve(ctx, "マツタケ")
		if gw.calls != 0 {
			t.Errorf("該当なしと確定済みなら聞き直すべきではありません: %d回", gw.calls)
		}
		if len(got.Unresolved) != 1 {
			t.Errorf("未解決に入るべきです: %+v", got)
		}
	})

	t.Run("キャッシュに無い語だけがGatewayに渡る", func(t *testing.T) {
		gw := &countingResolver{mapping: map[string]string{"ぎゅうこま": "豚肉"}}
		cache := &fakeResolutionRepo{data: map[string]*domain.IngredientID{"ぶたこま": &porkID}}
		svc := service.NewIngredientResolveService(&fakeIngredientRepo{items: items}, cache, gw)

		if _, err := svc.Resolve(ctx, "豚こま、牛こま"); err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if gw.calls != 1 {
			t.Errorf("Gateway は1回だけ呼ばれるべきです: %d回", gw.calls)
		}
	})
}
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/service/ -run TestResolve_Cache -v
```
Expected: FAIL（Gateway が呼ばれてしまう）

- [ ] **Step 3: 実装を書く**

`Resolve` の①ループの後、③に渡す前にキャッシュを引く段を挟む。

```go
	if len(pending) == 0 {
		return result, nil
	}

	// ② 解決キャッシュ。①で解けなかった語だけを引く。
	pending = s.applyCache(ctx, pending, byName, seen, &result)
	if len(pending) == 0 {
		// キャッシュで全部片付いた。LLM を呼ばずに返す。
		return result, nil
	}

	s.resolveByGateway(ctx, pending, byName, items, seen, &result)
	return result, nil
}

// applyCache はキャッシュで解決できる語を result に足し、残りを返す。
//
// キャッシュの引きに失敗しても機能は止めない。③で聞き直すだけで、
// 余分なコストがかかるものの結果は同じになる。
func (s *IngredientResolveService) applyCache(
	ctx context.Context,
	pending []resolveEntry,
	byName map[string]domain.Ingredient,
	seen map[domain.IngredientID]bool,
	result *ResolveResult,
) []resolveEntry {
	words := make([]string, 0, len(pending))
	for _, e := range pending {
		words = append(words, e.normalized)
	}

	cached, err := s.cache.FindByWords(ctx, words)
	if err != nil {
		slog.WarnContext(ctx, "解決キャッシュの取得に失敗しました", "error", err)
		return pending
	}

	rest := make([]resolveEntry, 0, len(pending))
	for _, e := range pending {
		id, ok := cached[e.normalized]
		if !ok {
			// まだ問い合わせていない語。③へ回す。
			rest = append(rest, e)
			continue
		}
		if id == nil {
			// 「マスタに無い」と確定済み。聞き直さない。
			result.Unresolved = append(result.Unresolved, e.original)
			continue
		}
		ing, found := findByID(byName, *id)
		if !found {
			// 食材が消えた直後などに起こりうる。③へ回して聞き直す。
			rest = append(rest, e)
			continue
		}
		if !seen[ing.ID] {
			seen[ing.ID] = true
			result.Resolved = append(result.Resolved, ResolvedWord{Word: e.original, Ingredient: ing})
		}
	}
	return rest
}

// findByID は名前索引の中から食材IDで1件引く。
// 索引は名前引き用なので、IDでの逆引きはここで線形に探す（166件）。
func findByID(byName map[string]domain.Ingredient, id domain.IngredientID) (domain.Ingredient, bool) {
	for _, ing := range byName {
		if ing.ID == id {
			return ing, true
		}
	}
	return domain.Ingredient{}, false
}
```

- [ ] **Step 4: テストが通ることを確認**

```bash
cd backend && go test ./internal/service/ -run 'TestResolve_' -v
```
Expected: PASS（Task 5・6 のテストも引き続き通ること）

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/ingredient_resolve.go backend/internal/service/ingredient_resolve_test.go
git commit -m "feat: 解決キャッシュを引いてLLM呼び出しを減らす"
```

---

## Task 8: HTTPハンドラとルーティング

**Files:**
- Create: `backend/internal/handler/ingredient_resolve.go`
- Test: `backend/internal/handler/ingredient_resolve_test.go`
- Modify: `backend/cmd/server/main.go`

**Interfaces:**
- Consumes: `service.IngredientResolveService`（Task 5-7）
- Produces:
  - `type IngredientResolveHandler struct { svc IngredientResolveUseCase }`
  - `func NewIngredientResolveHandler(svc IngredientResolveUseCase) *IngredientResolveHandler`
  - `func (h *IngredientResolveHandler) RegisterRoutes(e *echo.Echo, mw ...echo.MiddlewareFunc)`
  - ルート: `POST /api/v1/ingredients/resolve`

- [ ] **Step 1: 失敗するテストを書く**

`backend/internal/handler/ingredient_resolve_test.go`:

```go
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

type stubResolveUseCase struct {
	result service.ResolveResult
	err    error
}

func (s stubResolveUseCase) Resolve(context.Context, string) (service.ResolveResult, error) {
	return s.result, s.err
}

func newResolveServer(uc handler.IngredientResolveUseCase) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = handler.ProblemErrorHandler // 既存のエラーハンドラ名に合わせること
	handler.NewIngredientResolveHandler(uc).RegisterRoutes(e)
	return e
}

func TestResolveHandler(t *testing.T) {
	ing := domain.Ingredient{
		ID: domain.NewIngredientID(), Name: "豚肉", NameKana: "ぶたにく",
		Category: domain.CategoryMeat,
	}

	t.Run("200で解決結果を返す", func(t *testing.T) {
		uc := stubResolveUseCase{result: service.ResolveResult{
			Resolved:   []service.ResolvedWord{{Word: "豚こま", Ingredient: ing}},
			Unresolved: []string{"マツタケ"},
			Degraded:   false,
		}}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingredients/resolve",
			strings.NewReader(`{"text":"豚こま、マツタケ"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		newResolveServer(uc).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("200 を期待しましたが %d でした: %s", rec.Code, rec.Body.String())
		}
		var got struct {
			Resolved []struct {
				Word       string `json:"word"`
				Ingredient struct {
					Name string `json:"name"`
				} `json:"ingredient"`
			} `json:"resolved"`
			Unresolved []string `json:"unresolved"`
			Degraded   bool     `json:"degraded"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("レスポンスを解釈できませんでした: %v", err)
		}
		if len(got.Resolved) != 1 || got.Resolved[0].Word != "豚こま" {
			t.Errorf("resolved が想定と違います: %+v", got.Resolved)
		}
		if got.Resolved[0].Ingredient.Name != "豚肉" {
			t.Errorf("食材名が違います: %q", got.Resolved[0].Ingredient.Name)
		}
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "マツタケ" {
			t.Errorf("unresolved が想定と違います: %+v", got.Unresolved)
		}
	})

	t.Run("空テキストは400", func(t *testing.T) {
		uc := stubResolveUseCase{err: service.ErrEmptyResolveText}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingredients/resolve",
			strings.NewReader(`{"text":""}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		newResolveServer(uc).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("400 を期待しましたが %d でした: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("長すぎるテキストは400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		body := `{"text":"` + strings.Repeat("あ", 201) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingredients/resolve",
			strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		newResolveServer(stubResolveUseCase{}).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("400 を期待しましたが %d でした", rec.Code)
		}
	})

	t.Run("語数が多すぎるテキストは400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		words := make([]string, 21)
		for i := range words {
			words[i] = "卵"
		}
		body := `{"text":"` + strings.Join(words, "、") + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingredients/resolve",
			strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		newResolveServer(stubResolveUseCase{}).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("400 を期待しましたが %d でした", rec.Code)
		}
	})

	t.Run("縮退していてもJSONにdegradedが出る", func(t *testing.T) {
		uc := stubResolveUseCase{result: service.ResolveResult{
			Resolved: []service.ResolvedWord{}, Unresolved: []string{"豚こま"}, Degraded: true,
		}}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingredients/resolve",
			strings.NewReader(`{"text":"豚こま"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		newResolveServer(uc).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("縮退でも200であるべきです: %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"degraded":true`) {
			t.Errorf("degraded が出ていません: %s", rec.Body.String())
		}
	})
}
```

> `handler.ProblemErrorHandler` は仮の名前。既存の `handler` パッケージで RFC 7807 のエラーハンドラを組み立てている実際の関数名を `grep -rn "HTTPErrorHandler" backend/internal/handler/` で確認して合わせること。

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/handler/ -run TestResolveHandler -v
```
Expected: FAIL（`undefined: handler.NewIngredientResolveHandler`）

- [ ] **Step 3: 実装を書く**

`backend/internal/handler/ingredient_resolve.go`:

```go
package handler

import (
	"context"
	"net/http"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// 入力の上限（設計 4.1）。冷蔵庫の中身を書く用途に対して十分に広く、
// かつ LLM に投げるトークンが青天井にならない値。
const (
	maxResolveTextLen  = 200
	maxResolveWordNum  = 20
)

// IngredientResolveUseCase は手持ちの食材テキストを食材に解決する。
type IngredientResolveUseCase interface {
	Resolve(ctx context.Context, text string) (service.ResolveResult, error)
}

// IngredientResolveHandler は食材テキスト解決のHTTP境界。
type IngredientResolveHandler struct {
	svc IngredientResolveUseCase
}

// NewIngredientResolveHandler は IngredientResolveHandler を生成する。
func NewIngredientResolveHandler(svc IngredientResolveUseCase) *IngredientResolveHandler {
	return &IngredientResolveHandler{svc: svc}
}

// RegisterRoutes は解決APIのルーティングを登録する。
//
// **既存の /menus/search-by-ingredients には手を入れない**（設計 3.8）。
// 新機能を独立したエンドポイントに閉じ込めることで、最悪これを無効化する
// だけで元の状態に戻せる。
func (h *IngredientResolveHandler) RegisterRoutes(e *echo.Echo, mw ...echo.MiddlewareFunc) {
	g := e.Group(APIBasePath, mw...)
	g.POST("/ingredients/resolve", h.Resolve)
}

// resolveRequest は POST /ingredients/resolve のリクエストボディ。
type resolveRequest struct {
	Text string `json:"text"`
}

// resolvedWordDTO は解決できた語1件。
type resolvedWordDTO struct {
	Word       string        `json:"word"`
	Ingredient ingredientDTO `json:"ingredient"`
}

// resolveResponse は解決結果。
type resolveResponse struct {
	Resolved   []resolvedWordDTO `json:"resolved"`
	Unresolved []string          `json:"unresolved"`
	Degraded   bool              `json:"degraded"`
}

// Resolve は手持ちの食材テキストを食材に対応づける。
//
//	POST /api/v1/ingredients/resolve  {"text": "豚こま、玉ねぎ、マツタケ"}
//
// 未認証でも使える（spec.md 2.9 の検索と同じ扱い）。
// **LLM が落ちても 502 にはしない。** ①完全一致・②キャッシュで解けた分を
// 200 で返し、degraded を立てる（設計 3.6）。
func (h *IngredientResolveHandler) Resolve(c echo.Context) error {
	var req resolveRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "リクエストの形式が不正です")
	}

	// 長さの検証は service に入る前に済ませる。LLM に渡す前に落とすことが
	// コスト面の一次防御になるため（設計 8章）。
	if utf8.RuneCountInString(req.Text) > maxResolveTextLen {
		return echo.NewHTTPError(http.StatusBadRequest,
			"食材のテキストが長すぎます（最大200文字）")
	}
	if len(domain.SplitIngredientWords(req.Text)) > maxResolveWordNum {
		return echo.NewHTTPError(http.StatusBadRequest,
			"食材の数が多すぎます（最大20件）")
	}

	result, err := h.svc.Resolve(c.Request().Context(), req.Text)
	if err != nil {
		return err
	}

	resolved := make([]resolvedWordDTO, 0, len(result.Resolved))
	for _, r := range result.Resolved {
		resolved = append(resolved, resolvedWordDTO{
			Word: r.Word, Ingredient: toIngredientDTO(r.Ingredient),
		})
	}
	unresolved := result.Unresolved
	if unresolved == nil {
		// 0件でも null にしない。フロントが length を見られるようにする。
		unresolved = []string{}
	}

	return c.JSON(http.StatusOK, resolveResponse{
		Resolved: resolved, Unresolved: unresolved, Degraded: result.Degraded,
	})
}
```

- [ ] **Step 4: `ErrEmptyResolveText` が 400 になるようエラーマッピングを追加**

既存のエラーハンドラ（`grep -rn "ErrInvalidIngredientIDs" backend/internal/handler/`）に倣い、`service.ErrEmptyResolveText` を 400 に対応づける。

- [ ] **Step 5: テストが通ることを確認**

```bash
cd backend && go test ./internal/handler/ -run TestResolveHandler -v
```
Expected: PASS

> **main.go への結線はこのタスクでは行わない。** `gateway.NewResolver` が
> まだ存在せずビルドが通らないため、Task 9 Step 7 でまとめて結線する。
> このタスクの完了条件は「ハンドラ単体のテストが緑」であり、
> ルートが本番サーバに生えていることは含まない。

- [ ] **Step 6: コミット**

```bash
git add backend/internal/handler/ingredient_resolve.go backend/internal/handler/ingredient_resolve_test.go
git commit -m "feat: 食材テキスト解決のエンドポイントを足す"
```

---

## Task 9: Claude Haiku 4.5 の実装とファクトリ

**Files:**
- Create: `backend/internal/gateway/resolver_claude.go`
- Create: `backend/internal/gateway/resolver_factory.go`
- Test: `backend/internal/gateway/resolver_factory_test.go`
- Modify: `backend/cmd/server/main.go`（Task 8 Step 6 をここで実行）
- Modify: `DEPLOY.md`

**Interfaces:**
- Consumes: `service.IngredientResolveGateway`（Task 4）
- Produces:
  - `func NewResolver(cfg ResolverConfig) (service.IngredientResolveGateway, error)`
  - `const ResolverProviderStub = "stub"` / `ResolverProviderClaude = "claude"`

- [ ] **Step 1: SDKのバインディングを確認する**

**推測で書かないこと。** 使うのは公式の Anthropic Go SDK。

```bash
cd backend && go get github.com/anthropics/anthropic-sdk-go
```

使うAPIは次の4つだけに絞る（いずれも公式ドキュメントに載っている範囲）。

- `anthropic.NewClient(option.WithAPIKey(key))`
- `client.Messages.New(ctx, anthropic.MessageNewParams{...})`
- `anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))`
- レスポンスの `resp.Content` を `block.AsAny().(anthropic.TextBlock)` で取り出す

**構造化出力のパラメータは使わない。** プロンプトでJSONを要求し、`encoding/json` で解釈する。設計 3.5 のとおり、返ってきた名前はマスタと突き合わせて検証するため、壊れたJSONもマスタ外の名前も「該当なし」に落ちるだけで安全に扱える。SDKのバインディングを1つでも減らす方が、バージョン差で壊れにくい。

- [ ] **Step 2: 失敗するテストを書く（ファクトリのみ）**

実LLMは叩かない。ファクトリが正しく分岐することだけを検証する。

`backend/internal/gateway/resolver_factory_test.go`:

```go
package gateway_test

import (
	"errors"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
)

func TestNewResolver(t *testing.T) {
	t.Run("stub を組み立てられる", func(t *testing.T) {
		got, err := gateway.NewResolver(gateway.ResolverConfig{Provider: "stub"})
		if err != nil {
			t.Fatalf("NewResolver が失敗しました: %v", err)
		}
		if got == nil {
			t.Fatal("nil が返りました")
		}
	})

	t.Run("大文字や空白が混じっても受ける", func(t *testing.T) {
		if _, err := gateway.NewResolver(gateway.ResolverConfig{Provider: " STUB "}); err != nil {
			t.Errorf("設定ミスではないので受けるべきです: %v", err)
		}
	})

	t.Run("claude はAPIキーが要る", func(t *testing.T) {
		_, err := gateway.NewResolver(gateway.ResolverConfig{Provider: "claude"})
		if err == nil {
			t.Error("APIキー無しはエラーにするべきです")
		}
	})

	t.Run("未知のプロバイダはエラー", func(t *testing.T) {
		_, err := gateway.NewResolver(gateway.ResolverConfig{Provider: "gpt"})
		if !errors.Is(err, gateway.ErrUnknownResolverProvider) {
			t.Errorf("ErrUnknownResolverProvider を返すべきです: %v", err)
		}
	})

	t.Run("空でも stub に既定しない", func(t *testing.T) {
		_, err := gateway.NewResolver(gateway.ResolverConfig{Provider: ""})
		if err == nil {
			t.Error("設定を忘れたまま本番が動くのを防ぐため、エラーにするべきです")
		}
	})
}
```

- [ ] **Step 3: テストが落ちることを確認**

```bash
cd backend && go test ./internal/gateway/ -run TestNewResolver -v
```
Expected: FAIL（`undefined: gateway.NewResolver`）

- [ ] **Step 4: ファクトリを書く**

`backend/internal/gateway/resolver_factory.go`:

```go
package gateway

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// ErrUnknownResolverProvider は食材解決のプロバイダ指定が解釈できないことを表す。
var ErrUnknownResolverProvider = errors.New("食材解決のプロバイダが不正です")

// 対応する食材解決プロバイダ。INGREDIENT_RESOLVER_PROVIDER に指定する値と一致する。
const (
	// ResolverProviderStub は外部APIを呼ばない（開発・CI・E2E用）。
	ResolverProviderStub = "stub"
	// ResolverProviderClaude は Claude Haiku 4.5 を使う。
	ResolverProviderClaude = "claude"
)

// ResolverConfig は食材解決ゲートウェイの設定。
// 環境変数は読まず値として受け取る（Config と同じ方針）。
type ResolverConfig struct {
	Provider string
	APIKey   string
}

// NewResolver は設定に対応する食材解決ゲートウェイを組み立てる。
//
// **Provider が空でも stub に既定しない。** 設定を忘れたまま本番が動くと、
// 表記揺れが一切吸収されない状態を誰も気付けないまま配り続けることになる
// （Config.New と同じ判断）。
func NewResolver(cfg ResolverConfig) (service.IngredientResolveGateway, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case ResolverProviderStub:
		// 対応表を持たないスタブ。すべて「該当なし」を返す。
		return NewStubResolver(nil), nil
	case ResolverProviderClaude:
		return NewClaudeResolver(cfg.APIKey)
	default:
		return nil, fmt.Errorf("%w: %q（%s または %s）",
			ErrUnknownResolverProvider, cfg.Provider,
			ResolverProviderStub, ResolverProviderClaude)
	}
}
```

- [ ] **Step 5: Claude 実装を書く**

`backend/internal/gateway/resolver_claude.go`:

```go
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// ErrMissingResolverAPIKey はAPIキーが設定されていないことを表す。
var ErrMissingResolverAPIKey = errors.New("食材解決のAPIキーが未設定です")

// claudeResolverTimeout は1回の問い合わせの上限（設計 4.3）。
// 利用者は「読み取る」を押して待っているため、長く待たせるより
// 縮退して手動チェックに促す方がよい。
const claudeResolverTimeout = 3 * time.Second

// claudeResolverModel は使用するモデル。
//
// この対応づけはフロンティアモデルの知性を必要としないため、
// 最も安い層を選ぶ。プロバイダの最終決定は eval の数字で行う（設計 3.7）。
const claudeResolverModel = "claude-haiku-4-5"

// claudeMaxTokens は応答の上限。20語 × 1件あたり十数トークンで足りる。
const claudeMaxTokens = 1024

// ClaudeResolver は Claude で未解決語を食材名に対応づける。
type ClaudeResolver struct {
	client anthropic.Client
}

// NewClaudeResolver は ClaudeResolver を生成する。
func NewClaudeResolver(apiKey string) (*ClaudeResolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrMissingResolverAPIKey
	}
	return &ClaudeResolver{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}, nil
}

// resolverAnswer は LLM に返させるJSONの1件。
type resolverAnswer struct {
	Word string `json:"word"`
	Name string `json:"name"`
}

// Resolve は未解決語を食材名に対応づける。
//
// **食材IDではなく名前をやり取りする**（設計 3.5）。UUID を渡すと
// 166件で約3000トークンに達し、1文字の取り違えが解決失敗になる。
// 名前で受ければ、マスタに無い名前が返っても呼び出し側で「該当なし」に落ちる。
func (r *ClaudeResolver) Resolve(
	ctx context.Context, words []string, catalog []string,
) ([]service.GatewayResolution, error) {
	if len(words) == 0 {
		return []service.GatewayResolution{}, nil
	}

	prompt := buildResolverPrompt(words, catalog)

	// **リトライは1回だけ**（設計 4.3）。利用者は「読み取る」を押して待っており、
	// 何度も粘るより縮退して手動チェックに促す方がよい。
	// JSON の解釈失敗はリトライしない（同じ応答が返る見込みが高く、待たせるだけ）。
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		text, err := r.callOnce(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		answers, err := parseResolverAnswers(text)
		if err != nil {
			return nil, err
		}
		out := make([]service.GatewayResolution, 0, len(answers))
		for _, a := range answers {
			out = append(out, service.GatewayResolution{Word: a.Word, Name: a.Name})
		}
		return out, nil
	}
	return nil, lastErr
}

// callOnce は1回だけ問い合わせ、本文のテキストを連結して返す。
func (r *ClaudeResolver) callOnce(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, claudeResolverTimeout)
	defer cancel()

	resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     claudeResolverModel,
		MaxTokens: claudeMaxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("食材解決の問い合わせに失敗しました: %w", err)
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(b.Text)
		}
	}
	return text.String(), nil
}

// buildResolverPrompt は問い合わせ文を組み立てる。
//
// catalog は name のみ（設計 4.2）。カナは呼び出し側の完全一致で
// 既に効いており、ここに届く語はカナ一致で拾えなかったものに限られる。
func buildResolverPrompt(words, catalog []string) string {
	var b strings.Builder
	b.WriteString("あなたは料理アプリの食材名の正規化を担当します。\n")
	b.WriteString("利用者が書いた語を、下の食材リストのいずれか1つに対応づけてください。\n\n")
	b.WriteString("規則:\n")
	b.WriteString("- 対応する食材がリストに無ければ name を空文字にしてください。\n")
	b.WriteString("- リストに無い食材名を作らないでください。\n")
	b.WriteString("- 解釈が複数あり得る場合は、家庭料理で最も一般的なものを1つ選んでください。\n")
	b.WriteString("- JSON配列だけを出力してください。説明文は不要です。\n\n")
	b.WriteString("食材リスト:\n")
	b.WriteString(strings.Join(catalog, "\n"))
	b.WriteString("\n\n対応づける語:\n")
	b.WriteString(strings.Join(words, "\n"))
	b.WriteString("\n\n出力形式:\n")
	b.WriteString(`[{"word":"入力語","name":"食材名または空文字"}]`)
	return b.String()
}

// parseResolverAnswers は応答からJSON配列を取り出す。
//
// 前後に説明文が付く場合に備え、最初の '[' から最後の ']' までを切り出す。
// 壊れていればエラーにする。呼び出し側は縮退して手動チェックに促すため、
// 誤った解決を通すより落とす方がよい。
func parseResolverAnswers(s string) ([]resolverAnswer, error) {
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end < start {
		return nil, fmt.Errorf("食材解決の応答をJSONとして解釈できませんでした")
	}

	var answers []resolverAnswer
	if err := json.Unmarshal([]byte(s[start:end+1]), &answers); err != nil {
		return nil, fmt.Errorf("食材解決の応答をJSONとして解釈できませんでした: %w", err)
	}
	return answers, nil
}
```

- [ ] **Step 6: ビルドとテストが通ることを確認**

```bash
cd backend && go mod tidy && go build ./... && go test ./internal/gateway/ -v
```
Expected: PASS

- [ ] **Step 7: main.go に結線する**

`backend/cmd/server/main.go` に追加（`ingredientHandler` の近く）:

```go
	resolutionRepo := repository.NewResolutionRepository(pool)
	resolveGateway, err := gateway.NewResolver(gateway.ResolverConfig{
		Provider: os.Getenv("INGREDIENT_RESOLVER_PROVIDER"),
		APIKey:   os.Getenv("INGREDIENT_RESOLVER_API_KEY"),
	})
	if err != nil {
		slog.Error("食材解決の設定に失敗しました", "error", err)
		os.Exit(1)
	}
	slog.Info("食材解決を設定しました", "provider", os.Getenv("INGREDIENT_RESOLVER_PROVIDER"))

	resolveSvc := service.NewIngredientResolveService(ingredientRepo, resolutionRepo, resolveGateway)
	resolveHandler := handler.NewIngredientResolveHandler(resolveSvc)
```

ルート登録（既存の `ingredientHandler.RegisterRoutes(e, searchLimit)` の近く）:

```go
	resolveHandler.RegisterRoutes(e, searchLimit)
```

`ingredientRepo` は既存の変数名。`main.go` で食材リポジトリを受けている実際の
変数名を確認して合わせること。

```bash
cd backend && go build ./... && INGREDIENT_RESOLVER_PROVIDER=stub go test ./...
```

- [ ] **Step 8: DEPLOY.md に環境変数を追記**

`DEPLOY.md` の環境変数の節に以下を足す。

- `INGREDIENT_RESOLVER_PROVIDER`: `stub` または `claude`。**空にすると起動に失敗する**（設定忘れで表記揺れ吸収が黙って死ぬのを防ぐため）。
- `INGREDIENT_RESOLVER_API_KEY`: `claude` のとき必須。

- [ ] **Step 9: コミット**

```bash
git add backend/internal/gateway/resolver_claude.go backend/internal/gateway/resolver_factory.go \
        backend/internal/gateway/resolver_factory_test.go backend/cmd/server/main.go \
        backend/go.mod backend/go.sum DEPLOY.md
git commit -m "feat: Claude Haiku 4.5 で食材の表記揺れを解決する"
```

---

## Task 10: DeepSeek の実装

**Files:**
- Create: `backend/internal/gateway/resolver_deepseek.go`
- Modify: `backend/internal/gateway/resolver_factory.go`
- Modify: `backend/internal/gateway/resolver_factory_test.go`

**Interfaces:**
- Consumes: `buildResolverPrompt` / `parseResolverAnswers`（Task 9。プロンプトと解釈は共用する）
- Produces: `func NewDeepSeekResolver(apiKey string) (*DeepSeekResolver, error)`、`const ResolverProviderDeepSeek = "deepseek"`

DeepSeek は OpenAI 互換の HTTP API を持つため、SDK を足さず `net/http` で実装する。**依存を増やさないため**であり、eval で比較して採用しなかった場合に削除しやすい。

- [ ] **Step 1: ファクトリのテストを追加**

```go
	t.Run("deepseek はAPIキーが要る", func(t *testing.T) {
		_, err := gateway.NewResolver(gateway.ResolverConfig{Provider: "deepseek"})
		if err == nil {
			t.Error("APIキー無しはエラーにするべきです")
		}
	})

	t.Run("deepseek を組み立てられる", func(t *testing.T) {
		got, err := gateway.NewResolver(gateway.ResolverConfig{
			Provider: "deepseek", APIKey: "sk-dummy",
		})
		if err != nil {
			t.Fatalf("NewResolver が失敗しました: %v", err)
		}
		if got == nil {
			t.Fatal("nil が返りました")
		}
	})
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/gateway/ -run TestNewResolver -v
```
Expected: FAIL（`deepseek` が `ErrUnknownResolverProvider` になる）

- [ ] **Step 3: 実装を書く**

`backend/internal/gateway/resolver_deepseek.go`:

```go
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// deepSeekEndpoint は OpenAI 互換のチャット補完エンドポイント。
const deepSeekEndpoint = "https://api.deepseek.com/chat/completions"

// deepSeekModel は使用するモデル。最も安い層を選ぶ（設計 3.7）。
const deepSeekModel = "deepseek-chat"

// DeepSeekResolver は DeepSeek で未解決語を食材名に対応づける。
//
// **公式SDKを足さず net/http で実装する。** eval で比較して採用しなかった
// 場合に、依存ごと削除できるようにするため。
type DeepSeekResolver struct {
	apiKey string
	client *http.Client
}

// NewDeepSeekResolver は DeepSeekResolver を生成する。
func NewDeepSeekResolver(apiKey string) (*DeepSeekResolver, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrMissingResolverAPIKey
	}
	return &DeepSeekResolver{
		apiKey: apiKey,
		client: &http.Client{Timeout: claudeResolverTimeout},
	}, nil
}

// deepSeekRequest はチャット補完のリクエスト。
type deepSeekRequest struct {
	Model    string              `json:"model"`
	Messages []deepSeekMessage   `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// deepSeekResponse は必要な部分だけを取り出す。
type deepSeekResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Resolve は未解決語を食材名に対応づける。
// プロンプトと応答の解釈は Claude 実装と共用する（比較を公平にするため）。
func (r *DeepSeekResolver) Resolve(
	ctx context.Context, words []string, catalog []string,
) ([]service.GatewayResolution, error) {
	if len(words) == 0 {
		return []service.GatewayResolution{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, claudeResolverTimeout)
	defer cancel()

	body, err := json.Marshal(deepSeekRequest{
		Model:     deepSeekModel,
		MaxTokens: claudeMaxTokens,
		Messages: []deepSeekMessage{
			{Role: "user", Content: buildResolverPrompt(words, catalog)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("食材解決のリクエストを組み立てられませんでした: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepSeekEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("食材解決のリクエストを組み立てられませんでした: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("食材解決の問い合わせに失敗しました: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("食材解決の問い合わせが失敗しました: status=%d", resp.StatusCode)
	}

	var parsed deepSeekResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("食材解決の応答を解釈できませんでした: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("食材解決の応答が空でした")
	}

	answers, err := parseResolverAnswers(parsed.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}

	out := make([]service.GatewayResolution, 0, len(answers))
	for _, a := range answers {
		out = append(out, service.GatewayResolution{Word: a.Word, Name: a.Name})
	}
	return out, nil
}
```

> import に `time` は要らない。`claudeResolverTimeout` は `resolver_claude.go` 側で
> `time.Duration` として定義済みで、ここではその値を使うだけ。

`resolver_factory.go` に分岐を追加:

```go
	// ResolverProviderDeepSeek は DeepSeek V4 Flash を使う。
	ResolverProviderDeepSeek = "deepseek"
```

```go
	case ResolverProviderDeepSeek:
		return NewDeepSeekResolver(cfg.APIKey)
```

エラーメッセージの候補一覧にも `deepseek` を足す。

- [ ] **Step 4: テストが通ることを確認**

```bash
cd backend && go build ./... && go test ./internal/gateway/ -v
```
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add backend/internal/gateway/resolver_deepseek.go backend/internal/gateway/resolver_factory.go backend/internal/gateway/resolver_factory_test.go
git commit -m "feat: DeepSeek の食材解決実装を足す（evalでの比較用）"
```

---

## Task 11: eval ハーネス

**Files:**
- Create: `backend/internal/gateway/testdata/ingredient_resolution_cases.json`
- Create: `backend/internal/gateway/resolver_eval_test.go`（`//go:build eval`）

**Interfaces:**
- Consumes: `NewResolver`（Task 9-10）
- Produces: なし（テストのみ）

- [ ] **Step 1: 正解データを作る**

`backend/internal/gateway/testdata/ingredient_resolution_cases.json`。**50件以上**を手で作る。内訳は次のバランスにする。

| 種別 | 件数の目安 | 例 |
| --- | --- | --- |
| 略語 | 15 | 豚こま → 豚肉、鶏むね → 鶏むね肉 |
| 漢字/かな/カナの揺れ | 15 | 人参 → にんじん、ジャガイモ → じゃがいも |
| マスタ外 | 10 | マツタケ → null、パクチー → null |
| 曖昧語 | 5 | ねぎ → 長ねぎ |
| 完全一致（対照群） | 5 | 玉ねぎ → 玉ねぎ |

```json
[
  { "input": "豚こま",   "expected": "豚肉" },
  { "input": "ブタこま", "expected": "豚肉" },
  { "input": "豚小間",   "expected": "豚肉" },
  { "input": "人参",     "expected": "にんじん" },
  { "input": "ジャガイモ", "expected": "じゃがいも" },
  { "input": "マツタケ", "expected": null },
  { "input": "パクチー", "expected": null },
  { "input": "ねぎ",     "expected": "長ねぎ" },
  { "input": "玉ねぎ",   "expected": "玉ねぎ" }
]
```

> `expected` はシード済みの食材マスタに実在する `name` にすること。`SELECT name FROM ingredients ORDER BY name_kana;` で確認しながら作る。

- [ ] **Step 2: eval を書く**

`backend/internal/gateway/resolver_eval_test.go`:

```go
//go:build eval

// 実LLMを叩く評価。**CIでは走らせない**（APIキーと課金が絡むため）。
//
//	INGREDIENT_RESOLVER_PROVIDER=claude \
//	INGREDIENT_RESOLVER_API_KEY=sk-... \
//	go test -tags=eval ./internal/gateway/ -run TestResolverEval -v
package gateway_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
)

type evalCase struct {
	Input    string  `json:"input"`
	Expected *string `json:"expected"`
}

func TestResolverEval(t *testing.T) {
	raw, err := os.ReadFile("testdata/ingredient_resolution_cases.json")
	if err != nil {
		t.Fatalf("正解データを読めませんでした: %v", err)
	}
	var cases []evalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("正解データを解釈できませんでした: %v", err)
	}

	resolver, err := gateway.NewResolver(gateway.ResolverConfig{
		Provider: os.Getenv("INGREDIENT_RESOLVER_PROVIDER"),
		APIKey:   os.Getenv("INGREDIENT_RESOLVER_API_KEY"),
	})
	if err != nil {
		t.Fatalf("Resolver を組み立てられませんでした: %v", err)
	}

	catalog := loadCatalogNames(t)

	words := make([]string, 0, len(cases))
	for _, c := range cases {
		words = append(words, c.Input)
	}

	start := time.Now()
	answers, err := resolver.Resolve(context.Background(), words, catalog)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Resolve が失敗しました: %v", err)
	}

	got := make(map[string]string, len(answers))
	for _, a := range answers {
		got[a.Word] = a.Name
	}

	correct := 0
	for _, c := range cases {
		want := ""
		if c.Expected != nil {
			want = *c.Expected
		}
		if got[c.Input] == want {
			correct++
			continue
		}
		t.Logf("誤答: input=%q want=%q got=%q", c.Input, want, got[c.Input])
	}

	rate := float64(correct) / float64(len(cases)) * 100
	t.Logf("=== eval 結果 ===")
	t.Logf("プロバイダ : %s", os.Getenv("INGREDIENT_RESOLVER_PROVIDER"))
	t.Logf("件数       : %d", len(cases))
	t.Logf("正解       : %d", correct)
	t.Logf("正解率     : %.1f%%", rate)
	t.Logf("所要時間   : %v", elapsed)
}

// loadCatalogNames は食材マスタの name を返す。
// DBに繋がず、シードSQLから読むか、固定のリストを testdata に置く。
func loadCatalogNames(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("testdata/ingredient_catalog.json")
	if err != nil {
		t.Fatalf("食材リストを読めませんでした: %v", err)
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		t.Fatalf("食材リストを解釈できませんでした: %v", err)
	}
	return names
}
```

- [ ] **Step 3: 食材リストを testdata に書き出す**

```bash
cd backend && psql "$DATABASE_URL" -t -A -c \
  "SELECT json_agg(name ORDER BY name_kana) FROM ingredients" \
  > internal/gateway/testdata/ingredient_catalog.json
```

- [ ] **Step 4: 通常のテストで eval が走らないことを確認**

```bash
cd backend && go test ./internal/gateway/ -v | grep -c TestResolverEval
```
Expected: `0`（ビルドタグで除外されている）

- [ ] **Step 5: 両プロバイダで実行して記録する**

```bash
cd backend
INGREDIENT_RESOLVER_PROVIDER=claude   INGREDIENT_RESOLVER_API_KEY=$ANTHROPIC_KEY go test -tags=eval ./internal/gateway/ -run TestResolverEval -v
INGREDIENT_RESOLVER_PROVIDER=deepseek INGREDIENT_RESOLVER_API_KEY=$DEEPSEEK_KEY  go test -tags=eval ./internal/gateway/ -run TestResolverEval -v
```

結果（正解率・所要時間・誤答の傾向）を設計書 3.7 と 10章に追記し、**本番のデフォルトプロバイダを決める**。

- [ ] **Step 6: コミット**

```bash
git add backend/internal/gateway/resolver_eval_test.go backend/internal/gateway/testdata/
git commit -m "test: 食材解決の eval ハーネスと正解データを足す"
```

---

## Task 12: フロントエンドのAPIクライアントと型

**Files:**
- Modify: `frontend/src/api/types.ts`
- Modify: `frontend/src/features/menu/api.ts`
- Test: `frontend/src/features/menu/api.test.ts`（既存が無ければ作る）

**Interfaces:**
- Consumes: `POST /api/v1/ingredients/resolve`（Task 8）
- Produces:
  - `type ResolvedWord = { word: string; ingredient: Ingredient }`
  - `type ResolveResult = { resolved: ResolvedWord[]; unresolved: string[]; degraded: boolean }`
  - `async function resolveIngredients(text: string): Promise<ResolveResult>`

- [ ] **Step 1: 型を足す**

`frontend/src/api/types.ts`:

```ts
/** ResolvedWord は自由記述から対応づいた食材1件。 */
export type ResolvedWord = {
  /** 利用者が書いた元の語。正規化前の文字列。 */
  word: string
  ingredient: Ingredient
}

/** ResolveResult は自由記述の解決結果（spec.md 2.9 / 設計 4.1）。 */
export type ResolveResult = {
  resolved: ResolvedWord[]
  /** マスタに無かった語。検索には使われない。 */
  unresolved: string[]
  /** LLM への問い合わせをスキップしたか。立っていても resolved は使える。 */
  degraded: boolean
}
```

- [ ] **Step 2: 失敗するテストを書く**

`frontend/src/features/menu/api.test.ts`:

```ts
import { describe, expect, it, vi, afterEach } from 'vitest'

import { resolveIngredients } from './api'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('resolveIngredients', () => {
  it('テキストを送って解決結果を返す', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          resolved: [
            {
              word: '豚こま',
              ingredient: {
                id: 'i1',
                name: '豚肉',
                nameKana: 'ぶたにく',
                category: 'meat',
              },
            },
          ],
          unresolved: ['マツタケ'],
          degraded: false,
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    )

    const got = await resolveIngredients('豚こま、マツタケ')

    expect(got.resolved).toHaveLength(1)
    expect(got.resolved[0].ingredient.name).toBe('豚肉')
    expect(got.unresolved).toEqual(['マツタケ'])
    expect(got.degraded).toBe(false)
    expect(fetchSpy).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 3: テストが落ちることを確認**

```bash
cd frontend && npx vitest run src/features/menu/api.test.ts
```
Expected: FAIL（`resolveIngredients` が無い）

- [ ] **Step 4: 実装を書く**

`frontend/src/features/menu/api.ts` に追記（import に `ResolveResult` を足す）:

```ts
/**
 * resolveIngredients は自由記述の食材テキストを食材に対応づける（設計 4.1）。
 *
 * 解決できなかった語は unresolved に入る。**LLM が落ちても 200 が返り**、
 * degraded が立つ。呼び出し側は resolved をチェック状態に反映しつつ、
 * degraded なら「一部だけ読み取れました」と伝える。
 */
export async function resolveIngredients(text: string): Promise<ResolveResult> {
  return apiPost<ResolveResult>('/ingredients/resolve', { text })
}
```

- [ ] **Step 5: テストが通ることを確認**

```bash
cd frontend && npx vitest run src/features/menu/api.test.ts && npx tsc --noEmit
```
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add frontend/src/api/types.ts frontend/src/features/menu/api.ts frontend/src/features/menu/api.test.ts
git commit -m "feat: 食材テキスト解決のAPIクライアントを足す"
```

---

## Task 13: フロントエンドUI

**Files:**
- Create: `frontend/src/features/menu/IngredientTextInput.tsx`
- Create: `frontend/src/features/menu/ResolveResultPanel.tsx`
- Modify: `frontend/src/features/menu/SearchByIngredientsPage.tsx`
- Modify: `frontend/src/features/menu/SearchByIngredientsPage.test.tsx`

**Interfaces:**
- Consumes: `resolveIngredients`（Task 12）
- Produces: なし（画面のみ）

**`IngredientPicker.tsx` の props とロジックは変更しない**（設計 3.8）。`selected` を親が更新するだけでチェックが入る。

- [ ] **Step 1: 失敗するテストを書く**

`SearchByIngredientsPage.test.tsx` に追記:

```tsx
it('読み取るとピッカーにチェックが入る', async () => {
  vi.spyOn(api, 'resolveIngredients').mockResolvedValue({
    resolved: [
      { word: '豚こま', ingredient: { id: 'meat-1', name: '豚肉', nameKana: 'ぶたにく', category: 'meat' } },
    ],
    unresolved: [],
    degraded: false,
  })

  renderPage()
  await screen.findByText('野菜')

  await userEvent.type(screen.getByLabelText('冷蔵庫にあるものを書く'), '豚こま')
  await userEvent.click(screen.getByRole('button', { name: '読み取る' }))

  expect(await screen.findByText('1個を選択中')).toBeInTheDocument()
})

it('未解決の語を明示する', async () => {
  vi.spyOn(api, 'resolveIngredients').mockResolvedValue({
    resolved: [],
    unresolved: ['マツタケ', 'パクチー'],
    degraded: false,
  })

  renderPage()
  await screen.findByText('野菜')
  await userEvent.type(screen.getByLabelText('冷蔵庫にあるものを書く'), 'マツタケ、パクチー')
  await userEvent.click(screen.getByRole('button', { name: '読み取る' }))

  expect(await screen.findByText(/登録がありませんでした/)).toBeInTheDocument()
  expect(screen.getByText(/マツタケ/)).toBeInTheDocument()
  expect(screen.getByText(/パクチー/)).toBeInTheDocument()
})

it('縮退したことを伝える', async () => {
  vi.spyOn(api, 'resolveIngredients').mockResolvedValue({
    resolved: [], unresolved: ['豚こま'], degraded: true,
  })

  renderPage()
  await screen.findByText('野菜')
  await userEvent.type(screen.getByLabelText('冷蔵庫にあるものを書く'), '豚こま')
  await userEvent.click(screen.getByRole('button', { name: '読み取る' }))

  expect(await screen.findByText(/一部だけ読み取れました/)).toBeInTheDocument()
})
```

> `renderPage` と `api` の import は既存テストの書き方に合わせること。

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd frontend && npx vitest run src/features/menu/SearchByIngredientsPage.test.tsx
```
Expected: FAIL（入力欄が無い）

- [ ] **Step 3: 入力コンポーネントを書く**

`frontend/src/features/menu/IngredientTextInput.tsx`:

```tsx
type Props = {
  value: string
  onChange: (value: string) => void
  onSubmit: () => void
  isPending: boolean
}

// IngredientTextInput は冷蔵庫の中身を自由記述で受け取る（設計 6.1）。
//
// **チェックボックスを置き換えない。** 読み取り結果は IngredientPicker の
// チェック状態として反映され、利用者が目で確認・修正してから検索する。
// これにより確認の場が既存UIで賄え、LLM が落ちても手動チェックで機能が残る。
export function IngredientTextInput({ value, onChange, onSubmit, isPending }: Props) {
  return (
    <div className="space-y-3">
      <label
        htmlFor="ingredient-text"
        className="block font-medium text-kon-ink"
      >
        冷蔵庫にあるものを書く
      </label>
      <textarea
        id="ingredient-text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={3}
        maxLength={200}
        placeholder="豚こま、玉ねぎ、にんじん、卵"
        className="w-full rounded-2xl border border-kon-leaf-soft bg-white p-4 text-kon-ink placeholder:text-kon-ink/40 focus:border-kon-leaf focus:outline-none"
      />
      <button
        type="button"
        onClick={onSubmit}
        // 空のまま押しても 400 になるだけなので押させない。
        disabled={value.trim() === '' || isPending}
        className="rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95 disabled:cursor-not-allowed disabled:bg-kon-leaf-soft disabled:text-kon-ink/70"
      >
        {isPending ? '読み取っています…' : '読み取る'}
      </button>
    </div>
  )
}
```

- [ ] **Step 4: 結果表示コンポーネントを書く**

`frontend/src/features/menu/ResolveResultPanel.tsx`:

```tsx
type Props = {
  unresolved: string[]
  degraded: boolean
}

// ResolveResultPanel は読み取りの結果のうち、チェックに現れないものを伝える。
//
// **ピッカーの上に置く。** IngredientPicker は max-h-[55vh] のスクロール領域を
// 持つため、入ったチェックが画面外になりうる。変化に気付ける位置に出す（設計 6.2）。
export function ResolveResultPanel({ unresolved, degraded }: Props) {
  if (unresolved.length === 0 && !degraded) return null

  return (
    <div className="space-y-2" role="status">
      {degraded && (
        <p className="rounded-2xl bg-kon-cream px-5 py-3 text-sm text-kon-ink/80">
          一部だけ読み取れました。残りは下から選んでください。
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

- [ ] **Step 5: ページに組み込む**

`SearchByIngredientsPage.tsx` を変更する。

```tsx
  const [text, setText] = useState('')
  const [unresolved, setUnresolved] = useState<string[]>([])
  const [degraded, setDegraded] = useState(false)

  const resolve = useMutation({
    mutationFn: () => resolveIngredients(text),
    onSuccess: (result) => {
      // **マージする（置き換えない）。** すでに手で入れたチェックを消さない。
      setSelected((prev) => {
        const next = new Set(prev)
        for (const r of result.resolved) next.add(r.ingredient.id)
        return next
      })
      setUnresolved(result.unresolved)
      setDegraded(result.degraded)
      // 食材が変わったので前の検索結果は消す（既存の toggle と同じ理由）。
      search.reset()
    },
  })
```

JSX は `IngredientPicker` の**上**に差し込む。

```tsx
      {ingredients && (
        <>
          <IngredientTextInput
            value={text}
            onChange={setText}
            onSubmit={() => resolve.mutate()}
            isPending={resolve.isPending}
          />

          {resolve.error && <ErrorMessage error={resolve.error} />}
          <ResolveResultPanel unresolved={unresolved} degraded={degraded} />

          <p className="text-sm text-kon-ink/60">または下から選ぶ</p>

          <IngredientPicker … />
```

`clear()` でも読み取り結果を消す。

```tsx
  function clear() {
    setSelected(new Set())
    setUnresolved([])
    setDegraded(false)
    search.reset()
  }
```

- [ ] **Step 6: テストが通ることを確認**

```bash
cd frontend && npx vitest run src/features/menu/ && npx tsc --noEmit && npm run lint
```
Expected: PASS

- [ ] **Step 7: コミット**

```bash
git add frontend/src/features/menu/IngredientTextInput.tsx \
        frontend/src/features/menu/ResolveResultPanel.tsx \
        frontend/src/features/menu/SearchByIngredientsPage.tsx \
        frontend/src/features/menu/SearchByIngredientsPage.test.tsx
git commit -m "feat: 冷蔵庫の中身を自由記述で入力できるようにする"
```

---

## Task 14: 仕様と既存コメントの更新

**Files:**
- Modify: `spec.md`（2.9 と 5.6）
- Modify: `frontend/src/features/menu/IngredientPicker.tsx`（コメントのみ）
- Modify: `api/openapi.yaml`
- Modify: `frontend/src/api/schema.d.ts`

**Interfaces:** なし

- [ ] **Step 1: `IngredientPicker.tsx` のコメントを直す**

現在の記述はこの機能を否定している。**削除ではなく上書き**して、判断が覆った経緯を残す。

```tsx
// IngredientPicker は手持ちの食材をカテゴリ別に選ぶ（spec.md 2.9）。
//
// **かつては「自由入力にしない」としていた**（表記揺れを自前で吸収する必要があり、
// 一致しなかったとき利用者に理由を説明できないため）。この制約は
// `POST /ingredients/resolve`（LLM による解決、設計 2026-08-02）が引き受けた。
//
// 現在このコンポーネントは**解決結果の確認と修正**を担う。読み取った結果が
// チェック状態として入り、利用者が目で見て直せる。LLM が落ちたときは
// ここだけで完結して食材を選べる（フォールバック）。
```

- [ ] **Step 2: `spec.md` 2.9 を更新**

「入力は自由記述にしない」の引用ブロックを、経緯を残した形に書き換える。

```markdown
> **入力は当初「自由記述にしない」としていた。** 食材は自前マスタ（14.1）なので、
> 「にんじん／人参」のような表記揺れをこちらで吸収する必要が出て、
> 一致しなかったときに利用者へ理由を説明できないためだった。
>
> **2026-08-02、この制約は解除した。** `POST /ingredients/resolve` が
> ①完全一致 → ②解決キャッシュ → ③LLM の3段で解決し、対応づかなかった語は
> 明示して見せる。一覧から選ぶ形は残り、読み取り結果の確認・修正の場になる
> （設計 `2026-08-02-ingredient-text-resolve-design.md`）。
```

2.9 の箇条書きにも1行足す。

```markdown
- **自由記述でも入力できる**（`POST /ingredients/resolve`）。解決結果は一覧のチェックに反映され、
  対応づかなかった語は明示する。LLM が落ちても一覧からの選択は使える
```

- [ ] **Step 3: `spec.md` 5.6 の表にエンドポイントを足す**

```markdown
| POST | `/ingredients/resolve` | 不要 | 自由記述の食材テキストを食材IDに解決する |
```

レスポンス例と、`degraded` の意味、400 の条件（200文字 / 20語）を追記する。

- [ ] **Step 4: OpenAPI と生成型を更新**

```bash
# api/openapi.yaml に POST /ingredients/resolve を追加したのち
cd frontend && npm run generate:api   # 既存のスクリプト名を package.json で確認すること
npx tsc --noEmit
```

- [ ] **Step 5: 全テストを通す**

```bash
cd backend && go test ./...
cd ../frontend && npx vitest run && npx tsc --noEmit && npm run lint
```
Expected: すべて PASS

- [ ] **Step 6: コミット**

```bash
git add spec.md api/openapi.yaml frontend/src/api/schema.d.ts frontend/src/features/menu/IngredientPicker.tsx
git commit -m "docs: 自由記述の解禁を spec.md と既存コメントに反映する"
```

---

## Task 15: 未解決キャッシュを消す運用コマンド

**Files:**
- Modify: `backend/internal/repository/ingredient_resolution.go`
- Modify: `backend/internal/repository/ingredient_resolution_test.go`
- Create: `backend/cmd/resolutions/main.go`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `ResolutionRepository`（Task 3）
- Produces: `func (r *ResolutionRepository) DeleteUnresolved(ctx context.Context) (int64, error)`

食材マスタに新しい食材を足すと、**過去に「マスタに無い」と保存した語が解決可能になる**（設計 5章）。TTL を持たない設計なので、この掃除は手で行う。

- [ ] **Step 1: 失敗するテストを追加する**

`ingredient_resolution_test.go` に追記:

```go
	t.Run("未解決の行だけを消せる", func(t *testing.T) {
		if err := repo.Save(ctx, "けすたいしょう", nil); err != nil {
			t.Fatalf("Save が失敗しました: %v", err)
		}
		if err := repo.Save(ctx, "のこすたいしょう", &id); err != nil {
			t.Fatalf("Save が失敗しました: %v", err)
		}

		n, err := repo.DeleteUnresolved(ctx)
		if err != nil {
			t.Fatalf("DeleteUnresolved が失敗しました: %v", err)
		}
		if n < 1 {
			t.Errorf("1件以上消えるべきです: %d", n)
		}

		got, _ := repo.FindByWords(ctx, []string{"けすたいしょう", "のこすたいしょう"})
		if _, ok := got["けすたいしょう"]; ok {
			t.Error("未解決の行が残っています")
		}
		if _, ok := got["のこすたいしょう"]; !ok {
			t.Error("解決済みの行まで消えています")
		}
	})
```

- [ ] **Step 2: テストが落ちることを確認**

```bash
cd backend && go test ./internal/repository/ -run TestResolutionRepository -v
```
Expected: FAIL（`repo.DeleteUnresolved undefined`）

- [ ] **Step 3: リポジトリにメソッドを足す**

```go
// DeleteUnresolved は「マスタに無い」と保存された行だけを消す。
//
// 食材マスタに新しい食材を足すと、過去に未解決として保存した語が
// 解決可能になる（設計 5章）。TTL を持たない設計なので、
// マスタ更新のたびにこれを流して聞き直させる。
//
// **解決済みの行は消さない。** 食材の別名は変わらないため、
// 消すと LLM への問い合わせが無駄に増えるだけになる。
func (r *ResolutionRepository) DeleteUnresolved(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM ingredient_resolutions WHERE ingredient_id IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("未解決キャッシュの削除に失敗しました: %w", err)
	}
	return tag.RowsAffected(), nil
}
```

- [ ] **Step 4: テストが通ることを確認**

```bash
cd backend && go test ./internal/repository/ -run TestResolutionRepository -v
```
Expected: PASS

- [ ] **Step 5: CLIを書く**

`backend/cmd/resolutions/main.go`:

```go
// Command resolutions は食材の解決キャッシュを運用するためのコマンド。
//
//	go run ./cmd/resolutions purge-unresolved
//
// 食材マスタを更新したあとに流す。「マスタに無い」と保存された語を消し、
// 次回のアクセスで LLM に聞き直させる（設計 9章）。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "purge-unresolved" {
		fmt.Fprintln(os.Stderr, "usage: resolutions purge-unresolved")
		os.Exit(2)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("DBに接続できませんでした", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	n, err := repository.NewResolutionRepository(pool).DeleteUnresolved(ctx)
	if err != nil {
		slog.Error("未解決キャッシュの削除に失敗しました", "error", err)
		os.Exit(1)
	}
	slog.Info("未解決キャッシュを削除しました", "deleted", n)
}
```

- [ ] **Step 6: Makefile にターゲットを足す**

既存の `grant` / `revoke` と同じ書き方に合わせる。

```makefile
## purge-unresolved: 食材マスタ更新後に、未解決の解決キャッシュを消す
purge-unresolved:
	cd backend && go run ./cmd/resolutions purge-unresolved
```

- [ ] **Step 7: 動作を確認**

```bash
cd backend && go build ./... && go run ./cmd/resolutions 2>&1 | head -2
```
Expected: usage が出て終了コード2

- [ ] **Step 8: DEPLOY.md に運用手順を追記**

「食材マスタ（シード）を更新したら `make purge-unresolved` を流す」ことと、その理由（新しい食材が足されると過去の未解決が解決可能になる）を書く。

- [ ] **Step 9: コミット**

```bash
git add backend/internal/repository/ingredient_resolution.go \
        backend/internal/repository/ingredient_resolution_test.go \
        backend/cmd/resolutions/main.go Makefile DEPLOY.md
git commit -m "feat: 未解決の解決キャッシュを消す運用コマンドを足す"
```

---

## 完了条件

- [ ] `backend`: `go test ./...` が緑
- [ ] `frontend`: `npx vitest run` / `npx tsc --noEmit` / `npm run lint` が緑
- [ ] `INGREDIENT_RESOLVER_PROVIDER=stub` で全機能が動く（APIキー無しで開発できる）
- [ ] `make purge-unresolved` が動く（食材マスタ更新後の運用手段）
- [ ] eval を両プロバイダで実行し、結果を設計書 3.7・10章に追記して**本番デフォルトを決定**
- [ ] DeepSeek を採用する場合は**プライバシーポリシーの改定**を別タスクとして起票
- [ ] `/pre-pr-review` を通してから作業PRを出す（base は `feature/ingredient-text-resolve`）

## デプロイ手順（feature → main のマージ時）

```
1. ローカルから本番 Neon に migrate up を流す
   DATABASE_URL=<Neon> go run ./cmd/migrate up
2. Cloud Run に環境変数を設定
   INGREDIENT_RESOLVER_PROVIDER / INGREDIENT_RESOLVER_API_KEY
3. main にマージ（自動デプロイ）
4. seed は不要（ingredient_resolutions は空で始まる）
```

**順序を守ること。** 逆順にすると、デプロイ後の一瞬だけテーブルが無く `/ingredients/resolve` が 500 を返す。
