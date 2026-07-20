# 実装タスクリスト

`spec.md` を細分化した作業単位。上から順に消化する。

## 進め方

- **1PR単位 = 1つの振る舞い**（🔴テスト + 🟢実装の対）。`main` から `feat/xxx` ブランチを切る
- **PRの下限は 🔴+🟢 の対**。🔴 だけで出すとテストが失敗した状態になり CI が赤でマージできない
- TDD を厳守する。🔴 は**失敗を確認してから** 🟢 に進む
- PR を作ったら CI 全緑を確認し、レビュー待ちで止める（マージはユーザーが行う）
- マージ後にバグが出たら Issue を立て、修正PRで `Closes #N` する

## 記号

| 記号 | 意味 |
| --- | --- |
| 🔴 | 失敗するテストを書く（Red） |
| 🟢 | テストを通す実装（Green） |
| 🔧 | 設定・雑務 |

---

## 完了済み

<details>
<summary><b>フェーズ0: 環境構築</b> — PR #1 ✅（13タスク）</summary>

`docker compose up` で Go が `/health` に200を返すところまで。
Go / Docker Desktop / make の導入、echo サーバ、Dockerfile（dev: air / prod: distroless）、
Vite スキャフォールド、compose、Makefile、CI 3ジョブ、.golangci.yml。

</details>

<details>
<summary><b>フェーズ1: ドメイン層 + 献立マスタ</b> — PR #2 ✅（39タスク）</summary>

repository の統合テストが緑。
ドメイン型5つ（カバレッジ100%）、マイグレーション（embed）、pgx接続プール（リトライ付き）、
献立マスタ120件（冪等シード）、MenuRepository（統合テスト12件）、service/ports.go。

</details>

---

## フェーズ2: 献立検索（1食分）

> 完了条件: 絞り込み・候補枯渇の単体テストが緑

### PR 2-A: RFC 7807 エラーレスポンス `feat/problem-json` ✅

他のPRが全てこの形式でエラーを返すため最初に入れる。

- [x] 🔴 `Problem` 型のテスト: 各フィールドがJSONに出る
- [x] 🔴 テスト: Content-Type が `application/problem+json`
- [x] 🔴 テスト: ドメインのエラーが対応するHTTPステータスに変換される
- [x] 🔴 テスト: 未知のエラーは500になり、詳細を外部に漏らさない
- [x] 🟢 `Problem` 型と echo のカスタムエラーハンドラを実装
- [x] 🔧 `main.go` に結線

### PR 2-B: 乱数源の抽象化 `feat/random-source` ✅

`SuggestMenu` のテストを決定的にするため、実装より先に入れる。

- [x] 🔴 テスト: 決定的な乱数源が指定した値を返す
- [x] 🔴 テスト: 候補が空のとき `Pick` がエラーになる
- [x] 🔴 テスト: 候補1件なら必ずそれを返す
- [x] 🟢 `Randomizer` インターフェースと実装（crypto/rand ベース）
- [x] 🟢 テスト用の決定的な実装

### PR 2-C: SuggestMenu の絞り込み `feat/suggest-menu-filter` ✅

- [x] 🔧 fake の `MenuRepository` を作る
- [x] 🔴 テスト: genre 指定で該当ジャンルのみ候補になる
- [x] 🔴 テスト: difficulty 指定で該当難易度のみ候補になる
- [x] 🔴 テスト: 両方指定で両方に合うもののみ
- [x] 🔴 テスト: 両方 nil で全件が候補
- [x] 🔴 テスト: 不正な genre は `ErrInvalidGenre`
- [x] 🟢 `service.SuggestMenu` を実装

### PR 2-D: SuggestMenu の候補枯渇 `feat/suggest-menu-exhausted` ✅

- [x] 🔴 テスト: 候補0件で `ErrNoMenuFound`
- [x] 🔴 テスト: 候補1件ならそれが返る（境界値）
- [x] 🔴 テスト: repository のエラーがラップされて返る
- [x] 🔴 テスト: `ErrNoMenuFound` と repository のエラーが区別できる
- [x] 🟢 エラーハンドリングを実装

### PR 2-E: SuggestMenu の除外指定 `feat/suggest-menu-exclude` ✅

- [x] 🔴 テスト: `ExcludeIDs` の献立が候補から外れる
- [x] 🔴 テスト: 全件除外すると `ErrNoMenuFound`
- [x] 🟢 除外を実装（フェーズ6の履歴除外で使う）
      → **実装は不要だった**。除外は PR #2 で repository が実装済み
      （`AND NOT (id = ANY($3::uuid[]))`、統合テスト3件）で、SuggestMenu は
      filter をそのまま渡すため既に動いていた。テストのみ追加して契約を固定した。

### PR 2-F: `GET /menus/suggest` `feat/api-suggest-menu` ✅

- [x] 🔴 テスト: 200 とJSON構造（id / name / genre / difficulty / description）
- [x] 🔴 テスト: `?genre=japanese` が service に渡る
- [x] 🔴 テスト: `?difficulty=easy` が service に渡る
- [x] 🔴 テスト: クエリ無しで両方 nil が渡る
- [x] 🔴 テスト: 不正な genre で 400
- [x] 🔴 テスト: 不正な difficulty で 400
- [x] 🔴 テスト: 候補0件で 422
- [x] 🟢 `MenuHandler.Suggest` を実装
- [x] 🔧 `/api/v1` にルーティング（`MenuHandler.RegisterRoutes`。main.go への結線は PR 2-H）

### PR 2-G: `GET /menus/:id` `feat/api-get-menu` ✅

- [x] 🔴 テスト: 200 と献立の詳細
- [x] 🔴 テスト: 存在しないIDで 404
- [x] 🔴 テスト: 不正なUUIDで 400（ゼロ値UUIDを含む）
- [x] 🔴 テスト: `/menus/suggest` が `:id` に飲み込まれない
- [x] 🟢 `MenuHandler.Get` を実装
- [x] 🟢 `service.GetMenu` を実装（repository の `FindByID` は PR #2 で実装済み）

### PR 2-H: 実機結線 `feat/wire-menu-api` ✅

- [x] 🔧 `main.go` で pool / repository / service / handler を組み立て
- [x] 🔧 DBに繋がらない場合の起動時エラー（`DATABASE_URL` 未設定・接続不可とも exit 1）
- [x] 🔧 実機確認: `curl "localhost:8080/api/v1/menus/suggest?genre=japanese&difficulty=easy"`
- [x] 🔧 実機確認: 120件のマスタから実際に献立が返ること

---

## フェーズ3: レシピ取得

> 完了条件: 障害時フォールバックのテストが緑

### PR 3-A: RecipeLink 型 `feat/recipe-link` ✅

