# 冷蔵庫検索の「作れるものだけ」と「最大限使う」実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 冷蔵庫検索に「この中だけで作れるものに絞る」と「手持ちを多く使う順に並べる」の2つのつまみを足し、絞った結果が0件のときは「あと1品で作れる」候補を別枠で示す。

**Architecture:** 既存の `POST /menus/search-by-ingredients` に省略可能な2フィールド（`onlyMakeable` / `sort`）を足す。新エンドポイントもマイグレーションも作らない。不足0の判定に要る「候補献立の全食材」は `buildMatches` が既に引いているので、絞り込みは service 層の述語1つで済み、SQL とリポジトリのインターフェースは無変更。

**Tech Stack:** Go 1.25 / echo / testify、React 19 / TypeScript / TanStack Query / Vitest + Testing Library + MSW、Playwright、OpenAPI + openapi-typescript

## Global Constraints

- 仕様は `spec.md` 2.9 / 5.6。設計は `docs/superpowers/specs/2026-08-07-fridge-only-search-design.md`
- **マイグレーションなし。** `ingredients` / `menu_ingredients` のまま
- **リポジトリのインターフェースと SQL は変更しない**
- **`onlyMakeable` / `sort` を省略したときの挙動は現行と完全に同一**（後方互換）
- `sort` は `missing_asc`（既定）と `matched_desc` の2値。**未知の値は 400**（既定に丸めない）
- `nearMisses` は **`onlyMakeable: true` かつ `matches` が0件のときだけ**埋める。それ以外は常に空配列
- 上限は `matches` / `nearMisses` それぞれ20件。**切り詰めは必ず並べ替えの後**
- 配列は0件のとき `null` ではなく `[]`
- 未認証でも使える（現行の扱いを保つ）。履歴には記録しない
- テスト名は既存に倣い日本語（`func TestSearchByIngredients_不足0だけが返る`）
- テスト実行: `make test-backend` / `make test-frontend`、E2E は `make test-e2e`
- **全タスクを1ブランチ `feature/fridge-only-search` で実装し、最後にPRを1本出す**
  （2026-08-07 決定）。13-C の事故は「ブランチを積んだ」ことが原因なので、
  1本にまとめれば起きない。**タスクの途中でブランチを切り替えたりPRを出したりしない**
- 作業ツリーは `.claude/worktrees/fridge-only-search`

---

## File Structure

| ファイル | 変更 | 責務 |
| --- | --- | --- |
| `backend/internal/service/ingredient.go` | 変更 | 入力型・結果型・絞り込み・並び順2種・`nearMisses` |
| `backend/internal/service/ingredient_search_test.go` | 変更 | 既存8箇所の呼び出し更新＋新規テスト |
| `backend/internal/handler/shopping_list.go` | 変更 | リクエスト/レスポンスDTO・`sort` の検証・use case の型 |
| `backend/internal/handler/ingredient_search_test.go` | 変更 | 新規テスト |
| `backend/internal/handler/ingredient_catalog_test.go` | 変更 | `fakeIngredientCatalog` の署名更新 |
| `api/openapi.yaml` | 変更 | リクエストの2フィールドと `nearMisses` |
| `frontend/src/api/schema.d.ts` | 再生成 | `make gen-api` の出力。手で書かない |
| `frontend/src/features/menu/api.ts` | 変更 | `searchByIngredients` の引数と戻り値 |
| `frontend/src/features/menu/SearchByIngredientsPage.tsx` | 変更 | ラジオ2組・0件表示・`nearMisses` の別枠 |
| `frontend/src/features/menu/SearchByIngredientsPage.test.tsx` | 変更 | 新規テスト |
| `frontend/e2e/from-fridge.spec.ts` | 変更 | 切り替えで候補が減ることの確認 |
| `README.md` | 変更 | 機能の記述 |

`IngredientPicker.tsx` は**無変更**。

---

## Task 1: service に入力型・結果型・並び順の切り替えを入れる

**Files:**
- Modify: `backend/internal/service/ingredient.go`
- Test: `backend/internal/service/ingredient_search_test.go`

**Interfaces:**
- Consumes: 既存の `buildMatches` / `sortMatches` / `maxIngredientSearchResults` / `MenuMatch`
- Produces:
  - `type MatchSort string`、定数 `SortMissingAsc MatchSort = "missing_asc"` / `SortMatchedDesc MatchSort = "matched_desc"`
  - `type SearchByIngredientsInput struct { IngredientIDs []domain.IngredientID; OnlyMakeable bool; Sort MatchSort }`
  - `type SearchByIngredientsResult struct { Matches []MenuMatch; NearMisses []MenuMatch }`
  - `func (s *IngredientService) SearchByIngredients(ctx context.Context, in SearchByIngredientsInput) (SearchByIngredientsResult, error)`

> **`MatchSort` のゼロ値 `""` は `SortMissingAsc` と同じ扱いにする。** これにより
> `SearchByIngredientsInput{IngredientIDs: ids}` だけで現行の挙動になり、
> 後方互換のテストが自然に書ける。未知の値の拒否は handler の仕事（Task 3）。

- [ ] **Step 1: 既存テストの呼び出しを新しい署名に直す**

`backend/internal/service/ingredient_search_test.go` の8箇所を機械的に置き換える。
戻り値がスライスから構造体になるため `got` → `got.Matches` も併せて直す。

```go
// 変更前
got, err := svc.SearchByIngredients(context.Background(),
    []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID})
require.NoError(t, err)
assert.Equal(t, []string{"オニオンスープ", "肉じゃが", "ポテトサラダ"}, matchNames(got))

// 変更後
got, err := svc.SearchByIngredients(context.Background(),
    service.SearchByIngredientsInput{
        IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID},
    })
require.NoError(t, err)
assert.Equal(t, []string{"オニオンスープ", "肉じゃが", "ポテトサラダ"}, matchNames(got.Matches))
```

エラーを確認しているテスト（0件指定・存在しないID）は `got` を使っていないので
`_, err := ...` のまま引数だけ直す。

