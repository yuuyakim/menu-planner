# プレミアム加入導線の再建（料金プランページと文脈内導線）設計

- 日付: 2026-07-27
- 対象: `menu-planner`（献立くん）
- 状態: 設計確定・未実装
- 前提:
  - `2026-07-24-weekly-premium-retier-design.md`（週間まわりを premium 限定にする再編。`PremiumLock` はここで導入された）
  - `2026-07-25-payment-core-design.md`（決済コア。`/checkout` と Stripe Checkout）
  - `2026-07-25-plan-management-design.md`（`/account`。**対象外に「料金プラン LP（別サブプロジェクト）」と明記されており、本設計はその積み残しを回収する**）
- ブランチ: `feat/upgrade-entrypoints`

## 1. 背景と目的

プレミアム機能（週間献立まわり）と決済（`/checkout` → Stripe Checkout）はいずれも実装・マージ済みだが、**加入画面へ到達する導線が実質的に機能していない**。現状の `/checkout` への入口は次の2つしかない。

| 導線 | 場所 | 問題 |
| --- | --- | --- |
| 買い物リストの案内バナー | `frontend/src/features/menu/ShoppingListPage.tsx:387` | free が初めてチェックしたときの1回だけ。`useOnceFlag('premium-shopping')` が端末に恒久記録し、閉じると二度と出ない |
| プランの管理 | `frontend/src/features/billing/AccountPage.tsx:109` | ログイン済みのみ。「アカウント設定」の中にあり、加入前の利用者が探しに行く場所ではない |

そして**最も効くはずの場所が抜けている**。`/weekly`・`/saved-weekly` は premium の中核機能であり、free はここでブロックされる。加入への意思が最も高まる瞬間だが、`PremiumLock` はログイン済み free に対してリンクを持たないただのテキストしか出さない。

```
frontend/src/features/premium/PremiumLock.tsx:14-16
// 決済フローは未実装（設計スコープ外）。ログイン済み free の
// 「アップグレード」導線は当面は案内文言に留め、決済導入時に差し替える。
```

これは `2026-07-24-weekly-premium-retier-design.md` の時点で残された TODO であり、決済コア（PR #147）で `/checkout` を実装した際に差し替えられないまま残った取りこぼしである。

### 1.1 これは仕様違反である（新機能ではない）

`spec.md:297-298` は既に導線を要求している。

> **未ログイン/free が週間の文脈（2.2の週間画面・保存一覧）を開いたときは、ロック付きプレビューにログイン/アップグレード導線を出す**

`PremiumLock` の修正は仕様への追随であり、設計判断を要しない。

### 1.2 常設導線は仕様の方針転換である

一方 `spec.md:345-352` は常設導線を明示的に禁じている。

> 常設の押し売りにはせず、その文脈を開いたときにだけ出す。

本設計はこの方針を**意図的に転換する**。理由は、この方針が書かれた 2026-07-24 から前提が変わったこと。同じ再編で**週間献立という看板機能を free から取り上げた**（`spec.md:319-322`「これは意図的な『取り上げ』である」）。取り上げた以上、「何がいくらで使えるのか」を知る場が存在しないこと自体が不誠実になる。特に未ログインの利用者は、現状では特商法ページ以外に料金を知る手段がない。

ただし転換は限定的にする。各画面へ「プレミアムにする」ボタンを散らすことはせず、**料金ページ1枚とそこへの控えめな入口**に留める。

## 2. スコープ

### 対象

- `PremiumLock` に加入導線を戻す（仕様違反の解消）
- 新ルート `/pricing`（公開・未ログイン可）＝ free / premium の比較と料金の提示
- 全ページ共通フッターに「料金プラン」リンク
- ホーム（`/`）に free 向けの紹介カード1枚
- バックエンド: `GET /billing/plan`（**未認証可**。料金の公開提示用）
- `spec.md` 2.11 の方針更新と 5.7 の API 表への追記

### 対象外