- [ ] 🔧 検索APIを最終選定（Brave / Google CSE）※spec.md 13章 未決事項1
      → **3-C の直前に先送り**。3-B の stub まではキー無しで進められるため。
- [x] 🔴 テスト: URLの検証（http/https のみ、不正なURLを拒否）
- [x] 🔴 テスト: ドメイン抽出（`https://a.example.com/x` → `a.example.com`）
- [x] 🔴 テスト: タイトルが空なら拒否
- [x] 🟢 `domain.RecipeLink` を実装

### PR 3-B: stub gateway `feat/recipe-stub-gateway` ✅

APIキー無しで全機能が動く状態を保つため、実装より先に入れる。

- [x] 🔧 `RecipeSearchGateway` インターフェースを定義（`service/ports.go`）
- [x] 🔴 テスト: 決定的に3件返る（別インスタンス間でも同一）
- [x] 🔴 テスト: 献立名がタイトルに含まれる
- [x] 🔴 テスト: `limit` で件数が絞られる（0・負数・3超の境界）
- [x] 🔴 テスト: 献立名が空なら `ErrEmptyMenuName`
- [x] 🟢 stub gateway を実装（`internal/gateway`）

### PR 3-C: 実 gateway の正常系 `feat/recipe-search-gateway`

> 検索APIは **Brave** に決定（spec.md 13.1）。Google CSE は新規申込が締め切られたため。
> 本PRは `httptest.Server` でHTTPをスタブするため、**APIキー無しで実装・テストできる**。
> 実キーが要るのは 3-F の実機確認から。

- [x] 🔴 テスト: 正常系で3件返る（httptest.Server でスタブ）
- [x] 🔴 テスト: 3件未満しか返らない場合はその件数
- [x] 🔴 テスト: 4件以上返っても3件に切り詰める
- [x] 🔴 テスト: 0件でも成功扱い（空スライス）
- [x] 🔴 テスト: スニペットの `<strong>` を除去し、HTMLエンティティを戻す
- [x] 🔴 テスト: 壊れた1件は飛ばして残りを返す
- [x] 🔴 テスト: APIキーはヘッダのみで渡し、URLに載せない
- [x] 🟢 実 gateway を実装

### PR 3-D: 実 gateway の異常系 `feat/recipe-gateway-resilience`

- [x] 🔴 テスト: HTTP 500 でエラー
- [x] 🔴 テスト: 不正なJSONでエラー
- [x] 🔴 テスト: タイムアウト（3秒）※試行ごと
- [x] 🔴 テスト: 指数バックオフで最大2回リトライ
- [x] 🔴 テスト: リトライ後も失敗ならエラー
- [x] 🔴 テスト: 4xx はリトライしない（無駄な再試行を避ける）
- [x] 🔴 テスト: **429 だけはリトライする**（レート制限は時間をおけば回復する）
- [x] 🔴 テスト: context が切れたらリトライを打ち切る
- [x] 🟢 タイムアウトとリトライを実装（失敗は `ErrSearchFailed` に包む → 3-F で 502）

> ⚠️ 最悪の所要時間は 3s × 3回 + バックオフ 0.6s ≒ **9.6秒**。spec.md 11章の
> 「レシピ取得 p95 2s以内」は通常時（キャッシュ有り: 数ms / 無し: 1回で成功）の話で、
> 全滅時のテール。全体の締め切りを設けるかは 3-F で判断する。

### PR 3-E: gateway ファクトリ `feat/recipe-gateway-factory`

- [x] 🔴 テスト: `SEARCH_API_PROVIDER=stub` で stub が返る
- [x] 🔴 テスト: `brave` で実装が返る
- [x] 🔴 テスト: 未知の値でエラー（`google_cse` を含む）
- [x] 🔴 テスト: `brave` なのにAPIキーが空ならエラー
- [x] 🔴 テスト: **プロバイダが空でもエラー**（stub に既定しない）
- [x] 🔴 テスト: 表記ゆれ（大文字・前後の空白）は吸収する
- [x] 🟢 ファクトリを実装（環境変数は読まず、値として受け取る）

### PR 3-F: `GET /menus/:id/recipes` `feat/api-get-recipes`

- [x] 🔴 テスト: 200 と3件
- [x] 🔴 テスト: 存在しない献立で 404
- [x] 🔴 テスト: gateway 障害で 502（`service.ErrRecipeSearchFailed` → 502）
- [x] 🔴 テスト: 3件未満でも200 / 0件でも200で空配列
- [x] 🔴 テスト: 不正なUUIDで400（service を呼ばない）
- [x] 🟢 `MenuHandler.Recipes` と `service.RecipeLinks` を実装
- [x] 🔧 `main.go` にファクトリ経由で結線（プロバイダ不正なら起動時エラー）
- [x] 🔧 3-D の最悪9.6秒に対し、全体の締め切りを設けるか判断する
      → **設ける**。`service` 側で5秒の上限を課した。gateway は自身の締め切り超過を
        素の context エラーで返すため、それを `ErrRecipeSearchFailed` に寄せて 502 にする。
        呼び出し側の中断（利用者が画面を離れた）とは区別する。
- [x] 🔧 実機確認: stub で3件返ること
- [x] 🔧 実機確認: **Brave の実キーで本物のレシピが3件返ること**
      → 「かつ丼」で デリッシュキッチン / キッコーマン / ニチレイフーズ が0.54秒で返った
- [x] 🔧 実機確認: **日本語のレシピが返ること**。返らなければ `country` / `search_lang` を付ける
      → **必要だった**。無指定だと「親子丼」の3件中2件が英語版（`kurashiru.com/us/`、
        `cookpad.com/eng/`）。`country=JP` / `search_lang=jp` を追加して解消。
        なお **`search_lang=ja` は 422 で拒否される**（実測）。3-C で推測せず保留した判断が正しかった。

### PR 3-G: レシピリンクのキャッシュ `feat/recipe-link-cache`

spec.md 13.2 で MVP に含めると決定。献立120件固定のため、キャッシュすれば
API消費は生涯約120クエリで頭打ちになる。

- [x] 🔧 マイグレーション `000002_create_recipe_link_caches`（menu_id PK / links jsonb / fetched_at）
- [x] 🔴 テスト: 初回は gateway を呼び、2回目は呼ばない
- [x] 🔴 テスト: TTL 7日を過ぎたら再取得する（境界値: 7日ちょうどはヒット / 7日と1秒で失効）
- [x] 🔴 テスト: キャッシュが壊れていても gateway にフォールバックする
- [x] 🔴 テスト: gateway が0件を返した場合もキャッシュする（毎回叩き直さない）
- [x] 🔴 テスト: gateway 障害時はキャッシュを書かない
- [x] 🔴 テスト: 書き込みが失敗しても検索結果は返す
- [x] 🔴 統合テスト: 保存・上書き・CASCADE・壊れた値の拒否
- [x] 🟢 キャッシュを実装（**service に持たせた**。キャッシュのキーが `menu_id` である一方、
      `RecipeSearchGateway.Search` は献立名しか受け取らないため、gateway を包む形にはできない）
