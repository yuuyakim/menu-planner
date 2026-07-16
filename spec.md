# 献立提案アプリケーション 仕様書

> 本書は `PLAN.md` を出発点に、壁打ちで確定した設計判断を仕様として固めたもの。

## 1. 概要

ジャンル（和食・洋食など）と調理難易度（簡単・普通・凝っている）で献立を提案し、
選ばれた献立のレシピ掲載サイトを3件、新しいタブで開けるようにするWebアプリケーション。
1食分の提案と、1週間分（7日 × 夕食）の献立表作成の2モードを持つ。

### 1.1 確定した設計判断

| 論点 | 決定 | 理由 |
| --- | --- | --- |
| 献立候補の供給源 | 自前の献立マスタ（DB） | 検索が高速・決定的でTDDと相性が良い |
| レシピサイト取得 | 外部検索API | 主要レシピサイトは robots.txt でクローラを拒否しており、直接スクレイピングは規約違反リスクが高い |
| アーキテクチャ | Go REST API + React/TS SPA | 責務が明確で、GoとTypeScriptの双方をモダンに活かせる |
| DB | ローカル Postgres → 本番 Neon 無料枠 | 同一のPostgresで環境差分が出ず、コストがかからない |
| 認証 | Goで自前実装 | 外部依存ゼロでローカル完結。インフラ抽象化方針と一致する |
| 週間献立 | 7日 × 夕食1食 | MVPとして適切な粒度 |
| 履歴 | 提示された献立1件＝1レコード、FIFO 15件 | 重複回避ロジックにそのまま再利用できる |

### 1.2 MVPスコープ

**含む**：献立検索（1食分／1週間分）、レシピ3件提示、認証、履歴15件、お気に入り

**含まない（将来対応）**：アレルギー・苦手食材の除外、買い物リスト生成、朝食・昼食の献立、
献立のユーザー投稿、栄養価計算

---

## 2. 機能要件

### 2.1 献立検索（1食分）

- ジャンルと難易度を指定して献立を1件提案する
- ジャンル・難易度はいずれも未指定（＝すべて）を許容する
- 直近の履歴に存在する献立は候補から除外する（後述の重複回避ルール）
- 提案結果に対して「別の献立を見る」で引き直せる
- 提案が確定した時点で履歴に1件記録する

### 2.2 献立検索（1週間分）

- 7日分（各日1献立、夕食想定）をまとめて提案する
- 重複回避ルール：
  1. 同一献立が同じ週内に2度出現しない
  2. 同一ジャンルが3日以上連続しない
  3. 直近履歴15件に含まれる献立は可能な限り避ける（候補が枯渇する場合はこの条件を緩和する）
- 特定の日だけを引き直せる（他の日は保持したまま、重複回避ルールを再適用する）
- 提案が確定した時点で、7件すべてを履歴に記録する

### 2.3 レシピサイト提示

- 献立を選択すると、外部検索APIで「{献立名} レシピ」を検索し、上位3件を提示する
- 各件について、タイトル・ドメイン・スニペットを表示する
- リンクは `target="_blank" rel="noopener noreferrer"` で新しいタブで開く
- 検索結果が3件未満の場合は取得できた件数のみを表示する
- 外部APIが失敗した場合は献立の提案自体は成功として扱い、レシピ欄にエラー表示とリトライ導線を出す
  （レシピ取得の失敗が献立提案を巻き込んで落とさない）

### 2.4 認証

- Google SSO（OAuth 2.0 / OIDC 認可コードフロー + PKCE）
- メールアドレス + パスワード（bcrypt、cost 12）
- 同一メールアドレスに対する両方式の共存を許可する（`auth_identities` で表現）
- セッションはJWT（アクセストークン15分 / リフレッシュトークン30日）
- トークンは HttpOnly + Secure + SameSite=Lax の Cookie で受け渡す
- 未認証でも献立検索は利用できる（履歴とお気に入りのみ要認証）

### 2.5 履歴

- 提示された献立1件につき1レコードを記録する
- ユーザーごとに最新15件を保持し、16件目の挿入時に最古の1件を削除する（FIFO）
- 履歴一覧は新しい順に表示し、各件からレシピ再検索へ遷移できる
- 履歴は手動で個別削除・全件削除できる