| 項目 | 理由 |
| --- | --- |
| `ShoppingListPage` の once-flag バナーの作り直し | 一度閉じると二度と出ない問題は残るが、`/pricing` とフッターで恒久的な経路ができるため緊急性が下がる。別件とする |
| `frontend/src/features/legal/content/tokushoho.md:11` の「月額 300円」 | 法務文書は版として固定されるべきで、動的化すべきでない。バックエンドとの二重管理は承知のうえで維持する |
| Stripe API から価格を引く | 本来の正は Stripe の Price だが、今回はコード内の定数集約に留める |
| ヘッダへの導線追加 | ヘッダは既にナビ6本＋認証表示で混んでおり、`App.tsx:63` に狭い画面での折り返し問題がコメントされている。`AuthMenu.tsx:55-58` の「重複した勧誘をしない」判断も維持する |

## 3. 設計判断

### 3.1 公開エンドポイントを新設する（決定）

`backend/internal/handler/billing.go:43-47` の billing 系は全て `requireAuth` で、価格 `300` は `backend/internal/service/billing.go:85` の `Preview` 内にリテラルで埋まっている。`/pricing` を未ログインに見せるには公開の取得手段が要る。

**`GET /api/v1/billing/plan` を認証なしで追加する。** 既存 `GET /billing/preview` を未認証に開放する案は採らない。`preview` は申込確認画面（特商法12条の6）のための**個人化データ**（`trialEligible`・`firstBillingAt`）を含み、未認証に返す値は実際の申込内容とずれる。特商法上の表示と同じ API を推測値で共用するのは誤表示のリスクになる。責務を分ける。

| | `GET /billing/plan`（新規・公開） | `GET /billing/preview`（既存・要認証） |
| --- | --- | --- |
| 用途 | 料金の公開提示 | 申込確認（特商法12条の6） |
| `price` / `currency` / `trialDays` | ○ | ○ |
| `trialEligible` / `firstBillingAt` / `planManagementPath` | × | ○ |

フロントに価格定数を置く案も採らない。`tokushoho.md` が既に 300 を持っており、3箇所目を作ると値上げ時に必ず取りこぼす。

### 3.2 価格リテラルを1箇所に集約する（決定）

`BillingService` に `Plan() PlanInfo` を足し、`Price: 300, Currency: "jpy"` の定義をその中だけに置く。`Preview` はこれを呼ぶ。`trialDays` は既に `main.go:158` から注入されているため、そのフィールドを引き継ぐ。

### 3.3 `PremiumLock` は `/checkout` に直行させる（決定）

`/pricing` を経由させない。ロックに当たった利用者は既に「この機能が使いたい」状態であり、比較表を挟むのは遠回りになる。`/checkout` 自身が特商法12条の6の申込確認画面として価格・無料期間・解約方法・返金をすべて提示するため、情報不足にはならない。比較を見たい利用者のために副リンクで `/pricing` を添える。

**未ログインも同じ CTA を出す。** `RequireAuth` が `state.from` を残し（`RequireAuth.tsx:24`）、`LoginPage.tsx:94` がログイン後にそこへ戻すため、往復は既存の仕組みで成立する。新しい仕掛けは要らない。ただしボタンの下に「ログインが必要です」を添え、`spec.md:314`「401→ログイン導線」の趣旨を満たす。

### 3.4 `/pricing` の CTA は加入状態で分ける（決定）

`/pricing` は公開ページなので premium の利用者も到達しうる。premium が `/checkout` を踏むと `ErrAlreadySubscribed`（409）で行き止まりになる。`AccountPage.tsx:70-75` が既に同じ配慮をしているので、それに揃える。

| 状態 | CTA | 遷移先 |
| --- | --- | --- |
| premium | プランを管理する | `/account` |
| free・未ログイン | プレミアムを試す | `/checkout` |

### 3.5 比較表に保存件数を書かない（決定）

`spec.md:341-343` は「上限の数値を返さない理由。フロントエンドが件数を持つと二重管理になる」と定めている。`/pricing` の表には「週間献立の保存」とだけ書き、50件という数値は載せない。

