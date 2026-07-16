# 実装タスクリスト

`spec.md` を細分化した作業単位。上から順に消化する。

## 進め方

- **1フェーズ = 1PR**。フェーズ着手時に `main` から `phase-N/xxx` ブランチを切る
- TDD を厳守する。`🔴` はテストを書いて**失敗を確認する**タスク、`🟢` は通す実装
- タスクを1つ終えるごとにこのファイルのチェックを更新する
- フェーズ完了条件を満たしたら PR を作成する。CI 全緑が必須
- マージ後に見つかったバグは Issue を立て、修正PRで `Closes #N` する

## 記号

| 記号 | 意味 |
| --- | --- |
| 🔴 | 失敗するテストを書く（Red） |
| 🟢 | テストを通す実装（Green） |
| 🔧 | リファクタ・設定・雑務 |
| ✅ | 完了 |

---

## フェーズ0: 環境構築 ✅

> 完了条件: `docker compose up` で Go が `/health` に200を返す
> PR #1（マージ済み）

- [x] 🔧 Go / Docker Desktop / make の導入
- [x] 🔧 `go mod init github.com/yuuyakim/menu-planner/backend`
- [x] 🔴 `/health` が200と `{"status":"ok"}` を返すテスト
- [x] 🟢 `HealthHandler` の実装
- [x] 🟢 `cmd/server/main.go`（echo、graceful shutdown、slog）
- [x] 🔧 backend Dockerfile（dev: air / prod: distroless）
- [x] 🔧 frontend スキャフォールド（Vite + React + TS）
- [x] 🔧 frontend Dockerfile と `/api` プロキシ設定
- [x] 🔧 `docker-compose.yml`（db / backend / frontend）
- [x] 🔧 `.env.example`（`SEARCH_API_PROVIDER=stub` を既定に）
- [x] 🔧 `Makefile`
- [x] 🔧 CI（backend / frontend / docker の3ジョブ）
- [x] 🔧 `.golangci.yml`（v2形式）

---

## フェーズ1: ドメイン層 + 献立マスタ

> 完了条件: repository の統合テストが緑
> ブランチ: `phase-1/domain-and-menu-master`

### 1-1. ドメイン型

- [x] 🔴 `Genre` の妥当性検証テスト（`japanese`/`western`/`chinese`/`other` と不正値）
- [x] 🟢 `domain.Genre` を実装（`ParseGenre`、`Valid()`、`String()`）
- [x] 🔴 `Difficulty` の妥当性検証テスト（`easy`/`normal`/`elaborate` と不正値）
- [x] 🟢 `domain.Difficulty` を実装
- [x] 🔴 `MenuID` のテスト（UUIDのパース、空文字の拒否）
- [x] 🟢 `domain.MenuID` を実装
- [x] 🔴 `Menu` エンティティのテスト（必須項目の検証）
- [x] 🟢 `domain.Menu` を実装
- [x] 🔴 `MenuFilter` のテスト（genre/difficulty が nil = 絞り込みなし）
- [x] 🟢 `domain.MenuFilter` を実装

### 1-2. DB基盤

- [x] 🔧 `golang-migrate` を導入し `make migrate` / `make migrate-down` を追加
- [x] 🔧 マイグレーション `000001_create_menus`（up/down）
- [x] 🔧 `menus` に INDEX (genre, difficulty) を張る
- [ ] 🔧 `sqlc` を導入し `sqlc.yaml` を作成
- [x] 🔧 pgx の接続プール初期化（Neonのコールドスタート対策でリトライ可能にする）
- [x] 🔴 接続プールのテスト（DSN不正時にエラーを返す）
- [x] 🟢 `db.NewPool` を実装

### 1-3. 献立マスタのシード

