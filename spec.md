# 献立提案アプリケーション 仕様書

> 本書は `PLAN.md` を出発点に、壁打ちで確定した設計判断を仕様として固めたもの。

**アプリ名は「献立くん」**（利用者に見せる名前）。リポジトリ・パッケージ・
JWTの issuer などの識別子は `menu-planner` のままとする（改名の影響を
利用者向けの表示だけに閉じるため）。

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

**フェーズ11で追加**：必要食材の表示、買い物リスト生成（14章）

**含まない（将来対応）**：苦手食材の除外、朝食・昼食の献立、
献立のユーザー投稿、栄養素の管理（栄養価計算・期間集計）、
週間献立での共通食材の再利用、
手持ち食材からの献立検索

**フェーズ12で追加**：週間献立の保存（2.8）

**含まない（2026-07-22 の利用者レビューで挙がったもの）**：
献立の日常性（庶民的かどうか）による出し分け、ターゲット層に応じたモード選択、
季節・旬の考慮、献立の主菜／副菜の区別、献立の画像

**やらないと決めたもの（2026-07-22）**：
予算内での献立提案（実額の算出）、週間献立の日付の並べ替え

> 検討の経緯・依存関係・着手順の提案は task.md「利用者レビューからの後続タスク」。
>
> **「日常性による出し分け」が最優先。** `difficulty` が「手間」と「高価・入手困難」を
> 1つの軸に同居させており、`elaborate` 40件に3種類（日常の手間・ハレの日・
> 家庭で作らない料理）が混在している。これが「フカヒレの姿煮とポテトサラダが並ぶ」
> 違和感の直接の原因。**予算の要望もこれで実質的に満たせるため、実額の算出はやらない。**
>
> **「主菜／副菜の区別」は 2.1 の「1食＝主菜1品」という前提を変える**ため、
> 着手する場合は本仕様の 2.1 / 2.2 と 5.1 の見直しを伴う。

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

### 2.7 必要食材と買い物リスト（フェーズ11）

- 献立の詳細画面で、その献立に必要な食材を表示する
- 週間献立から買い物リストを作れる。7日分の食材を1つのリストにまとめる
- 同じ食材が複数の献立に出てきたら1件にまとめ、**どの献立で使うかを併記する**
- 食材はカテゴリ（野菜・肉・魚介・卵乳・主食・その他）ごとに分けて並べる
- **調味料は持たない**（14.4）。醤油・塩・砂糖・油・だしなどは食材として登録しない
- 未認証でも使える（週間献立が未認証で使えるため、買い物リストも同様）
- **分量は持たない。** 食材名のチェックリストとして提供する（14.2 参照）

> **これは「そのレシピの正確な材料表」ではない。** 献立マスタと同様に自前で持つ
> 代表的な食材の例であり、実際の材料はレシピ元で確認してもらう前提。画面にもその旨を示す。
> このため**アレルギー対応とは位置づけない**（14.3 参照）。

### 2.8 週間献立の保存（フェーズ12）

組み立てた1週間分の献立に名前を付けずそのまま保存し、後から呼び出せるようにする。

- **保存には認証が要る。** 週間献立の作成自体は未認証でも使えるが、保存はユーザーに紐づく
- 保存できるのは**7日分をひとまとまり**として。個々の献立の保存はお気に入り（2.6）が担う
- 保存した週を**開く**と、その週が作成直後と同じ状態に戻る。
  引き直し（2.2）も買い物リスト（2.7）もそのまま続けられる
- 保存一覧から個別に削除できる
- **保存の上限は10件**。超過した状態での保存は 409 を返し、古いものを消すよう促す

> **履歴（2.5）のように FIFO で黙って消さない。** 履歴は自動で溜まるものなので
> 押し出されても困らないが、保存は利用者が明示的に行う操作であり、
> 黙って消えると「保存した」という行為の意味が壊れる。上限に達したことを
> 伝えて利用者に選ばせる。

> **名前は付けない。** 保存日時と、その週に含まれる献立名で識別できる。
> 命名を必須にすると保存のたびに入力を強いることになり、
> 「買い物前にさっと取っておく」という動機に対して重い。必要になったら後から足せる。