## 4. バックエンド

### 4.1 `service/billing.go`

```go
// PlanInfo は誰にでも同じ、プランの公開情報。
type PlanInfo struct {
	Price     int
	Currency  string
	TrialDays int
}

// Plan はプランの公開情報を返す。価格の定義はここだけに置く。
func (s *BillingService) Plan() PlanInfo {
	return PlanInfo{Price: 300, Currency: "jpy", TrialDays: s.trialDays}
}
```

`Preview` は `p := s.Plan()` を呼び、その値を詰めるだけにする。価格リテラルは `Preview` から消える。

### 4.2 `handler/billing.go`

`RegisterRoutes` に1行足す。**`requireAuth` を渡さない**のがこのルートの要点なので、その旨をコメントに残す。

```go
// 料金の提示は未ログインにも見せる（/pricing）。個人依存の値は返さないため認証を要さない。
g.GET("/billing/plan", h.Plan)
```

レスポンス:

```json
{ "price": 300, "currency": "jpy", "trialDays": 5 }
```

### 4.3 `api/openapi.yaml`

`/billing/plan` を `security: []` で追記し、`make gen-api` で `frontend/src/api/schema.d.ts` を再生成してコミットする（`Makefile:95`「api/openapi.yaml が API 仕様の正」）。

## 5. フロントエンド

### 5.1 `features/billing/api.ts`

```ts
export interface PlanInfo {
  price: number
  currency: string
  trialDays: number
}

/** getPlan はプランの公開情報を取得する（未ログインでも呼べる）。 */
export function getPlan(): Promise<PlanInfo> {
  return apiGet<PlanInfo>('/billing/plan')
}
```

クエリキーは既存の `['billing', 'subscription']` / `['billing', 'preview']` に揃えて `['billing', 'plan']` を使う。

### 5.2 `features/premium/PremiumLock.tsx`（修正）

ログイン済み free に出している `<p>プレミアムプランでご利用いただけます。</p>` を導線に差し替え、未ログインの「ログインする」も同じ形に統一する。冒頭の「決済フローは未実装（設計スコープ外）」コメントを削除する。

```
┌──────────────────────────┐
│ 1週間の献立はプレミアム限定  │  ← title（既存 props）
│ 7日分をまとめて組み立てます  │  ← description（既存 props）
│                          │
│ ┌────────────────────┐   │
│ │プレミアムにアップグレード│   │  → /checkout
│ └────────────────────┘   │
│ 月額300円・5日間無料        │  ← getPlan()
│ ログインが必要です          │  ← 未ログインのときだけ
│ プランの詳細を見る         │  → /pricing
└──────────────────────────┘
```

props（`title` / `description`）は変えない。呼び出し側（`WeeklyPage.tsx:122`・`SavedWeeklyPage.tsx:88`）は無改修。

**premium 向けの分岐は持たない。** 呼び出し側が premium でないときにだけ `PremiumLock` を描画するため、この中に premium の枝を作ると到達しない死んだコードになる。既存の `isLoading` 中のローディング表示はそのまま残す（`WeeklyPage.tsx:117-119` が判定中の描画をこの中に委ねている）。

`getPlan` の取得中・失敗時も CTA は出す。**価格行だけを落とす。** 価格が引けないことで加入導線ごと消えると、今の不具合を再現してしまう。

### 5.3 `features/pricing/PricingPage.tsx`（新規）

公開ページ。比較表の内容は `spec.md:305-312` を正とする。

| 機能 | 無料 | プレミアム |
| --- | --- | --- |
| 献立を1食提案・レシピ・履歴・お気に入り | ○ | ○ |
| 冷蔵庫の食材から探す | ○ | ○ |
| 買い物リストの作成 | ○ | ○ |
| 1週間の献立を組み立てる | × | ○ |
| 1日だけ引き直す | × | ○ |
| 週間献立の保存・呼び出し | × | ○ |
| 買い物リストのチェックを残す | × | ○ |

料金と無料期間は `getPlan()` から出す。CTA は 3.4 の表に従い、`useCurrentUser` で加入状態を見る。