- [ ] 🔧 シードデータの形式を決める（SQL or JSON）
- [ ] 🔧 和食 × easy/normal/elaborate を各10件（計30件）
- [ ] 🔧 洋食 × easy/normal/elaborate を各10件（計30件）
- [ ] 🔧 中華 × easy/normal/elaborate を各10件（計30件）
- [ ] 🔧 その他 × easy/normal/elaborate を各10件（計30件）
- [ ] 🔧 `make seed` を追加（冪等にする。再実行で重複しないこと）
- [ ] 🔴 シード投入後に120件、各(genre,difficulty)が10件ずつあることの検証テスト
- [ ] 🟢 シード投入コマンドを実装

### 1-4. MenuRepository

- [ ] 🔧 testcontainers-go を導入し、テスト用Postgresの起動ヘルパを作る
- [ ] 🔧 テストヘルパ: 各テストでスキーマをクリーンにする仕組み
- [ ] 🔴 `FindByID` のテスト（存在する / 存在しない）
- [ ] 🟢 `FindByID` を実装
- [ ] 🔴 `FindByFilter` のテスト: genre のみ指定
- [ ] 🔴 `FindByFilter` のテスト: difficulty のみ指定
- [ ] 🔴 `FindByFilter` のテスト: 両方指定
- [ ] 🔴 `FindByFilter` のテスト: 両方 nil（全件返る）
- [ ] 🔴 `FindByFilter` のテスト: 該当0件（空スライスを返す。nilではない）
- [ ] 🟢 `FindByFilter` を実装
- [ ] 🔴 `FindByFilter` の除外指定テスト（`ExcludeIDs` で指定したIDが返らない）
- [ ] 🟢 `ExcludeIDs` に対応
- [ ] 🔧 service 側に `ports.go` を作り `MenuRepository` インターフェースを定義
- [ ] 🔧 CI に testcontainers が動く設定を追加（サービスコンテナ or Docker in Docker）

---

## フェーズ2: 献立検索（1食分）

> 完了条件: 絞り込み・候補枯渇の単体テストが緑
> ブランチ: `phase-2/suggest-single`

### 2-1. service

- [ ] 🔧 fake の `MenuRepository` を作る（テスト用）
- [ ] 🔴 `SuggestMenu` のテスト: genre 指定で該当ジャンルのみ返る
- [ ] 🔴 `SuggestMenu` のテスト: difficulty 指定で該当難易度のみ返る
- [ ] 🔴 `SuggestMenu` のテスト: 両方 nil で全件から選ばれる
- [ ] 🔴 `SuggestMenu` のテスト: 候補0件で `ErrNoMenuFound` を返す
- [ ] 🔴 `SuggestMenu` のテスト: 候補1件ならそれが返る
- [ ] 🔴 `SuggestMenu` のテスト: repository のエラーがラップされて返る
- [ ] 🟢 `service.SuggestMenu` を実装
- [ ] 🔴 ランダム選択のテスト（乱数源を注入し、決定的に検証できること）
- [ ] 🟢 乱数源をインターフェース化して注入する

### 2-2. handler

- [ ] 🔴 `GET /menus/suggest` のテスト: 200 とJSON構造
- [ ] 🔴 `GET /menus/suggest` のテスト: `?genre=japanese` がserviceに渡る
- [ ] 🔴 `GET /menus/suggest` のテスト: 不正な genre で 400
- [ ] 🔴 `GET /menus/suggest` のテスト: 不正な difficulty で 400
- [ ] 🔴 `GET /menus/suggest` のテスト: 候補0件で 422
- [ ] 🟢 `MenuHandler.Suggest` を実装
- [ ] 🔴 `GET /menus/:id` のテスト: 200 / 404 / 不正なUUIDで400
- [ ] 🟢 `MenuHandler.Get` を実装

### 2-3. 横断

- [ ] 🔧 RFC 7807 のエラーレスポンス型を作る
- [ ] 🔴 エラーハンドラのテスト（`problem+json` の Content-Type と各フィールド）
- [ ] 🟢 echo のカスタムエラーハンドラを実装
- [ ] 🔧 `main.go` に `/api/v1` のルーティングを結線
- [ ] 🔧 実機確認: `curl "localhost:8080/api/v1/menus/suggest?genre=japanese&difficulty=easy"`

