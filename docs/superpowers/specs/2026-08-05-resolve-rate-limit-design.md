# 食材テキスト解決のレート制限（設計）

- 日付: 2026-08-05
- 対象: `POST /api/v1/ingredients/resolve`（`feature/ingredient-text-resolve` で実装済み）
- 前提: [2026-08-03-resolve-rate-limit-notes.md](2026-08-03-resolve-rate-limit-notes.md) のブレスト結果
- 目的: LLM 呼び出しのコストに天井を作る

## 1. 背景

`/ingredients/resolve` は認証不要で、レート制限は既存の `searchLimit`
（60回/分/IP）を共有している。これはレシピ検索向けの値で、LLM を叩くエンドポイントには緩すぎる。

1回の問い合わせは Claude Haiku 4.5 で約 0.3円（典型3語）〜 0.7円（上限20語）。
無認証 × 60回/分では **1IPあたり 1,000〜2,500円/時間、2.6〜6万円/日** に達する。
毎回ランダムな語を投げられるとキャッシュが効かず、200文字/20語の上限内なら
正当なリクエストと区別がつかない。

平常時はほとんど LLM に行かない（完全一致で解ける語は問い合わせ自体が発生せず、
解決キャッシュはグローバル）。問題は平常運転ではなく、悪用されたときの上限が無いことにある。

プロンプトキャッシュは使えない。Claude Haiku 4.5 のキャッシュ最小サイズは 4,096 トークンで、
このプロンプトは約1,400トークン。`cache_control` を付けても静かに無効になる。

## 2. 制限の対象

**「読み取る」（`/ingredients/resolve`）だけを制限する。**

「この食材で探す」（`/menus/search-by-ingredients`）は DB を引くだけで料金が増えないため対象外。
チェックボックスで選んで探す経路は非ログインでも無制限のまま残す。
これにより `spec.md` 2.9 の「未認証でも使える」を崩さずにコストだけ抑えられる。

## 3. 層の構成

| 層 | 実装 | キー | 上限 | 状態 |
| --- | --- | --- | --- | --- |
| バースト制御 | 既存 `RateLimiter`（メモリ） | IP | 60回/分 | 変更なし |
| 利用者の日次 | 新規（DB） | IPのHMAC / ユーザーID | 10 / 30回/日 | 新規 |
| 全体の日次 | 新規（DB） | なし（1行） | 300回/日 | 新規 |

### 3.1 なぜ DB に持つのか

本番は Cloud Run の `min-instances=0`・`max-instances=2`。メモリ上のカウンタは
**アイドルのたびに消え、2インスタンスある間は実効上限が2倍**になる。
日次上限としては当てにならず、全体上限に至っては原理的に守れない。

分単位のバースト制御はメモリのままでよい。ウィンドウが1分なので、
インスタンスが落ちて消えても守りたい性質（瞬間的な連打を止める）は保てる。

### 3.2 なぜ利用者ごとの制限だけでは足りないのか

会員登録は無料でアカウントをいくつでも作れ、IPも変えられる。
利用者ごとの制限は行儀の良い使われ方を整えるだけで、総額の天井にはならない。
**全体上限だけが請求額の天井を確定させる。**

### 3.3 なぜ非ログインを IP で数えるのか

ブラウザ保存（localStorage）はシークレットウィンドウで無限にリセットできるため、
会員登録への導線にはなってもコスト対策にはならない。
欠点は共有IP（社内・学校・携帯NAT）で他人と枠を共有すること。
10回/日という値は、この巻き込みを見込んで想定利用（2〜3回）の約3倍を取っている。

## 4. 何を「1回」として数えるか

**LLM 呼び出しが発生したときだけ数える。** 完全一致やキャッシュで解けたリクエストは
料金が発生しないため、枠を消さない。

上限値をコストから決めた以上、料金が発生しない操作で枠を消すのは筋が悪い。
解決キャッシュがグローバルであることにより定常状態で LLM に行くのは1〜3割なので、
リクエスト単位で数えると実効的に3〜10倍きつい制限になってしまう。

加算は **`gateway.Resolve` の呼び出し1回につき1**。gateway 内部のリトライ（最悪2回）は
数えない。試算とわずかにズレるが、上限値には十分な余裕がある。

## 5. 制御の流れ

LLM 呼び出し単位で数えるため、**ミドルウェアで完結しない。** リクエスト到着時点では、
その語が①完全一致や②キャッシュで解けるかが分からず、LLM に行くかは
service が①②を試すまで確定しないため。

```
handler:  今日の残枠を「読むだけ」
          → allowLLM (bool) と 拒否理由 を service に渡す

service:  ① 食材マスタとの完全一致
          ② 解決キャッシュ
          ここで pending が残った場合だけ:
            allowLLM=false → ③ をスキップし Degraded と理由を立てて返す
            allowLLM=true  → gateway.Resolve を呼び、直後に3スコープを +1
```