- [ ] **Step 2: 新しいテストを追加する**

同じファイルの末尾に足す。`searchFixture` の献立は
肉じゃが（一致2/不足1）・ポテトサラダ（一致1/不足1）・オニオンスープ（一致1/**不足0**）・麻婆豆腐（重ならない）。

```go
func TestSearchByIngredients_省略時は現行と同じ結果(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)
	ids := []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID}

	// ゼロ値の Input が、つまみを足す前の挙動と一致することを固定する。
	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{IngredientIDs: ids})
	require.NoError(t, err)

	assert.Equal(t, []string{"オニオンスープ", "肉じゃが", "ポテトサラダ"}, matchNames(got.Matches))
	assert.Empty(t, got.NearMisses)
}

func TestSearchByIngredients_作れるものだけに絞る(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID},
			OnlyMakeable:  true,
		})
	require.NoError(t, err)

	// 不足1の肉じゃが・ポテトサラダは落ちる。
	assert.Equal(t, []string{"オニオンスープ"}, matchNames(got.Matches))
	for _, m := range got.Matches {
		assert.Empty(t, m.Missing, "不足のある献立が混ざっている: %s", m.Menu.Name)
	}
}

func TestSearchByIngredients_手持ちを多く使う順(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID},
			Sort:          service.SortMatchedDesc,
		})
	require.NoError(t, err)

	// 一致数は 肉じゃが2 > ポテトサラダ1 = オニオンスープ1。
	// 同数どうしは不足の少ない方（オニオンスープ 不足0）が先。
	assert.Equal(t, []string{"肉じゃが", "オニオンスープ", "ポテトサラダ"}, matchNames(got.Matches))
}
```

- [ ] **Step 3: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/service/ -run TestSearchByIngredients -v`
Expected: コンパイルエラー（`SearchByIngredientsInput` / `SortMatchedDesc` が未定義）

- [ ] **Step 4: 型と並び順を実装する**

`backend/internal/service/ingredient.go` の `MenuMatch` 定義の下に足す。

```go
// MatchSort は候補の並び順（spec.md 5.6）。
//
// ゼロ値（"")は SortMissingAsc と同じ扱いにする。これにより
// つまみを指定しない呼び出しが現行と同じ挙動になる。
// 未知の値の拒否は handler が担う（値をここまで持ち込ませない）。
type MatchSort string

const (
	// SortMissingAsc は「不足の少ない順 → 一致の多い順 → カナ順」。既定。
	// 知りたいのが「いま作れるか」だから第1が不足になる。
	SortMissingAsc MatchSort = "missing_asc"

	// SortMatchedDesc は「一致の多い順 → 不足の少ない順 → カナ順」。
	// 手持ちを使い切りたい（買い足しを減らし、余らせたくない）とき。
	SortMatchedDesc MatchSort = "matched_desc"
)

// SearchByIngredientsInput は食材からの検索条件。
//
// 引数を並べず構造体にしているのは、ids に真偽値と列挙が続くと
// 呼び出し側でどれが何か読めなくなるため。
type SearchByIngredientsInput struct {
	// IngredientIDs は手持ちの食材。1件以上。重複は1件として扱う。
	IngredientIDs []domain.IngredientID
	// OnlyMakeable は true のとき、不足のある献立を返さない。
	OnlyMakeable bool
	// Sort は並び順。ゼロ値は SortMissingAsc。
	Sort MatchSort
}

// SearchByIngredientsResult は食材からの検索結果。
type SearchByIngredientsResult struct {
	// Matches は条件に合った候補。
	Matches []MenuMatch
	// NearMisses は「あと1品買えば作れる」候補。
	// OnlyMakeable かつ Matches が0件のときだけ埋まる。それ以外は空。
	NearMisses []MenuMatch
}
```

既存の `sortMatches` を並び順で分岐する形に置き換える。

```go
// sortMatches は候補を指定された並び順にする（spec.md 5.6）。
//
// カナ順は同値のときに並びを安定させるためだけのもの。
func sortMatches(matches []MenuMatch, by MatchSort) {
	if by == SortMatchedDesc {
		sort.SliceStable(matches, func(a, b int) bool {
			if ma, mb := len(matches[a].Matched), len(matches[b].Matched); ma != mb {
				return ma > mb
			}
			if la, lb := len(matches[a].Missing), len(matches[b].Missing); la != lb {
				return la < lb
			}
			return matches[a].Menu.NameKana < matches[b].Menu.NameKana
		})
		return
	}

	// 既定（SortMissingAsc とゼロ値）。第1が不足なのは、
	// 知りたいのが「いま作れるか」であって「手持ちを何品使うか」ではないため。
	sort.SliceStable(matches, func(a, b int) bool {
		if la, lb := len(matches[a].Missing), len(matches[b].Missing); la != lb {
			return la < lb
		}
		if ma, mb := len(matches[a].Matched), len(matches[b].Matched); ma != mb {
			return ma > mb
		}
		return matches[a].Menu.NameKana < matches[b].Menu.NameKana
	})
}
```

`SearchByIngredients` の署名と後半を差し替える。前半（重複除去・存在確認・
`FindMenuIDsByIngredientIDs` / `FindByMenuIDs` / `FindByIDs`）は**そのまま**で、
`ids` の参照だけ `in.IngredientIDs` に変える。

```go
// SearchByIngredients は手持ちの食材で作れる献立を探す。
//
// 既定では完全一致に絞らない。献立1件の食材は平均4.4種で、それを全部持っている
// 状況はまれなため（spec.md 2.9）。in.OnlyMakeable が true のときだけ不足0に絞る。
func (s *IngredientService) SearchByIngredients(
	ctx context.Context, in SearchByIngredientsInput,
) (SearchByIngredientsResult, error) {
	empty := SearchByIngredientsResult{Matches: []MenuMatch{}, NearMisses: []MenuMatch{}}

	have := uniqueIngredientIDs(in.IngredientIDs)
	if len(have) == 0 {
		return empty, fmt.Errorf("%w: 1件以上指定してください", ErrInvalidIngredientIDs)
	}

	// （中略：存在確認・menuIDs・pairs・menus の取得は現行のまま。
	//   エラー時の戻り値だけ empty に、0件時は empty, nil にする）

	all := buildMatches(menus, pairs, have)

	matches := all
	if in.OnlyMakeable {
		matches = withMissingAtMost(all, 0)
	}
	sortMatches(matches, in.Sort)
	matches = truncateMatches(matches)

	return SearchByIngredientsResult{Matches: matches, NearMisses: []MenuMatch{}}, nil
}

// withMissingAtMost は不足が n 件以下の候補だけを新しいスライスで返す。
//
// 元のスライスと backing array を共有しないよう append で作り直す。
// 共有すると、返した側を並べ替えたときに元の並びまで崩れる。
func withMissingAtMost(matches []MenuMatch, n int) []MenuMatch {
	out := make([]MenuMatch, 0, len(matches))
	for _, m := range matches {
		if len(m.Missing) <= n {
			out = append(out, m)
		}
	}
	return out
}

// truncateMatches は上限（20件）で打ち切る。
//
// **必ず並べ替えの後に呼ぶ。** 手持ち50種なら不足0だけで132件出るため、
// 切ってから並べると上位を取りこぼす。
func truncateMatches(matches []MenuMatch) []MenuMatch {
	if len(matches) > maxIngredientSearchResults {
		return matches[:maxIngredientSearchResults]
	}
	return matches
}
```

- [ ] **Step 5: テストが通ることを確認する**

Run: `make test-backend`
Expected: PASS（service の既存18本＋新規3本）

- [ ] **Step 6: コミット**

```bash
git add backend/internal/service/ingredient.go backend/internal/service/ingredient_search_test.go
git commit -m "feat: 食材検索に絞り込みと並び順の指定を足す"
```

> この時点では handler がまだ旧署名を呼んでいるためビルドが壊れる。
> **Task 2 と Task 3 まで通してから push する**（CI に赤を出さない）。

---

## Task 2: 0件のときの `nearMisses` を実装する

**Files:**
- Modify: `backend/internal/service/ingredient.go`
- Test: `backend/internal/service/ingredient_search_test.go`

**Interfaces:**
- Consumes: Task 1 の `SearchByIngredientsInput` / `SearchByIngredientsResult` / `withMissingAtMost` / `truncateMatches` / `sortMatches`
- Produces: `SearchByIngredientsResult.NearMisses` が埋まる条件

- [ ] **Step 1: 失敗するテストを書く**

`searchFixture` は手持ちに「牛肉」を含めなければ不足0が出ない構成にできる。
「じゃがいも」だけを手持ちにすると、オニオンスープは候補から外れ（重ならない）、
ポテトサラダが一致1/不足1、肉じゃがが一致1/不足2になる。

```go
func TestSearchByIngredients_作れるものが0件ならあと1品を返す(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	// じゃがいもだけ。不足0の献立は存在しない。
	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["じゃがいも"].ID},
			OnlyMakeable:  true,
		})
	require.NoError(t, err)

	assert.Empty(t, got.Matches)
	// ポテトサラダは不足1（きゅうり）。肉じゃがは不足2（玉ねぎ・牛肉）なので入らない。
	assert.Equal(t, []string{"ポテトサラダ"}, matchNames(got.NearMisses))
}

func TestSearchByIngredients_あと1品は不足ちょうど1件だけ(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["じゃがいも"].ID},
			OnlyMakeable:  true,
		})
	require.NoError(t, err)

	for _, m := range got.NearMisses {
		assert.Len(t, m.Missing, 1, "不足が1件でない献立が混ざっている: %s", m.Menu.Name)
	}
}

func TestSearchByIngredients_候補があるならあと1品は返さない(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	// 玉ねぎ＋じゃがいもならオニオンスープが不足0で見つかる。
	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["玉ねぎ"].ID, ing["じゃがいも"].ID},
			OnlyMakeable:  true,
		})
	require.NoError(t, err)

	require.NotEmpty(t, got.Matches)
	assert.Empty(t, got.NearMisses, "候補があるのに あと1品 を返している")
}

func TestSearchByIngredients_絞っていなければあと1品は常に空(t *testing.T) {
	t.Parallel()

	menus, ings, ing, _ := searchFixture()
	svc := newSearchService(menus, ings)

	// OnlyMakeable でなければ、たとえ結果が0件でも nearMisses は埋めない。
	got, err := svc.SearchByIngredients(context.Background(),
		service.SearchByIngredientsInput{
			IngredientIDs: []domain.IngredientID{ing["じゃがいも"].ID},
		})
	require.NoError(t, err)

	assert.NotEmpty(t, got.Matches)
	assert.Empty(t, got.NearMisses)
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/service/ -run 'TestSearchByIngredients_(作れるものが0件|あと1品|候補があるなら|絞っていなければ)' -v`
Expected: FAIL（`got.NearMisses` が常に空のため、最初の2本が落ちる）

- [ ] **Step 3: 実装する**

`SearchByIngredients` の末尾を差し替える。

```go
	all := buildMatches(menus, pairs, have)

	matches := all
	if in.OnlyMakeable {
		matches = withMissingAtMost(all, 0)
	}
	sortMatches(matches, in.Sort)
	matches = truncateMatches(matches)

	// 行き止まりを防ぐ。ただし条件を勝手に外して再検索はしない
	// （「この中以外は出さない」という指定に反する）。
	// 0件だと明言した上で、別枠として「あと1品」を添えるだけにする。
	nearMisses := []MenuMatch{}
	if in.OnlyMakeable && len(matches) == 0 {
		// 不足0は上で0件だと分かっているので、1件以下＝ちょうど1件になる。
		nearMisses = withMissingAtMost(all, 1)
		// 不足はどれも1件で並ばないため、一致の多い順に固定する。
		sortMatches(nearMisses, SortMatchedDesc)
		nearMisses = truncateMatches(nearMisses)
	}

	return SearchByIngredientsResult{Matches: matches, NearMisses: nearMisses}, nil
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `make test-backend`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add backend/internal/service/ingredient.go backend/internal/service/ingredient_search_test.go
git commit -m "feat: 作れる献立が0件のとき あと1品 の候補を返す"
```

---

## Task 3: handler にフィールドと検証を足す

**Files:**
- Modify: `backend/internal/handler/shopping_list.go:129-208`
- Test: `backend/internal/handler/ingredient_search_test.go`
- Test: `backend/internal/handler/ingredient_catalog_test.go:35-41`（fake の署名）

**Interfaces:**
- Consumes: Task 1/2 の `service.SearchByIngredientsInput` / `SearchByIngredientsResult` / `MatchSort`
- Produces: `IngredientCatalogUseCase.SearchByIngredients(ctx, service.SearchByIngredientsInput) (service.SearchByIngredientsResult, error)`

- [ ] **Step 1: fake と既存テストを新しい署名に直す**

`backend/internal/handler/ingredient_catalog_test.go` の `fakeIngredientCatalog`。

```go
type fakeIngredientCatalog struct {
	items       []domain.Ingredient
	matches     []service.MenuMatch
	nearMisses  []service.MenuMatch
	lastInput   service.SearchByIngredientsInput
	lastIDs     []domain.IngredientID
	err         error
	calls       int
	searchCalls int
}

func (s *fakeIngredientCatalog) SearchByIngredients(
	_ context.Context, in service.SearchByIngredientsInput,
) (service.SearchByIngredientsResult, error) {
	s.searchCalls++
	s.lastInput = in
	s.lastIDs = in.IngredientIDs
	return service.SearchByIngredientsResult{
		Matches: s.matches, NearMisses: s.nearMisses,
	}, s.err
}
```

`lastIDs` を残すのは既存テストがそれを見ているため。

- [ ] **Step 2: 新しいテストを書く**

`backend/internal/handler/ingredient_search_test.go` の末尾に足す。

```go
func TestSearchByIngredients_つまみが service に渡る(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{}
	e := catalogApp(t, catalog)
	id := domain.NewIngredientID().String()

	rec := postSearchByIngredients(t, e,
		`{"ingredientIds":["`+id+`"],"onlyMakeable":true,"sort":"matched_desc"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, catalog.lastInput.OnlyMakeable)
	assert.Equal(t, service.SortMatchedDesc, catalog.lastInput.Sort)
}

func TestSearchByIngredients_省略時は既定値が渡る(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{}
	e := catalogApp(t, catalog)
	id := domain.NewIngredientID().String()

	rec := postSearchByIngredients(t, e, `{"ingredientIds":["`+id+`"]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, catalog.lastInput.OnlyMakeable)
	assert.Equal(t, service.SortMissingAsc, catalog.lastInput.Sort)
}

func TestSearchByIngredients_未知の並び順で400(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{}
	e := catalogApp(t, catalog)
	id := domain.NewIngredientID().String()

	// 既定に丸めない。利用者の指定を黙って読み替えると、
	// 違う条件の結果を正しい答えとして返すことになる。
	rec := postSearchByIngredients(t, e,
		`{"ingredientIds":["`+id+`"],"sort":"newest"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, catalog.searchCalls, "検証前に service を呼んでいる")
}

func TestSearchByIngredients_空文字の並び順で400(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{}
	e := catalogApp(t, catalog)
	id := domain.NewIngredientID().String()

	// 指定した以上は2値のどちらかでなければならない。
	// 「省略」とは区別する（省略は既定、空文字は誤り）。
	rec := postSearchByIngredients(t, e,
		`{"ingredientIds":["`+id+`"],"sort":""}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, catalog.searchCalls)
}

func TestSearchByIngredients_onlyMakeableが真偽値でなければ400(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{}
	e := catalogApp(t, catalog)
	id := domain.NewIngredientID().String()

	rec := postSearchByIngredients(t, e,
		`{"ingredientIds":["`+id+`"],"onlyMakeable":"yes"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, catalog.searchCalls)
}

func TestSearchByIngredients_あと1品が本文に出る(t *testing.T) {
	t.Parallel()

	near := service.MenuMatch{
		Menu: domain.Menu{
			ID: domain.NewMenuID(), Name: "肉じゃが", NameKana: "にくじゃが",
			Genre: domain.GenreJapanese, Difficulty: domain.DifficultyEasy, Role: domain.RoleMain,
		},
		Matched: []domain.Ingredient{},
		Missing: []domain.Ingredient{},
	}
	catalog := &fakeIngredientCatalog{nearMisses: []service.MenuMatch{near}}
	e := catalogApp(t, catalog)
	id := domain.NewIngredientID().String()

	rec := postSearchByIngredients(t, e,
		`{"ingredientIds":["`+id+`"],"onlyMakeable":true}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"nearMisses"`)
	assert.Contains(t, rec.Body.String(), `"肉じゃが"`)
}

func TestSearchByIngredients_あと1品は0件でも空配列(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{}
	e := catalogApp(t, catalog)
	id := domain.NewIngredientID().String()

	rec := postSearchByIngredients(t, e, `{"ingredientIds":["`+id+`"]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	// null にすると画面側で length を見る前に落ちる。
	assert.Contains(t, rec.Body.String(), `"nearMisses":[]`)
}
```

- [ ] **Step 3: テストが失敗することを確認する**

Run: `cd backend && go test ./internal/handler/ -run TestSearchByIngredients -v`
Expected: コンパイルエラー、その後 400 を期待する4本が 200 で落ちる

- [ ] **Step 4: 実装する**

`backend/internal/handler/shopping_list.go` を変更する。

```go
type IngredientCatalogUseCase interface {
	All(ctx context.Context) ([]domain.Ingredient, error)
	SearchByIngredients(
		ctx context.Context, in service.SearchByIngredientsInput,
	) (service.SearchByIngredientsResult, error)
}

// searchByIngredientsRequest は POST /menus/search-by-ingredients のリクエストボディ。
//
// sort をポインタで受けるのは「省略」と「空文字の指定」を区別するため。
// 前者は既定、後者は誤りとして 400 にする。
type searchByIngredientsRequest struct {
	IngredientIDs []string `json:"ingredientIds"`
	OnlyMakeable  bool     `json:"onlyMakeable"`
	Sort          *string  `json:"sort"`
}

// searchByIngredientsResponse は候補の一覧。
//
// nearMisses は「あと1品買えば作れる」候補。onlyMakeable で0件だったときだけ
// 埋まる。常に配列で返す（null にすると画面側で length を見る前に落ちる）。
type searchByIngredientsResponse struct {
	Matches    []menuMatchDTO `json:"matches"`
	NearMisses []menuMatchDTO `json:"nearMisses"`
}

// parseMatchSort は並び順を解釈する。省略なら既定、未知の値は 400。
//
// 既定に丸めない。利用者の指定を黙って読み替えると、
// 違う条件で検索した結果を正しい答えとして返すことになる。
func parseMatchSort(raw *string) (service.MatchSort, error) {
	if raw == nil {
		return service.SortMissingAsc, nil
	}
	switch service.MatchSort(*raw) {
	case service.SortMissingAsc, service.SortMatchedDesc:
		return service.MatchSort(*raw), nil
	default:
		return "", echo.NewHTTPError(http.StatusBadRequest,
			"sort は missing_asc か matched_desc を指定してください")
	}
}

func (h *IngredientHandler) SearchByIngredients(c echo.Context) error {
	var req searchByIngredientsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "リクエストの形式が不正です")
	}

	sortBy, err := parseMatchSort(req.Sort)
	if err != nil {
		return err
	}

	ids := make([]domain.IngredientID, 0, len(req.IngredientIDs))
	for _, raw := range req.IngredientIDs {
		id, err := domain.ParseIngredientID(raw)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}

	res, err := h.catalog.SearchByIngredients(c.Request().Context(),
		service.SearchByIngredientsInput{
			IngredientIDs: ids,
			OnlyMakeable:  req.OnlyMakeable,
			Sort:          sortBy,
		})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, searchByIngredientsResponse{
		Matches:    toMenuMatchDTOs(res.Matches),
		NearMisses: toMenuMatchDTOs(res.NearMisses),
	})
}

// toMenuMatchDTOs は候補の並びをDTOに写す。0件でも null にしない。
func toMenuMatchDTOs(matches []service.MenuMatch) []menuMatchDTO {
	out := make([]menuMatchDTO, 0, len(matches))
	for _, m := range matches {
		out = append(out, menuMatchDTO{
			Menu:    toMenuDTO(m.Menu),
			Matched: toIngredientDTOs(m.Matched),
			Missing: toIngredientDTOs(m.Missing),
		})
	}
	return out
}
```

> `onlyMakeable` に文字列を渡したときは echo の `Bind` が失敗するので、
> 既存の 400 経路がそのまま効く。追加の検証は要らない。

- [ ] **Step 5: テストが通ることを確認する**

Run: `make test-backend`
Expected: PASS（backend 全体）

- [ ] **Step 6: Lint とコミット**

```bash
make lint
git add backend/internal/handler/
git commit -m "feat: 食材検索APIに onlyMakeable と sort を足す"
```

---

## Task 4: OpenAPI を更新して型を再生成する

**Files:**
- Modify: `api/openapi.yaml:220-260`（リクエスト）、`api/openapi.yaml:1493-1500`（`MenuMatchesResponse`）
- Regenerate: `frontend/src/api/schema.d.ts`

**Interfaces:**
- Consumes: Task 3 の JSON 形
- Produces: TS 型 `Schemas['MenuMatchesResponse']` に `nearMisses` が入る

- [ ] **Step 1: リクエストの定義を更新する**

`api/openapi.yaml` の `/api/v1/menus/search-by-ingredients` の `requestBody`。

```yaml
              properties:
                ingredientIds:
                  type: array
                  description: 手持ちの食材。1件以上。重複は1件として扱う。
                  minItems: 1
                  items:
                    type: string
                    format: uuid
                onlyMakeable:
                  type: boolean
                  default: false
                  description: |
                    true のとき、不足のある献立を返さない（この中だけで作れるもの）。
                    省略時は false で、従来どおり不足を許す。
                sort:
                  type: string
                  enum: [missing_asc, matched_desc]
                  default: missing_asc
                  description: |
                    missing_asc … 不足の少ない順 → 一致の多い順 → カナ順（既定）
                    matched_desc … 一致の多い順 → 不足の少ない順 → カナ順
                    未知の値は 400。既定に丸めない。
```

`description` の本文にも追記する。

```yaml
      description: |
        選んだ食材を1つでも使う献立を、当てはまり具合とともに返す（spec.md 2.9 / 5.6）。

        既定では完全一致に絞らない。献立1件の食材は平均4.4種で、それを全部持っている
        状況はまれなため。各候補に不足食材を示し「あと2品買えば作れる」が分かるようにする。

        `onlyMakeable: true` のときは不足0の献立だけを返す。手持ちを20種ほど
        選べば30件前後が該当する。0件だった場合に限り、`nearMisses` に
        「あと1品買えば作れる」候補を添える。

        並びは `sort` で選ぶ。上位20件まで（`matches` / `nearMisses` それぞれ）。
        手持ちと1つも重ならない献立は返さない。未認証でも使える。
```

- [ ] **Step 2: レスポンスの定義を更新する**

```yaml
    MenuMatchesResponse:
      type: object
      required: [matches, nearMisses]
      properties:
        matches:
          type: array
          items:
            $ref: '#/components/schemas/MenuMatch'
        nearMisses:
          type: array
          description: |
            あと1品買えば作れる候補。onlyMakeable が true で matches が
            0件のときだけ埋まる。それ以外は常に空配列。
          items:
            $ref: '#/components/schemas/MenuMatch'
```

- [ ] **Step 3: 型を再生成する**

Run: `make gen-api`
Expected: `frontend/src/api/schema.d.ts` に `nearMisses` が現れる

- [ ] **Step 4: 生成物が最新であることを確認する**

Run: `cd frontend && npx tsc -b`
Expected: PASS（`api.ts` はまだ `matches` しか読んでいないので型エラーは出ない）

- [ ] **Step 5: コミット**

```bash
git add api/openapi.yaml frontend/src/api/schema.d.ts
git commit -m "docs: 食材検索APIの OpenAPI に onlyMakeable / sort / nearMisses を足す"
```

> ここで API 側は完成する。**PRはまだ出さない**（全タスク終了後に1本）。

---

## Task 5: 画面につまみを足す

**Files:**
- Modify: `frontend/src/features/menu/api.ts:112-125`
- Modify: `frontend/src/features/menu/SearchByIngredientsPage.tsx`
- Test: `frontend/src/features/menu/SearchByIngredientsPage.test.tsx`

**Interfaces:**
- Consumes: Task 4 の `Schemas['MenuMatchesResponse']`
- Produces:
  - `type MatchSort = 'missing_asc' | 'matched_desc'`
  - `type SearchOptions = { onlyMakeable: boolean; sort: MatchSort }`
  - `searchByIngredients(ingredientIds: string[], options: SearchOptions): Promise<{ matches: MenuMatch[]; nearMisses: MenuMatch[] }>`

> ブランチはそのまま（`feature/fridge-only-search`）。切り替えない。

- [ ] **Step 1: 失敗するテストを書く**

`frontend/src/features/menu/SearchByIngredientsPage.test.tsx` に足す。
既存テストの MSW ハンドラは `matches` だけを返しているので、
**`nearMisses: []` を足さないと型が合わない**。既存ハンドラも併せて直す。

```tsx
it('作れるものだけを選ぶと並び順の選択肢が消える', async () => {
  renderPage()
  await screen.findByLabelText('玉ねぎ')

  // 既定では並び順を選べる。
  expect(screen.getByRole('group', { name: '並び順' })).toBeInTheDocument()

  await userEvent.click(screen.getByLabelText('この中だけで作れるもの'))

  // 不足が全件0になるので、並び順は結果に影響しない。出しても選ばせる意味がない。
  expect(screen.queryByRole('group', { name: '並び順' })).not.toBeInTheDocument()
})

it('つまみを送信に載せる', async () => {
  let sent: unknown = null
  server.use(
    http.post('/api/v1/menus/search-by-ingredients', async ({ request }) => {
      sent = await request.json()
      return HttpResponse.json({ matches: [], nearMisses: [] })
    }),
  )

  renderPage()
  await screen.findByLabelText('玉ねぎ')
  await userEvent.click(screen.getByLabelText('玉ねぎ'))
  await userEvent.click(screen.getByLabelText('手持ちを多く使う順'))
  await userEvent.click(screen.getByRole('button', { name: 'この食材で探す' }))

  await waitFor(() => {
    expect(sent).toMatchObject({ onlyMakeable: false, sort: 'matched_desc' })
  })
})

it('つまみを変えると前の結果が消える', async () => {
  renderPage()
  await screen.findByLabelText('玉ねぎ')
  await userEvent.click(screen.getByLabelText('玉ねぎ'))
  await userEvent.click(screen.getByRole('button', { name: 'この食材で探す' }))

  expect(await screen.findByText(/作れそうな献立/)).toBeInTheDocument()

  // 残したままだと、いまの条件の結果だと誤解させる（食材の選び直しと同じ理屈）。
  await userEvent.click(screen.getByLabelText('この中だけで作れるもの'))

  expect(screen.queryByText(/作れそうな献立/)).not.toBeInTheDocument()
})
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd frontend && npm test -- SearchByIngredientsPage`
Expected: FAIL（`この中だけで作れるもの` のラベルが見つからない）

- [ ] **Step 3: API クライアントを更新する**

`frontend/src/features/menu/api.ts`。

```ts
/** MatchSort は候補の並び順（spec.md 5.6）。 */
export type MatchSort = 'missing_asc' | 'matched_desc'

