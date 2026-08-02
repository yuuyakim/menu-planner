# サブスクリプションの撤廃（ゲート解放方式）

- 日付: 2026-08-02
- ブランチ: `feat/remove-subscription`（`main` = b9eea64 から分岐）

## 背景と目的

現在、以下の機能をプレミアムプラン（月額300円）の加入者に限定している。

- 1週間の献立の提案・引き直し（`/weekly`）
- 週間献立の保存と保存一覧（`/saved-weekly`）
- 買い物リストのチェック状態の永続化

このサブスクリプションを撤廃し、**ログインしていれば誰でも上記を使える**状態にする。

Stripe の本番加入者は存在しない（テストのみ）。したがって既存契約の解約・返金は不要で、
アプリ側の変更だけで完結する。

## 方針

**ゲートだけを外し、課金の配管は backend に残す。**

権限の判定は `domain.Entitlement` の3メソッドに集約されている。ここが常に「使える」を
返すようにすれば、呼び出し側（`RequirePremium` ミドルウェア、service の権限チェック）は
一切触らずに全機能が開放される。

この方式を採る理由は復活の容易さにある。ゲートを呼び出し側から削除すると、変更が
handler と service に広く散り、将来サブスクを再開するときに「どこにゲートを差していたか」
を探し直すことになる。1ファイルに閉じ込めておけば、そのファイルを戻すだけで復元できる。

一方、**frontend の課金画面は削除する。** ルートを消した時点で到達不能になり、残しても
テストだけが宙に浮く。逆にルートを残すと、導線が無くても URL 直打ちで加入できてしまい、
「撤廃」と矛盾する。backend の配管が残っているので、復活時に書き直すのは画面だけで済む。

## 1. 利用者から見た変化

| 機能 | 変更前 | 変更後 |
| --- | --- | --- |
| 1週間の献立の提案・引き直し | premium 限定 | ログインすれば誰でも |
| 週間献立の保存・保存一覧 | premium 限定 | ログインすれば誰でも |
| 週間献立の保存上限 | free 10件 / premium 50件 | 全員 50件 |
| 買い物リストのチェック永続化 | premium 限定 | ログインすれば誰でも |
| 料金プラン `/pricing` | あり | 削除 |
| 加入 `/checkout` `/checkout/complete` | あり | 削除 |
| プラン管理 `/account` | あり | 削除 |
| 特定商取引法に基づく表記 | フッターに常設 | 導線とルートを削除（ファイルは残す） |

**週間献立はログイン必須のままとする。** `backend/internal/handler/menu.go` の
`suggest-weekly` / `reroll-day` は `RequireAuth` → `RequirePremium` の順で守られており、
プレミアム限定になった時点で未ログインは 401 になっている。プレミアムのゲートを外しても
`RequireAuth` は残るため、未ログインでは引き続き使えない。

したがって frontend では `/weekly` を `RequireAuth` で包む。包まずにゲートだけ外すと、
未ログインの人に入力フォームが見えて、送信して初めて 401 で失敗する画面になる。
`/saved-weekly` は既に `RequireAuth` の内側にあり、変更は要らない。

`App.tsx` の「検索と週間献立は未認証でも使える（spec.md 1.3）」というコメントは、
プレミアム化以前の記述が残ったもの。実態に合わせて直す。

## 2. backend

### 2.1 変更するファイル

`backend/internal/domain/entitlement.go` のみ。

```go
// 上限はプランによらず一律。
const savedWeeklyMenuLimit = 50

func (e Entitlement) SavedWeeklyMenuLimit() int { return savedWeeklyMenuLimit }
func (e Entitlement) CanPersistShoppingList() bool { return true }
func (e Entitlement) CanUseWeeklyPlanning() bool { return true }
```

`Plan()` と `plan` フィールドは残す。`/auth/me` がプラン名を返しており、DB の
`subscriptions.plan` の値をそのまま表す役割は変わらないため。

`freeSavedWeeklyMenuLimit` / `premiumSavedWeeklyMenuLimit` の2定数は
`savedWeeklyMenuLimit` の1つに統合する。

