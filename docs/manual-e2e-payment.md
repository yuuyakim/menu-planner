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
※ 解約画面「アカウント設定 > プランの管理」は**次サブプロジェクトで未実装**のため、今は Stripe ダッシュボードで解約する。
1. Stripe ダッシュボード（テスト）→ 顧客 → 該当 Subscription → **Cancel subscription**（今すぐ or 期間末）。
2. `stripe listen` に `customer.subscription.updated`（cancel_at_period_end）や、即時取消なら `customer.subscription.deleted` が流れる。
3. 確認: 期間末解約なら期末まで premium（active + cancel_at_period_end）、即時取消なら `canceled` → free に落ちる。`/auth/me` で確認。

**期待**: トライアル中の解約で**課金は一切発生しない**。

### C.（応用）トライアル満了 → 初回課金、支払い失敗 → past_due 猶予
実時間の5日を待てないので **Stripe テストクロック**を使う（ダッシュボード → 開発者 → テストクロック、または CLI）。
- テストクロック上に顧客を作り、`trial_end` を超えて時計を進める → 初回 invoice が課金される（`4242…` なら成功、`customer.subscription.updated` が status=active に）。
- 失敗カード `4000 0000 0000 0341`（登録は通るが後続課金が失敗）で更新を迎えさせると status=**past_due** に → **猶予7日内は premium 維持**、7日超で free になることを確認（`GivesPremiumAt` の分岐）。
- これは手順が重いので、まず A/B を確実に。C は本番課金を有効化する前までに一度通せばよい。

## 6. 確認ポイントまとめ
- [ ] A: 加入で trialing→premium 反映（Webhook 経由）
- [ ] B: トライアル中解約で無課金、free へ復帰
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