判定は「前回までの実績」を読むので、同時実行中のリクエストは互いを見ない。
`max-instances=2` なので超過は最大でも数件、金額にして数円。これは受け入れる。

`/ingredients/resolve` には現在 `OptionalAuth` が付いていない（`backend/cmd/server/main.go`）。
ユーザーIDを取るために追加する。

### 5.1 判定の順序

**全体 → 利用者 の順に判定する。**

逆にすると、全体が詰まっているときに非ログインの人へ「ログインすると増えます」と出てしまう。
ログインしても改善しないため、誤導になる。

## 6. データモデル

マイグレーション `000014_create_resolve_usage_counters`:

```sql
CREATE TABLE resolve_usage_counters (
    usage_date date NOT NULL,
    scope      text NOT NULL,  -- 'ip' | 'user' | 'total'
    subject    text NOT NULL,  -- IPのHMAC / ユーザーID / '' (total)
    count      int  NOT NULL DEFAULT 0,

    PRIMARY KEY (usage_date, scope, subject),
    CONSTRAINT resolve_usage_counters_scope_valid
        CHECK (scope IN ('ip', 'user', 'total')),
    -- total は subject を持たない。他は必ず持つ。
    CONSTRAINT resolve_usage_counters_subject_matches_scope
        CHECK ((scope = 'total' AND subject = '')
            OR (scope <> 'total' AND btrim(subject) <> ''))
);
```

加算は3スコープぶんを1文の複数 VALUES で撃つ:

```sql
INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
VALUES ($1, 'total', '', 1), ($1, $2, $3, 1)
ON CONFLICT (usage_date, scope, subject)
DO UPDATE SET count = resolve_usage_counters.count + 1;
```

（非ログインは `('ip', <HMAC>)`、ログインは `('user', <userID>)`。）

### 6.1 日付の境界

**JST。** Cloud Run は UTC で動くので、日付は Go 側で計算して渡す。

`time.LoadLocation("Asia/Tokyo")` はコンテナに tzdata を要求するため使わず、
`time.FixedZone("JST", 9*3600)` を使う（日本にサマータイムはない）。

`now func() time.Time` を注入する形は既存 `RateLimiter` と揃える。日付境界のテストがこれで書ける。

### 6.2 IP のハッシュ化

`HMAC-SHA256(RESOLVE_IP_HASH_SECRET, IP)` の hex を保存する。
数を数えるには十分で、元に戻せない文字列なのでプライバシーポリシーの改定が要らない。

IP は `c.RealIP()` から取る（既存 `RateLimiter` と同じ。X-Forwarded-For / X-Real-IP を見てから
RemoteAddr に落ちる）。

### 6.3 行の掃除

日付ごとに行が増えるため、既存の運用コマンドに揃えて掃除の経路を足す:

```
go run ./cmd/resolutions prune-counters
```

30日より古い行を消す。30日残すのは、あとから使用量を振り返れるようにするため。

## 7. 設定

既存の「`0` で無制限」の約束に揃える。Vite プロキシ配下では全リクエストが単一IPに
集約されるため、開発と E2E ではこれで切る。

```
# --- 読み取り（LLM）の日次上限（0で無制限） ---
RESOLVE_DAILY_LIMIT_ANON=10
RESOLVE_DAILY_LIMIT_USER=30
RESOLVE_DAILY_LIMIT_TOTAL=300

# IPを数えるためのハッシュ鍵。openssl rand -base64 32 で生成する。
# **空にすると起動に失敗する**（設定忘れで生IPが保存されるのを防ぐため）。
RESOLVE_IP_HASH_SECRET=dev-only-secret-do-not-use-in-production
```

### 7.1 値の根拠

| 上限 | 値 | 根拠 |
| --- | --- | --- |
| 非ログイン | 10回/日 | 想定利用（2〜3回）の約3倍。共有IPで数人が重なっても通常利用は弾かれない。1IP最悪 約7円/日 |
| ログイン | 30回/日 | 非ログインの3倍。「ログインすると増えます」が実感できる差。1アカウント最悪 約21円/日 |
| 全体 | 300回/日 | 最悪 約210円/日（約6,300円/月）、典型的にはその半分。非ログイン100人が1日3回使っても届かない |

ログインユーザーに解決専用の分単位制限は設けない。既存の IP ベース 60回/分が
バースト制御として効いており、日次上限がある以上、層を増やす価値が薄いため。

## 8. API の変更

`degraded: bool` は残したまま `degradedReason` を足す。`degraded` の意味は
「LLM をスキップした」で変わらないため、潰す必要がない。