### 2.2 触らないもの

- `handler/middleware.go` の `RequirePremium`（常に通過するが、位置ごと保存する）
- `service/saved_weekly.go` / `service/saved_shopping_list.go` の権限チェック
- `handler/billing.go` / `service/billing.go` / `service/subscription.go`
- `repository/subscription.go` / `repository/payment_stripe.go`
- migration `000010_create_subscriptions` / `000012_add_provider_customer_id`
- `api/openapi.yaml`

`/billing/*` エンドポイントは生き続ける。UI からは呼ばれないが、Stripe webhook の
受け口を含めて配管として機能する状態を保つ。openapi 上の 403 応答も残す
（実際には到達しないが、ゲートを戻せば再び有効になる記述であるため）。

`cmd/server/main.go` は `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` /
`STRIPE_PRICE_ID` が未設定だと起動時に落ちる。この挙動も変えない。配管を残す以上、
設定が欠けたまま起動できる方が危険なため。**デプロイには引き続き Stripe の
環境変数が必要**であり、`.env.example` の `STRIPE_*` も据え置く。

### 2.3 テスト

403 を期待しているテストは削除せず「free でも通る」に書き換える。ゲートの配管が
残っている以上、その配管が全員を通すことを検証する価値があるため。

- `domain/entitlement_test.go` — free / premium / ゼロ値のいずれでも同じ結果を返すこと
- `handler/middleware_test.go` — `RequirePremium` が free を通すこと
- `handler/menu_test.go` — `suggest-weekly` / `reroll-day` が free で 200
- `handler/saved_weekly_test.go` — 保存・一覧・削除が free で成功
- `handler/saved_shopping_list_test.go` — GET/PUT が free で成功
- `service/saved_weekly_test.go` — 上限が 50 であること
- `service/saved_shopping_list_test.go` — free でも永続化できること
- `handler/contract_test.go` / `handler/problem_coverage_test.go` — premium 由来の
  403 ケースが残っていれば整理する

## 3. frontend

### 3.1 削除するディレクトリ

- `src/features/billing/`（`CheckoutPage` / `CheckoutCompletePage` / `AccountPage` / `api.ts` と各テスト）
- `src/features/pricing/`（`PricingPage` と テスト）
- `src/features/premium/`（`PremiumLock` と テスト）

### 3.2 ゲートの解除

| ファイル | 変更 |
| --- | --- |
| `features/menu/WeeklyPage.tsx` | `user?.plan !== 'premium'` の分岐と `PremiumLock` 描画を削除 |
| `features/menu/SavedWeeklyPage.tsx` | 同上。クエリの `enabled: user?.plan === 'premium'` はログイン判定に変更 |
| `features/menu/ShoppingListPage.tsx` | `canPersist` を `savedId != null` のみに。`/checkout` バナーと未加入者向けガイダンス（`useOnceFlag` 由来）を削除 |
| `features/home/HomePage.tsx` | `/pricing` への常設導線を削除 |
| `components/Footer.tsx` | 「料金プラン」「特定商取引法に基づく表記」のリンクを削除 |
| `features/auth/AuthMenu.tsx` | 「プレミアム会員」バッジと `/account` 導線を削除 |
| `app/App.tsx` | `/checkout` `/checkout/complete` `/account` `/pricing` `/legal/tokushoho` のルートと import を削除。`/weekly` を `RequireAuth` で包む |
| `test/handlers.ts` | `/billing/plan` の MSW ハンドラを削除 |

`User.plan` は自動的に残る。`api/types.ts` の `User` は `schema.d.ts`（openapi から
生成）の別名であり、openapi を触らない以上 `plan` フィールドも変わらない。参照する
画面が無くなるだけで、明示的な作業は不要。

`hooks/useOnceFlag.ts` は削除する。利用箇所は `ShoppingListPage` の
`useOnceFlag('premium-shopping')` のみで、プレミアム誘導を一度だけ出すために
導入されたフックであり、ガイダンスと同時に役目を終える。`useOnceFlag.test.ts` も削除。

### 3.3 unit test