### 2.6 お気に入り

- 献立をブックマークし、いつでも一覧から参照できる
- 件数の上限は設けない
- 履歴と異なり自動削除されない

---

## 3. アーキテクチャ

### 3.1 全体構成

```
┌──────────────────────────────────────────┐
│  Browser                                  │
│  React 19 + TypeScript + Vite            │  ← プレゼンテーション層
└────────────────┬─────────────────────────┘
                 │ REST / JSON (HttpOnly Cookie)
┌────────────────▼─────────────────────────┐
│  Go API Server (echo)                     │
│  ┌────────────────────────────────────┐  │
│  │ handler   … HTTP境界・DTO変換       │  │  ← Controller
│  ├────────────────────────────────────┤  │
│  │ service   … ドメインロジック         │  │  ← ビジネスロジック層
│  ├────────────────────────────────────┤  │
│  │ repository / gateway … I/O抽象      │  │  ← データアクセス層
│  └────────────────────────────────────┘  │
└──────┬───────────────────────┬───────────┘
       │                       │
┌──────▼──────┐        ┌───────▼──────────┐
│ PostgreSQL  │        │ 外部検索API       │
│ (local:Docker│        │ (Brave Search)   │
│  prod:Neon) │        └──────────────────┘
└─────────────┘
```

### 3.2 Web3層の対応

| 層 | 実体 | 責務 |
| --- | --- | --- |
| プレゼンテーション層 | React SPA + Go handler | 画面描画、HTTP境界、リクエスト検証、DTO変換 |
| ビジネスロジック層 | Go service | 献立選定、重複回避、履歴FIFO、認証・認可 |
| データアクセス層 | Go repository / gateway | Postgres永続化、外部検索API呼び出し |

**依存の向き**：`handler → service → repository`（一方向）。
service は repository の**インターフェース**にのみ依存し、実装を知らない。
インターフェースは service 側パッケージで定義する（依存関係逆転の原則）。

### 3.3 インフラ抽象化

PLAN.mdの「一旦ローカルで動かす、インフラは抽象化して留める」方針に対応する。
以下をインターフェースで抽象化し、実装差し替えのみでクラウド移行できる状態にする。

```go
// 献立の永続化
type MenuRepository interface {
    FindByFilter(ctx context.Context, f MenuFilter) ([]Menu, error)
    FindByID(ctx context.Context, id MenuID) (*Menu, error)
}

// レシピサイト検索（Brave / Google CSE / スタブを差し替え可能）
type RecipeSearchGateway interface {
    Search(ctx context.Context, menuName string, limit int) ([]RecipeLink, error)
}

// 認証プロバイダ（自前実装 / 将来のIDaaS を差し替え可能）
type AuthProvider interface {
    Authenticate(ctx context.Context, cred Credential) (*User, error)
}
```

抽象化の対象は上記に留め、メッセージキューやオブジェクトストレージなど
現時点で使わないものは**先回りして抽象化しない**。

---

## 4. データモデル

### 4.1 ER概要

```
users ──< auth_identities
  │
  ├──< search_histories >── menus
  └──< favorites        >──┘
                            │
menus >── menu_genres ──────┘   (将来: 1献立に複数ジャンル)
```

### 4.2 テーブル定義

#### users
| カラム | 型 | 制約 |
| --- | --- | --- |
| id | uuid | PK |
| email | text | UNIQUE NOT NULL |
| display_name | text | NOT NULL |
| created_at | timestamptz | NOT NULL DEFAULT now() |
| updated_at | timestamptz | NOT NULL DEFAULT now() |

#### auth_identities
| カラム | 型 | 制約 |
| --- | --- | --- |
| id | uuid | PK |
| user_id | uuid | FK → users.id, ON DELETE CASCADE |
| provider | text | NOT NULL（`google` \| `password`） |
| provider_uid | text | NULL可（google の sub。password では NULL） |
| password_hash | text | NULL可（password のみ。bcrypt） |
| created_at | timestamptz | NOT NULL DEFAULT now() |

