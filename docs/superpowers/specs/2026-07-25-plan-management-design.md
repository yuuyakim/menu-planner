# プラン管理・解約画面（アカウント設定 > プランの管理）設計

- 日付: 2026-07-25
- 対象: `menu-planner`（献立くん）
- 状態: 設計確定・未実装
- 前提:
  - `2026-07-25-payment-core-design.md`（決済コア。subscriptions 同期・Webhook・PaymentGateway port・BillingService）。**本設計はその上に積む**。
  - `frontend/src/features/legal/content/{tokushoho,terms}.md`（特商法・利用規約が解約導線「アカウント設定 > プランの管理」を名指し＝本画面はローンチ必須）。
- ブランチ: `feat/payment-core` に継続（launch バンドルとして決済コアと一緒にマージする）。

## 1. 背景と目的

決済コアで加入・課金・Webhook 同期は実装したが、**利用者が自分で解約する導線が無い**。特商法・利用規約は解約方法を「アプリ内『アカウント設定 > プランの管理』から」と明記しており、この画面が無いまま実課金を開始できない（ローンチゲート）。本サブプロジェクトはその「プランの管理」画面と、解約・支払い方法変更・請求履歴を **Stripe 顧客ポータル**で提供する導線を実装する。

`SubscriptionService.Revoke` は即時失効・管理者用であり利用者都合の解約ではない（コード内に明記）。利用者の解約は**期末解約**（`cancel_at_period_end`、利用規約4条5項）で、Stripe ポータル側の操作 → 既存 Webhook で同期する。

## 2. スコープ

### 対象
- 新ルート `/account`（要ログイン）＝「アカウント設定」画面。中に「プランの管理」セクション。
- 現在のプラン状態の表示（無料 / トライアル中 / 有効 / 解約予約中 / お支払い確認中）。
- 「プランを管理する」ボタン → **Stripe 顧客ポータル**（解約=期末・支払い方法の更新・請求履歴）。
- 無料利用者へのアップグレード導線（既存 `/checkout` へ）。
- バックエンド: `GET /billing/subscription`（表示用）、`POST /billing/portal-session`（ポータルセッション作成）。
- AuthMenu からの導線追加。

### 対象外
- **解約・カード変更・請求履歴の自前UI実装**（すべて Stripe ポータルが担う）。
- 解約の取り消し（resume）の自前実装（ポータルで可能）。
- 料金プラン LP（別サブプロジェクト）。
- Webhook の新規実装（決済コアの `HandleWebhook` がポータル由来の `subscription.updated`/`deleted` をそのまま同期する。新イベント購読は不要）。
- live 課金の有効化そのもの（本設計は実装・テストまで。ポータルの本番設定は §7 のローンチ手当てで触れる）。

## 3. 設計判断

### 3.1 操作は Stripe 顧客ポータルに委譲する（決定）

解約・支払い方法の更新・請求履歴を自前で作らず、Stripe 顧客ポータル（ホスト画面）に委ねる。カード情報を自社に通さない方針（決済コア 3.1）と一致し、実装量が小さく、past_due 利用者がカード更新で復帰できる。アプリ内「プランの管理」画面は**状態表示＋ポータルへの入口**を持ち、特商法の「アプリ内で解約できる」要件を満たす（Checkout と同じく、操作の入口はアプリ内、実行は Stripe ホスト画面）。

### 3.2 状態表示は自前、真実は subscriptions（決定）

`GET /billing/subscription` が `subscriptions` 行（決済コアで Webhook 同期済み）を読んで表示用の値を返す。プランの真偽は既存 `EntitlementService`（`GivesPremiumAt`：trialing・past_due 猶予も premium）に合わせ、生の `status` も併せて返して画面のラベルを出し分ける。

### 3.3 ポータル対象は Stripe 顧客がある加入のみ（決定）

`provider_customer_id` が無い加入（手動付与 `provider=manual`、または未加入）はポータルを開けない。この場合「プランを管理する」ボタンを出さず、状態のみ表示する。`portal-session` を叩かれた場合は `ErrNoBillingCustomer` を返す（フェイルセーフ）。

## 4. バックエンド API（`/api/v1/billing`）

### 4.1 `GET /billing/subscription`（要ログイン）

表示用。`store.Find(userID)` を引き、無ければ free を返す。

レスポンス:
```json
{
  "plan": "premium",
  "status": "active",
  "currentPeriodEnd": "2026-08-25T00:00:00Z",
  "cancelAtPeriodEnd": false,
  "hasPortal": true
}
```
- `plan`: `EntitlementService.For` 由来（`premium`/`free`。trialing・past_due 猶予は premium）。
- `status`: 生の加入状態（`none`（行なし）/`trialing`/`active`/`past_due`/`canceled`）。
- `currentPeriodEnd`: 行があれば RFC3339、無ければ null。
- `cancelAtPeriodEnd`: 行の値（無ければ false）。
- `hasPortal`: `provider_customer_id` が非空なら true（＝「プランを管理する」ボタンを出す条件）。