/** SearchOptions は冷蔵庫検索のつまみ。 */
export type SearchOptions = {
  /** true のとき、不足のある献立を出さない。 */
  onlyMakeable: boolean
  sort: MatchSort
}

/** SearchByIngredientsResult は候補と「あと1品」の候補。 */
export type SearchByIngredientsResult = {
  matches: MenuMatch[]
  /** onlyMakeable で0件だったときだけ埋まる。それ以外は空配列。 */
  nearMisses: MenuMatch[]
}

/**
 * searchByIngredients は手持ちの食材で作れる献立を探す。
 *
 * 既定では完全一致に絞らず、各候補に不足食材が付く（spec.md 2.9）。
 * onlyMakeable で不足0だけに絞れる。上位20件まで。
 */
export async function searchByIngredients(
  ingredientIds: string[],
  options: SearchOptions,
): Promise<SearchByIngredientsResult> {
  const res = await apiPost<SearchByIngredientsResult>(
    '/menus/search-by-ingredients',
    { ingredientIds, ...options },
  )
  return res
}
```

- [ ] **Step 4: 画面につまみを足す**

`SearchByIngredientsPage.tsx`。`useState` を2つ増やし、`toggle` / `clear` と
同じく**つまみの変更でも `search.reset()` を呼ぶ**。

```tsx
const [onlyMakeable, setOnlyMakeable] = useState(false)
const [sort, setSort] = useState<MatchSort>('missing_asc')

