# 決済コア（Stripe サブスクリプション課金）設計

- 日付: 2026-07-25
- 対象: `menu-planner`（献立くん）
- 状態: 設計確定・未実装
- 前提:
  - `2026-07-23-premium-entitlement-design.md`（エンタイトルメント基盤。subscriptions テーブル、読み取り時算出、cancel=状態遷移）
  - `2026-07-23-premium-plan-split-design.md`（free/premium の機能線と in-context アップグレード CTA）
  - `docs/legal/checkout-display.md`（特商法12条の6：申込み最終確認画面の表示要件）
  - `frontend/src/features/legal/content/{tokushoho,terms,privacy}.md`（本番公開済みの法務文書＝課金の確定条件）

## 1. 背景と目的

サブスク基盤（`subscriptions` テーブル・`SubscriptionStore.Find/Upsert`・`SubscriptionService.Grant/Revoke`・
`EntitlementService.For`）は完成しているが、**決済経路が一切存在しない**。現在プレミアムは CLI
（`cmd/grant`）による手動付与でしか有効化できない。本サブプロジェクトは、利用者が自分で
**Stripe を通じてプレミアムに加入し、正しく課金される**購入フロー一式を実装する。

基盤は最初から「CLI と将来の決済 Webhook が同じ service 層を通る」設計であり、本設計はその
Webhook 経路を Stripe で実装する。

## 2. スコープ

### 対象（購入フロー一式）
- 申込み最終確認画面（特商法12条の6）＝自前画面。`/checkout`。
- Stripe Checkout（ホスト型・subscription モード）への受け渡しと復帰処理。
- Stripe Webhook 受信 → `subscriptions` 行への状態同期（真実の源）。
- 5日間トライアル（初回のみ）。
- `past_due` 猶予（利用規約4条7項の7日）と `trialing` を premium と評価するエンタイトルメント修正。
- Stripe 連携の env・adapter・`stripe-go` 依存の追加。

### 対象外（別サブプロジェクト）
- **解約・プラン管理画面「アカウント設定 > プランの管理」**。ただし特商法/利用規約が名指しする
  ローンチ必須要素であり、確認画面はこれを参照する（§10 のローンチゲート）。
- 料金プラン LP（マーケティング用ページ）。
- Stripe Customer Portal の導入判断（解約サブプロジェクトで扱う）。
- live（本番）課金の有効化。本設計の実装・テストはすべて Stripe **テストモード**で行う。

## 3. 設計判断

### 3.1 Stripe Checkout（ホスト型）を使う（決定）

カード入力は Stripe のホストページに任せ、自前は申込み確認画面までを持つ。`checkout-display.md §1`
が「Stripe Checkout 自体を法的な最終確認画面にしない。自前の確認画面を挟み、Checkout はカード
入力のみ」と規定しているため、この分担が要件に合致する。カード番号が自社サーバを一切通らないので、
利用規約の「当方はクレジットカード番号を保持しません」を自然に満たす（PCI 範囲を最小化）。

Payment Element（埋め込み型）は PCI 範囲と実装量が増え、¥300 サブスクに対して過剰。Payment Links は
ユーザー紐付け（`client_reference_id`）やトライアル可否の動的出し分けに不向き。いずれも採らない。

### 3.2 真実の源は Webhook（決定）

`subscriptions` 行の書き込みは **Stripe Webhook を唯一の権威**とする。Checkout 成功後の
`success_url`（復帰ページ）は行を書かず、`/auth/me` の `plan` をポーリングして反映を待つ。
Webhook は非同期のため数秒のラグが出るが、UI 側で吸収する。復帰リダイレクトの取りこぼし
（ユーザーがタブを閉じる等）に対しても Webhook が状態を確定させるため、経路が二重化される。

### 3.3 Stripe SDK は adapter に隔離する（決定）

service / handler は自前の `PaymentGateway` port 越しに決済を叩き、`stripe-go` は薄い adapter に
閉じ込める。Webhook イベントは adapter で署名検証・正規化してから service に渡す。これにより
service のテストは fake gateway で差し替え可能になり、**CI で実 Stripe を叩かない**。

### 3.4 Webhook は Stripe の値をそのまま書く（決定）

