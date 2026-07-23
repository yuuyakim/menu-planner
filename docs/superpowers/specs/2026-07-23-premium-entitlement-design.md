# プレミアムプラン基盤（エンタイトルメント）設計

- 日付: 2026-07-23
- 対象: `menu-planner`（献立くん）
- 状態: 設計確定・未実装

## 1. 背景と目的

献立くんに有料のプレミアムプランを設けたい。最終的には月額サブスクリプションで課金するが、
**この設計では決済を実装しない**。理由は2つある。

第一に、決済を有効化するには特定商取引法に基づく事業者情報の開示が必要で、
販売主体をどうするか（個人事業主／バーチャルオフィス／MoR）がまだ決まっていない。
販売主体が決まらないと採るべき決済構成が決まらないため、コードを先に書くと作り直しになる。

第二に、決済が無くても「誰がプレミアムか」を保持し機能を出し分ける土台は独立して作れる。
土台を先に固めておけば、決済は Webhook から同じサービス層を呼ぶだけで載る。

したがって本設計の目的は、**後から決済を差し込める形でエンタイトルメント基盤を作ること**である。

## 2. スコープ

### 対象

- `subscriptions` テーブルと、そこからエンタイトルメントを導出するサービス
- CLI によるプレミアムの付与・取消（将来の Webhook と同じサービス層を通る）
- 最初の機能差分として、週間献立の保存上限をプラン由来にする
- `GET /auth/me` がプランを返し、`AuthMenu` に premium のバッジを表示する
- `spec.md` への「有料化の前提条件」章の追加（法務要件の記録）

### 対象外（決済フェーズ送り）

- 決済事業者の接続、Webhook 受信、カスタマーポータル
- 課金UI・アップグレード導線
- 利用規約／特商法に基づく表示／プライバシーポリシーの文面作成
- プラン変更（アップグレード・ダウングレード）と日割り計算
- 無料トライアル、招待コード、クーポン

## 3. 設計判断

### 3.1 プレミアムは「上乗せ」であり「取り上げ」ではない（決定）

献立くんは 2026-07-21 から本番稼働しており、利用者がいる。
今まで無料で使えていた機能を有料化すると、既存利用者の体験が劣化する。

したがって**既存の上限値は free の据え置き値とし、premium はそれを上回る値にする**。
週間献立の保存上限は free = 10件（現行維持）、premium = 50件とする。

### 3.2 加入状態は専用テーブルで持つ（決定）

`users.plan` のような単一カラムは採らない。月額サブスクリプションは
「いつまで有効か」「解約予約中か」「支払いが滞っていないか」を表現できなければ成立せず、
1カラムでは決済導入時に必ず作り直しになる。

初期コストの差はマイグレーション1本とサービス1つ程度であり、
後で捨てるコードを書くより安い。

機能単位の権限テーブル（`entitlements`）は採らない。プランが2つしかない現状では過剰である。

### 3.3 free はレコードを持たない（決定）

`subscriptions` に行が無いことを free とする。
「無料の加入」という概念を作ると、サインアップ時に行を作る責務が増え、
行が無い既存ユーザーの移行も必要になる。行の有無で判定すれば、どちらも起きない。

### 3.4 エンタイトルメントは読み取り時に計算する（決定）

`current_period_end` を過ぎた加入をバッチで free に書き換えることはしない。
バッチが停止すると、課金していない利用者がプレミアムのまま残るためである。

「行があり、`status = active` で、`current_period_end > now()`」を参照のたびに評価すれば、
真実は常に1つになる。

### 3.5 プランごとの上限値はコードに持つ（決定）

上限値（free = 10 / premium = 50）は DB に置かない。上限値は仕様であってデータであり、
DB に置くと変更のたびにマイグレーションが必要になり、テストもDBの状態に依存して不安定になる。

### 3.6 取消は行を削除せず `canceled` に遷移させる（決定）

取消時に行を消すと「いつ解約したか」の記録が失われる。
後に「解約したのに課金された」と申し立てられたとき、これが唯一の反証材料になる。

## 4. データモデル

### 4.1 マイグレーション `000010_create_subscriptions`