---

## フェーズ3: レシピ取得

> 完了条件: 障害時フォールバックのテストが緑
> ブランチ: `phase-3/recipe-search`

- [ ] 🔧 検索APIを最終選定する（Brave / Google CSE）※spec.md 13章の未決事項1
- [ ] 🔴 `RecipeLink` 型のテスト（URL検証、ドメイン抽出）
- [ ] 🟢 `domain.RecipeLink` を実装
- [ ] 🔧 service に `RecipeSearchGateway` インターフェースを定義
- [ ] 🔴 stub gateway のテスト（決定的に3件返る）
- [ ] 🟢 stub gateway を実装（`SEARCH_API_PROVIDER=stub`）
- [ ] 🔴 実gateway のテスト: 正常系（httptest.Server でレスポンスをスタブ）
- [ ] 🔴 実gateway のテスト: 3件未満しか返らない場合
- [ ] 🔴 実gateway のテスト: 4件以上返っても3件に切り詰める
- [ ] 🔴 実gateway のテスト: HTTP 500 でエラー
- [ ] 🔴 実gateway のテスト: タイムアウト（3秒）
- [ ] 🔴 実gateway のテスト: 指数バックオフで最大2回リトライ
- [ ] 🔴 実gateway のテスト: 不正なJSONでエラー
- [ ] 🟢 実gateway を実装
- [ ] 🔧 環境変数で gateway を切り替えるファクトリ
- [ ] 🔴 `GET /menus/:id/recipes` のテスト: 200 と3件
- [ ] 🔴 `GET /menus/:id/recipes` のテスト: 存在しない献立で404
- [ ] 🔴 `GET /menus/:id/recipes` のテスト: gateway 障害で502
- [ ] 🟢 `MenuHandler.Recipes` を実装
- [ ] 🔧 実機確認: stub で3件返ること

---

## フェーズ4: 週間献立

> 完了条件: 重複回避と枯渇時の緩和テストが緑
> ブランチ: `phase-4/suggest-weekly`

- [ ] 🔧 週の開始曜日を決める（月曜固定 or 当日起点）※spec.md 13章の未決事項4
- [ ] 🔴 テスト: 7件返る
- [ ] 🔴 テスト: 同一献立が週内に2度出現しない
- [ ] 🔴 テスト: 同一ジャンルが3日以上連続しない
- [ ] 🔴 テスト: 直近履歴15件に含まれる献立を避ける
- [ ] 🔴 テスト: **候補がちょうど7件**のとき7件返る（境界値）
- [ ] 🔴 テスト: **候補が6件**のとき緩和ルールで同一献立の再利用を許す
- [ ] 🔴 テスト: **候補が1件**のとき7日とも同じ献立になる（緩和の極端値）
- [ ] 🔴 テスト: 候補0件で `ErrNoMenuFound`
- [ ] 🔴 テスト: 候補が全て同一ジャンルのとき、3連続禁止を緩和する
- [ ] 🔴 テスト: 緩和が起きたことを呼び出し側が判別できる（フラグ or ログ）
- [ ] 🟢 `service.SuggestWeekly` を実装
- [ ] 🔴 `POST /menus/suggest-weekly` のテスト: 200 と `week` 配列7件
- [ ] 🔴 テスト: `day` が 1..7 の連番
- [ ] 🔴 テスト: 不正なリクエストボディで400
- [ ] 🟢 `MenuHandler.SuggestWeekly` を実装
- [ ] 🔴 1日だけ引き直すテスト（他の日は保持、重複回避を再適用）
- [ ] 🟢 引き直しAPIを実装
- [ ] 🔧 実機確認

---

## フェーズ5: 認証

> 完了条件: 認証境界のテストが緑
> ブランチ: `phase-5/auth`

### 5-1. マイグレーション