`SubscriptionService.Grant` は月数計算で期末を延ばすが、Webhook 経路では使わない。Stripe が返す
`status` / `current_period_end` / `cancel_at_period_end` を**そのまま** `subscriptions` に upsert する
（真実は Stripe 側にあるため、二重に計算しない）。`Grant/Revoke` は CLI 手動付与用に据え置く。

### 3.5 トライアルは初回加入のみ（決定）

利用規約「再申込みには（トライアルが）付かない」を満たすため、**加入行が過去に一度も存在しない
ユーザーだけ**にトライアルを付与する。行は解約後も削除されず `canceled` で残る（基盤設計 = cancel は
状態遷移）ため、「トライアル→解約→再申込」は自動的にトライアル無し（即課金）になる。手動付与済み
ユーザーも行を持つのでトライアル対象外（実害なし）。

### 3.6 「解約予約中」に特別扱いを入れない（決定）

利用者が期末解約すると Stripe は期間末まで `status=active, cancel_at_period_end=true` を保つため、
エンタイトルメントの通常の active 分岐でそのまま premium と評価される。期末に Stripe が
`customer.subscription.deleted` を発火し `canceled` へ落とすと free になる。利用規約「解約は当該課金
期間の末日に発効」と自動的に一致するため、`cancel_at_period_end` を読む特別分岐は不要。

## 4. データモデル

### 4.1 マイグレーション `000012`

`subscriptions` に Stripe Customer ID を1列追加する。

```sql
-- up
ALTER TABLE subscriptions ADD COLUMN provider_customer_id text;
-- down
ALTER TABLE subscriptions DROP COLUMN provider_customer_id;
```

- 顧客の再利用（同一ユーザーの再申込で Customer を作り直さない）と、将来の解約/ポータル画面で必要。
- null 可（`manual` 付与では空）。
- `status` は元々 CHECK 制約が無い自由テキストのため、`trialing` 値の保存に DDL 変更は不要。
- **本番 Neon で手動実行**してからバックエンドをデプロイする（`Find` が新列を SELECT するため。000011 と同じ運用）。

### 4.2 ドメイン

- `SubscriptionTrialing SubscriptionStatus = "trialing"` を追加。`ParseSubscriptionStatus` / `Valid()` に組み込む。
- `ProviderStripe = "stripe"` 定数を追加。
- `Subscription` 構造体に `ProviderCustomerID string` を追加。`repository.Find` の SELECT と `Upsert` の
  INSERT/UPDATE を1列ぶん拡張（空文字は SQL NULL として書く。既存の `provider_subscription_id` と同じ扱い）。

### 4.3 エンタイトルメント判定（核心の変更）

現状の `Subscription.IsActiveAt(t)` は `status == active` を要求するため、`trialing` と `past_due` が
誤って free に落ちる。これをドメインの新メソッド `GivesPremiumAt(now time.Time) bool` に置き換える。
猶予は `PaymentGracePeriod = 7 * 24 * time.Hour` 定数（利用規約4条7項）。

判定分岐:

| status | 条件 | 判定 | 根拠 |
| --- | --- | --- | --- |
| `active` | `current_period_end > now` | premium | 通常加入 |
| `active` | 期限切れ（DB未書換） | free | 既存踏襲（フェイルセーフ） |
| `trialing` | `current_period_end (= trial_end) > now` | premium | トライアル中も利用可 |
| `past_due` | `now < current_period_end + PaymentGracePeriod` | premium | 利用規約4条7項の7日猶予 |
| `past_due` | 猶予超過 | free | Stripe の督促が最終的に canceled へ遷移 |
| `canceled` | — | free | 失効 |

`EntitlementService.For` は `sub.GivesPremiumAt(s.now())` が真なら `NewEntitlement(sub.Plan)`、偽なら free。
ストアエラー（not-found 以外）は従来どおり握りつぶさず伝播する（障害中に課金済みを黙って free に
落とさない、という既存の方針を維持）。`IsActiveAt` は `GivesPremiumAt` に置き換えて削除する
（唯一の呼び出し元が `For` のため）。

## 5. バックエンド API（`/api/v1/billing`）

すべて `APIBasePath`（`/api/v1`）配下。RFC7807 problem+json は既存の `problem.go` 方式に従う。

### 5.1 `GET /billing/preview`（要ログイン）