- UNIQUE (provider, provider_uid)
- CHECK: `provider='password'` なら `password_hash IS NOT NULL`、
  `provider='google'` なら `provider_uid IS NOT NULL`

#### menus（献立マスタ）
| カラム | 型 | 制約 |
| --- | --- | --- |
| id | uuid | PK |
| name | text | NOT NULL（例: 親子丼） |
| name_kana | text | NOT NULL（検索・ソート用） |
| genre | text | NOT NULL（`japanese` \| `western` \| `chinese` \| `other`） |
| difficulty | text | NOT NULL（`easy` \| `normal` \| `elaborate`） |
| description | text | NOT NULL |
| created_at | timestamptz | NOT NULL DEFAULT now() |

- INDEX (genre, difficulty) — 検索の主経路
- 初期シードは各ジャンル × 各難易度あたり最低10件、**合計120件以上**を目標とする
  （週間献立で7件を重複なく引くための下限を確保する）

#### search_histories
| カラム | 型 | 制約 |
| --- | --- | --- |
| id | uuid | PK |
| user_id | uuid | FK → users.id, ON DELETE CASCADE |
| menu_id | uuid | FK → menus.id |
| search_mode | text | NOT NULL（`single` \| `weekly`） |
| searched_at | timestamptz | NOT NULL DEFAULT now() |

- INDEX (user_id, searched_at DESC) — FIFO判定と一覧表示の主経路
- 15件を超えた分は削除する（実装方針は 4.3 を参照）

#### favorites
| カラム | 型 | 制約 |
| --- | --- | --- |
| id | uuid | PK |
| user_id | uuid | FK → users.id, ON DELETE CASCADE |
| menu_id | uuid | FK → menus.id |
| created_at | timestamptz | NOT NULL DEFAULT now() |

- UNIQUE (user_id, menu_id)

#### recipe_link_caches（任意・第2段階）
外部検索APIの消費削減用。TTL 7日。MVPでは省略可。

| カラム | 型 | 制約 |
| --- | --- | --- |
| menu_id | uuid | PK, FK → menus.id |
| links | jsonb | NOT NULL |
| fetched_at | timestamptz | NOT NULL |

### 4.3 履歴のFIFO実装方針

トリガではなく**アプリケーション層（service）で実装する**。
理由：ロジックがGoのテストで検証でき、DBを差し替えても挙動が変わらないため。

```
INSERT 後、同一トランザクション内で
DELETE FROM search_histories
WHERE user_id = $1
  AND id NOT IN (
    SELECT id FROM search_histories
    WHERE user_id = $1
    ORDER BY searched_at DESC
    LIMIT 15
  );
```

週間献立（7件）を一括登録する場合も、同一トランザクション内で7件INSERT後に1度だけ実行する。

---

## 5. API仕様

ベースURL：`/api/v1`。認証は Cookie の JWT。エラーは RFC 7807（application/problem+json）に準拠する。

### 5.1 献立

| Method | Path | 認証 | 説明 |
| --- | --- | --- | --- |
| GET | `/menus/suggest?genre=&difficulty=` | 任意 | 1食分を提案 |
| POST | `/menus/suggest-weekly` | 任意 | 7日分を提案 |
| GET | `/menus/:id/recipes` | 任意 | レシピサイト3件を取得 |
| GET | `/menus/:id` | 任意 | 献立詳細 |

**`GET /menus/suggest` レスポンス例**
```json
{
  "menu": {
    "id": "018f...",
    "name": "親子丼",
    "genre": "japanese",
    "difficulty": "easy",
    "description": "鶏肉と卵を甘辛い出汁でとじた定番の丼もの"
  }
}
```

**`POST /menus/suggest-weekly` リクエスト / レスポンス例**
```json
// request
{ "genre": "japanese", "difficulty": null }

// response
{
  "week": [
    { "day": 1, "menu": { "id": "018f...", "name": "親子丼", "...": "..." } },
    { "day": 2, "menu": { "id": "018g...", "name": "肉じゃが", "...": "..." } }
  ]
}
```

**`GET /menus/:id/recipes` レスポンス例**
```json
{
  "recipes": [
    {
      "title": "基本の親子丼レシピ",
      "url": "https://example.com/recipes/oyakodon",
      "domain": "example.com",
      "snippet": "鶏もも肉と卵で作る失敗しない親子丼の作り方"
    }
  ]
}
```