- [x] 🔧 実機確認: 実キーで **0.565秒 → 0.0033秒**（約170倍）。DBに1件×3リンクが保存された

---

## フェーズ4: 週間献立

> 完了条件: 重複回避と枯渇時の緩和テストが緑

### PR 4-A: 週間献立の骨格 `feat/suggest-weekly-basic`

> 起点は **当日** に決定（spec.md 13.3）。サーバは曜日を持たず `day` 1..7 のみを返す。

- [x] 🔧 週の開始曜日を決める ※spec.md 13章 未決事項4 → **当日起点**（13.3）
- [x] 🔴 テスト: 7件返る
- [x] 🔴 テスト: `day` が 1..7 の連番（起点当日が 1）
- [x] 🔴 テスト: 候補0件で `ErrNoMenuFound`
- [x] 🔴 テスト: 候補は1度だけ問い合わせる（日ごとに引くとDB負荷が7倍）
- [x] 🔴 テスト: 不正な条件はDBに問い合わせず弾く
- [x] 🔴 テスト: 骨格の段階では重複を許す（4-B での変化を差分で見えるようにする）
- [x] 🟢 `service.SuggestWeekly` の骨格を実装（`domain.DayMenu` / `domain.WeekLength`）

### PR 4-B: 同一献立の重複回避 `feat/weekly-no-duplicate-menu` ✅

- [x] 🔴 テスト: 同一献立が週内に2度出現しない
- [x] 🔴 テスト: 候補がちょうど7件なら7件とも異なる（境界値）
- [x] 🔴 テスト: 選ばれた献立は以降の候補から外れる
- [x] 🔴 テスト: 候補が7件未満なら現時点では `ErrNoMenuFound`（**4-D で緩和する中間状態**）
- [x] 🟢 重複回避を実装（候補から取り除く方式。再抽選ループは使わない）

### PR 4-C: 同ジャンル3連続の回避 `feat/weekly-no-genre-streak` ✅

- [x] 🔴 テスト: 同一ジャンルが3日以上連続しない
- [x] 🔴 テスト: 2連続は許容される
- [x] 🔴 テスト: 連続の判定は直前2日だけを見る（週内の出現回数は数えない）
- [x] 🔴 テスト: 重複回避と連続回避が同時に効く
- [x] 🔴 テスト: 候補が全て同一ジャンルなら現時点では `ErrNoMenuFound`（**4-D で緩和**）
- [x] 🟢 ジャンル連続の回避を実装

> ⚠️ **ジャンルで絞った週間献立が現時点では必ず失敗する**。候補が全て同一ジャンルに
> なり、件数が足りていても3日目で選べる候補が尽きるため。spec.md 2.2 はジャンル指定を
> 許しているので 4-D で必ず解消すること。週間献立のAPIは 4-F まで公開されないため
> 利用者には見えない。

### PR 4-D: 候補枯渇時の緩和 `feat/weekly-relaxation` ✅

ここが週間献立の最も壊れやすい部分。単独のPRで集中的にテストする。

- [x] 🔴 テスト: 候補が6件のとき緩和して同一献立の再利用を許す
- [x] 🔴 テスト: 候補が1件のとき7日とも同じ献立になる（極端値）
- [x] 🔴 テスト: 候補が全て同一ジャンルなら3連続禁止を緩和する ← **4-C の制約を解消**
- [x] 🔴 テスト: 緩和が起きたことを呼び出し側が判別できる（`DayMenu.Relaxed*`）
- [x] 🔴 テスト: 緩和は必要最小限（7件・ちょうど7件とも緩和しない）
- [x] 🔴 テスト: **絞り込み条件は緩めない**（利用者の指定を勝手に外さない）
- [x] 🔴 テスト: 緩和の順序（重複よりジャンル連続を先に緩める）
- [x] 🟢 段階的な緩和ロジックを実装（4段階）

> 緩める順序は spec.md 2.2 のルール列挙順の逆（重要度の低いものから）。
> 候補が7件以上あれば重複は決して起きない（残りが尽きないため）。
>
> **先読みはしない。** 週全体では3連続を避けられる並びがあるのに袋小路に入ることが
> 理論上ある。候補1〜12件 × ジャンル1〜4種を総当たり2400回試して5回のみ、いずれも
> 2ジャンルしか無い作為的な形。実マスタ相当（120件・4ジャンル）は2000回試して0回。

### PR 4-E: 履歴除外の連携 `feat/weekly-exclude-history` ✅

- [x] 🔴 テスト: 直近履歴に含まれる献立を避ける
- [x] 🔴 テスト: 履歴を除くと7件に満たない場合は履歴除外を緩和する
- [x] 🔴 テスト: 履歴除外は重複回避より先に緩む
- [x] 🔴 テスト: **連続を緩めても履歴は避ける**（段階の抜けをストレステストが検出）
- [x] 🔴 テスト: 全候補が履歴にあっても7日分を返す（極端値）
- [x] 🔴 テスト: **履歴を repository の絞り込みに使わない**
- [x] 🟢 履歴除外を実装（緩和は8段階に。フェーズ6で実データと結線）

> 履歴は `SuggestWeekly` の第3引数で渡す。`MenuFilter.ExcludeIDs` に入れてはならない。
> あちらは repository が SQL で除外する**強い条件**で、緩和の余地が無くなるため。
>
> 「履歴の献立を出す」と「ジャンル3連続にする」の二択では**前者を採る**。
> spec.md 2.2 がルール3(履歴)にだけ「候補が枯渇する場合はこの条件を緩和する」と
> 明記しているため。履歴を避けられる候補が残っていても履歴の献立が出ることがある。

### PR 4-F: `POST /menus/suggest-weekly` `feat/api-suggest-weekly` ✅

- [x] 🔴 テスト: 200 と `week` 配列7件
- [x] 🔴 テスト: 不正なリクエストボディで 400（壊れたJSON・配列・型違い・未知の値）
- [x] 🔴 テスト: 候補0件で 422
- [x] 🔴 テスト: 未指定・null・空文字はいずれも絞り込まない
- [x] 🔴 テスト: ボディが空でも 200（条件なしの提案）
- [x] 🔴 テスト: POST 以外は受け付けない
- [x] 🟢 `MenuHandler.SuggestWeekly` を実装
- [x] 🔧 実機確認: 7日分・重複なし・3連続なし（0.008秒）
- [x] 🔧 実機確認: **和食×凝っている（候補10件・全て同一ジャンル）でも7日分**が重複なしで返る
      → 4-C で「ジャンル絞りは必ず失敗する」としていた制約が 4-D で解消済みだと実データで確認