- [ ] 🔧 `000002_create_users`（users / auth_identities）
- [ ] 🔧 CHECK制約（password なら hash 必須 / google なら uid 必須）
- [ ] 🔧 UNIQUE (provider, provider_uid)
- [ ] 🔴 制約の統合テスト（不正な組み合わせが弾かれる）

### 5-2. パスワード認証

- [ ] 🔴 テスト: パスワードのハッシュ化と検証（bcrypt cost 12）
- [ ] 🟢 パスワードハッシュのユーティリティを実装
- [ ] 🔴 テスト: 8文字未満のパスワードを拒否
- [ ] 🟢 パスワードの妥当性検証を実装
- [ ] 🔴 テスト: サインアップで user と auth_identity が作られる
- [ ] 🔴 テスト: 登録済みメールで409
- [ ] 🔴 テスト: メール形式が不正で400
- [ ] 🟢 `service.SignUp` を実装
- [ ] 🔴 テスト: 正しいパスワードでログイン成功
- [ ] 🔴 テスト: 誤ったパスワードで401
- [ ] 🔴 テスト: 存在しないメールで401（存在するメールと**同じ**エラーを返す＝ユーザー列挙対策）
- [ ] 🟢 `service.Login` を実装

### 5-3. JWT

- [ ] 🔴 テスト: アクセストークンの発行と検証
- [ ] 🔴 テスト: 有効期限切れトークンを拒否（15分）
- [ ] 🔴 テスト: 署名が異なるトークンを拒否
- [ ] 🔴 テスト: `alg=none` のトークンを拒否
- [ ] 🟢 JWT の発行・検証を実装
- [ ] 🔴 テスト: リフレッシュトークンで再発行（30日）
- [ ] 🟢 `service.Refresh` を実装
- [ ] 🔴 テスト: Cookie 属性（HttpOnly / Secure / SameSite=Lax）
- [ ] 🟢 Cookie の発行・失効を実装

### 5-4. Google SSO

- [ ] 🔧 Google Cloud Console で OAuth クライアントを作成（要ユーザー操作）
- [ ] 🔴 テスト: 認可URLの生成（PKCE の code_challenge、state を含む）
- [ ] 🟢 `GET /auth/google` を実装
- [ ] 🔴 テスト: state 不一致で401（CSRF対策）
- [ ] 🔴 テスト: code_verifier 不一致で401
- [ ] 🔴 テスト: 初回コールバックで user が作られる
- [ ] 🔴 テスト: 2回目は既存 user に紐づく（重複作成しない）
- [ ] 🔴 テスト: **既存のパスワードユーザーと同じメール**なら同一userに identity を追加
- [ ] 🟢 `GET /auth/google/callback` を実装

### 5-5. ミドルウェア

- [ ] 🔴 テスト: 認証必須エンドポイントに未認証で401
- [ ] 🔴 テスト: 有効なCookieでコンテキストにユーザーが入る
- [ ] 🟢 認証ミドルウェアを実装
- [ ] 🔴 テスト: 献立検索は未認証でも200（認証不要の確認）
- [ ] 🔴 テスト: `GET /auth/me` が現在のユーザーを返す
- [ ] 🟢 `/auth/me` と `/auth/logout` を実装
- [ ] 🔧 実機確認: サインアップ→ログイン→/auth/me

---

## フェーズ6: 履歴

> 完了条件: 16件目でFIFOが働くテストが緑
> ブランチ: `phase-6/history`

