# 献立くん (menu-planner)

ジャンル×難易度で夕食の献立を提案し、レシピサイトへのリンクを3件開ける Web アプリ。
週間献立の提案、**必要な食材の表示と買い物リスト**、検索履歴、お気に入り、
メール/パスワード・Google の認証に対応する。

**本番: https://kondatekun.yuuyakim.com**
（Cloudflare Pages ＋ Cloud Run ＋ Neon。構成と手順は [`DEPLOY.md`](./DEPLOY.md)）

仕様の正は [`spec.md`](./spec.md)。実装の進め方は [`task.md`](./task.md)。

## 技術スタック

| 層 | 採用 |
| --- | --- |
| バックエンド | Go 1.26 / echo v4 / pgx v5 / golang-jwt v5 / golang-migrate v4 |
| フロントエンド | React 19 / TypeScript / Vite / React Router 7 / TanStack Query 5 / Tailwind CSS 4 |
| データベース | PostgreSQL 17 |
| レシピ検索 | Brave Search API（`stub` に差し替え可能） |
| テスト | Go標準 + testcontainers / Vitest + Testing Library + MSW / Playwright (E2E) |
| 実行環境 | Docker Compose |

## アーキテクチャ

```mermaid
flowchart LR
    Browser["ブラウザ (React SPA)"]
    subgraph Frontend["frontend :5173 (Vite)"]
        SPA["React SPA"]
        Proxy["/api プロキシ"]
    end
    subgraph Backend["backend :8080 (echo)"]
        MW["middleware<br/>Recover / RequestID / ログ / CORS / レート制限"]
        H["handler"]
        S["service (ドメインロジック)"]
        R["repository (pgx)"]
        G["gateway (Brave / stub)"]
    end
    DB[("PostgreSQL :5432<br/>献立マスタ360件 / users / histories / favorites / recipe_link_caches")]
    Brave["Brave Search API"]

    Browser --> SPA --> Proxy --> MW --> H --> S
    S --> R --> DB
    S --> G --> Brave
```

- **レイヤードアーキテクチャ**: handler → service → repository / gateway。依存は内向き（service はインフラの具体を知らず、`ports.go` のインターフェースにのみ依存）。
- **フロントは同一オリジン**: Vite dev server が `/api` を backend に転送するため、ブラウザから見て同一オリジンになる。
- **食材は自前のマスタで持つ**（[spec.md 14章](./spec.md)）。レシピをクロールしない判断のため外部から取得できず、
  自前の献立マスタと同様に自前で持つ。調味料は含めず、分量も持たない（買い物リストは食材名のチェックリスト）。
  代表的な食材の例であり、**アレルギー対応とは位置づけない**。
- **レシピリンクはキャッシュ**する（`recipe_link_caches`、TTL 7日）。献立は360件固定のため、外部APIの消費は生涯およそ360クエリで頭打ちになる（[spec.md 13.2](./spec.md)）。

## クイックスタート

Docker と Docker Compose、GNU Make が必要。

```bash
cd menu-planner
cp .env.example .env      # 既定は SEARCH_API_PROVIDER=stub。APIキー無しで全機能が動く
make up                   # コンテナ起動（db / backend / frontend）
make migrate              # マイグレーション適用
make seed                 # 献立マスタ360件を投入
# → http://localhost:5173 を開く
```

停止は `make down`、コンテナとデータの完全削除は `make clean`。

`make help` で全ターゲットを一覧できる（主なもの: `up` / `down` / `logs` / `migrate` / `seed` / `test` / `lint` / `build` / `health`）。

## 環境変数

`.env.example` をコピーして `.env` を作る。`.env` はコミットしない。