> 緩和の情報（`DayMenu.Relaxed*`）はレスポンスに含めない。spec.md 5.1 のレスポンスに
> 無いため。画面に出す必要が生じたら仕様から決める（8-G）。
>
> `GET /menus/suggest-weekly` は 405 ではなく 400 になる。`GET /menus/:id` が
> `suggest-weekly` を献立IDとして拾い、UUIDでないため弾かれる。素通りはしないので実害なし。

### PR 4-G: 1日だけの引き直し `feat/api-reroll-day` ✅

- [x] 🔧 **spec.md 5.1 に `POST /menus/reroll-day` を追記**（2.2 が要求しているのに API 一覧に無かった）
- [x] 🔴 テスト: 指定日だけ変わり、他の日は保持される
- [x] 🔴 テスト: 引き直し後も重複回避が効く
- [x] 🔴 テスト: 範囲外の day で 400 / 週が7件でなければ 400
- [x] 🔴 テスト: **前後どちらの連続も避ける**（挟まれた形も含む）
- [x] 🔴 テスト: 同じ献立を返さない（候補に余裕があるとき）
- [x] 🔴 テスト: 候補が週と同数なら同じ献立が返る（重複を作るより据え置く）
- [x] 🔴 テスト: 他の日はまとめて引く（`FindByIDs` 1回）
- [x] 🟢 引き直しAPIを実装（`service.RerollDay` / `MenuRepository.FindByIDs`）
- [x] 🔧 実機確認: 和食で絞って3日目を5回引き直し、毎回異なり重複0

> サーバは週の状態を持たない。現在の週をリクエストで受け取り、引き直した1日だけを返す。
>
> 連続判定は `SuggestWeekly` の「直前2日」ではなく**前後両側**を見る。3日目を引き直すなら
> [1,2,3] [2,3,4] [3,4,5] のどの窓でも3連続にしない。
>
> 引き直す日の献立自身も優先候補から外す（同じものが返るのでは引き直しにならない）。
> ただし候補が週と同数なら、重複を作るより据え置く（重複回避が最重要ルールのため）。

---

## フェーズ5: 認証

> 完了条件: 認証境界のテストが緑

### PR 5-A: users / auth_identities `feat/auth-schema` ✅

- [x] 🔧 マイグレーション `000003_create_users`
      → 番号は **`000002` ではなく `000003`**。`000002` は 3-G の
        `recipe_link_caches` で使用済みのため繰り上げた。
- [x] 🔴 テスト: CHECK制約（password なら hash 必須）
- [x] 🔴 テスト: CHECK制約（google なら uid 必須）
- [x] 🔴 テスト: UNIQUE (provider, provider_uid)
- [x] 🔴 テスト: メールの UNIQUE
- [x] 🔴 テスト: user 削除で identity も消える（CASCADE）
- [x] 🟢 マイグレーションを実装（users / auth_identities）
      → 統合テストの後始末 `TRUNCATE` に users を追加（menus と参照が無く、
        メールの UNIQUE がテスト間で衝突するため）。

### PR 5-B: パスワードのハッシュ化 `feat/password-hashing` ✅

- [x] 🔴 テスト: ハッシュ化と検証（bcrypt cost 12）
- [x] 🔴 テスト: 同じパスワードでも毎回異なるハッシュ
- [x] 🔴 テスト: 誤ったパスワードで検証が失敗（`ErrPasswordMismatch`）
- [x] 🔴 テスト: 8文字未満を拒否（**文字数**で数える。全角8文字は通る）
- [x] 🔴 テスト: 72バイト超の扱い（bcryptの上限）→ **切り詰めず拒否**
- [x] 🔴 テスト: 壊れたハッシュは不一致と区別する
- [x] 🟢 パスワードユーティリティを実装（`internal/auth`）

> **72バイト超は切り詰めず拒否**する。切り詰めると別々のパスワードが同じ
> ハッシュになりうるため。最小長は spec.md 11章の「8文字以上」に忠実に
> ルーン数で数える（全角8文字＝24バイトでも通す）。検証の不一致
> （`ErrPasswordMismatch`）と壊れたハッシュのエラーは呼び出し側が
> 区別できるよう分ける。`golang.org/x/crypto` が direct 依存に昇格。

### PR 5-C: サインアップ `feat/signup` ✅

- [x] 🔴 テスト: user と auth_identity が作られる（統合: 対で1件ずつ・トランザクション）
- [x] 🔴 テスト: 登録済みメールで 409（`ErrEmailTaken`。制約名まで見て判定）
- [x] 🔴 テスト: メール形式が不正で 400（`domain.ErrInvalidEmail`）
- [x] 🔴 テスト: パスワードが短いと 400（`auth.ErrPasswordTooShort`）
- [x] 🟢 `service.SignUp` と `POST /auth/signup` を実装
- [x] 🔧 実機確認: 正常→201 / **大文字メールの再登録→409**（正規化が効く）/
      不正メール→400 / 短いパスワード→400。DBに user 1件 + password identity
      1件（hash は `$2a$12$`）

> **表示名はメールのローカル部から導出**する。サインアップは表示名を受け取らない
> （spec.md 5.2）が users.display_name は NOT NULL のため。
>
> ドメインに `Email`（正規化・検証つき）/ `User` / `UserID` を追加。メールは
> 小文字に正規化して持ち、大小違いでの二重登録を防ぐ。パスワードのハッシュ化は
> `PasswordHasher` として注入（乱数源と同じDIの形。テストは実物 `auth.Hasher`）。
> user と auth_identity は必ず対で作るため repository でトランザクションに束ねる。
> **成功時は 201。Cookie/JWT の発行は 5-E / 5-F で行う**。

### PR 5-D: ログイン `feat/login` ✅

- [x] 🔴 テスト: 正しいパスワードで成功
- [x] 🔴 テスト: 誤ったパスワードで 401
- [x] 🔴 テスト: 存在しないメールで 401（存在時と**同一の**エラー＝ユーザー列挙対策）
- [x] 🔴 テスト: Google のみのユーザーがパスワードログインを試みると 401
- [x] 🟢 `service.Login` と `POST /auth/login` を実装
- [x] 🔧 実機確認: 正しいパスワード→200 / 誤り→401 / 存在しないメール→**2と同一の401** /
      大文字メールも正規化して200。応答時間も ~0.19s で一致（タイミング等化が効く）