- [ ] 🔧 `000003_create_search_histories`
- [ ] 🔧 INDEX (user_id, searched_at DESC)
- [ ] 🔴 テスト: 履歴を1件記録できる
- [ ] 🔴 テスト: 15件までは削除されない（境界値）
- [ ] 🔴 テスト: **16件目の投入で最古が消え、ちょうど15件残る**
- [ ] 🔴 テスト: FIFOはユーザー単位（他ユーザーの履歴が消えない）
- [ ] 🔴 テスト: 週間献立の7件を1トランザクションで記録し、FIFOが1回だけ走る
- [ ] 🔴 テスト: 21件を一度に入れても15件に収まる
- [ ] 🟢 `service.RecordHistory` を実装（FIFOはアプリ層、トランザクション内）
- [ ] 🔴 テスト: `searched_at` が同値の場合でも順序が安定する（タイブレーク）
- [ ] 🟢 タイブレーク処理を実装
- [ ] 🔴 テスト: 履歴一覧が新しい順に返る
- [ ] 🔴 テスト: 未認証で401
- [ ] 🔴 テスト: 他ユーザーの履歴は返らない
- [ ] 🟢 `GET /histories` を実装
- [ ] 🔴 テスト: 履歴の個別削除
- [ ] 🔴 テスト: 他ユーザーの履歴削除で403
- [ ] 🔴 テスト: 履歴の全件削除
- [ ] 🟢 `DELETE /histories/:id` と `DELETE /histories` を実装
- [ ] 🔴 テスト: 献立提案時に履歴が記録される（結線の確認）
- [ ] 🔴 テスト: 未認証時は履歴を記録しない（エラーにしない）
- [ ] 🟢 検索フローに履歴記録を結線
- [ ] 🔴 テスト: 検索時に直近履歴が除外候補として渡る
- [ ] 🟢 履歴除外を検索に結線
- [ ] 🔧 実機確認

---

## フェーズ7: お気に入り

> 完了条件: 重複追加が409になるテストが緑
> ブランチ: `phase-7/favorites`

- [ ] 🔧 `000004_create_favorites`（UNIQUE (user_id, menu_id)）
- [ ] 🔴 テスト: お気に入り追加
- [ ] 🔴 テスト: **同一献立の重複追加で409**
- [ ] 🔴 テスト: 存在しない献立の追加で404
- [ ] 🔴 テスト: 未認証で401
- [ ] 🟢 `POST /favorites` を実装
- [ ] 🔴 テスト: 一覧が返る（新しい順）
- [ ] 🔴 テスト: 他ユーザーのお気に入りは返らない
- [ ] 🟢 `GET /favorites` を実装
- [ ] 🔴 テスト: 削除
- [ ] 🔴 テスト: 他ユーザーのお気に入り削除で403
- [ ] 🔴 テスト: 存在しないお気に入りの削除で404
- [ ] 🟢 `DELETE /favorites/:menuId` を実装
- [ ] 🔴 テスト: 履歴と違い自動削除されない（15件超でも残る）
- [ ] 🔧 実機確認

---

## フェーズ8: フロントエンド

> 完了条件: Vitest + Playwright が緑
> ブランチ: `phase-8/frontend`

### 8-1. 基盤

- [ ] 🔧 Vitest + Testing Library + MSW を導入
- [ ] 🔧 Playwright を導入
- [ ] 🔧 Tailwind CSS を導入
- [ ] 🔧 React Router を導入
- [ ] 🔧 TanStack Query を導入
- [ ] 🔧 OpenAPI スキーマから型を生成（`openapi-typescript`）
- [ ] 🔧 `api/client.ts`（Cookie送信、エラーを problem+json として解釈）
- [ ] 🔴 テスト: APIクライアントのエラーハンドリング
- [ ] 🟢 APIクライアントを実装

### 8-2. 献立検索画面

- [ ] 🔴 テスト: ジャンル選択UIが4種を表示
- [ ] 🔴 テスト: 難易度選択UIが3種を表示
- [ ] 🔴 テスト: 未選択（すべて）を選べる
- [ ] 🟢 検索フォームを実装
- [ ] 🔴 テスト: 検索ボタンで結果が表示される
- [ ] 🔴 テスト: ローディング表示
- [ ] 🔴 テスト: 422（候補なし）でメッセージ表示
- [ ] 🟢 検索結果表示を実装
- [ ] 🔴 テスト: 「別の献立を見る」で引き直せる
- [ ] 🟢 引き直しを実装