const search = useMutation({
  mutationFn: () => searchByIngredients([...selected], { onlyMakeable, sort }),
})

// つまみを変えたら前の結果は消す。食材を選び直したときと同じ理由で、
// 残したままだと、いまの条件の結果だと誤解させる。
function changeOnlyMakeable(next: boolean) {
  setOnlyMakeable(next)
  search.reset()
}

function changeSort(next: MatchSort) {
  setSort(next)
  search.reset()
}
```

`IngredientPicker` と「探す」ボタンの間にラジオを2組置く。

```tsx
{/* 各組を fieldset/legend で囲む。囲まないと支援技術もテストも
    どちらの組の選択肢か区別できない（8-D と同じ）。 */}
<fieldset className="space-y-2">
  <legend className="text-sm text-kon-ink/60">探し方</legend>
  <label className="flex items-center gap-2 text-sm text-kon-ink">
    <input
      type="radio"
      name="makeable"
      checked={!onlyMakeable}
      onChange={() => changeOnlyMakeable(false)}
    />
    足りないものがあってもよい
  </label>
  <label className="flex items-center gap-2 text-sm text-kon-ink">
    <input
      type="radio"
      name="makeable"
      checked={onlyMakeable}
      onChange={() => changeOnlyMakeable(true)}
    />
    この中だけで作れるもの
  </label>