| カラム | 型 | 制約 |
| --- | --- | --- |
| `user_id` | uuid | PK, `REFERENCES users(id) ON DELETE CASCADE` |
| `plan` | text | NOT NULL |
| `status` | text | NOT NULL（`active` / `past_due` / `canceled`） |
| `current_period_end` | timestamptz | NOT NULL |
| `cancel_at_period_end` | boolean | NOT NULL DEFAULT false |
| `provider` | text | NOT NULL（現在は `manual`、将来 `stripe`） |
| `provider_subscription_id` | text | NULL 可 |
| `created_at` | timestamptz | NOT NULL DEFAULT now() |
| `updated_at` | timestamptz | NOT NULL DEFAULT now() |

- **`user_id` を主キー**とし、1ユーザー1加入に固定する。複数同時加入は仕様にない。
- **`provider_subscription_id` に NULL を除く部分UNIQUE索引**を張る。
  将来 Webhook が同一イベントを二度配送したときの二重適用を DB が弾く。
  今はコストがゼロで、後から張ると既存データの重複を掃除する必要が出る。
- `plan` と `status` は CHECK 制約ではなくアプリ側で検証する。
  既存テーブル（`menus.role` ほか）の流儀に合わせる。

### 4.2 マイグレーション番号の衝突に注意

`000009_add_menu_role` まで採番済み。並行して進んでいる `feature/menu-role` 系の作業が
`000010` を採る可能性がある。着手時に採番済み番号を確認し、衝突していれば振り直す。

## 5. ドメイン層

### 5.1 `domain.Plan`

`PlanFree` / `PlanPremium` の2値。文字列との相互変換を持つ。

### 5.2 `domain.Entitlement`

```go
type Entitlement struct{ plan Plan }        // フィールドは非公開

func NewEntitlement(p Plan) Entitlement
func (e Entitlement) Plan() Plan
func (e Entitlement) SavedWeeklyMenuLimit() int   // free:10  premium:50
```

**上限値をフィールドではなくメソッドで導出する**のが要点である。
仮に `SavedWeeklyMenuLimit int` をフィールドで持たせると、
取得し忘れた `Entitlement{}` のゼロ値が「上限0件」を意味してしまい、
既存利用者が1件も保存できなくなる。

`plan` を非公開フィールドにしてメソッドで導出すれば、
ゼロ値の `plan` は空文字となり free と同じ扱いに落ちる。
3.1 の「既存の体験を壊さない」という前提が型の側で保証される。

### 5.3 `domain.Subscription`

`user_id` / `plan` / `status` / `current_period_end` / `cancel_at_period_end` /
`provider` / `provider_subscription_id` を持つエンティティ。
`IsActiveAt(t time.Time) bool` で有効判定を持たせる。

## 6. サービス層

### 6.1 `service.SubscriptionStore`（ポート）

`internal/service/ports.go` に追加する。

```go
type SubscriptionStore interface {
    Find(ctx context.Context, userID domain.UserID) (domain.Subscription, error)
    Upsert(ctx context.Context, sub domain.Subscription) error
}
```

該当が無ければ `repository.ErrSubscriptionNotFound` を返す。

### 6.2 `service.EntitlementService`

```go
func (s *EntitlementService) For(ctx context.Context, userID string) (domain.Entitlement, error)
```

分岐は5通り。

| 状況 | 返す値 |
| --- | --- |
| `userID` が空（未認証） | free。エラーにしない |
| 行が無い | free |
| `active` かつ `current_period_end > now()` | premium |
| `active` だが期限切れ | free（DBは書き換えない） |
| `canceled` / `past_due` | free |

現在時刻は `now func() time.Time` として注入し、期限判定をテスト可能にする。

### 6.3 `service.SubscriptionService`

```go
func (s *SubscriptionService) Grant(ctx context.Context, userID domain.UserID, months int) error
func (s *SubscriptionService) Revoke(ctx context.Context, userID domain.UserID) error
```

CLI と将来の Webhook の**両方がここを通る**ことで、状態遷移が一箇所に集まる。

**引数はメールアドレスではなく `UserID` を取る。** CLI は人が使うのでメールアドレスで指定したいが、
将来の Webhook が決済事業者から受け取るのは顧客IDであってメールアドレスではない。
サービスをメールアドレス起点にすると Webhook 側が不自然な逆引きを強いられるため、
**メールアドレスから `UserID` への解決は CLI の責務**とする（既存の `UserStore` を使う）。