> **3つの失敗（メール不在・パスワード違い・Google のみ）を全て `ErrInvalidCredentials`
> (401) に丸める**。エラーの差からアカウントの有無を推測されないようにするため。
> repository は `provider='password'` を内部結合で引くので、Google のみのユーザーは
> 自然に0行＝`ErrCredentialNotFound` になる。
>
> **タイミング等化**: 照合対象が無いときも固定ハッシュで bcrypt を1回走らせ、
> 「存在するがパスワード違い」と応答時間を揃える。即座に返すと時間差で存在が漏れるため。
>
> 成功時は 200。Cookie/JWT の発行は 5-E / 5-F。メール形式の不正だけは 400
> （照合以前の問題で存在推測に使えないため）。

### PR 5-E: JWT の発行と検証 `feat/jwt` ✅

- [x] 🔴 テスト: アクセストークンの発行と検証
- [x] 🔴 テスト: 有効期限切れを拒否（15分。exp は排他的で15分ちょうどは失効）
- [x] 🔴 テスト: 署名が異なるトークンを拒否
- [x] 🔴 テスト: `alg=none` を拒否
- [x] 🔴 テスト: 改竄されたペイロードを拒否
- [x] 🔴 テスト: 空の秘密鍵での生成を拒否
- [x] 🟢 JWT の発行・検証を実装（`internal/auth`、golang-jwt/v5）

> **署名方式を HS256 に固定**（`WithValidMethods`）。これで `alg=none`（署名なし）や、
> 公開鍵を秘密鍵と取り違えさせる非対称方式へのすり替えをまとめて拒否する。
> 失敗の内訳（期限切れ/署名不正/改竄）は攻撃者への手掛かりになるため、外向きには
> `ErrTokenInvalid` 1つに丸める。`exp` は RFC 7519 通り排他的（現在時刻が exp より
> 前でなければ失効）なので、15分ちょうどは失効が正しい。`now` を差し替え可能にし、
> 期限判定をテストと本番で同じ経路に通す。ライブラリ単体のため main 結線は 5-F 以降。

### PR 5-F: Cookie とリフレッシュ `feat/auth-cookie` ✅

- [x] 🔴 テスト: Cookie 属性（HttpOnly / Secure / SameSite=Lax）
- [x] 🔴 テスト: リフレッシュトークンで再発行（30日。exp は排他的）
- [x] 🔴 テスト: 期限切れリフレッシュトークンで 401
- [x] 🔴 テスト: ログアウトで Cookie が失効する
- [x] 🔴 テスト: **リフレッシュ⇄アクセスの取り違えを拒否**（種別 typ クレーム）
- [x] 🔴 テスト: リフレッシュ Cookie 欠落で 401
- [x] 🟢 Cookie の発行・失効と `/auth/refresh`、`/auth/logout` を実装
- [x] 🔧 実機確認: ログインで両Cookie発行（access Max-Age=900/refresh 2592000）/
      refresh(Cookie有)→204・(無)→401 / logout で両Cookie Max-Age=0

> **リフレッシュは stateless**（署名付きJWT・30日）。スキーマにトークン表が無く、
> spec.md 66-67 も「JWTで受け渡し」のため。ログアウトは Cookie 失効で表す。
>
> **種別クレーム `typ`(access/refresh) を追加**。これが無いと長寿命の
> リフレッシュトークンを短寿命のはずのアクセストークンとして使い回せてしまう。
> **リフレッシュ Cookie はパスを `/api/v1/auth` に絞る**（長寿命トークンの露出範囲を最小化）。
> サインアップ／ログイン成功時に両Cookieを発行（サインアップは自動ログイン）。
> `/auth/refresh` はアクセスCookieだけ差し替えて 204、`/auth/logout` は冪等で 204。
> `main.go` は `JWT_SECRET` 未設定なら起動時エラー。

### PR 5-G: 認証ミドルウェア `feat/auth-middleware` ✅

- [x] 🔴 テスト: 認証必須エンドポイントに未認証で 401（Cookie欠落・不正トークン）
- [x] 🔴 テスト: 有効なCookieでコンテキストにユーザーが入る（service に userID が渡る）
- [x] 🔴 テスト: 献立検索は未認証でも 200（menu ルートを包まないことで担保）
- [x] 🔴 テスト: `GET /auth/me` が現在のユーザーを返す
- [x] 🔴 テスト: リフレッシュトークンをアクセス Cookie に載せても 401（種別違い）
- [x] 🟢 ミドルウェア `RequireAuth` と `/auth/me` を実装
- [x] 🔧 実機確認: 未認証 /auth/me→401 / ログイン→Cookie→200 /
      献立検索は未認証でも200 / ログアウト後→401

> `RequireAuth` はアクセス Cookie を検証し、userID を echo コンテキストに載せる。
> 認証不要のエンドポイント（献立検索）は**包まないことで未認証でも 200 のまま**。
> `/auth/me` はトークンの sub から DB を引いてユーザーを返す（`UserRepository.FindByID`
> / `AuthService.CurrentUser`）。有効なトークンが指すユーザーが消えていたら 401
> （`ErrUserNotFound` → 401）。フェーズ5の認証境界はここで完成（残りは 5-H/5-I の Google SSO）。

### PR 5-H: Google SSO の認可URL `feat/google-auth-url` ✅（コード）

- [ ] 🔧 Google Cloud Console で OAuth クライアントを作成（**要ユーザー操作・未完**）
      → コード・単体テストはダミー設定で完了。**実クレデンシャルは 5-I の
        コールバック実機確認で必要**。リダイレクトURIは
        `http://localhost:8080/api/v1/auth/google/callback`。
- [x] 🔴 テスト: 認可URLに PKCE の code_challenge が含まれる（S256）
- [x] 🔴 テスト: state が生成され Cookie に保存される（URL の state と一致）
- [x] 🔴 テスト: state は毎回異なる
- [x] 🔴 テスト: Cookie 属性（HttpOnly / Secure / SameSite=Lax / Path=/api/v1/auth）
- [x] 🔴 テスト: 未設定（client_id 空）なら 503
- [x] 🟢 `GET /auth/google` を実装（`internal/auth` の `GoogleOAuth`、x/oauth2）
- [x] 🔧 実機確認: 未設定→503 / ダミー設定→302 で Google 認可URLへ
      （code_challenge・state・scope が乗り、state と oauth_state Cookie が一致）