> **買い物リストは保存しない。** 保存するのは献立の組み合わせだけで、
> 買い物リストは開いたときに毎回作り直す。食材は献立マスタ由来で変わらないため
> 結果は同じであり、二重に持つと更新漏れの余地だけが増える。

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

// レシピサイト検索（Brave / スタブを差し替え可能。13.1 参照）
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
  │
  └──< menu_ingredients >── ingredients   (フェーズ11)
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

#### recipe_link_caches
外部検索APIの消費削減用。TTL 7日。**MVPに含める**（13.2 で決定）。

| カラム | 型 | 制約 |
| --- | --- | --- |
| menu_id | uuid | PK, FK → menus.id |
| links | jsonb | NOT NULL |
| fetched_at | timestamptz | NOT NULL |

#### ingredients（フェーズ11）
| カラム | 型 | 制約 |
| --- | --- | --- |
| id | uuid | PK |
| name | text | NOT NULL、UNIQUE、空文字不可 |
| name_kana | text | NOT NULL、空文字不可 |
| category | text | NOT NULL、CHECK（vegetable / meat / seafood / dairy_egg / staple / other） |
| created_at | timestamptz | NOT NULL DEFAULT now() |

調味料のカテゴリは持たない（14.4）。**常備品フラグのような仕掛けも要らない**。
「常備品は別枠で出す」は調味料を登録するから必要になる仕掛けであり、
そもそも登録しないなら不要になる。

#### menu_ingredients（フェーズ11）
| カラム | 型 | 制約 |
| --- | --- | --- |
| menu_id | uuid | PK(複合)、FK → menus(id) ON DELETE CASCADE |
| ingredient_id | uuid | PK(複合)、FK → ingredients(id) ON DELETE RESTRICT |

献立と食材の多対多。**分量は持たない**（14.2）。
`ON DELETE RESTRICT` は、どこかの献立が使っている食材を消せないようにするため。

#### saved_weekly_menus（フェーズ12）
| カラム | 型 | 制約 |
| --- | --- | --- |
| id | uuid | PK |
| user_id | uuid | FK → users.id, ON DELETE CASCADE |
| created_at | timestamptz | NOT NULL DEFAULT now() |

- INDEX (user_id, created_at DESC) — 一覧表示（新しい順）の主経路
- 1ユーザーあたり10件まで（2.8）。上限判定はアプリ側で行う

#### saved_weekly_menu_days（フェーズ12）
| カラム | 型 | 制約 |
| --- | --- | --- |
| saved_weekly_menu_id | uuid | PK(複合)、FK → saved_weekly_menus(id) ON DELETE CASCADE |
| day | smallint | PK(複合)、CHECK (day BETWEEN 1 AND 7) |
| menu_id | uuid | FK → menus.id |

保存した週の中身。`day` は週間献立と同じ 1..7 の連番（13.3 のとおり当日起点）。

> **jsonb に7件まとめて持つ形も検討したが、正規化した。** `menu_id` に外部キーが効き、
> 献立マスタとの不整合を DB が防げる。また保存済みの週から献立を辿る集計
> （どの献立がよく保存されるか）も素直に書ける。7行×10件と量も知れている。

> **`day` の重複は複合主キーが防ぐ。** 同じ週に「3日目」が2件入ることはない。

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
| POST | `/menus/reroll-day` | 任意 | 週間献立の1日だけを引き直す |
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

**`POST /menus/reroll-day` リクエスト / レスポンス例**

2.2 の「特定の日だけを引き直せる（他の日は保持したまま、重複回避ルールを再適用する）」。

サーバは週の状態を持たないため、現在の週を `week`（day 1..7 の順に並べた献立IDの配列）
として受け取る。引き直した1日分だけを返し、他の日は呼び出し側が保持する。

```json
// request
{
  "day": 3,
  "genre": "japanese",
  "difficulty": null,
  "week": ["018f...", "018g...", "018h...", "018i...", "018j...", "018k...", "018l..."]
}

// response
{
  "menu": { "id": "018m...", "name": "肉じゃが", "...": "..." }
}
```