</fieldset>

{/* 「作れるものだけ」は不足が全件0になるため、並び順は結果を変えない。
    出しても選ばせる意味がないので隠す。サーバ側では特別扱いしない。 */}
{!onlyMakeable && (
  <fieldset className="space-y-2">
    <legend className="text-sm text-kon-ink/60">並び順</legend>
    <label className="flex items-center gap-2 text-sm text-kon-ink">
      <input
        type="radio"
        name="sort"
        checked={sort === 'missing_asc'}
        onChange={() => changeSort('missing_asc')}
      />
      買い足しが少ない順
    </label>
    <label className="flex items-center gap-2 text-sm text-kon-ink">
      <input
        type="radio"
        name="sort"
        checked={sort === 'matched_desc'}
        onChange={() => changeSort('matched_desc')}
      />
      手持ちを多く使う順
    </label>
  </fieldset>
)}
```

`search.data` は構造体になったので、描画を `<Matches matches={search.data.matches} />` に直す。

- [ ] **Step 5: テストが通ることを確認する**

Run: `make test-frontend`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add frontend/src/features/menu/api.ts frontend/src/features/menu/SearchByIngredientsPage.tsx frontend/src/features/menu/SearchByIngredientsPage.test.tsx
git commit -m "feat: 冷蔵庫検索の画面に探し方と並び順のつまみを足す"
```