> Google 認可フローは **Authorization Code + PKCE**。verifier と CSRF 対策の
> state を生成し、短命 Cookie（10分・SameSite=Lax）に保存してから認可URLへ 302。
> **SameSite=Lax はコールバック（トップレベルGET遷移）で Cookie を送るために必須**
> （Strict だと送られない）。認可URLの生成は x/oauth2 に任せる（5-I のトークン交換でも使う）。
> Google 未設定でも起動し、`/auth/google` だけ 503 にする（任意機能）。

### PR 5-I: Google SSO のコールバック `feat/google-callback` ✅（コード）

- [x] 🔴 テスト: state 不一致で 401（CSRF対策）
- [x] 🔴 テスト: state が無い場合も 401
- [x] 🔴 テスト: code_verifier 不一致で 401（Exchange 失敗を 401 に丸める）
- [x] 🔴 テスト: 初回コールバックで user が作られる（統合）
- [x] 🔴 テスト: 2回目は既存 user に紐づく（重複作成しない）（統合）
- [x] 🔴 テスト: **既存のパスワードユーザーと同じメール**なら同一userに identity を追加（統合）
- [x] 🔴 テスト: メール未確認は拒否（紐付け乗っ取り対策）
- [x] 🟢 `GET /auth/google/callback` を実装
- [x] 🔧 実機確認(自動): callback 結線（state無し→401）/ パスワード回帰（サインアップ→/auth/me→200）
- [x] 🔧 実機確認(ブラウザ): **Google 実ログイン通し** → 成功。/auth/me が
      `yuuya.kim0801@gmail.com`「キムさん」を返し、DBに user + google identity
      （provider_uid あり・password_hash なし）が作られた

> コールバックは **state を Cookie と定数時間比較（CSRF対策）** → コードを本人情報に交換
> （x/oauth2、verifier で PKCE 突合）→ ユーザー upsert → 認証 Cookie 発行 → フロントへ 302。
> **メール未確認は拒否**（未確認メールでの紐付けは乗っ取りに使えるため）。
> upsert は3分岐（既存google / 同一メールに紐付け / 新規）をトランザクションで。
> トークン交換・userinfo は httptest でスタブ、ハンドラは fake で検証。
> 実フローは実クレデンシャル＋ブラウザ同意が要るためユーザーが踏む。

---

## フェーズ6: 履歴

> 完了条件: 16件目でFIFOが働くテストが緑

### PR 6-A: search_histories `feat/history-schema` ✅

- [x] 🔧 マイグレーション `000004_create_search_histories`
      → 番号は `000003` ではなく **`000004`**（`000003` は users で使用済み）。
- [x] 🔧 INDEX (user_id, searched_at DESC)
- [x] 🔴 テスト: 履歴を1件記録できる
- [x] 🔴 テスト: user 削除で履歴も消える（CASCADE）
- [x] 🔴 テスト: `SearchMode`(single/weekly) の検証（ドメイン）
- [x] 🟢 マイグレーションと Repository の記録を実装
- [x] 🔧 実機確認: migrate version 4 適用、INDEX・CHECK・FK を確認

> `search_mode` は single/weekly を CHECK と `domain.SearchMode` の両方で縛る。
> menu_id の FK は CASCADE を付けない（献立マスタは固定で削除しない）。
> FIFO（15件超の削除）は 6-B で。ここは1件INSERTのみ。

### PR 6-B: FIFO 15件 `feat/history-fifo` ✅

フェーズ6の核。単独のPRで集中的にテストする。

- [x] 🔴 テスト: 15件までは削除されない（境界値）
- [x] 🔴 テスト: **16件目の投入で最古が消え、ちょうど15件残る**
- [x] 🔴 テスト: FIFOはユーザー単位（他ユーザーの履歴が消えない）
- [x] 🔴 テスト: 21件を一度に入れても15件に収まる
- [x] 🔴 テスト: `searched_at` が同値でも順序が安定する（タイブレーク＝seq）
- [x] 🟢 `service.RecordHistory` を実装（アプリ層・トランザクション内）
- [x] 🔧 実機確認: migrate 000005 up/down/up、seq 列と3列インデックスを確認

> **タイブレーク用に `seq`(bigint IDENTITY) 列を追加**（migration 000005）。
> `searched_at` は now()=トランザクション時刻で、週間一括登録では7件が同値になる。
> `ORDER BY searched_at DESC` だけでは同値の行の順序が定まらず「最新15件」が
> 非決定的になるため、`searched_at DESC, seq DESC` で挿入順に確定する。
> FIFO は repository が INSERT+DELETE を1トランザクションで（spec.md 4.3）。
> 保持件数15は業務ルールとして service（`HistoryLimit`）が repository に渡す。

### PR 6-C: 週間献立の一括記録 `feat/history-bulk-record` ✅

- [x] 🔴 テスト: 7件を1トランザクションで記録
- [x] 🔴 テスト: FIFOが7回ではなく1回だけ走る（全件INSERT後に1度だけ prune）
- [x] 🔴 テスト: 途中で失敗したら全件ロールバック（FK違反で0件）
- [x] 🟢 一括記録を実装（`RecordManyWithLimit` / `HistoryService.RecordMany`）

> 7件を1トランザクションで INSERT し、**prune は最後に1度だけ**走らせる
> （挿入ごとに走らせる必要はなく無駄）。seq タイブレークにより一括の7件は
> 同一 searched_at でも挿入順が保たれる。途中失敗は全件ロールバック。

### PR 6-D: `GET /histories` `feat/api-list-histories` ✅

- [x] 🔴 テスト: 新しい順に返る（searched_at DESC, seq DESC）
- [x] 🔴 テスト: 未認証で 401
- [x] 🔴 テスト: 他ユーザーの履歴は返らない（user_id で絞る）
- [x] 🔴 テスト: 0件のとき空配列（null ではなく []）
- [x] 🟢 `GET /histories` を実装
- [x] 🔧 実機確認: 未認証→401 / ログイン→空配列 / 履歴1件で献立情報つき一覧

> 読み取りモデル `domain.HistoryEntry`（HistoryID + Menu + Mode + SearchedAt）を追加。
> repository が menus を JOIN して組み立てる。並びは 6-B の seq タイブレークに揃える。
> 認証済み userID は RequireAuth がコンテキストに載せたものを使い、SQL で user_id を
> 絞るので他人の履歴は構造上返らない。個別削除(6-E)に使うため履歴IDも返す。

### PR 6-E: 履歴の削除 `feat/api-delete-histories` ✅

- [x] 🔴 テスト: 個別削除（204）
- [x] 🔴 テスト: 他ユーザーの履歴削除で 403（`ErrHistoryForbidden`）
- [x] 🔴 テスト: 存在しない履歴の削除で 404（`ErrHistoryNotFound`）
- [x] 🔴 テスト: 全件削除（他ユーザーの履歴は残る）
- [x] 🔴 テスト: 不正なUUIDで 400 / 未認証で 401
- [x] 🟢 `DELETE /histories/:id` と `DELETE /histories` を実装
- [x] 🔧 実機確認: 個別204 / 存在しない404 / 不正UUID400 / 全件204 / 削除後は空配列