### 8-3. レシピ表示

- [ ] 🔴 テスト: レシピ3件が表示される
- [ ] 🔴 テスト: **リンクが `target="_blank"` と `rel="noopener noreferrer"` を持つ**
- [ ] 🔴 テスト: 3件未満でも表示できる
- [ ] 🔴 テスト: **502でも献立表示は消えず、レシピ欄だけエラーとリトライ導線**
- [ ] 🟢 レシピ表示を実装

### 8-4. 週間献立画面

- [ ] 🔴 テスト: 7日分が表示される
- [ ] 🔴 テスト: 各日から引き直せる
- [ ] 🔴 テスト: 各日からレシピへ遷移できる
- [ ] 🟢 週間献立画面を実装

### 8-5. 認証画面

- [ ] 🔴 テスト: サインアップフォームの検証（メール形式、パスワード8文字）
- [ ] 🔴 テスト: ログインフォーム
- [ ] 🔴 テスト: 401でエラー表示
- [ ] 🔴 テスト: Googleログインボタン
- [ ] 🟢 認証画面を実装
- [ ] 🔴 テスト: 未認証でも検索画面は使える
- [ ] 🔴 テスト: 未認証で履歴画面にアクセスするとログインへ誘導
- [ ] 🟢 認証状態によるルーティングを実装

### 8-6. 履歴・お気に入り画面

- [ ] 🔴 テスト: 履歴一覧が新しい順で表示
- [ ] 🔴 テスト: 履歴が0件のときの表示
- [ ] 🔴 テスト: 履歴の削除
- [ ] 🟢 履歴画面を実装
- [ ] 🔴 テスト: お気に入りの追加・削除がトグルする
- [ ] 🔴 テスト: お気に入り一覧
- [ ] 🟢 お気に入り画面を実装

### 8-7. E2E

- [ ] 🔴 E2E: 和食×簡単で検索 → 献立が出る → レシピ3件が出る（**PLAN.md の中核シナリオ**）
- [ ] 🔴 E2E: サインアップ → 検索 → 履歴に残る
- [ ] 🔴 E2E: 週間献立の作成
- [ ] 🔴 E2E: お気に入りの追加と一覧表示
- [ ] 🔧 CI に Playwright を追加（stub プロバイダで実行）

---

## フェーズ9: 仕上げ

> 完了条件: E2E全通過
> ブランチ: `phase-9/hardening`

- [ ] 🔴 テスト: レート制限（認証 10req/min/IP）
- [ ] 🔴 テスト: レート制限（検索 60req/min/IP）
- [ ] 🔴 テスト: 制限超過で429
- [ ] 🟢 レート制限ミドルウェアを実装
- [ ] 🔴 テスト: リクエストIDが全ログに伝播する
- [ ] 🔴 テスト: **パスワード・トークンがログに出ない**
- [ ] 🟢 ロギングミドルウェアを実装
- [ ] 🔧 CORS が `FRONTEND_ORIGIN` のみ許可することの確認
- [ ] 🔧 フロントのエラーバウンダリ
- [ ] 🔧 404画面
- [ ] 🔧 README（起動手順、環境変数、アーキテクチャ図）
- [ ] 🔧 `recipe_link_caches` の要否を判断 ※spec.md 13章の未決事項3
- [ ] 🔧 応答時間の計測（検索 p95 200ms以内）
- [ ] 🔧 E2E 全通過の確認

---

## 積み残し / 将来対応

spec.md 1.2 で MVP 対象外としたもの。

- [ ] アレルギー・苦手食材の除外
- [ ] 買い物リスト生成
- [ ] 朝食・昼食の献立（現在は夕食のみ）
- [ ] 献立のユーザー投稿
- [ ] 栄養価計算
- [ ] 本番デプロイ（Neon / Cloud Run / Cloudflare Pages）※spec.md 12章