| 変数 | 用途 | 既定 / 例 |
| --- | --- | --- |
| `DATABASE_URL` | Postgres 接続文字列 | `postgres://app:password@db:5432/menu_planner?sslmode=disable` |
| `JWT_SECRET` | JWT 署名鍵。**本番は `openssl rand -base64 32` で生成** | （開発用のダミー） |
| `SEARCH_API_PROVIDER` | レシピ検索の実装 | `brave` \| `stub` |
| `SEARCH_API_KEY` | Brave Search API キー（`stub` なら空でよい） | — |
| `FRONTEND_ORIGIN` | CORS で許可する唯一のオリジン | `http://localhost:5173` |
| `GOOGLE_CLIENT_ID` / `_SECRET` / `_REDIRECT_URL` | Google SSO（空でもメール認証は動く） | — |
| `AUTH_RATE_LIMIT_PER_MIN` | 認証エンドポイントの上限（IP/分、0で無制限） | 本番 `10` / compose `0` |
| `SEARCH_RATE_LIMIT_PER_MIN` | 検索エンドポイントの上限（IP/分、0で無制限） | 本番 `60` / compose `0` |

> **レート制限を compose で 0（無制限）にしている理由**: Vite プロキシ配下では全リクエストが
> プロキシの単一IPに集約され、正当な操作でも即座に上限へ達してしまう。本番は実クライアントIPが
> 分かる構成で spec 値（認証10 / 検索60）を設定する。

## 開発

```bash
make test            # backend(Go) + frontend(型/Lint/テスト)
make test-backend    # go test ./... -cover
make test-frontend   # tsc -b / oxlint / vitest
make test-e2e        # Playwright（事前に make up と make seed が必要）
make lint            # go vet / golangci-lint / oxlint
make gen-api         # api/openapi.yaml から TS の型を再生成
```

- **API 仕様の正は [`api/openapi.yaml`](./api/openapi.yaml)**。変更したら `make gen-api` で型を再生成してコミットする（CIが差分で落とす）。
- Go 実装と仕様のズレは契約テスト（`internal/handler/contract_test.go`）が検出する。
- ローカルの `go test` は `-race` を使わない（cgo=gcc が必要なため）。CI の Linux 上では有効。

## 性能

非機能要件の目標は「検索 p95 200ms以内」（[spec.md 11章](./spec.md)）。`stub` プロバイダ・シード済みDBの
ローカル環境（docker compose）で、backend（`localhost:8080`）へ逐次 curl した実測値:

| エンドポイント | p50 | p95 | p99 | 目標 |
| --- | --- | --- | --- | --- |
| `GET /menus/suggest`（検索） | 2.7ms | **3.0ms** | 11.6ms | 200ms |
| `GET /menus/:id`（詳細） | 2.7ms | 3.0ms | 3.2ms | — |
| `POST /menus/suggest-weekly`（週間） | 3.0ms | 3.6ms | 3.8ms | — |

いずれもDBのみで完結し外部APIを叩かないため、目標を大きく下回る。レシピ取得（`GET /menus/:id/recipes`）は
外部API依存のため別目標（p95 2s）で、キャッシュヒット時は数ms（[spec.md 11章 / task.md 3-G](./task.md)）。

## ディレクトリ構成

```
menu-planner/
├── backend/
│   ├── cmd/            # server / migrate / seed のエントリポイント
│   ├── internal/
│   │   ├── handler/    # HTTPハンドラ・ミドルウェア（CORS/ログ/レート制限）
│   │   ├── service/    # ドメインロジック（献立選定・重複回避・履歴・認証）
│   │   ├── repository/ # データアクセス（pgx）
│   │   ├── gateway/    # 外部レシピ検索API（brave / stub）
│   │   ├── domain/     # エンティティ・値オブジェクト
│   │   ├── auth/       # パスワードハッシュ・JWT・Google OAuth
│   │   └── logctx/     # request_id 付き logger の受け渡し
│   └── db/             # migrations / seeds（embed.FS で埋め込み）
├── frontend/
│   ├── src/
│   │   ├── features/   # menu / auth / history / favorite
│   │   ├── components/ # 共通UI（ErrorBoundary / NotFoundPage ほか）
│   │   ├── api/        # 生成された型 + fetch クライアント
│   │   └── app/        # ルーティング・プロバイダ
│   └── e2e/            # Playwright
├── api/openapi.yaml    # API 仕様（正）
├── docker-compose.yml
├── Makefile
├── spec.md             # 仕様（正）
└── task.md             # 実装フェーズと進捗
```

## MVP 対象外

アレルギー除外、買い物リスト、朝昼の献立、献立のユーザー投稿、栄養価計算、本番デプロイ（[spec.md 1.2 / 12章](./spec.md)）。