> **所有権を先に確認してから消す**。DELETE の件数だけでは「存在しない(404)」と
> 「他人のもの(403)」を区別できないため、先に user_id を引いて判定する。
> spec は他人の履歴を 403 と定めるため、存在の有無が漏れることは許容する。
> 静的な `DELETE /histories`（全件）は `:id` より優先照合され飲み込まれない。

### PR 6-F: 検索フローへの結線 `feat/wire-history` ✅

- [x] 🔴 テスト: 献立提案時に履歴が記録される（single / weekly）
- [x] 🔴 テスト: **未認証時は履歴を記録しない（エラーにしない）**
- [x] 🔴 テスト: 検索時に直近履歴が除外候補として渡る
- [x] 🔴 テスト: 履歴記録の失敗が献立提案を失敗させない
- [x] 🔴 テスト: 直近履歴の取得失敗でも提案は返す / 履歴除外で枯渇したら緩和
- [x] 🟢 結線を実装（OptionalAuth + MenuHistory）
- [x] 🔧 実機確認: 認証3回で3回とも異なる献立（除外が効く）/ 未認証は記録なし /
      /histories はちょうど3件

> **OptionalAuth** ミドルウェアを新設（Cookie が無くても拒否せず素通り、あれば
> userID を載せる）。献立検索を未認証でも 200 に保つため。**ルート単位で付ける**
> （グループ全体に付けると echo の 405 判定が 404 に変わるため）。
> 単発検索は履歴を `ExcludeIDs`（ハード除外）に足し、**除外で枯渇したら外して再試行**。
> 週間/引き直しは第3引数 `recentIDs`（4-E のソフト緩和）で渡す。引き直しは
> 確定前の操作なので記録しない。記録・取得の失敗は best-effort（ログのみ、提案は成功）。

---

## フェーズ7: お気に入り

> 完了条件: 重複追加が409になるテストが緑

### PR 7-A: favorites `feat/favorites-schema` ✅

- [x] 🔧 マイグレーション `000006_create_favorites`（UNIQUE (user_id, menu_id)）
      → 番号は `000004` ではなく **`000006`**（004=search_histories, 005=seq で使用済み）。
- [x] 🔴 テスト: UNIQUE 制約が効く（同一ユーザー×同一献立の二重登録を拒否）
- [x] 🔴 テスト: 別ユーザーなら同じ献立を登録できる
- [x] 🔴 テスト: user 削除で消える（CASCADE）
- [x] 🟢 マイグレーションを実装
- [x] 🔧 実機確認: migrate 000006 up/down/up、UNIQUE・INDEX・FK を確認

> menu_id の FK は CASCADE を付けない（献立マスタは固定）。user_id は CASCADE。
> INDEX (user_id, created_at DESC) は一覧表示の主経路。重複追加の 409 は 7-B。

### PR 7-B: お気に入りの追加 `feat/api-add-favorite` ✅

- [x] 🔴 テスト: 追加できる
- [x] 🔴 テスト: **同一献立の重複追加で 409**
- [x] 🔴 テスト: 存在しない献立で 404
- [x] 🔴 テスト: 未認証で 401
- [x] 🔴 テスト: 不正な献立IDで 400 / 壊れたボディで 400
- [x] 🟢 `POST /favorites` を実装
- [x] 🔧 実機確認（201 / 409 / 404 / 401 / 400）

> 重複と存在しない献立はどちらも DB の制約に判定させ、SQLSTATE と制約名で
> 振り分ける（23505 + favorites_user_menu_uniq → 409、23503 +
> favorites_menu_id_fkey → 404）。事前に SELECT で確かめる方式は確認と INSERT の
> 間に他リクエストが割り込むとすり抜けるため採らない。制約名まで見るのは
> user_id 側の外部キー違反と取り違えないため。
> レスポンスはお気に入り自体のIDを出さず menuId だけ返す。削除も
> `DELETE /favorites/:menuId` で献立IDを使う（7-C）ため、利用側に不要。

### PR 7-C: お気に入りの一覧と削除 `feat/api-list-delete-favorite` ✅

- [x] 🔴 テスト: 一覧が新しい順に返る
- [x] 🔴 テスト: 他ユーザーのものは返らない
- [x] 🔴 テスト: 削除できる
- [x] 🔴 テスト: 他ユーザーのもの削除は **403 ではなく 404**（下記）
- [x] 🔴 テスト: **15件を超えても自動削除されない**（履歴との違い）
- [x] 🟢 `GET /favorites` と `DELETE /favorites/:menuId` を実装
- [x] 🔧 実機確認（一覧の順序・ユーザー分離・204 / 404 / 400 / 401）

> **403 ではなく 404 にした。** 当初の想定は履歴に合わせた 403 だったが、
> お気に入りの削除はパスに献立ID（`:menuId`）を取る。削除は
> `WHERE user_id = $1 AND menu_id = $2` で絞るため、他人の行には構造上
> たどり着けず、403 を返す状態が発生しない。仮に返すと「その献立は
> 他人が登録している」と教えることになり、他人の登録内容が漏れる。
> 履歴が 403 なのは、グローバルな履歴IDで指定するため他人の行を
> 名指しでき、所有者確認が必要だから。指定子が違えば正しい応答も違う。
>
> 所有者を事前 SELECT せず `RowsAffected() == 0` で 404 を判定する
> （履歴と違い所有者確認が要らないので1クエリで済む）。
> 一覧は履歴と違い切り詰めをしない。並びは `created_at DESC`、
> 同値時は `menu_id DESC` をタイブレークにして順序を確定させる。

---

## フェーズ8: フロントエンド

> 完了条件: Vitest + Playwright が緑

### PR 8-A: テスト基盤 `feat/frontend-test-setup` ✅

- [x] 🔧 Vitest + Testing Library + MSW を導入
- [x] 🔧 CI に frontend のテストを追加
- [x] 🔴 サンプルテストが動くことの確認
- [x] 🟢 設定を実装

> MSW は `onUnhandledRequest: 'error'` で運用する。モックし忘れたAPI呼び出しが
> 素通りして「なぜか通った」状態になるのを防ぐため、未定義のリクエストは失敗させる。
> jsdom の URL を `http://localhost:5173` に固定するのは、`/api/...` の相対URLを
> 解決するため（既定のままだと fetch が Invalid URL になる）。
> `afterEach` で `resetHandlers` と `cleanup` を必ず走らせ、テスト間の依存を断つ。
> これが効いていることをテスト自体でも確認している（上書き → 次のテストで元に戻る）。

