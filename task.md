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

### PR 4-D: 候補枯渇時の緩和 `feat/weekly-relaxation`

ここが週間献立の最も壊れやすい部分。単独のPRで集中的にテストする。

- [ ] 🔴 テスト: 候補が6件のとき緩和して同一献立の再利用を許す
- [ ] 🔴 テスト: 候補が1件のとき7日とも同じ献立になる（極端値）
- [ ] 🔴 テスト: 候補が全て同一ジャンルなら3連続禁止を緩和する ← **4-C の制約の解消**
- [ ] 🔴 テスト: 緩和が起きたことを呼び出し側が判別できる
- [ ] 🔴 テスト: 緩和は必要最小限（7件あるなら緩和しない）
- [ ] 🟢 段階的な緩和ロジックを実装

### PR 4-E: 履歴除外の連携 `feat/weekly-exclude-history`

- [ ] 🔴 テスト: 直近履歴に含まれる献立を避ける
- [ ] 🔴 テスト: 履歴を除くと7件に満たない場合は履歴除外を緩和する
- [ ] 🟢 履歴除外を実装（フェーズ6で実データと結線）

### PR 4-F: `POST /menus/suggest-weekly` `feat/api-suggest-weekly`

- [ ] 🔴 テスト: 200 と `week` 配列7件
- [ ] 🔴 テスト: 不正なリクエストボディで 400
- [ ] 🔴 テスト: 候補0件で 422
- [ ] 🟢 `MenuHandler.SuggestWeekly` を実装

### PR 4-G: 1日だけの引き直し `feat/api-reroll-day`

- [ ] 🔴 テスト: 指定日だけ変わり、他の日は保持される
- [ ] 🔴 テスト: 引き直し後も重複回避が効く
- [ ] 🔴 テスト: 範囲外の day で 400
- [ ] 🟢 引き直しAPIを実装
- [ ] 🔧 実機確認

---

## フェーズ5: 認証

> 完了条件: 認証境界のテストが緑

### PR 5-A: users / auth_identities `feat/auth-schema`

- [ ] 🔧 マイグレーション `000002_create_users`
- [ ] 🔴 テスト: CHECK制約（password なら hash 必須）
- [ ] 🔴 テスト: CHECK制約（google なら uid 必須）
- [ ] 🔴 テスト: UNIQUE (provider, provider_uid)
- [ ] 🔴 テスト: メールの UNIQUE
- [ ] 🔴 テスト: user 削除で identity も消える（CASCADE）
- [ ] 🟢 マイグレーションを実装

### PR 5-B: パスワードのハッシュ化 `feat/password-hashing`

- [ ] 🔴 テスト: ハッシュ化と検証（bcrypt cost 12）
- [ ] 🔴 テスト: 同じパスワードでも毎回異なるハッシュ
- [ ] 🔴 テスト: 誤ったパスワードで検証が失敗
- [ ] 🔴 テスト: 8文字未満を拒否
- [ ] 🔴 テスト: 72バイト超の扱い（bcryptの上限）
- [ ] 🟢 パスワードユーティリティを実装

### PR 5-C: サインアップ `feat/signup`

- [ ] 🔴 テスト: user と auth_identity が作られる
- [ ] 🔴 テスト: 登録済みメールで 409
- [ ] 🔴 テスト: メール形式が不正で 400
- [ ] 🔴 テスト: パスワードが短いと 400
- [ ] 🟢 `service.SignUp` と `POST /auth/signup` を実装

### PR 5-D: ログイン `feat/login`

- [ ] 🔴 テスト: 正しいパスワードで成功
- [ ] 🔴 テスト: 誤ったパスワードで 401
- [ ] 🔴 テスト: 存在しないメールで 401（存在時と**同一の**エラー＝ユーザー列挙対策）
- [ ] 🔴 テスト: Google のみのユーザーがパスワードログインを試みると 401
- [ ] 🟢 `service.Login` と `POST /auth/login` を実装

### PR 5-E: JWT の発行と検証 `feat/jwt`

- [ ] 🔴 テスト: アクセストークンの発行と検証
- [ ] 🔴 テスト: 有効期限切れを拒否（15分）
- [ ] 🔴 テスト: 署名が異なるトークンを拒否
- [ ] 🔴 テスト: `alg=none` を拒否
- [ ] 🔴 テスト: 改竄されたペイロードを拒否
- [ ] 🟢 JWT の発行・検証を実装

### PR 5-F: Cookie とリフレッシュ `feat/auth-cookie`

- [ ] 🔴 テスト: Cookie 属性（HttpOnly / Secure / SameSite=Lax）
- [ ] 🔴 テスト: リフレッシュトークンで再発行（30日）
- [ ] 🔴 テスト: 期限切れリフレッシュトークンで 401
- [ ] 🔴 テスト: ログアウトで Cookie が失効する
- [ ] 🟢 Cookie の発行・失効と `/auth/refresh`、`/auth/logout` を実装

### PR 5-G: 認証ミドルウェア `feat/auth-middleware`

- [ ] 🔴 テスト: 認証必須エンドポイントに未認証で 401
- [ ] 🔴 テスト: 有効なCookieでコンテキストにユーザーが入る
- [ ] 🔴 テスト: 献立検索は未認証でも 200（認証不要の確認）
- [ ] 🔴 テスト: `GET /auth/me` が現在のユーザーを返す
- [ ] 🟢 ミドルウェアと `/auth/me` を実装

### PR 5-H: Google SSO の認可URL `feat/google-auth-url`

- [ ] 🔧 Google Cloud Console で OAuth クライアントを作成（**要ユーザー操作**）
- [ ] 🔴 テスト: 認可URLに PKCE の code_challenge が含まれる
- [ ] 🔴 テスト: state が生成され Cookie に保存される
- [ ] 🔴 テスト: state は毎回異なる
- [ ] 🟢 `GET /auth/google` を実装