`Grant` は `provider = manual` で upsert する。`Revoke` は `status = canceled` に遷移させる。
いずれも既存の構造化ログ（`logctx`）に付与・取消を記録する。
手動付与は将来 Stripe 側の履歴に残らないため、ログが唯一の記録になる。

**`Revoke` は即時失効であり、利用者都合の解約とは別物である。**
`Revoke` が想定するのは誤付与の是正や規約違反への対応といった運用上の取消なので、
期末まで待つ理由がない。一方、利用者が自分の意思で解約する場合は
`cancel_at_period_end = true` を立てて期末に失効させる（12.8）。
後者は決済フェーズで実装するため、本設計では列を用意するだけで書き込む経路を作らない。

専用の監査テーブルは作らない。決済導入後は決済事業者側が正の履歴を持つ。

### 6.4 `SavedWeeklyMenuService` の変更

定数 `SavedWeeklyMenuLimit = 10` を廃し、`Entitlements` ポート越しに上限を引く。

```go
type Entitlements interface {
    For(ctx context.Context, userID string) (domain.Entitlement, error)
}
```

`Save` は保存前に `For` を呼び、`store.Count()` と `ent.SavedWeeklyMenuLimit()` を比較する。

## 7. エラー

現行の `ErrSavedWeeklyMenuLimitReached` はメッセージに「10件までです」を固定で含む。
プラン依存にするため、件数を持つエラー型に置き換える。

```go
type SavedWeeklyMenuLimitError struct{ Limit int }

func (e *SavedWeeklyMenuLimitError) Error() string
func (e *SavedWeeklyMenuLimitError) Is(target error) bool  // 既存の sentinel と一致させる
```

`Is` を実装することで、既存の `errors.Is(err, ErrSavedWeeklyMenuLimitReached)` 呼び出しと
handler のエラー写像テストを壊さずに、メッセージだけをプラン由来にできる。
HTTPステータスは 409 のまま変わらない。

## 8. CLI `backend/cmd/grant`

既存の `cmd/migrate` / `cmd/seed` と同じ構成で `DATABASE_URL` を読む。

```
go run ./cmd/grant -email=foo@example.com -months=1   # 付与
go run ./cmd/grant -email=foo@example.com -revoke     # 即時取消
```

SQL を直接書かず、メールアドレスを `UserID` に解決したうえで `SubscriptionService` を呼ぶ（6.3）。
該当するメールアドレスが無ければ何もせずエラー終了する。
`Makefile` に `make grant EMAIL=... MONTHS=1` と `make revoke EMAIL=...` を追加する。

## 9. API

`api/openapi.yaml` の `userResponse` に `plan: "free" | "premium"` を追加する。
`make gen-api` で TypeScript の型を再生成し、契約テスト（`internal/handler/contract_test.go`）が
仕様と実装のズレを検出する。

**上限値そのものは返さない。** フロントエンドが件数を持つと二重管理になる。
409 のメッセージにサーバがプラン由来の件数を入れるため、フロントは受け取って表示するだけでよい。

`/subscriptions` などの新規エンドポイントは作らない。決済フェーズの仕事である。

## 10. フロントエンド

`features/auth` の `useCurrentUser` が返す user に `plan` が乗る。
保存上限に達したときの表示は、既にサーバのメッセージを出しているため変更不要
（着手時に実装を確認すること）。

表示は `AuthMenu.tsx` に**プレミアムであることを示すバッジを1つ出すだけ**とする。
アカウント画面は存在しないため、ログイン中の利用者情報を既に出しているここが自然な置き場になる。

**free の利用者には何も新しく表示しない。** バッジは premium のときだけ出す。
**アップグレード導線も作らない。** 決済が存在しない状態で「プレミアムにする」ボタンを出すのは
利用者に対して不誠実である。バッジは勧誘ではなく状態の表示なので、この線引きに反しない。

## 11. テスト戦略

既存の TDD 方針（`spec.md` 9章）に沿い、🔴テスト → 🟢実装の対で進める。

| 層 | 検証すること |
| --- | --- |
| domain | 上限の導出。**ゼロ値の `Entitlement` が free に落ちること**（3.1 の保証） |
| repository | `subscriptions` のスキーマ検査（既存 `*_schema_test.go` の流儀）、Upsert と Find |
| service | `For` の5分岐、`Save` がプラン由来の上限で 409 を返すこと（fake ストアを使う） |
| handler | `/auth/me` が `plan` を返すこと、契約テスト |
| frontend | premium のときだけ `AuthMenu` にバッジが出ること（free では出ないこと） |
| E2E | premium ユーザーでログインするとバッジが表示されること |

