# 週間まわりを premium にする（free / premium 再編）設計

- 日付: 2026-07-24
- 対象: `menu-planner`（献立くん）
- 状態: 設計確定・未実装
- 前提: `2026-07-23-premium-entitlement-design.md`（エンタイトルメント基盤）、
  `2026-07-23-premium-plan-split-design.md`（買い物リスト永続化。**本設計はその線引きを上書きする**）

## 1. 背景と目的

**目的は無料→有料の転換率を上げること。** 現状の premium は「週間献立の保存上限50件」と
「買い物リストの永続化」の2つで、月額を払う理由として弱い。そこで free / premium の線を引き直す。

**新しい線: free =「今日の1食」、premium =「1週間まとめて計画」。**
単発の献立検索という**本当の価値（アハ体験）は無料のまま**にして離脱を防ぎ、
「1週間を組んで買い物まで回す」という深い使い方を premium にする。フリーミアムの定石
（無料で味見させ、"それを保持・拡張する"ところを壁にする）に沿う。基盤設計3.1が
「premium は毎週の買い物ループに紐づく」と述べていた方向とも一致する。

### 1.1 これは意図的な「取り上げ」である（重要）

週間献立は**現在 free で本番稼働中**であり、献立アプリの看板機能でもある。これを premium に
移すのは、基盤設計/plan-split 設計3.2「**プレミアムは上乗せであり取り上げではない**」からの
**意図的な逸脱**であり、既存の無料利用者から機能を取り上げる戦略転換である。転換率の向上と
引き換えに、獲得・口コミ・既存ユーザーの体感を損なうリスクを負う。これを承知のうえで進める。

## 2. スコープ

### 対象
- 週間まわり一式を premium 限定にする: 週間献立の提案・1日引き直し・週間献立の保存/一覧/削除・
  保存済み週の買い物リスト（取得と差分の永続化）。
- free / 未ログインが週間を開いたときの **ロック付きプレビュー＋アップグレード導線**。
- 各エンドポイントの認可（未ログイン 401 / free 403）。
- `spec.md` の更新（週間・プレミアム表・API 一覧。旧「永続化だけ premium」記述の上書き）。

### 対象外
- 価格・決済（Stripe 等）・広告モデル。今回は線引きのみ。
- **単発フローは一切変えない**: 単発の献立検索、献立詳細・レシピ、単発の買い物リスト、
  手持ちの食材から探す、履歴、お気に入り。
- 既存の永続化（チェック・手動品目）の実装そのもの。premium 判定の下に置き直すだけ。

## 3. 設計判断

### 3.1 「深さで課金」する（決定）

free は単発（`GET /menus/suggest` とその周辺）で、献立アプリの中核価値を無料で提供する。
premium は週間まわり。丸ごと有料化や広告と違い、無料利用者が価値を体験してから壁に当たるため、
取り上げの度合いを最小にしつつ upgrade 動機を作れる。

### 3.2 ロック付きプレビューで見せる（決定）

free / 未ログインが週間画面を開いたら、生成UIの代わりに「プレミアムなら1週間まとめて計画できる」
という価値提示とログイン/アップグレード導線を出す。**存在を隠さない**（隠すと upgrade 動機が
弱い）。ただし常設の押し売りにはせず、週間の文脈でだけ出す（基盤設計3.6「文脈に触れたときだけ」）。

### 3.3 未ログインは 401、ログイン済み free は 403（決定）

週間エンドポイントは「ログイン＋premium 加入」の二段の要件。未ログインは 401（`token-invalid`）、
ログイン済みだが free は 403（`ErrPremiumRequired`。plan-split で写像済み）。フロントは
401→ログイン導線、403→アップグレード導線に分岐する。

### 3.4 grandfather しない（決定）

既存の free 利用者がすでに保存した週間献立は、**データは削除しない**が、free の間は一覧が 403 で
**見えない**。upgrade すれば再び見える。特別扱い（旧データだけ閲覧可）を入れると認可の分岐が
増え、実装と説明が複雑になる。データを消さないので、取り上げは可逆である。

### 3.5 単発の買い物リストと手持ち食材は free 据え置き（決定）

- `POST /shopping-list`（ステートレス・献立IDから導出）は free のまま、挙動も変えない。
  最大7件を受ける形も据え置く（**件数で free を縛らない**）。free は週間献立を生成できないため、
  7件ぶんの献立を自然に揃える経路が無く、実害のある抜け道にならない。過剰な制限は入れない（YAGNI）。
- 手持ちの食材から探す（`POST /menus/search-by-ingredients`）は単発候補を返すため free。

## 4. free / premium の線引き

| 機能 | free（未ログイン可の範囲は現状踏襲） | premium（要ログイン＋加入） |
| --- | --- | --- |
| 単発の献立検索（1食分） | ○ | ○ |
| 献立詳細・レシピ3件 | ○ | ○ |
| 単発の買い物リスト（`POST /shopping-list`） | ○ | ○ |
| 手持ちの食材から探す | ○ | ○ |
| 履歴 / お気に入り | ○ | ○ |
| **週間献立の提案（1週間分）** | **×（ロック）** | ○ |
| **1日だけ引き直し** | **×** | ○ |
| **週間献立の保存 / 一覧 / 削除** | **×** | ○ |
| **保存済み週の買い物リスト（取得）** | **×** | ○ |
| **同・差分の永続化（チェック/手動品目/非表示）** | **×** | ○ |