`WeeklyPage.test.tsx` / `SavedWeeklyPage.test.tsx` / `ShoppingListPage.test.tsx` /
`HomePage.test.tsx` / `AuthMenu.test.tsx` / `App.auth.test.tsx` /
`App.favorite-prompt.test.tsx` から premium 分岐のケースを削除する。

`App.checkout.test.tsx` は対象画面ごと消えるため削除。`App.account-switch.test.tsx`
はアカウント切替の検証で premium とは無関係のため触らない（`me` のスタブに `plan`
が含まれる場合はそのままでよい）。

## 4. e2e

- 削除: `premium.spec.ts`（加入導線の通し）、`weekly-premium.spec.ts`（ロック表示）
- 書き換え: `weekly.spec.ts` / `saved-weekly.spec.ts` / `shopping-list.spec.ts` /
  `shopping-list-persist.spec.ts` / `menu-role.spec.ts` から、premium 付与
  （`grantPremium`）とその呼び出しを削除し、素のログインに戻す

`grantPremium` は共通ヘルパではなく、各 spec が個別に定義している
（`docker compose run --rm backend go run ./cmd/grant` を叩く関数）。`e2e/helpers.ts`
には無いため、削除は spec ごとに行う。

`backend/cmd/grant`（プレミアムを手で付与する CLI）は**残す**。課金配管の一部であり、
配管を残す方針と揃える。e2e から呼ばれなくなるだけ。

CI（`.github/workflows/ci.yml`）には premium 付与に固有のステップは無いため、変更不要。

## 5. 法務

### 5.1 利用規約 `features/legal/content/terms.md`

- **第4条（プレミアムプラン）を全削除**し、以降の条番号を繰り上げる
- 第2条3項の「プレミアムプラン」の定義を削除
- 第6条2項（サービス停止時に利用料金を日割返金する旨）を、無料サービスを前提とした
  記述に整理する
- 第8条2項の賠償上限「支払った利用料金の総額（3,000円に満たない場合は3,000円）」は
  無料でも 3,000円 として成立するが、前提となる利用料金が存在しなくなるため
  「3,000円を上限とする」と直接書く形に整える

条番号の繰り上げにより、他条からの相互参照がずれないか通しで確認する。

### 5.2 特定商取引法に基づく表記

有料の役務提供が無くなるため表示義務も無くなる。`content/tokushoho.md` と
`TokushohoPage.tsx` は**ファイルとして残し**、`App.tsx` のルートと `Footer.tsx` の
導線だけを外す。`e2e/legal-pages.spec.ts` から特商法ページのケースを削除する。

`docs/legal/` 配下（`checkout-display.md` など）は資料として残し、触らない。

## 6. ドキュメント

- `spec.md` — プレミアム限定の記述（週間献立・保存・買い物リストの節、保存上限、
  加入導線、料金の提示）を無料前提に更新する
- `README.md` — 課金機能の記述を更新する
- `DEPLOY.md` — Stripe の環境変数は引き続き必要である旨を明記する
  （配管を残すため設定は消せない、という理由まで書く）
- `docs/manual-e2e-payment.md` — 手動決済確認の手順。撤廃後は実施しないため、
  冒頭に「現在サブスクは撤廃済み。復活時に使う」と注記する

## 7. やらないこと

- Stripe ダッシュボード側の操作（本番加入者が居ないため不要）
- `subscriptions` テーブルおよび migration の削除
- backend の課金コードの削除（`cmd/grant` を含む）
- `STRIPE_*` 環境変数の撤去
- `api/openapi.yaml` の変更

## 8. 検証

- `make test`（backend の Go テスト、frontend の unit テスト）が通る
- `make lint` / golangci-lint が通る（未使用シンボルの警告に注意）
- e2e が通る
- 手で確認する動線:
  1. 未ログインで `/weekly` を開くとログイン画面へ送られる
  2. ログインして週間献立が提案でき、保存でき、`/saved-weekly` に出る
  3. 買い物リストのチェックがリロード後も残る
  4. フッターと画面のどこにも料金・加入への導線が無い
  5. `/pricing` `/checkout` `/account` `/legal/tokushoho` が 404 になる
