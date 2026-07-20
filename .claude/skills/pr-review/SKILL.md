---
name: pr-review
description: menu-planner の Pull Request をレビューする。PR番号を引数で受け取り、差分・仕様(spec.md)・CI結果を突き合わせて指摘をまとめる。「PRをレビューして」「#12 を見て」などで起動。引数なしなら現在のブランチのPRを対象にする。
---

# menu-planner PR レビュー

## 1. 対象PRを特定する

引数が PR 番号（`#12` / `12`）ならそれを使う。引数なし、または「このPR」なら現在のブランチから解決する。

```bash
gh pr view <番号 or 省略> --json number,title,body,headRefName,baseRefName,state,isDraft,files,additions,deletions
```

PR が見つからない場合はそこで止めてユーザーに確認する（勝手に未コミット差分のレビューに切り替えない）。

## 2. 情報を集める

以下を並行で取得する。

```bash
gh pr diff <番号>                                    # 差分本体
gh pr checks <番号>                                  # CI の成否
gh pr view <番号> --json comments,reviews            # 既存の指摘（重複を避ける）
```

CI が落ちている場合は `gh run view <run-id> --log-failed` で失敗内容を確認し、レビューの冒頭で報告する。**CI が落ちているPRに設計上の細かい指摘を並べる前に、まず落ちている事実を伝える。**

差分だけでは判断できない箇所は、必ず該当ファイルの前後を Read で読んでから指摘する。差分の見た目だけで推測した指摘は書かない。

## 3. レビュー観点

### 3.1 仕様との整合（最優先）

- `spec.md` がこのプロジェクトの仕様の正。差分の挙動が spec.md の記述と食い違っていないか。
- 食い違う場合、「実装が間違っている」のか「spec.md を更新すべき変更」なのかを切り分けて書く。勝手にどちらかを前提にしない。
- API を変えたなら `api/openapi.yaml` も更新されているか。OpenAPI が API 仕様の正。
- `frontend/src/api/schema.d.ts` は生成物。`openapi.yaml` を変えたのに再生成コミットが無ければ CI（`npm run gen:api` の差分チェック）で落ちる。

### 3.2 バグ・正しさ

- nil / undefined、境界値、空配列、エラーの握り潰し。
- Go: `err` を返さず握り潰していないか、`context` を引き回しているか、goroutine とデータ競合（CI は `-race` で回る）。
- 認証まわり（`internal/auth`）: 認可チェックの抜け、他ユーザーのリソースを取得・変更できないか。
- 履歴の FIFO 実装（spec.md 4.3）や、お気に入りの重複登録など、仕様に固有のルールが守られているか。
- SQL: N+1、インデックスの無いカラムでの検索、トランザクション境界。

### 3.3 レイヤリング

`backend/internal` は handler → service → repository の3層（spec.md 3.2）。

- handler に業務ロジックが漏れていないか、repository が service を呼んでいないか。
- `domain` が下位レイヤに依存していないか。
- ランダム性は `internal/random` 経由か（テストで固定できる形になっているか）。

フロントは `features/<機能>` 単位。features 間の直接依存が増えていないか、共通化すべきものが `components` / `hooks` に無いか。

### 3.4 テスト

このプロジェクトは TDD で進めている。

- 変更に対応するテストがあるか。バグ修正なら**そのバグを再現するテスト**が入っているか。
- テストが実装の写経になっていないか（振る舞いを検証しているか）。
- Go のテストは testcontainers で実 Postgres を使う構成。モックで済ませて実クエリを検証していない箇所は指摘する。

### 3.5 移行・運用

- `backend/db/migrations` の追加は up/down が対になっているか。既存データがある前提で安全か（NOT NULL 追加時のデフォルト等）。
- 環境変数を増やしたら `.env.example` と docker-compose.yml、spec.md 8.2 も更新されているか。
- Makefile のレシピを触ったなら、ASCII のみ・1コマンド単位という制約（Makefile 冒頭のコメント）を守っているか。

### 3.6 PRの粒度

「1機能 = 1PR」が運用ルール。無関係な変更が混ざっていたら、分割を提案する。

## 4. 指摘の書き方

各指摘に必ず次を付ける。

- 深刻度: **must**（マージ前に直す） / **should**（直したほうがよい） / **nit**（好みの範囲）
- 場所: `path/to/file.go:123` 形式
- 何が問題か、どういう入力・状況で壊れるか（推測ではなく具体的に）

**憶測で指摘しない。** 確信が持てないものは「確認したい」として質問の形で出し、must には入れない。指摘が無い観点は無理に埋めず、省く。

## 5. 出力

まず要約を数行（マージしてよさそうか、ブロッカーの有無、CI の状態）。そのあと must → should → nit の順に列挙する。最後に、良かった点があれば簡潔に1〜2行。

出力は日本語。

デフォルトはこの会話への報告のみ。**GitHub にコメントを投稿するのは、ユーザーが明示的に頼んだときだけ**（`gh pr review` / `gh pr comment`）。投稿前に内容を見せて確認を取る。