### PR 8-B: UI基盤 `feat/frontend-foundation` ✅

- [x] 🔧 Tailwind CSS を導入
- [x] 🔧 React Router を導入
- [x] 🔧 TanStack Query を導入
- [x] 🔴 ルーティングと Query の疎通テスト
- [x] 🔧 実機確認（dev server で / と /histories が 200）

> **OpenAPI の型生成は 8-B2 に分けた。** `api/openapi.yaml` がまだ無く、
> 全エンドポイントの仕様を書き起こす作業は分量も判断も別物のため。
>
> ルータは App の中に置かず、包む側（本番=BrowserRouter / テスト=MemoryRouter）を
> 差し替えられるようにした。QueryClient はモジュールスコープに1つ持つ
> （再描画のたびに作り直すとキャッシュが消えるため）。テストは
> `createTestQueryClient` で毎回新しいインスタンスを使い、キャッシュを持ち越さない。
>
> 🐛 依存を追加すると dev コンテナが起動しなくなる問題に当たった。
> node_modules が匿名ボリュームのため、イメージを再ビルドしても古いままになる。
> `make deps`（`docker compose up -d -V`）を追加した。

### PR 8-B2: OpenAPI から型を生成 `feat/api-types`

- [ ] 🔧 `api/openapi.yaml` を起こす
- [ ] 🔧 `openapi-typescript` で `src/api/schema.d.ts` を生成
- [ ] 🔧 生成物が最新かをCIで確認する

### PR 8-C: APIクライアント `feat/api-client`

- [ ] 🔴 テスト: Cookie が送信される
- [ ] 🔴 テスト: problem+json のエラーを解釈する
- [ ] 🔴 テスト: ネットワークエラーの扱い
- [ ] 🟢 `api/client.ts` を実装

### PR 8-D: 検索フォーム `feat/ui-search-form`

- [ ] 🔴 テスト: ジャンル4種が表示される
- [ ] 🔴 テスト: 難易度3種が表示される
- [ ] 🔴 テスト: 未選択（すべて）を選べる
- [ ] 🟢 検索フォームを実装

### PR 8-E: 検索結果 `feat/ui-search-result`

- [ ] 🔴 テスト: 検索ボタンで結果が表示される
- [ ] 🔴 テスト: ローディング表示
- [ ] 🔴 テスト: 422 でメッセージ表示
- [ ] 🔴 テスト: 「別の献立を見る」で引き直せる
- [ ] 🟢 検索結果表示を実装

### PR 8-F: レシピ表示 `feat/ui-recipes`

- [ ] 🔴 テスト: 3件表示される
- [ ] 🔴 テスト: **`target="_blank"` と `rel="noopener noreferrer"`**
- [ ] 🔴 テスト: 3件未満でも表示できる
- [ ] 🔴 テスト: **502でも献立表示は消えず、レシピ欄だけエラーとリトライ導線**
- [ ] 🟢 レシピ表示を実装

### PR 8-G: 週間献立画面 `feat/ui-weekly`

- [ ] 🔴 テスト: 7日分が表示される
- [ ] 🔴 テスト: 各日から引き直せる
- [ ] 🔴 テスト: 各日からレシピへ遷移できる
- [ ] 🟢 週間献立画面を実装

### PR 8-H: 認証画面 `feat/ui-auth`

- [ ] 🔴 テスト: サインアップの検証（メール形式、パスワード8文字）
- [ ] 🔴 テスト: ログインフォーム
- [ ] 🔴 テスト: 401 でエラー表示
- [ ] 🔴 テスト: Googleログインボタン
- [ ] 🟢 認証画面を実装

### PR 8-I: 認証ルーティング `feat/ui-auth-routing`

- [ ] 🔴 テスト: 未認証でも検索画面は使える
- [ ] 🔴 テスト: 未認証で履歴画面はログインへ誘導
- [ ] 🟢 ルーティングを実装

### PR 8-J: 履歴画面 `feat/ui-history`

- [ ] 🔴 テスト: 新しい順に表示
- [ ] 🔴 テスト: 0件のときの表示
- [ ] 🔴 テスト: 削除できる
- [ ] 🟢 履歴画面を実装

### PR 8-K: お気に入り画面 `feat/ui-favorites`

- [ ] 🔴 テスト: 追加・削除がトグルする
- [ ] 🔴 テスト: 一覧表示
- [ ] 🟢 お気に入り画面を実装

### PR 8-L: E2E `feat/e2e`

- [ ] 🔧 Playwright を導入し CI に追加（stub プロバイダで実行）
- [ ] 🔴 E2E: **和食×簡単 → 献立 → レシピ3件が新しいタブ**（PLAN.md の中核シナリオ）
- [ ] 🔴 E2E: サインアップ → 検索 → 履歴に残る
- [ ] 🔴 E2E: 週間献立の作成
- [ ] 🔴 E2E: お気に入りの追加と一覧

---

## フェーズ9: 仕上げ

> 完了条件: E2E全通過

### PR 9-A: レート制限 `feat/rate-limit`

- [ ] 🔴 テスト: 認証 10req/min/IP
- [ ] 🔴 テスト: 検索 60req/min/IP
- [ ] 🔴 テスト: 制限超過で 429
- [ ] 🔴 テスト: IPごとに独立している
- [ ] 🟢 レート制限ミドルウェアを実装

### PR 9-B: ロギング `feat/logging`

- [ ] 🔴 テスト: リクエストIDが全ログに伝播する
- [ ] 🔴 テスト: **パスワードがログに出ない**
- [ ] 🔴 テスト: **トークンがログに出ない**
- [ ] 🟢 ロギングミドルウェアを実装

### PR 9-C: 仕上げ `feat/hardening`

- [ ] 🔧 CORS が `FRONTEND_ORIGIN` のみ許可することの確認
- [ ] 🔧 フロントのエラーバウンダリ
- [ ] 🔧 404画面
- [ ] 🔧 README（起動手順、環境変数、アーキテクチャ図）
- [ ] 🔧 `recipe_link_caches` の要否を判断 ※spec.md 13章 未決事項3
- [ ] 🔧 応答時間の計測（検索 p95 200ms以内）

---

## 積み残し / 将来対応

spec.md 1.2 で MVP 対象外としたもの。

- [ ] アレルギー・苦手食材の除外
- [ ] 買い物リスト生成
- [ ] 朝食・昼食の献立（現在は夕食のみ）
- [ ] 献立のユーザー投稿
- [ ] 栄養価計算
- [ ] 本番デプロイ（Neon / Cloud Run / Cloudflare Pages）※spec.md 12章