### 5.2 認証

| Method | Path | 説明 |
| --- | --- | --- |
| POST | `/auth/signup` | メール + パスワードで登録 |
| POST | `/auth/login` | メール + パスワードでログイン |
| GET | `/auth/google` | Google OAuth 開始（PKCE、stateをCookieに保存） |
| GET | `/auth/google/callback` | Google OAuth コールバック |
| POST | `/auth/refresh` | アクセストークン再発行 |
| POST | `/auth/logout` | ログアウト（Cookie失効） |
| GET | `/auth/me` | 現在のユーザー情報 |

### 5.3 履歴・お気に入り

| Method | Path | 認証 | 説明 |
| --- | --- | --- | --- |
| GET | `/histories` | 必須 | 履歴一覧（最新15件、新しい順） |
| DELETE | `/histories/:id` | 必須 | 履歴を1件削除 |
| DELETE | `/histories` | 必須 | 履歴を全件削除 |
| GET | `/favorites` | 必須 | お気に入り一覧 |
| POST | `/favorites` | 必須 | お気に入り追加（body: `{"menuId": "..."}`） |
| DELETE | `/favorites/:menuId` | 必須 | お気に入り削除 |

### 5.4 エラーレスポンス

```json
{
  "type": "https://example.com/probs/menu-not-found",
  "title": "Menu not found",
  "status": 404,
  "detail": "献立 018f... は存在しません"
}
```

| ステータス | 用途 |
| --- | --- |
| 400 | リクエスト検証エラー（不正なgenre値など） |
| 401 | 未認証 |
| 403 | 他ユーザーのリソースへのアクセス |
| 404 | リソース不存在 |
| 409 | 重複（登録済みメールアドレス、お気に入り重複） |
| 422 | 条件に合致する献立が存在しない |
| 502 | 外部検索APIの障害 |
| 429 | レート制限超過 |

---

## 6. 技術スタック

### 6.1 バックエンド（Go）

| 項目 | 選定 |
| --- | --- |
| Go | 1.23+ |
| Webフレームワーク | echo v4 |
| DBドライバ | pgx v5 |
| クエリ生成 | sqlc（SQLからGoの型安全なコードを生成） |
| マイグレーション | golang-migrate |
| JWT | golang-jwt/jwt v5 |
| OAuth | golang.org/x/oauth2 |
| テスト | 標準 testing + testify/assert |
| モック | 手書きのfake（DIしやすい設計のため生成ツールは使わない） |
| 統合テスト | testcontainers-go（実Postgresを立ち上げて検証） |
| Lint | golangci-lint |

### 6.2 フロントエンド（TypeScript）

| 項目 | 選定 |
| --- | --- |
| TypeScript | 5.6+（`strict: true`） |
| UI | React 19 |
| ビルド | Vite |
| ルーティング | React Router |
| サーバ状態 | TanStack Query |
| スタイル | Tailwind CSS |
| テスト | Vitest + Testing Library |
| E2E | Playwright |
| Lint / Format | ESLint + Prettier |

### 6.3 型の共有

Goのhandlerから OpenAPI 3.1 スキーマを生成し、`openapi-typescript` でTSの型を生成する。
API仕様の変更が型エラーとしてフロントに伝播する状態を維持する。

---

## 7. ディレクトリ構成