申込確認画面の表示値を**サーバ計算**で返す（`checkout-display.md §4`：初回課金は「5日後」ではなく
日時まで算出する）。

レスポンス例:
```json
{
  "price": 300,
  "currency": "jpy",
  "trialDays": 5,
  "trialEligible": true,
  "firstBillingAt": "2026-07-30T12:34:56+09:00",
  "planManagementPath": "アカウント設定 > プランの管理"
}
```

- `trialEligible` は加入行の有無で判定（§3.5）。
- `firstBillingAt` はトライアル終了時刻 ＝ サーバの現在時刻 + `trialDays`。`trialEligible=false`（返済者）
  の場合は `firstBillingAt = now`（即課金）とし、トライアル文言を出さない。
- `planManagementPath` は確認画面が解約方法を表示するための固定文言。

### 5.2 `POST /billing/checkout-session`（要ログイン）

1. `EntitlementService.For` が premium を返すなら `ErrAlreadySubscribed`（409 `already-subscribed`）で
   二重加入を防ぐ。
2. トライアル可否を加入行の有無で判定。
3. Customer の扱い:
   - 既存行に `provider_customer_id` があれば `customer=<id>` を渡す（返済者。トライアル無し）。
   - 無ければ `customer_email` を渡し Stripe に Customer を作らせる（初回。トライアル付与）。
     ※ 中断された Checkout で孤児 Customer が生じ得るが無害。二重 active 加入は §5.2-1 の
     already-subscribed ガードとトライアル可否判定で防がれる。
4. `PaymentGateway.CreateCheckoutSession` を呼ぶ。パラメータ:
   - `mode=subscription`, `line_items=[{price: STRIPE_PRICE_ID, quantity: 1}]`
   - `subscription_data.trial_period_days = 5`（適格時のみ）
   - `client_reference_id = userID`, `subscription_data.metadata.user_id = userID`
   - `success_url = {FRONTEND_ORIGIN}/checkout/complete?session_id={CHECKOUT_SESSION_ID}`
   - `cancel_url = {FRONTEND_ORIGIN}/checkout`
5. `{ "url": "<Checkout URL>" }` を返す。フロントがそこへリダイレクトする。

### 5.3 `POST /billing/webhook`（認証なし・公開）

- **署名検証**: 生ボディ ＋ `Stripe-Signature` ヘッダ ＋ `STRIPE_WEBHOOK_SECRET` を検証。失敗は
  400（Stripe に再送させない）。生ボディが必要なため、ボディを消費する前に読む。
- 認証ミドルウェアを付けない（Stripe が直接叩く）。Cloud Run の公開URLに着弾するため、守りは
  プロキシ秘密ではなく署名検証。
- 処理するイベント（`PaymentGateway.ParseWebhookEvent` が正規化して返す）:
  - `customer.subscription.created` / `customer.subscription.updated` — **加入状態の権威**。
    subscription オブジェクトが `status`（trialing/active/past_due）・`current_period_end`・
    `cancel_at_period_end`・`customer` を全て含む。userID は作成時に仕込んだ `metadata.user_id`。
    trialing→active、active→past_due、past_due→active 復帰、`cancel_at_period_end` 切替、期末延長を反映。
  - `customer.subscription.deleted` — `status=canceled`。
  - `checkout.session.completed` — 加入確定の冪等シグナル。userID は `client_reference_id`。
    subscription.* を取りこぼした場合の保険（adapter が必要なら subscription を取得して同じ upsert を行う）。
  - 上記以外は 200 で無視。
- 反映内容: `plan=premium`, `status`, `current_period_end`, `cancel_at_period_end`,
  `provider=stripe`, `provider_subscription_id`, `provider_customer_id` を **Stripe の値のまま**
  `SubscriptionStore.Upsert`（§3.4）。
- 冪等性: `user_id` upsert ＋ `provider_subscription_id` 部分ユニークインデックスで重複配信を吸収。
- 処理成功は 200、処理失敗（DB エラー等）は 500（Stripe が再送）。

### 5.4 Stripe 隔離（port と正規化イベント）