重複回避は再適用する。引き直した日の献立は他の6日と重複せず、同一ジャンルが
3日以上連続しない。**前後どちらの側も見る**（3日目を引き直すなら 1-2日目と
4-5日目の双方との連続を避ける）。候補が枯渇する場合は 2.2 と同じ順序で緩和する。

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

#### 週間献立の保存（フェーズ12）

| Method | Path | 認証 | 説明 |
| --- | --- | --- | --- |
| GET | `/weekly-menus` | 必須 | 保存した週の一覧（新しい順、中身の7件を含む） |
| POST | `/weekly-menus` | 必須 | 保存（body: `{"days": [{"day": 1, "menuId": "..."}, ...]}`） |
| DELETE | `/weekly-menus/:id` | 必須 | 保存した週を1件削除 |

- `POST` は 7日分ちょうどを要求する。過不足は 400
- 上限10件に達している状態の `POST` は **409**（2.8）
- 他人の保存を `DELETE` しようとした場合は 404（存在を明かさない。お気に入りと同じ扱い）
- 一覧は中身の献立を含めて返す。**個別取得のエンドポイントは設けない**
  （最大10件×7日と小さく、一覧を開いた時点で「開く」操作まで往復を増やす意味がない）

> **開く操作にAPIは要らない。** 一覧の応答に7日分が入っているため、
> 「開く」はクライアントがその週を作業中の状態に戻すだけで済む。

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

### 5.5 必要食材・買い物リスト（フェーズ11）

| Method | Path | 認証 | 説明 |
| --- | --- | --- | --- |
| GET | `/menus/:id/ingredients` | 任意 | その献立に必要な食材 |
| POST | `/shopping-list` | 任意 | 複数の献立から買い物リストを作る |

`POST /shopping-list` は献立IDの配列を受け取る。週間献立は保存していない（クライアントが
持っている）ため、サーバに状態を持たせずリクエストで受け取る形にする。

**リクエスト**
```json
{ "menuIds": ["018f...a1", "018f...a2"] }
```

**レスポンス**
```json
{
  "items": [
    {
      "ingredient": { "id": "...", "name": "玉ねぎ", "nameKana": "たまねぎ", "category": "vegetable" },
      "usedIn": [ { "id": "...", "name": "肉じゃが" }, { "id": "...", "name": "親子丼" } ]
    }
  ]
}
```

- `items` は買うもの。`usedIn` に、その食材を使う献立を並べる（分量が無い分、
  「何のために買うか」が分かるようにするため）
- 調味料は含まない（14.4）ため、リストは常に「買う食材」だけになる
- 並び順はカテゴリ（野菜→肉→魚介→卵乳→主食→その他）、同カテゴリ内は名前のカナ順
- `menuIds` は1〜7件。0件は400、8件以上も400。重複IDは1件として扱う
- 存在しない献立IDが含まれる場合は404

---

## 6. 技術スタック

### 6.1 バックエンド（Go）

