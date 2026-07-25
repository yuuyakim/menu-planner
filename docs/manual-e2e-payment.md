# 決済コア 手動E2E手順（Stripe テストモード）

`docs/legal/checkout-display.md §6` のチェックリストを、ローカルで実際に回すための具体手順。
**すべて Stripe テストモード**で行う（本番課金は発生しない）。CI では実 Stripe を叩かないため、この通し確認は手動で行う。

## 0. 前提（あなたが用意するもの）

- **Stripe アカウント**（テストモードで可。右上のトグルを「テスト環境」に）。
- **`sk_test_...`**（シークレットキー）: ダッシュボード → 開発者 → APIキー。
- **`price_...`**（価格ID）: 商品カタログ → 商品を追加 → 名前「プレミアム」/ 継続(recurring)・月次 / **¥300 JPY** → 保存すると価格に `price_...` が付く。
- **Stripe CLI**: Windows なら `scoop install stripe` か [公式](https://stripe.com/docs/stripe-cli) からDL。導入後 `stripe login`（ブラウザで認可）。
- Docker Desktop 起動中。

> トライアル5日はコード側で `trial_period_days` を渡すので、価格側にトライアル設定は不要。

### 0'. 顧客ポータルを有効化（シナリオB'の前に一度だけ）

Stripe ダッシュボード（テストモード）→ 設定 → 課金 → **顧客ポータル**で以下を設定して保存する。

- **キャンセル**: 有効化し、**「期間終了時にキャンセル」**を選ぶ（即時キャンセルにしない。利用規約4条5項と一致させる）。
- **お支払い方法の更新**: 有効化。
- **請求履歴**: 有効化。
- **戻るリンク**（Return URL）: `<フロントのオリジン>/account`（ローカルなら `http://localhost:5173/account`）。

未設定のままポータルへ遷移すると、Stripe が「ポータルを設定してください」という案内画面を出すだけになり、B'の手順が進められない。

## 1. `.env` に Stripe 変数を設定

リポジトリ直下の `.env` に追記（`DATABASE_URL` 等の既存行はそのまま）:

```
STRIPE_SECRET_KEY=sk_test_あなたの値
STRIPE_PRICE_ID=price_あなたの値
STRIPE_WEBHOOK_SECRET=   # ← 手順2で埋める
```

> ⚠️ この `.env` の `DATABASE_URL` は本番 Neon を指しているが、**`make up` のローカルスタックには影響しない**（compose は backend の `DATABASE_URL` をローカルdb `db:5432` にハードコードしている）。ローカルE2Eが本番DBを汚すことはない。

## 2. Webhook 転送を起動して署名シークレットを取得

別ターミナルで（起動したまま放置）:

```bash
stripe listen \
  --events checkout.session.completed,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted \
  --forward-to localhost:8080/api/v1/billing/webhook
```

起動時に `Your webhook signing secret is whsec_...` が表示される。これを `.env` の `STRIPE_WEBHOOK_SECRET=` に貼る。

## 3. ローカルスタックを起動・準備

`.env` を保存してから（compose は起動時に `.env` を読む）:

```bash
make up          # db + backend + frontend を起動（= docker compose up -d --build）
make migrate     # ローカルdbに 000012 まで適用
make seed        # 献立マスタを投入（アプリが動くように）
```

`make health` で backend の疎通確認。起動しない場合は `make logs` で STRIPE_* 未設定（fatal）を確認。

## 4. テストユーザーを用意

1. ブラウザで http://localhost:5173 を開く。
2. サインアップ（メール＋パスワード）→ ログイン。**付与はしない**（free のまま。Stripe 経由で premium になることを確認するため）。

## 5. シナリオ（Stripe テストカード）

テストカード: `4242 4242 4242 4242` / 有効期限=未来の任意 / CVC=任意3桁 / 郵便番号=任意。

### A. 加入 → プレミアム反映（基本・必須）
1. `/checkout` を開く（買い物リストで初回チェック時の案内バナー「アップグレードする」からでも、URL直打ちでも可）。
2. 6項目（¥300税込 / 5日トライアル / 初回課金日時 / 解約は「アカウント設定 > プランの管理」/ 返金 / 規約同意）が出る。同意にチェック → **「無料お試しを開始する」**。
3. Stripe のホスト画面へ遷移 → テストカード入力 → 完了。
4. `/checkout/complete` に戻り、「プレミアムの有効化を確認しています…」→ 数秒で **「プレミアムが有効になりました」**。
5. 確認: `stripe listen` のターミナルに `checkout.session.completed` と `customer.subscription.created`（status=**trialing**）が流れ、backend が 200 を返していること。ヘッダの会員バッジ/`/auth/me` が premium になること。

**期待**: トライアル中（trialing）でも premium。この時点では課金されない（Stripe ダッシュボード → 支払い が空）。

### B. トライアル中に解約 → 無課金（必須）
※ ここでは Stripe ダッシュボードから直接解約する（Webhook 経路自体の確認が目的）。アプリ画面（`/account`）からの解約はシナリオB'で確認する。
1. Stripe ダッシュボード（テスト）→ 顧客 → 該当 Subscription → **Cancel subscription**（今すぐ or 期間末）。
2. `stripe listen` に `customer.subscription.updated`（cancel_at_period_end）や、即時取消なら `customer.subscription.deleted` が流れる。
3. 確認: 期間末解約なら期末まで premium（active + cancel_at_period_end）、即時取消なら `canceled` → free に落ちる。`/auth/me` で確認。

**期待**: トライアル中の解約で**課金は一切発生しない**。

### B'. 顧客ポータルで解約・カード変更（必須。plan-management サブプロジェクトで追加）
※ 事前に「0'. 顧客ポータルを有効化」（下記）を済ませておくこと。テストモードで未設定だとポータル遷移後に設定画面へ誘導される。
1. premium の状態（シナリオA後など）でログインし、`/account`（アカウント設定 > プランの管理）を開く。
2. 現在のプラン状態（プラン名・次回請求日 or トライアル中の表示）が `GET /billing/subscription` の値と一致することを確認する。
3. **「プランを管理する」**をクリック → Stripe 顧客ポータル（ホスト画面）へ遷移する。
4. ポータルで **Cancel plan**（期末解約）を選ぶ。
5. `stripe listen` のターミナルに `customer.subscription.updated`（`cancel_at_period_end=true`）が流れ、backend が200を返すことを確認する。
6. `/account` に戻る（ポータルの「戻る」リンク、または再訪）→ 表示が **「プレミアム（〈日付〉で解約予定。それまでご利用いただけます）」** に変わっていることを確認する。
7. テストクロックで期末を過ぎさせる（またはダッシュボードで即時取消して代替）→ `customer.subscription.deleted` が流れる → `/account` が free 表示に戻ることを確認する。
8. 別途、premium の状態から再度ポータルを開き **カード情報の更新**（テストカードを別番号に変更）を行い、正常に保存できることを確認する（課金への影響はここでは確認しない）。

**期待**: `/account` の表示は常に `GET /billing/subscription` が返す最新状態を反映し、ポータルでの操作は既存 Webhook だけで同期される（このAPI自身は `subscriptions` を更新しない）。手動付与など Stripe 顧客が無いユーザーでログインした場合、「プランを管理する」ボタンが**出ないこと**も確認する（`hasPortal: false`）。

### C.（応用）トライアル満了 → 初回課金、支払い失敗 → past_due 猶予
実時間の5日を待てないので **Stripe テストクロック**を使う（ダッシュボード → 開発者 → テストクロック、または CLI）。
- テストクロック上に顧客を作り、`trial_end` を超えて時計を進める → 初回 invoice が課金される（`4242…` なら成功、`customer.subscription.updated` が status=active に）。
- 失敗カード `4000 0000 0000 0341`（登録は通るが後続課金が失敗）で更新を迎えさせると status=**past_due** に → **猶予7日内は premium 維持**、7日超で free になることを確認（`GivesPremiumAt` の分岐）。
- これは手順が重いので、まず A/B を確実に。C は本番課金を有効化する前までに一度通せばよい。

## 6. 確認ポイントまとめ
- [ ] A: 加入で trialing→premium 反映（Webhook 経由）
- [ ] B: トライアル中解約で無課金、free へ復帰
- [ ] B': `/account` →「プランを管理する」→ 顧客ポータルで期末解約 → `/account` に「解約予定」表示 → 期末で free。カード変更もポータルで確認
- [ ] C: （応用）満了で初回課金 / 失敗で past_due＋7日猶予中も premium

## 7. 後片付け
```bash
make down        # スタック停止
make clean       # コンテナ+ボリューム削除（ローカルdbも消える）
```
`stripe listen` は Ctrl+C。テストモードのデータは本番と分離されているので放置してもよい。

## 注意
- カード番号は Stripe のホスト画面にしか入らない（自社は一切通らない）。
- ローカルは compose の local db。本番 Neon には影響しない。
- 本番(live)課金の有効化は別途（本人確認・口座審査、live キー、本番URLでの Webhook 登録）。
- **ローンチ前に必須**: 「0'. 顧客ポータルを有効化」を **test / live 両方の環境**で設定すること（解約=期末・支払い方法の更新・請求履歴を有効化、戻りURLを本番なら実際のドメインの `/account` にする）。設定を忘れると本番で「プランを管理する」を押した利用者がStripeの案内画面に迷い込む。