---

## Task 6: 0件のときの表示と「あと1品」を出す

**Files:**
- Modify: `frontend/src/features/menu/SearchByIngredientsPage.tsx`
- Test: `frontend/src/features/menu/SearchByIngredientsPage.test.tsx`

**Interfaces:**
- Consumes: Task 5 の `SearchByIngredientsResult`

- [ ] **Step 1: 失敗するテストを書く**

```tsx
it('作れるものが0件なら、その旨とあと1品の候補を別の見出しで出す', async () => {
  server.use(
    http.post('/api/v1/menus/search-by-ingredients', () =>
      HttpResponse.json({
        matches: [],
        nearMisses: [
          {
            menu: {
              id: 'm1', name: '肉じゃが', genre: 'japanese',
              difficulty: 'easy', role: 'main', description: '',
            },
            matched: [{ id: 'i1', name: 'じゃがいも', nameKana: 'じゃがいも', category: 'vegetable' }],
            missing: [{ id: 'i2', name: '牛肉', nameKana: 'ぎゅうにく', category: 'meat' }],
          },
        ],
      }),
    ),
  )

  renderPage()
  await screen.findByLabelText('玉ねぎ')
  await userEvent.click(screen.getByLabelText('玉ねぎ'))
  await userEvent.click(screen.getByLabelText('この中だけで作れるもの'))
  await userEvent.click(screen.getByRole('button', { name: 'この食材で探す' }))

  // 0件であることを先に明言する。約束を破っていないことを示すため。
  expect(
    await screen.findByText(/この中だけで作れる献立はありませんでした/),
  ).toBeInTheDocument()

  // 地続きに見せると約束を破ったように見えるので、見出しを分ける。
  expect(
    screen.getByRole('heading', { name: /あと1品買えば作れます/ }),
  ).toBeInTheDocument()
  expect(screen.getByLabelText('肉じゃが')).toBeInTheDocument()
})

it('作れるものだけの結果にも調味料の断りを出す', async () => {
  server.use(
    http.post('/api/v1/menus/search-by-ingredients', () =>
      HttpResponse.json({
        matches: [
          {
            menu: {
              id: 'm1', name: 'オニオンスープ', genre: 'western',
              difficulty: 'easy', role: 'soup', description: '',
            },
            matched: [{ id: 'i1', name: '玉ねぎ', nameKana: 'たまねぎ', category: 'vegetable' }],
            missing: [],
          },
        ],
        nearMisses: [],
      }),
    ),
  )

  renderPage()
  await screen.findByLabelText('玉ねぎ')
  await userEvent.click(screen.getByLabelText('玉ねぎ'))
  await userEvent.click(screen.getByLabelText('この中だけで作れるもの'))
  await userEvent.click(screen.getByRole('button', { name: 'この食材で探す' }))

  // 「不足0」でも調味料は要る。この経路でこそ要る断り（spec.md 14.1 / 14.4）。
  expect(await screen.findByText(/調味料は含みません/)).toBeInTheDocument()
})
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd frontend && npm test -- SearchByIngredientsPage`
Expected: FAIL（「この中だけで作れる献立はありませんでした」が出ない）