保存上限（premium 50件）とチェック永続化は premium のまま。週間まわり一式が premium の傘に入る。

## 5. ドメイン層

`domain.Entitlement` に週間まわりの権限を足す（基盤設計5.2 と同型・メソッドで導出）。

```go
// CanUseWeeklyPlanning は週間献立の計画一式（提案・保存・週間の買い物リスト）を
// 使えるかを返す。premium だけ true。ゼロ値の Entitlement は false（安全側）。
func (e Entitlement) CanUseWeeklyPlanning() bool {
	return e.Plan() == PlanPremium
}
```

`CanPersistShoppingList()`（既存）はそのまま残す。週間の買い物リストは
`CanUseWeeklyPlanning()` の傘下に入るが、永続化の可否判定として既存の呼び出しは維持する
（両者とも premium で true になり整合する）。

## 6. サービス / API の認可

現状の各エンドポイントに premium 判定を足す。判定は既存 `EntitlementService.For(userID)` を引き、
`ent.CanUseWeeklyPlanning()` が false なら `service.ErrPremiumRequired`（403）を返す。

| エンドポイント | 現状 | 変更後 |
| --- | --- | --- |
| `POST /menus/suggest-weekly` | 公開（OptionalAuth） | **RequireAuth + premium**。未ログイン401 / free403 |
| `POST /menus/reroll-day` | 公開（OptionalAuth） | **RequireAuth + premium** |
| `POST /weekly-menus`（保存） | RequireAuth | + premium 判定。free403 |
| `GET /weekly-menus`（一覧） | RequireAuth | + premium 判定。free403 |
| `DELETE /weekly-menus/:id` | RequireAuth | + premium 判定。free403 |
| `GET /weekly-menus/:id/shopping-list` | RequireAuth（free可・導出のみ） | + premium 判定。free403 |
| `PUT /weekly-menus/:id/shopping-list` | RequireAuth + premium（既存） | 変更なし |

- **`suggest-weekly` / `reroll-day` は公開→認証必須に変わる**のが最大の挙動変更。
  ルートの `OptionalAuth` を `RequireAuth` に差し替え、ハンドラ/サービスで premium を判定する。
- 履歴記録: 週間検索の履歴は premium 利用者の経路でのみ発生する（単発は free のまま記録）。
  未ログインで記録しない既存の best-effort はそのまま働く。
- 認可判定はハンドラの入口で行い、`service` を薄く保つ（既存 `ReplaceOverrides` が
  サービス内で `CanPersistShoppingList` を見ているのと揃えるか、ハンドラで揃えるかは実装時に
  1つに決める。**判定の位置は1箇所に統一する**）。

## 7. フロントエンド

- `/weekly`（週間の提案）と保存一覧の画面: `useCurrentUser()` の `plan` を見て、
  **premium 以外はロック付きプレビュー＋CTA** を出す（生成UI・保存UIは出さない）。
  - 未ログイン → 「ログインして…」＋ログイン導線。
  - ログイン済み free → 「プレミアムで1週間まとめて計画」＋アップグレード導線。
- 週間への導線（ナビ/リンク）は**見せる**（ロック表示）。単発フローの画面は一切変えない。
- API 由来の 401/403 も保険として `<ErrorMessage>` 経由でハンドルするが、
  主経路は「開く前にプランで出し分け」。文言のプラン差はサーバ真実（403 detail）に寄せる既存流儀。

## 8. テスト戦略

| 層 | 検証 |
| --- | --- |
| domain | `CanUseWeeklyPlanning()`: premium true / free・ゼロ値 false |
| handler | 各 weekly エンドポイントが 未ログイン401・free403・premium200。契約テスト |
| handler | `suggest-weekly`/`reroll-day` が **公開でなくなった**こと（未認証401）を固定 |
| frontend | `/weekly` が 未ログイン/free で locked preview、premium で生成UI。保存一覧も同様 |
| frontend | 単発フローが**変わっていない**こと（回帰） |
| E2E | premium で週間が使える／free で週間がロックされる |

`ErrPremiumRequired` は写像済みなので追加の写像は不要。

## 9. 実装順序の見取り図

詳細は writing-plans で作成する。おおよその依存順序:

1. `domain.Entitlement.CanUseWeeklyPlanning()`
2. `suggest-weekly` / `reroll-day` を RequireAuth + premium に（公開→認証の変更）
3. `weekly-menus` 保存/一覧/削除に premium 判定
4. `GET /weekly-menus/:id/shopping-list` に premium 判定
5. フロント: `/weekly`・保存一覧のロック付きプレビュー＋CTA
6. E2E
7. `spec.md` 更新（週間・プレミアム表・API。旧「永続化だけ premium」を上書き）

## 10. 本番影響・ロールアウト

- マイグレーション不要（スキーマ変更なし）。デプロイ＝即、本番の free から週間が消える。
- これは live な free 機能の取り上げ（1.1）。告知の要否は運用判断（本設計では扱わない）。
- 既存 free ユーザーの保存済み週間献立は残る（3.4）。upgrade で復帰。

## 11. 未決事項

- ロック付きプレビューの具体的な文言・ビジュアル（実装時に frontend-design で詰める）。
- 週間検索の履歴を premium 化に伴いどう扱うか（記録し続ける想定だが、単発と混ざる見え方は
  実装時に確認）。
- 告知・移行のユーザーコミュニケーション（本設計の対象外）。