```
test_prj/
└── menu-planner/                        # プロジェクトルート
    ├── docker-compose.yml
    ├── PLAN.md
    ├── spec.md
    ├── Makefile                         # make dev / make test / make migrate
    ├── .env.example
    ├── api/
    │   └── openapi.yaml                 # API仕様の単一の情報源（型生成の元）
    ├── backend/
    │   ├── cmd/server/main.go           # エントリポイント・DI組み立て
    │   ├── internal/
    │   │   ├── handler/                 # Controller層
    │   │   │   ├── menu.go
    │   │   │   ├── auth.go
    │   │   │   ├── history.go
    │   │   │   └── favorite.go
    │   │   ├── service/                 # ビジネスロジック層
    │   │   │   ├── menu.go              # 献立選定・重複回避
    │   │   │   ├── auth.go
    │   │   │   ├── history.go           # FIFO
    │   │   │   ├── favorite.go
    │   │   │   └── ports.go             # repository/gatewayのインターフェース定義
    │   │   ├── repository/              # データアクセス層（Postgres実装）
    │   │   ├── gateway/                 # 外部API（検索API実装）
    │   │   ├── domain/                  # エンティティ・値オブジェクト
    │   │   └── middleware/              # 認証・CORS・ロギング・レート制限
    │   ├── db/
    │   │   ├── migrations/
    │   │   ├── queries/                 # sqlcの入力SQL
    │   │   └── seeds/                   # 献立マスタ 120件
    │   ├── Dockerfile
    │   └── go.mod                       # module github.com/yuuya/menu-planner/backend
    └── frontend/
        ├── src/
        │   ├── components/
        │   ├── features/
        │   │   ├── menu/
        │   │   ├── auth/
        │   │   ├── history/
        │   │   └── favorite/
        │   ├── api/                     # 生成された型 + fetchクライアント
        │   ├── hooks/
        │   └── main.tsx
        ├── e2e/
        ├── Dockerfile
        └── package.json
```

---

## 8. 環境構築

### 8.1 docker-compose 構成

| サービス | 内容 | ポート |
| --- | --- | --- |
| `db` | postgres:17-alpine、named volumeで永続化 | 5432 |
| `backend` | Go（air でホットリロード） | 8080 |
| `frontend` | Vite dev server（`/api` を backend にプロキシ） | 5173 |

- backend は `db` のヘルスチェック通過後に起動する（`depends_on.condition: service_healthy`）
- 本番Dockerfileはマルチステージビルドで distroless イメージに載せる

### 8.2 環境変数（`.env.example`）

```
DATABASE_URL=postgres://app:password@db:5432/menu_planner?sslmode=disable
JWT_SECRET=<openssl rand -base64 32 で生成>
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback
SEARCH_API_KEY=
SEARCH_API_PROVIDER=brave        # brave | google_cse | stub
FRONTEND_ORIGIN=http://localhost:5173
```

`SEARCH_API_PROVIDER=stub` を用意し、**APIキーなしでも `docker compose up` だけで全機能が動く**状態を保つ。
これにより開発初期とCIで外部依存を切り離せる。

### 8.3 起動手順

```bash
cd menu-planner
cp .env.example .env
docker compose up -d
make migrate     # マイグレーション適用
make seed        # 献立マスタ投入
# → http://localhost:5173
```

---

## 9. 開発方針（TDD）

PLAN.mdの「テストがパスしたら次に進む」方針に従い、以下を厳守する。

### 9.1 サイクル

1. **Red** — 失敗するテストを書く
2. **Green** — テストを通す最小の実装を書く
3. **Refactor** — テストが緑のまま設計を整える
4. 次のタスクへ進む

各フェーズの完了条件は「**そのフェーズのテストが全て緑であること**」とし、
赤いテストを残したまま次フェーズに進まない。

### 9.2 テスト戦略

| レイヤ | 対象 | 手法 |
| --- | --- | --- |
| service | 献立選定、重複回避、FIFO、認証ロジック | fakeリポジトリを注入した純粋な単体テスト。**ここを最も厚くする** |
| repository | SQL、制約、トランザクション | testcontainers-go で実Postgresに対して検証 |
| handler | ステータスコード、DTO変換、認証必須の境界 | httptest + fakeサービス |
| gateway | 検索APIのレスポンス解釈、タイムアウト、リトライ | httptest.Server でHTTPをスタブ |
| frontend | コンポーネント、フック | Vitest + Testing Library（APIはMSWでモック） |
| E2E | 主要導線 | Playwright（stubプロバイダで実行） |

### 9.3 重点的にテストすべき箇所

- 週間献立の重複回避ルール3種（同一献立、同ジャンル3連続、履歴除外）
- **候補が枯渇するケース**：条件に合う献立が7件未満のとき、緩和ルールが正しく働くか
- 履歴16件目投入時に最古が消え、ちょうど15件が残るか
- 検索APIが 500 / タイムアウト / 空配列を返したとき、献立提案自体は成功するか
- 他ユーザーの履歴・お気に入りに 403 が返るか
- 同一メールでGoogle SSOとパスワード認証が正しく同一ユーザーに紐づくか