| 項目 | 選定 |
| --- | --- |
| Go | 1.23+ |
| Webフレームワーク | echo v4 |
| DBドライバ | pgx v5 |
| クエリ | pgx に直接SQLを記述（sqlc は見送り。クエリが単純で生成の恩恵より依存の重さが上回るため） |
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
| Lint / Format | oxlint（Viteスキャフォールドの既定。Rust製で高速） |

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
    │   │   ├── migrations/               # embed.FS でバイナリに埋め込む
    │   │   └── seeds/                    # 献立マスタ 120件（同上）
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
SEARCH_API_PROVIDER=brave        # brave | stub（13.1 で google_cse を廃止）
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
go test ./... -race -cover   /   golangci-lint run   /   go vet ./...
npm run test  /  npm run lint  /  tsc --noEmit  /  npm run build
docker build --target prod   /   docker compose up + /health の疎通確認
```

`-race` は cgo（gcc）を必要とするため、Windowsのローカル環境では実行できない。
ローカルの `make test` は race 無し、CI（ubuntu-latest）では race 有りで実行する。

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
| 10 | 本番デプロイ | Cloudflare Pages + Cloud Run + Neon（12章） | 本番で全機能が動作 |
| 11 | 必要食材・買い物リスト | 食材マスタ、献立との紐付け、買い物リストAPI・画面（14章） | 週間献立から買い物リストが作れる |

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
| ~~1~~ | ~~検索APIの最終選定（Brave Search か Google Custom Search か）~~ | **決定済み（2026-07-17）→ 13.1** |
| 2 | 献立マスタ120件の具体的な内容 | フェーズ1。ジャンル×難易度が均等になるよう配分する |
| ~~3~~ | ~~recipe_link_caches の導入要否~~ | **決定済み（2026-07-17）→ 13.2** |
| ~~4~~ | ~~週間献立の開始曜日（月曜固定か当日起点か）~~ | **決定済み（2026-07-17）→ 13.3** |

### 13.1 検索APIは Brave Search API を使う（決定）

**Google Custom Search は選択肢から外れた。** 公式ドキュメントに
"The Custom Search JSON API is closed to new customers" と明記され、新規申込ができない
（既存顧客も2027-01-01までに移行が必要）。本プロジェクトは新規のため利用不可。

**Brave Search API を採用する。** 自前の検索インデックスを持ち、Google のスクレイピング
代行ではないため規約面が安定している。$5.00 / 1,000リクエストの従量課金で、毎月$5の
クレジット（≒1,000クエリ）が付く。2026-02に無期限の無料枠は廃止され、登録には
クレジットカードが必要。

コスト面の懸念は小さい。検索語は「{献立名} レシピ」で、献立マスタは120件固定のため
**検索の種類は最大120通り**。13.2 のキャッシュと併せてAPI消費は生涯約120クエリに収まる。

Exa / Tavily は月1,000件の無料枠がありカード登録も不要だが、レシピサイト探しでの
好相性が未検証であり、Tavily は2026-02にNebiusによる買収が発表され先行きが不透明。
Bing Search API は2025-08に終了済み。

環境変数 `SEARCH_API_PROVIDER` は `brave` | `stub` を取る（`google_cse` は廃止）。
Gateway は抽象化済み（`service.RecipeSearchGateway`）のため、将来の差し替えは可能。

### 13.2 recipe_link_caches を導入する（決定）

4.2 の「任意・第2段階」を**MVPに含める**。当初は「フェーズ3完了後に実際のAPI消費量を
見て判断」としていたが、13.1 で従量課金が前提になったため前倒しする。

- 献立は120件固定であり、キャッシュすればAPI消費は生涯約120クエリ（≒$0.60）で頭打ちになる
- 無い場合、検索のたびに外部APIを叩くため消費が利用量に比例する
- 応答時間の目標（レシピ取得 p95 2s以内、11章）もキャッシュヒット時は数msで満たせる

TTL 7日は 4.2 の記述通りとする。レシピサイトのURLは頻繁には変わらないため。

### 13.3 週間献立は当日起点とする（決定）

**作成した日から7日間**とする。月曜固定にはしない。

- 水曜に作っても7日分すべてが先の予定として使える。月曜固定だと過去3日分が無駄になる
- **サーバは曜日を知らなくてよい**。API は `day` を 1..7 の連番で返す（5.1）ため、
  「何曜日か」は起点が決まれば呼び出し側で決まる
- 月曜固定にすると「今が何曜日か」をサーバが判断することになり、タイムゾーン（JST）の
  考慮が要る。当日起点なら日付境界の解釈がずれても献立の中身は変わらない

「来週の献立を今のうちに作る」用途には向かないが、MVP の対象外（1.2）とする。
必要になれば起点日をリクエストで受け取る形に拡張できる。`day` が連番である限り
レスポンスの形は変わらない。

---

## 14. 食材データの扱い（フェーズ11・決定）

### 14.1 食材は自前の献立マスタと同様に自前で持つ（決定）

献立ごとの食材は **`ingredients` / `menu_ingredients` に自前で持つ**。外部から調達しない。

検討した調達手段と、採らなかった理由:

| 手段 | 判断 |
| --- | --- |
| レシピページのクロール | **不可**。1.1 の判断どおり主要レシピサイトは robots.txt でクローラを拒否している |
| レシピAPI（楽天等） | **不採用**。利用規約の制約に加え、**自前の献立名と確実に対応づかない**。「肉じゃが」で検索して出たレシピが、うちの献立の想定と一致する保証がない |
| 日本食品標準成分表 | **対象外**。「食材→栄養価」の表であって「料理→食材」ではない |

そもそも献立マスタ120件は自前で作ったデータであり、その食材も自前で持つのが筋が通る。
外部の実レシピを自前の献立名に紐付けようとすると、かえって不正確になる。

**この帰結として、食材リストは「代表的な食材の例」であって特定レシピの正確な材料表ではない。**
画面にもその旨を示し、実際の材料はレシピ元で確認してもらう（2.7）。

### 14.2 分量は持たない（決定）

`menu_ingredients` は献立と食材の対応だけを持ち、分量・単位は持たない。

- 買い物リストは**食材名のチェックリスト**として成立する。分量が無くても実用になる
- 分量を持つと「何人分を基準にするか」「単位の正規化（個 / g / 大さじ）」
  「合算規則」の設計が必要になり、手で作るデータの中で最も主観的で誤りが出やすい部分になる
- 分量が必須なのは**栄養価計算だけ**で、それは着手順として最後に置いている

分量が必要になった時点で `menu_ingredients` に列を足す。その際は基準人数を先に決めること。

**分量が無い分の補償として、買い物リストには「その食材をどの献立で使うか」を併記する**（5.5）。
「玉ねぎ … 肉じゃが・親子丼」と分かれば、必要量は利用者が判断できる。

### 14.3 アレルギー対応とは位置づけない（決定）

積み残しの「食べられない食材のブラックリスト」は、**苦手食材（嗜好フィルタ）**として実装する。
アレルギー対応とは謳わない。

**理由:** 食材データは手で作るものであり、実際のレシピと必ずしも一致しない。調味料由来の
小麦・大豆のような「表に出ない混入」も拾いきれない。それにもかかわらず「卵アレルギー対応」と
表示すると、**それを信じた利用者が実際には卵入りの料理を選んでしまい、現実に危害が生じ得る**。

機能としてはほぼ同じだが、画面の文言と位置づけを変える。除外機能を作る際は
「苦手な食材を候補から外す」と表現し、アレルギーの語を使わない。

### 14.4 調味料は食材として持たない（決定）

`ingredients` に登録するのは**食材そのもの**だけとする。醤油・塩・砂糖・みりん・酒・油・
だし・こしょうといった調味料は登録しない。

**理由:**
- 調味料は大半の家庭に常備されており、買い物リストに並べても**本当に買うものが埋もれる**
- 分量を持たない設計（14.2）では、調味料を載せても「醤油」と名前が並ぶだけで情報量がない
- 献立あたりの登録数が減り、**食材そのものの精度にデータ整備の労力を集中できる**
- 「常備品は別枠で出す」という仕掛けも不要になる。これは調味料を登録するから必要に
  なるものであり、そもそも登録しなければ設計から消える

**判断に迷う境界の扱い:**

| 例 | 扱い | 理由 |
| --- | --- | --- |
| にんにく・生姜・ねぎ | **食材として登録する** | 香味野菜であり、野菜として買うもの |
| バター・生クリーム | **食材として登録する** | 乳製品として買うもの |
| 豆腐・油揚げ・こんにゃく | **食材として登録する** | 主材料になる |
| カレールー・シチューのルウ | **食材として登録する** | 常備しておらず、その献立のために買う必要がある |
| 小麦粉・片栗粉・パン粉 | **登録しない** | 常備品の性格が強く、調味料と同じ扱いでよい |
| 醤油・味噌・塩・砂糖・油・だし | **登録しない** | 調味料そのもの |

境界は「**その献立を作るために買い足す必要があるか**」で判断する。
迷ったら登録しない側に倒す（買い物リストが短いほど使いやすいため）。