**E2E で上限の境界値は検証しない。** 11件保存するには週間献立を11回組む必要があり、
実行時間に見合わない。境界値は service のテストで担保する。

E2E 用のプレミアムユーザーは `docker compose exec backend go run ./cmd/grant` で作る。
既存 E2E が `make up` / `make seed` を前提にしているのと同じ流儀である。

## 12. 有料化の前提条件（法務）

> **注意**: 以下は法律の専門家によるものではなく、一般的な整理である。
> 課金を開始する前に、専門家または消費者庁のガイドラインで裏を取ること。
> この節は `spec.md` に新章として転記し、決済フェーズ着手時の確認リストとして使う。

決済を有効化する前に、以下を満たす必要がある。

### 12.1 特定商取引法に基づく表示（法11条）

通信販売の広告として、販売業者の氏名・住所・電話番号、税込の販売価格、
支払時期と方法、役務の提供時期、契約の申込みの撤回・解除に関する事項を表示する。

**氏名・住所・電話番号は省略可能事項に含まれないとされる。**
個人が販売者になる場合、原則として自宅住所と連絡可能な電話番号が公開される。
これを避けるには、バーチャルオフィスの契約か、MoR（Merchant of Record）による販売代行を採る。
**販売主体の決定が決済事業者の選定より先**である。

### 12.2 申込みの最終確認画面（法12条の6、2022年6月改正）

サブスクリプションでは、契約期間と自動更新の有無、価格、支払時期、解約方法を
**申込み確定の直前画面に表示する義務**がある。
違反すると申込みを取り消され得る（法15条の4）ため、実装に直結する。

### 12.3 解約の容易性

解約導線を申込みと同程度に容易にする。解約方法を分かりにくく配置しない。

### 12.4 カード情報の非保持化（割賦販売法・PCI DSS）

決済事業者のホスト型フォームまたはトークン化を用い、
**Cloud Run 上のバックエンドにカード番号を一切通さない**構成にする。

### 12.5 個人情報保護法

プライバシーポリシーに、決済事業者への個人データの提供と、
外国にある第三者への提供に関する情報提供を追加する。

### 12.6 定型約款（民法548条の2）

利用規約を有償契約に耐える内容へ改め、変更条項を置く。

### 12.7 その他

- 総額表示義務（消費者向け価格は税込表示）
- 未成年者の契約（18歳未満は法定代理人の同意。年齢確認の扱いを決める）
- 消費税・インボイス制度、開業届、帳簿保存
- 特定電子メール法（課金通知は取引メールだが、宣伝を混ぜるとオプトインが必要になる）

### 12.8 本設計に織り込み済みの項目

法務要件のうち、スキーマに影響するものは本設計に反映済みである。

- `cancel_at_period_end` を持つ（4.1）。解約後も期末まで利用できる。
  即時失効は返金の争いを招くため。
- `canceled` の行を削除しない（3.6）。解約時期の記録が反証材料になる。

## 13. 実装順序の見取り図

詳細な計画は別途 writing-plans で作成する。おおよその依存順序は次のとおり。

1. `domain.Plan` / `domain.Entitlement` / `domain.Subscription`
2. マイグレーション `000010` とスキーマ検査テスト
3. `repository.SubscriptionRepository`
4. `service.EntitlementService`
5. `service.SubscriptionService` と `cmd/grant`、Makefile ターゲット
6. `SavedWeeklyMenuService` を上限のプラン依存へ変更（エラー型の置き換えを含む）
7. `openapi.yaml` に `plan` を追加、`/auth/me` の実装と型再生成
8. フロントエンドへの反映と E2E
9. `spec.md` の更新（2.11 プレミアムプラン / 4.2 subscriptions / 15. 有料化の前提条件）

## 14. 未決事項

- **価格**（月額いくらか）。決済フェーズで決める。本設計には影響しない。
- **販売主体**（個人事業主／バーチャルオフィス／MoR）。12.1 のとおり決済事業者の選定より先に決める。
- **premium の2つ目以降の機能**。土台ができてから、利用者レビュー（`task.md`）を見て選ぶ。