### 4.2 `POST /billing/portal-session`（要ログイン）

1. `store.Find(userID)`。行が無い、または `provider_customer_id` が空 → `ErrNoBillingCustomer`（409 `no-billing-customer`）。
2. `PaymentGateway.CreateBillingPortalSession(ctx, customerID, returnURL)` を呼ぶ。`returnURL = {FRONTEND_ORIGIN}/account`。
3. `{ "url": "<portal URL>" }` を返す。フロントがそこへリダイレクト。

### 4.3 port / adapter

`service.PaymentGateway` にメソッド追加:
```go
CreateBillingPortalSession(ctx context.Context, customerID, returnURL string) (string, error)
```
Stripe adapter（`repository/payment_stripe.go`）で `billingportal/session` を用いて実装（`stripe-go` 隔離は維持）。

### 4.4 サービス層・エラー

`BillingService` にメソッド追加:
- `Subscription(ctx, userID) (SubscriptionView, error)` — §4.1 の値を組む。
- `CreatePortalSession(ctx, userID) (url string, error)` — §4.2 のロジック。

`ErrNoBillingCustomer`（Stripe 顧客が無い）を新設し、`problem.go` に 409 `no-billing-customer` を写像（`problem_coverage_test` 対応）。

## 5. フロントエンド

- 新ルート `/account`（`RequireAuth`）。AuthMenu に「アカウント設定」リンクを追加。
- `AccountPage` が「プランの管理」セクションを描画（`GET /billing/subscription` を取得）。状態別表示:

| status / 条件 | 表示 | ボタン |
| --- | --- | --- |
| `none`（無料） | 「無料プラン」 | 「プレミアムにアップグレード」→ `/checkout` |
| `trialing` | 「プレミアム（無料お試し中）／初回課金 〈currentPeriodEnd〉」 | 「プランを管理する」 |
| `active`（cancelAtPeriodEnd=false） | 「プレミアム／次回請求 〈currentPeriodEnd〉」 | 「プランを管理する」 |
| `active`（cancelAtPeriodEnd=true） | 「〈currentPeriodEnd〉で解約予定（それまで利用可）」 | 「プランを管理する」 |
| `past_due` | 「お支払いの確認中。カード情報の更新をお願いします」 | 「プランを管理する」（＝カード更新） |
| `canceled` | 「解約済み（無料プラン）」 | 「プレミアムにアップグレード」→ `/checkout` |

- `hasPortal=false`（手動付与など）: 状態のみ表示し「プランを管理する」は出さない。
- 「プランを管理する」→ `POST /billing/portal-session` → 返った `url` へ `window.location` で遷移。ポータルから戻ると `/account`。
- 日時は JST 表示（`/checkout` と同じ整形）。API 由来のエラーは既存 `ErrorMessage`。

## 6. テスト戦略

| 層 | 検証 |
| --- | --- |
| service | `CreatePortalSession`: customer 無し→`ErrNoBillingCustomer` / 有り→gateway を正しい customerID・returnURL で呼び url を返す |
| service | `Subscription`: 行なし→free/none/hasPortal=false、premium(active)→各フィールド、past_due・cancelAtPeriodEnd の反映 |
| adapter | `CreateBillingPortalSession` の Stripe 呼び出し（薄く。ネットワークは実行しない） |
| handler | `GET /billing/subscription`（401/200）、`POST /billing/portal-session`（401 / 409 no-billing-customer / 200 url） |
| handler | `problem_coverage_test` に `no-billing-customer`(409) |
| frontend | `AccountPage` の各状態の表示・出し分け、「プランを管理する」→ portal リダイレクト、無料→/checkout 導線、hasPortal=false でボタン非表示（MSW） |

**実ポータルの E2E は CI に入れない**。手動E2E（テストモード）で、ポータルの期末解約 → `subscription.updated`(cancel_at_period_end=true) → 画面「〈日〉で解約予定」→ 期末 `deleted` → free、およびカード変更を確認する（`docs/manual-e2e-payment.md` に追記）。

## 7. ローンチ手当て・前提

- **Stripe ダッシュボードで顧客ポータルを有効化**（設定 → Billing → 顧客ポータル）: 解約=**期末**、支払い方法の更新、請求履歴、戻りURL（`/account`）。**test・live 両方**で設定。
- 決済コアの Webhook（`subscription.updated`/`deleted`）が既に購読済みなので追加登録は不要。
- これで特商法・利用規約の「アカウント設定 > プランの管理」からの解約が実在＝ローンチゲートを1つ解消。
- マイグレーション不要（スキーマ変更なし）。

## 8. 未決事項

- 「アカウント設定」ページに将来ほかの設定（メール変更等）を載せるかは未定。今回はプランの管理セクションのみ。
- ポータルの return_url 後、画面が最新状態を反映するためのリロード/再取得のタイミング（実装時に確定。`/account` 再訪で `GET /billing/subscription` を取り直せば足りる想定）。