**加入状態の判定中は CTA を出さず、比較表と料金は出す。** 先に free 向けの CTA を描いてから premium 向けに差し替えると、premium の利用者に一瞬「プレミアムを試す」が見える。`AuthMenu.tsx:29-31` が同じ理由で判定前の描画を避けている。比較表と料金は加入状態に依らないため、待たせずに出す。

末尾に `/legal/tokushoho` へのリンクを添える。

### 5.4 `components/Footer.tsx`（修正）

`footerLinks` の先頭に足すだけ。フッターの控えめな配色をそのまま使う。

```ts
const footerLinks = [
  { to: '/pricing', label: '料金プラン' },
  { to: '/legal/tokushoho', label: '特定商取引法に基づく表記' },
  ...
]
```

### 5.5 `features/home/HomePage.tsx`（修正）

機能カード（`entries`）の下に紹介カードを1枚。`useCurrentUser` は既に呼ばれている。

- `isLoading` 中は出さない（`AuthMenu.tsx:29-31` と同じちらつき防止）
- `user?.plan === 'premium'` なら出さない
- free・未ログインに出す。CTA は「プランを見る」→ `/pricing`

### 5.6 `app/App.tsx`（修正）

法務ページの並びに `/pricing` を追加する。`RequireAuth` で包まない。

```tsx
{/* 料金の提示は未ログインにも見せる。加入を検討する前に見る画面のため。 */}
<Route path="/pricing" element={<PricingPage />} />
```

## 6. テスト

TDD で進める（テスト → 実装の順にコミットする）。

### バックエンド

- `service`: `Plan()` が価格・通貨・注入された `trialDays` を返す
- `service`: `Preview()` が `Plan()` と同じ価格を返す（集約後も値が変わっていないこと）
- `handler`: `GET /billing/plan` が**未認証で 200** を返す
- `handler`: レスポンス JSON に `trialEligible` 等の個人依存の値が**含まれない**

### フロントエンド（Vitest + MSW）

`test/handlers.ts` に `/billing/plan` のハンドラを追加する。

- `PremiumLock`: ログイン済み free に `/checkout` へのリンクが出る
- `PremiumLock`: 未ログインにも `/checkout` へのリンクと「ログインが必要です」が出る
- `PremiumLock`: `getPlan` が失敗しても CTA は残り、価格行だけ消える
- `PricingPage`: 未ログインで比較表と料金が出る
- `PricingPage`: premium には `/account` への CTA が出て、`/checkout` へのリンクは出ない
- `PricingPage`: 加入状態の判定中は CTA が出ない（比較表は出る）
- `PricingPage`: 保存件数の数値（50）が表示に含まれない
- `Footer`: 「料金プラン」リンクが出る
- `HomePage`: free に紹介カードが出る / premium には出ない / 判定中は出ない
- `App`: `/pricing` が未ログインで描画される（`App.legal.test.tsx` の流儀に倣う）

### E2E（`frontend/e2e/premium.spec.ts` に追加）

1. free が `/weekly` を開くとロックが出て、CTA から `/checkout` に着く
2. 未ログインが `/pricing` の CTA を押すと `/login` を経由し、ログイン後 `/checkout` に戻る

## 7. `spec.md` の更新

### 2.11

`spec.md:345-352` の引用ブロックを書き換える。

- 削除: 「常設の押し売りにはせず、その文脈を開いたときにだけ出す。」
- 追加: 文脈内の案内を主としつつ、料金ページ `/pricing` を常設する。週間献立を free から取り上げた（2.11「意図的な取り上げ」）以上、何がいくらで使えるのかを知る場が無いこと自体が不誠実になったため。ただし各画面へのボタン散らしはしない。
- `AuthMenu` にバッジのみを出す方針は従来通り維持する旨を残す。

### 5.7

API 表に1行足す。

| GET | `/billing/plan` | 不要 | プランの公開情報（価格・通貨・無料日数）。`/pricing` の表示に使う |