| `degradedReason` | 発生条件 |
| --- | --- |
| （省略） | `degraded` が false |
| `llm_error` | LLM への問い合わせが失敗した（既存の縮退） |
| `counter_unavailable` | カウンタが読めずフェイルクローズした（9.1） |
| `anon_daily_limit` | 非ログインの日次上限に達した |
| `user_daily_limit` | ログインユーザーの日次上限に達した |
| `service_daily_limit` | サービス全体の日次上限に達した |

`api/openapi.yaml` と `frontend/src/api/schema.d.ts` を更新する。

## 9. エラー処理

### 9.1 カウンタが読めないとき

**フェイルクローズする**（LLM をスキップし `counter_unavailable` を返す）。

カウンタと解決キャッシュは同じ Postgres にある。カウンタが読めない状況ではキャッシュも
死んでおり、**全語が LLM に行く＝最も高い状態**になっている。そこで素通しするのは、
コスト保護という目的そのものを裏切る。

利用者には①完全一致の結果が残るため、よくある食材は普通に通る。

これは「キャッシュの引きに失敗しても機能は止めない」（既存 `applyCache`）とは逆の判断になる。
キャッシュはコスト削減の仕組みで、失敗しても結果が同じだから止めなかった。
カウンタは請求額の天井そのものなので、失敗したら止める。

### 9.2 加算に失敗したとき

LLM 呼び出しはもう済んでいる。`slog.Warn` を出して結果はそのまま返す
（既存の `saveResolution` と同じ扱い）。数え漏れは許容する。

### 9.3 ログ

上限到達はすべて `slog.Warn` に出す。特に `service_daily_limit` は、
運用で気づけないと「なぜか読み取れない日」になる。

生の IP はログに出さない。

## 10. 画面

`ResolveResultPanel` を `degradedReason` で分岐させる。**値は5つ、文言は4つ。**

| 理由 | 文言 |
| --- | --- |
| `anon_daily_limit` | 今日の読み取り上限に達しました。ログインすると回数が増えます。<br>→ ログインへの導線を出す<br>下のリストから選んで探すことはできます。 |
| `user_daily_limit` | 今日の読み取り上限に達しました。明日また使えます。<br>下のリストから選んで探すことはできます。 |
| `service_daily_limit` | ただいま読み取りが混み合っています。時間をおいてお試しください。<br>下のリストから選んで探すことはできます。 |
| `llm_error` / `counter_unavailable` | 一部だけ読み取れました。残りは下から選んでください。（既存の文言、変更なし） |

ログインへの導線を出すのは `anon_daily_limit` のときだけ。他の3つではログインしても改善しない。

`counter_unavailable` を `llm_error` と同じ文言にするのは、利用者にとっては同じ
「今うまく読めない」だから。区別が要るのはログと運用の側だけなので、そこにだけ残す。

## 11. テスト

既存どおり TDD で、各層に置く。

| 層 | 見るもの |
| --- | --- |
| repository | UPSERT の加算、scope / 日付ごとの分離、スキーマ検証（既存 `ingredient_resolution_schema_test.go` に倣う） |
| 上限判定 | 境界値（10回目は通る・11回目で止まる）、`0` で無制限、全体→利用者の優先順位、JST の日付境界 |
| service | `allowLLM=false` で③をスキップし、①②の結果は返しつつ Degraded と理由が立つ |
| handler | 非ログイン/ログインでキーが切り替わる、IP がハッシュ化される |
| frontend | 4文言の出し分け、ログイン導線が `anon_daily_limit` のときだけ出る |

既存の e2e（`frontend/e2e/from-fridge.spec.ts`）は上限を `0` で切るため影響を受けない。

**既存テストの更新が要る。** service のシグネチャが変わるため、
`ingredient_resolve_test.go`（service / handler の両方）に手が入る。

## 12. スコープ外

- `/menus/search-by-ingredients` の制限（2章）
- 月次の上限・予算アラート。日次の全体上限で天井は確定するため、今は入れない
- 上限の管理画面。環境変数で足りる
- このブランチが `feat/remove-subscription` のマージ前から派生している件
  （`.env.example` に Stripe の記述が残っている）。この設計とは独立の問題として、
  マージ時に扱う

## 13. 参考: 既存の部品

| 部品 | 場所 |
| --- | --- |
| レート制限（分単位・メモリ・IPキー） | `backend/internal/handler/ratelimit.go` の `RateLimiter` |
| ログイン任意の認証 | `backend/internal/handler/middleware.go` の `OptionalAuth` |
| userID の取り出し | 同ファイルの `UserIDFromContext` |
| 解決エンドポイントのルート登録 | `backend/internal/handler/ingredient_resolve.go` の `RegisterRoutes` |
| 解決の本体（①②③） | `backend/internal/service/ingredient_resolve.go` の `Resolve` |
| 縮退の返し方 | `ResolveResult.Degraded`（同ファイル） |
| 縮退の画面表示 | `frontend/src/features/menu/ResolveResultPanel.tsx` |
| 運用コマンド | `backend/cmd/resolutions/main.go` |