- [ ] **Step 3: 実装する**

`SearchByIngredientsPage.tsx` の結果表示を差し替える。

```tsx
{search.data && (
  <Results result={search.data} onlyMakeable={onlyMakeable} />
)}
```

`Matches` の下に足す。

```tsx
// Results は検索結果。0件のときの文言が探し方によって変わる。
function Results({
  result,
  onlyMakeable,
}: {
  result: SearchByIngredientsResult
  onlyMakeable: boolean
}) {
  if (result.matches.length > 0) {
    return <Matches matches={result.matches} />
  }

  return (
    <div className="space-y-6">
      <MascotEmpty image="/mascot/face-thinking.png">
        {onlyMakeable
          ? 'この中だけで作れる献立はありませんでした。食材をもう少し選ぶと見つかりやすくなります。'
          : 'その食材で作れる献立が見つかりませんでした。食材を増やすと見つかりやすくなります。'}
      </MascotEmpty>

      {/* 見出しを分けて、絞り込みの結果ではないことを見た目で区別する。
          地続きに見せると「作れるものだけ」の約束を破ったように見える。 */}
      {result.nearMisses.length > 0 && (
        <div className="space-y-4">
          <h2 className="text-lg font-bold text-kon-ink">
            あと1品買えば作れます（{result.nearMisses.length}件）
          </h2>
          <MatchList matches={result.nearMisses} />
        </div>
      )}
    </div>
  )
}
```