### PR 5-I: Google SSO のコールバック `feat/google-callback`

- [ ] 🔴 テスト: state 不一致で 401（CSRF対策）
- [ ] 🔴 テスト: state が無い場合も 401
- [ ] 🔴 テスト: code_verifier 不一致で 401
- [ ] 🔴 テスト: 初回コールバックで user が作られる
- [ ] 🔴 テスト: 2回目は既存 user に紐づく（重複作成しない）
- [ ] 🔴 テスト: **既存のパスワードユーザーと同じメール**なら同一userに identity を追加
- [ ] 🟢 `GET /auth/google/callback` を実装
- [ ] 🔧 実機確認: サインアップ→ログイン→/auth/me

---

## フェーズ6: 履歴

> 完了条件: 16件目でFIFOが働くテストが緑

### PR 6-A: search_histories `feat/history-schema`

- [ ] 🔧 マイグレーション `000003_create_search_histories`
- [ ] 🔧 INDEX (user_id, searched_at DESC)
- [ ] 🔴 テスト: 履歴を1件記録できる
- [ ] 🔴 テスト: user 削除で履歴も消える（CASCADE）
- [ ] 🟢 マイグレーションと Repository の記録を実装

### PR 6-B: FIFO 15件 `feat/history-fifo`

フェーズ6の核。単独のPRで集中的にテストする。

- [ ] 🔴 テスト: 15件までは削除されない（境界値）
- [ ] 🔴 テスト: **16件目の投入で最古が消え、ちょうど15件残る**
- [ ] 🔴 テスト: FIFOはユーザー単位（他ユーザーの履歴が消えない）
- [ ] 🔴 テスト: 21件を一度に入れても15件に収まる
- [ ] 🔴 テスト: `searched_at` が同値でも順序が安定する（タイブレーク）
- [ ] 🟢 `service.RecordHistory` を実装（アプリ層・トランザクション内）

### PR 6-C: 週間献立の一括記録 `feat/history-bulk-record`

- [ ] 🔴 テスト: 7件を1トランザクションで記録
- [ ] 🔴 テスト: FIFOが7回ではなく1回だけ走る
- [ ] 🔴 テスト: 途中で失敗したら全件ロールバック
- [ ] 🟢 一括記録を実装

### PR 6-D: `GET /histories` `feat/api-list-histories`

- [ ] 🔴 テスト: 新しい順に返る
- [ ] 🔴 テスト: 未認証で 401
- [ ] 🔴 テスト: 他ユーザーの履歴は返らない
- [ ] 🔴 テスト: 0件のとき空配列
- [ ] 🟢 `GET /histories` を実装

### PR 6-E: 履歴の削除 `feat/api-delete-histories`

- [ ] 🔴 テスト: 個別削除
- [ ] 🔴 テスト: 他ユーザーの履歴削除で 403
- [ ] 🔴 テスト: 存在しない履歴の削除で 404
- [ ] 🔴 テスト: 全件削除
- [ ] 🟢 `DELETE /histories/:id` と `DELETE /histories` を実装

### PR 6-F: 検索フローへの結線 `feat/wire-history`

- [ ] 🔴 テスト: 献立提案時に履歴が記録される
- [ ] 🔴 テスト: **未認証時は履歴を記録しない（エラーにしない）**
- [ ] 🔴 テスト: 検索時に直近履歴が除外候補として渡る
- [ ] 🔴 テスト: 履歴記録の失敗が献立提案を失敗させない
- [ ] 🟢 結線を実装
- [ ] 🔧 実機確認

---

## フェーズ7: お気に入り

> 完了条件: 重複追加が409になるテストが緑

### PR 7-A: favorites `feat/favorites-schema`

- [ ] 🔧 マイグレーション `000004_create_favorites`（UNIQUE (user_id, menu_id)）
- [ ] 🔴 テスト: UNIQUE 制約が効く
- [ ] 🔴 テスト: user 削除で消える（CASCADE）
- [ ] 🟢 マイグレーションを実装

### PR 7-B: お気に入りの追加 `feat/api-add-favorite`

- [ ] 🔴 テスト: 追加できる
- [ ] 🔴 テスト: **同一献立の重複追加で 409**
- [ ] 🔴 テスト: 存在しない献立で 404
- [ ] 🔴 テスト: 未認証で 401
- [ ] 🟢 `POST /favorites` を実装

### PR 7-C: お気に入りの一覧と削除 `feat/api-list-delete-favorite`

- [ ] 🔴 テスト: 一覧が新しい順に返る
- [ ] 🔴 テスト: 他ユーザーのものは返らない
- [ ] 🔴 テスト: 削除できる
- [ ] 🔴 テスト: 他ユーザーのもの削除で 403
- [ ] 🔴 テスト: **15件を超えても自動削除されない**（履歴との違い）
- [ ] 🟢 `GET /favorites` と `DELETE /favorites/:menuId` を実装
- [ ] 🔧 実機確認

---

## フェーズ8: フロントエンド

> 完了条件: Vitest + Playwright が緑

### PR 8-A: テスト基盤 `feat/frontend-test-setup`

- [ ] 🔧 Vitest + Testing Library + MSW を導入
- [ ] 🔧 CI に frontend のテストを追加
- [ ] 🔴 サンプルテストが動くことの確認
- [ ] 🟢 設定を実装

### PR 8-B: UI基盤 `feat/frontend-foundation`

- [ ] 🔧 Tailwind CSS を導入
- [ ] 🔧 React Router を導入
- [ ] 🔧 TanStack Query を導入
- [ ] 🔧 OpenAPI スキーマから型を生成

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