### 9.4 CI

GitHub Actions で以下を実行する。すべて緑でなければマージしない。

```
go test ./... -race -cover   /   golangci-lint run
npm run test  /  npm run lint  /  tsc --noEmit
docker compose build
```

---

## 10. 実装フェーズ

各フェーズはテストが全て緑になった時点で完了とする。

| # | フェーズ | 主な成果物 | 完了条件 |
| --- | --- | --- | --- |
| 0 | 環境構築 | docker-compose、Makefile、CI雛形 | `docker compose up` でGoが `/health` に200を返す |
| 1 | ドメイン + 献立マスタ | domain層、マイグレーション、シード120件 | repositoryの統合テストが緑 |
| 2 | 献立検索（1食分） | service.SuggestMenu、`GET /menus/suggest` | 絞り込み・候補枯渇の単体テストが緑 |
| 3 | レシピ取得 | RecipeSearchGateway（stub + brave実装） | 障害時フォールバックのテストが緑 |
| 4 | 週間献立 | service.SuggestWeekly、重複回避3ルール | 重複回避と枯渇時の緩和テストが緑 |
| 5 | 認証 | パスワード認証 → Google SSO → JWTミドルウェア | 認証境界のテストが緑 |
| 6 | 履歴 | FIFO 15件、履歴API、検索への履歴除外の結線 | 16件目でFIFOが働くテストが緑 |
| 7 | お気に入り | favorites API | 重複追加が409になるテストが緑 |
| 8 | フロントエンド | 検索画面、週間献立表、履歴、お気に入り、ログイン | Vitest + Playwright が緑 |
| 9 | 仕上げ | レート制限、構造化ログ、エラー表示、README | E2E全通過 |

フェーズ2と3を先に通すことで、「和食×簡単 → 親子丼 → レシピ3件が新しいタブで開く」という
PLAN.mdの中核シナリオが最速で動く状態になる。

---

## 11. 非機能要件

| 項目 | 目標 |
| --- | --- |
| 応答時間 | 献立検索 200ms以内（p95）／レシピ取得 2s以内（p95、外部API依存） |
| 外部APIタイムアウト | 3秒。指数バックオフで最大2回リトライ |
| レート制限 | 認証エンドポイント 10req/min/IP、検索 60req/min/IP |
| セキュリティ | Cookie は HttpOnly + Secure + SameSite=Lax、CORSは `FRONTEND_ORIGIN` のみ許可、SQLはプレースホルダのみ（文字列連結を禁止）、パスワードは8文字以上 |
| ログ | log/slog による構造化ログ。リクエストIDを全ログに伝播。パスワード・トークンは出力しない |
| 可用性 | 外部検索APIの障害がコア機能（献立提案）を落とさないこと |

---

## 12. 本番デプロイ（将来）

MVPではローカル動作までを対象とするが、以下への移行を阻害しない設計を維持する。

| 要素 | 移行先 | 備考 |
| --- | --- | --- |
| DB | Neon 無料枠 | `DATABASE_URL` の差し替えのみ。同一Postgresで差分なし |
| backend | Cloud Run / Fly.io | 既存Dockerfileをそのまま利用 |
| frontend | Cloudflare Pages / Vercel | 静的ビルド成果物を配信 |

Neon は自動スリープするため、コールドスタート時の初回接続が遅延する。
接続プールの初期化はリトライ可能にしておく。

---

## 13. 未決事項

| # | 項目 | 判断時期 |
| --- | --- | --- |
| 1 | 検索APIの最終選定（Brave Search か Google Custom Search か） | フェーズ3着手時。Gateway抽象化済みのため後から差し替え可能 |
| 2 | 献立マスタ120件の具体的な内容 | フェーズ1。ジャンル×難易度が均等になるよう配分する |
| 3 | recipe_link_caches の導入要否 | フェーズ3完了後、実際のAPI消費量を見て判断する |
| 4 | 週間献立の開始曜日（月曜固定か当日起点か） | フェーズ4着手時 |