`Matches` から `<ul>` の部分を `MatchList` に切り出し、`Matches` と `Results` の
両方から使う（同じカードを2箇所に複製しない）。`Matches` は
見出し・調味料の断り・`MatchList` の3つを並べるだけになる。

**調味料の断りは 0件時には出さない**（並べる献立が無いため）が、
`onlyMakeable` の**結果がある**ときは従来どおり出る（`Matches` が持っているため
追加の変更は要らない）。

- [ ] **Step 4: テストが通ることを確認する**

Run: `make test-frontend`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add frontend/src/features/menu/SearchByIngredientsPage.tsx frontend/src/features/menu/SearchByIngredientsPage.test.tsx
git commit -m "feat: 作れる献立が0件のとき あと1品 の候補を別枠で出す"
```

- [ ] **Step 6: 実機で見る**

```bash
make up && make seed
```

ブラウザで `http://localhost:5173/from-fridge` を開き、次を目で確認する。

1. 食材を3つだけ選んで「この中だけで作れるもの」→ 0件の文言と「あと1品」が別の見出しで出る
2. 食材を20個ほど選んで同じ操作 → 候補が出る（30件前後が期待値だが上限20件で切れる）
3. 「作れるものだけ」を選ぶと並び順のラジオが消える
4. **つまみと結果が画面内に収まっている。** ピッカーは `max-h-[55vh]` の
   スクロール領域だが、ラジオを2組足したぶん下がる。13-E は
   「結果が画面外に出る」不具合を実機で初めて見つけている。**単体テストは緑のまま通る**

> 実機確認で見つけたことがあれば、直したうえで追加のコミットにする。
> **PRはまだ出さない**（全タスク終了後に1本）。

---

## Task 7: E2E と README

**Files:**
- Modify: `frontend/e2e/from-fridge.spec.ts`
- Modify: `README.md`

> ブランチはそのまま（`feature/fridge-only-search`）。切り替えない。

- [ ] **Step 1: E2E を書く**

既存の `from-fridge.spec.ts` に足す。チェックボックスは `label` を押す
（本体は `sr-only` で覆われている。`helpers.ts` の `choose` と同じ理由）。

```ts
test('作れるものだけに切り替えると候補が減る', async ({ page }) => {
  await page.goto('/from-fridge')

  // 頻出の食材を選ぶ。不足を許せば候補は必ず出る。
  for (const name of ['玉ねぎ', 'にんじん', 'じゃがいも']) {
    await page.getByText(name, { exact: true }).click()
  }

  await page.getByRole('button', { name: 'この食材で探す' }).click()
  const loose = await page.locator('li[aria-label]').count()
  expect(loose).toBeGreaterThan(0)

  await page.getByText('この中だけで作れるもの').click()
  await page.getByRole('button', { name: 'この食材で探す' }).click()

  // 3種では不足0の献立が存在しない（実データで確認済み）。
  // 0件だと明言した上で「あと1品」が出る。
  await expect(
    page.getByText(/この中だけで作れる献立はありませんでした/),
  ).toBeVisible()
  await expect(
    page.getByRole('heading', { name: /あと1品買えば作れます/ }),
  ).toBeVisible()
})
```

> **候補の中身は検証しない。** 何が出るかは献立マスタ次第なので、
> 件数と表示の形だけを見る（13-F と同じ方針）。

- [ ] **Step 2: E2E を実行する**

```bash
make up && make seed
make test-e2e
```

Expected: PASS

- [ ] **Step 3: README を更新する**

「冷蔵庫から探す」の記述に2つのつまみを足す。「完全一致に絞らない」と
断定している箇所があれば「既定では絞らない」に直す。

- [ ] **Step 4: コミットする**

```bash
git add frontend/e2e/from-fridge.spec.ts README.md
git commit -m "test: 冷蔵庫検索の切り替えをE2Eで確認する"
```

**PR はここでは出さない。** 全タスクの完了後、最終レビューを通してから1本出す。

- [ ] **Step 5: task.md のチェックを埋める**

`task.md` のフェーズ13拡張（13-G / 13-H / 13-I）を `[x]` にし、
実装中に分かったことがあれば `>` の注記として残す。

---

## 自己レビュー結果

**仕様の網羅** — spec.md 5.6 の箇条書きを1つずつ照合した。
`onlyMakeable`（Task 1）/ `sort` の2値と未知の値400（Task 1・3）/
`nearMisses` の条件（Task 2・3）/ 上限20件と切り詰めの順序（Task 1）/
`[]` で返す（Task 3）/ 未認証で使える（既存の扱いを変えていない）。
2.9 の「つまみ2つ」は Task 5、「0件時の別枠」は Task 6。抜けなし。

**型の一貫性** — `SearchByIngredientsInput` / `SearchByIngredientsResult` /
`MatchSort` / `SortMissingAsc` / `SortMatchedDesc` / `withMissingAtMost` /
`truncateMatches` / `toMenuMatchDTOs` / `MatchList` / `Results` を
Task 1→7 で通して同じ名前で使っている。TS 側は `MatchSort` /
`SearchOptions` / `SearchByIngredientsResult`。

**注意点** — Task 1 単体ではビルドが壊れる（handler が旧署名を呼ぶ）。
Task 3 まで通してから push する。計画にその旨を書いた。