```go
type PaymentGateway interface {
    CreateCheckoutSession(ctx context.Context, p CheckoutParams) (url string, err error)
    // ParseWebhookEvent は署名検証を行い、Stripe イベントを正規化して返す。
    // 未処理のイベント種別は WebhookEvent.Type を空にして返し、handler 側で 200 無視する。
    ParseWebhookEvent(payload []byte, sigHeader string) (WebhookEvent, error)
}

type CheckoutParams struct {
    UserID       string
    CustomerID   string // 再利用する Customer。空なら CustomerEmail で新規作成
    CustomerEmail string
    WithTrial    bool
    SuccessURL   string
    CancelURL    string
}

type WebhookEvent struct {
    Type              string // "subscription.active" 等の正規化済み種別。無視対象は空
    UserID            string
    SubscriptionID    string
    CustomerID        string
    Status            domain.SubscriptionStatus
    CurrentPeriodEnd  time.Time
    CancelAtPeriodEnd bool
    Canceled          bool // deleted イベントか
}
```

`stripe-go` は `backend/internal/repository`（または新設 `payment` パッケージ）の adapter だけが
import する。billing service は port だけを見る。

## 6. サービス層

新設 `BillingService`（`backend/internal/service/billing.go`）:
- `CreateCheckoutSession(ctx, userID) (url string, err error)` — §5.2 のロジック。依存: `EntitlementService`
  （already-subscribed 判定）、`SubscriptionStore`（トライアル可否・customer 再利用）、`PaymentGateway`、
  設定（success/cancel URL）。
- `HandleWebhookEvent(ctx, ev WebhookEvent) error` — 正規化イベントから `domain.Subscription` を組み、
  `SubscriptionStore.Upsert`。`Canceled` なら `status=canceled`。
- `Preview(ctx, userID) (PreviewResult, error)` — §5.1 の値を計算（trialEligible・firstBillingAt）。

エラー: `ErrAlreadySubscribed`（premium が既にある）。`problemMapping` に 409 `already-subscribed` を追加
（`problem_coverage_test` が未マッピングを 500 検出するため必須）。

## 7. フロントエンド

- `/checkout`（要ログイン）: `GET /billing/preview` の値で6必須項目を描画（`checkout-display.md §2/§3`）:
  価格 ¥300税込 / 5日トライアル / 初回課金日時 / 解約方法「アカウント設定 > プランの管理」/ 返金方針 /
  利用規約・プライバシーへのリンク付き同意チェックボックス。ボタン「無料お試しを開始する」は
  同意チェックまで無効。押下で `POST /billing/checkout-session` → 返った `url` へ `window.location` で遷移。
- `/checkout/complete`（success_url・要ログイン）: 「お手続きを受け付けました」を表示し、`/auth/me` を
  数回ポーリング（バックオフ）して `plan==premium` を待つ。反映後「プレミアムが有効になりました」＋
  週間への導線。所定回数で未反映なら「反映まで少し時間がかかることがあります」＋再読込導線。
- 既存の **in-context アップグレード CTA**（plan-split の保存上限 409 時・買い物リスト初回チェック時）の
  遷移先を `/checkout` に接続する。
  - ※週間ロック（`PremiumLock`）の CTA は `feat/weekly-premium`（#145）側にあるため、本スコープでは
    plan-split の CTA のみ配線する。#145 マージ時に `PremiumLock` の CTA も `/checkout` へ向ける。
- ルートは `RequireAuth` 相当（未ログインはログインへ）。API 由来の 401/403/409 は既存の
  `<ErrorMessage>` 経由で保険ハンドル。

## 8. 設定・依存

- 環境変数（すべて必須。欠落時は起動時に fatal＝`JWT_SECRET` と同じ扱い）:
  - `STRIPE_SECRET_KEY` — テスト/本番のシークレットキー。
  - `STRIPE_WEBHOOK_SECRET` — Webhook 署名シークレット。
  - `STRIPE_PRICE_ID` — ¥300/月 recurring Price の ID。
  - success/cancel URL は既存の `FRONTEND_ORIGIN` から導出。
- 追加先: `.env.example`（ルート）、`docker-compose.yml`（backend の `environment`）、`DEPLOY.md` の
  環境変数マトリクス、本番は GCP Secret Manager → Cloud Run。
- `backend/go.mod` に `stripe-go`（公式）を追加。

## 9. テスト戦略

| 層 | 検証 |
| --- | --- |
| domain 単体 | `GivesPremiumAt` の分岐表 全ケース（active/期限切れ/trialing/past_due 猶予内/猶予超過/canceled） |
| domain 単体 | `SubscriptionTrialing` の `Parse`/`Valid` |
| service 単体 | `CreateCheckoutSession`: 既 premium→`ErrAlreadySubscribed` / トライアル付与・非付与で gateway 引数が変わる / customer 再利用 |
| service 単体 | `HandleWebhookEvent`: 各イベント種別で正しい `domain.Subscription` を upsert / 冪等 |
| service 単体 | `Preview`: trialEligible と firstBillingAt の算出（返済者は即課金） |
| adapter 単体 | 署名検証（正/不正）＋ Stripe オブジェクト→`WebhookEvent` 変換。`stripe-go` のテスト用シークレットで署名を構成 |
| handler | checkout-session: 未ログイン401 / 既premium409 / url 返却。webhook: 不正署名400 / 正常→store 更新 / 未知200 |
| handler | `problem_coverage_test` に `already-subscribed`(409) が写像されていること |
| frontend | `/checkout`: 6必須項目の描画・同意ゲート（未同意でボタン無効）・session 作成→リダイレクト |
| frontend | `/checkout/complete`: `/auth/me` ポーリングで premium 反映後に成功表示 |

**実 Stripe を使う E2E は CI に入れない**（外部依存・要シークレット・不安定）。代わりに
`checkout-display.md §6` の**手動チェックリスト**を本設計のローンチ前ゲートとして残す（隠さない）:
Stripe テストカード ＋ `stripe listen` / `stripe trigger` を用い、
(1) トライアル中に解約 → 無課金、(2) トライアル満了 → 初回課金 → 解約 → 期末失効、
(3) 支払い失敗 → `past_due` → 猶予中も premium 維持、を手動で確認する。

## 10. ロールアウト・前提条件

実装前後に必要な実運用の手当て:

- **Stripe アカウント**。テストモードのキーは即発行される。live（本番）課金は本人確認・口座審査が
  必要で、初めての個人事業だと時間がかかる想定。**本設計の実装・テストはテストモードで完結**させ、
  live 有効化は別途。
- Stripe ダッシュボードで Product「プレミアム」＋ Price ¥300/月 JPY(recurring) を作成 → `STRIPE_PRICE_ID`。
- 督促（Smart Retries）を**約7日で最終的に解約**する設定にし、コードの7日猶予（§4.3）と揃える。
  コードの猶予は安全弁で、実際の打ち切りは Stripe の督促設定が行う。
- Webhook エンドポイントを Stripe に登録（Cloud Run 公開URL `/api/v1/billing/webhook`）→ signing secret を
  `STRIPE_WEBHOOK_SECRET` へ。
- マイグレーション `000012` を本番 Neon で手動実行 → その後バックエンドをデプロイ。
- **ローンチゲート（本スコープ外・実課金開始前に必須）**: 解約画面「アカウント設定 > プランの管理」
  （次サブプロジェクト）。特商法・利用規約が名指ししているため、無いまま課金を開始できない。
  確認画面は今この文言を参照するが、実課金しない段階なので許容する。
- **ローンチゲート**: `Preview`（申込確認画面・特商法12条の6の表示）は `Price: 300, Currency: "jpy"` を
  コード側にハードコードして表示している。実際の課金額は `STRIPE_PRICE_ID` が指す Stripe 側の
  Price が決める。実課金を有効化する前に、`STRIPE_PRICE_ID` の Price が実額¥300 JPY・recurring
  であることを Stripe ダッシュボードで必ず確認する（両者が食い違うと確認画面の表示が
  法的に不正確になる）。Price を変更する際は本ハードコード値も同時に見直すこと。
- 弁護士等による法務スポットレビュー（推奨、`docs/legal/README.md` の残タスク）。

## 11. 未決事項

- `/checkout/complete` のポーリング回数・間隔・最大待ち時間の具体値（実装時に決める）。
- 孤児 Stripe Customer（中断された Checkout）の清掃は行わない（無害。YAGNI）。
- `feat/weekly-premium`（#145）マージ後の `PremiumLock` CTA 配線は #145 側で行う。
