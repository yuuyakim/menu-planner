# 法務ページの公開 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development（推奨）or executing-plans。Steps は `- [ ]` で追跡。

**Goal:** `docs/legal/` の特商法・利用規約・プライバシーを公開ページ化し、全ページ共通フッターから到達可能にする。利用規約を weekly=premium に更新し、事業者情報の実値を記入して特商法11条の表示義務を満たす。

**Architecture:** `docs/legal/*.md`（publish 用に整形）を唯一ソースとして raw import → `react-markdown`+`remark-gfm` で描画。公開ルート3本＋共有 Footer。**PII と法務文言の記入・整形はコントローラが直接行い、コード実装のサブエージェントには渡さない。**

**Tech Stack:** React 19 / react-router / react-markdown / remark-gfm / Vitest + MSW / Playwright。

## Global Constraints
- 設計の正: `docs/superpowers/specs/2026-07-25-legal-pages-publishing-design.md`。
- base: 現在の `main`。作業ブランチ `feat/legal-pages`。1タスク=1PR相当（🔴+🟢別コミット）。🟢/🔴 コミットに Co-Authored-By は付けない。
- **PII をサブエージェントに渡さない。** 法務 md への実値記入・文言整形（Task 4）は**コントローラ専用**。コード側タスク（1–3, 5）は md の内容に依存する assert を**安定した部分文字列**（例:「特定商取引法」）で書き、「（草案）」等の可変文言に結合しない。
- **利用規約の weekly=premium 記述は #145 前提。** 法務公開は #145 と揃えて main に出す（本PJ単独で main に出さない）。
- バックエンド変更なし・マイグレーション不要。
- テスト: フロント `make test-frontend`（tsc -b + lint + vitest）、E2E `make test-e2e`。

## File Structure
- Create: `frontend/src/features/legal/LegalPage.tsx`（+test） — 共通 md 描画。
- Create: `frontend/src/features/legal/TokushohoPage.tsx` / `TermsPage.tsx` / `PrivacyPage.tsx` — 各 md を raw import して LegalPage に渡す薄いラッパ（+ページ/ルートの test）。
- Create: `frontend/src/components/Footer.tsx`（+test）。
- Modify: `frontend/src/app/App.tsx` — 3公開ルート＋`<Footer>`。
- Modify: `frontend/vite.config.ts` — dev の `server.fs.allow` に repo ルート追加（raw import のため）。
- Modify: `frontend/package.json` — `react-markdown` + `remark-gfm`。
- Modify（コントローラ）: `docs/legal/tokushoho.md` / `terms.md` / `privacy.md` — publish 整形＋実値記入。
- Create: `frontend/e2e/legal-pages.spec.ts`（任意）。

---

### Task 1: md 描画基盤（deps + LegalPage）

**Files:** `frontend/package.json`、`frontend/vite.config.ts`、`frontend/src/features/legal/LegalPage.tsx`（+ `LegalPage.test.tsx`）

**Interfaces:** `export function LegalPage({ markdown }: { markdown: string }): JSX.Element` — raw md 文字列を `react-markdown`+`remark-gfm` で描画（prose スタイル・`kon-*`）。

- [ ] **Step 1: 依存追加** — `cd frontend && npm install react-markdown remark-gfm`（`package.json`/lock に反映）。
- [ ] **Step 2: 🔴 テスト** — `LegalPage.test.tsx`: `renderWithProviders(<LegalPage markdown={"# 見出し\n\n| a | b |\n|---|---|\n| 1 | 2 |"} />)` → 見出しとテーブルセルが描画される（`getByRole('heading', {name:'見出し'})`、`getByText('1')`）。remark-gfm でテーブルが出ることを確認。
- [ ] **Step 3: 失敗確認 → 🔴 コミット** — `npm test -- LegalPage`（未実装で fail）→ `git commit -m "test: 法務ページ描画コンポーネント"`。
- [ ] **Step 4: 実装** — `LegalPage.tsx`:
```tsx
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

// LegalPage は法務文書の md をそのまま描画する。文言の正は docs/legal/*.md。
export function LegalPage({ markdown }: { markdown: string }) {
  return (
    <article className="prose prose-sm max-w-none text-kon-ink [&_table]:block [&_table]:overflow-x-auto">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{markdown}</ReactMarkdown>
    </article>
  )
}
```
（prose クラスが無ければ最小限の見出し/表スタイルを Tailwind で当てる。既存の文章表示の流儀に合わせる。）
- [ ] **Step 5: vite raw import 許可** — `vite.config.ts` の `server` に `fs: { allow: ['..'] }`（frontend の1つ上＝repo ルートまで raw import を許可）を追加。既存 `server` 設定は保持。
- [ ] **Step 6: PASS → 🟢 コミット** — `npx tsc -b && npm test -- LegalPage && npm run lint` → `git commit -m "feat: 法務ページ描画コンポーネントを足す"`。

---

### Task 2: 3法務ページ＋公開ルート

**Files:** `frontend/src/features/legal/{Tokushoho,Terms,Privacy}Page.tsx`、`frontend/src/app/App.tsx`（+ 各 test）

**Interfaces:** 各ページは `import md from '../../../docs/legal/xxx.md?raw'` して `<LegalPage markdown={md} />` を返す。ルート `/legal/tokushoho`・`/legal/terms`・`/legal/privacy`（**公開・RequireAuth で包まない**）。

- [ ] **Step 1: 🔴 テスト** — `TokushohoPage.test.tsx` ほか、または App ルーティングテスト: `/legal/tokushoho` を開くと「特定商取引法」を含む見出しが出る／`/legal/terms` で「利用規約」／`/legal/privacy` で「プライバシーポリシー」。**assert は安定部分文字列**（「（草案）」等に結合しない）。3ルートが未ログインでも表示される（リダイレクトされない）ことも確認。
- [ ] **Step 2: 失敗確認 → 🔴 コミット** — `git commit -m "test: 法務3ページの公開ルート"`。
- [ ] **Step 3: 実装** — 3ページ（`?raw` import → `<LegalPage>`）＋ `App.tsx` の `<Routes>` に3ルート追加（`/login` などと同じ公開扱い。`RequireAuth` で包まない）。
- [ ] **Step 4: PASS → 🟢 コミット** — `npx tsc -b && npm test && npm run lint`（raw import が vitest で解決されることを確認。解決しない場合は Task 1 の `fs.allow` を見直す）→ `git commit -m "feat: 法務3ページを公開する"`。

> `.md` の内容はこの時点では未整形（草案表記等が残る）だが、テストは安定部分文字列で assert するので通る。整形は Task 4。

---

### Task 3: 共通フッター

**Files:** `frontend/src/components/Footer.tsx`（+test）、`frontend/src/app/App.tsx`

- [ ] **Step 1: 🔴 テスト** — `Footer.test.tsx`: フッターに「特定商取引法に基づく表記」「利用規約」「プライバシーポリシー」の3リンクがあり、href が `/legal/tokushoho`・`/legal/terms`・`/legal/privacy`。`renderWithProviders`（router 必要）。
- [ ] **Step 2: 失敗確認 → 🔴 コミット** — `git commit -m "test: 共通フッターの法務リンク"`。
- [ ] **Step 3: 実装** — `Footer.tsx`（`<Link>`3本、`kon-*`・控えめ）＋ `App.tsx` の `</main>` 直後（外側 `div` 内）に `<Footer />` を配置。全ページ下部に出る。
- [ ] **Step 4: PASS → 🟢 コミット** — `npx tsc -b && npm test && npm run lint`（既存レイアウトの回帰なし）→ `git commit -m "feat: 全ページ共通フッターを足す"`。

---

### Task 4: 法務 md の publish 整形＋実値記入（**コントローラ専用・サブエージェント不可**）

**Files（コントローラが直接編集）:** `docs/legal/tokushoho.md` / `terms.md` / `privacy.md`

- [ ] **Step 1: publish 整形（公開してはいけない内部要素を除去）**
  - H1 から「（草案）」を削除。
  - 冒頭の内部向け blockquote（`> \`〈 〉\` は要記入`、`> 掲載場所…`、`> 値を変えるとき… README…`、`> 公開前に弁護士…`）を削除。
  - 末尾の `## 公開前チェック` セクションを削除（内容は README とこの計画の完了条件に集約）。
- [ ] **Step 2: 実値記入（PII 含む・ユーザー提供値）**
  - `tokushoho.md`: 販売事業者名/運営統括責任者=氏名、所在地=住所、電話番号、メール、販売URL=`https://kondatekun.yuuyakim.com`、動作環境=Chrome/Safari/Edge 各最新版。
  - `terms.md`/`privacy.md` の `〈氏名〉`・住所・メールを同一値で埋める。
  - `privacy.md` 第三者提供表: Neon Inc.／シンガポール、Brave Software／米国。解析・エラー監視の注記は「現状導入していない」に整理。
- [ ] **Step 3: 文言の確定**
  - `terms.md` 4条9項 `〈線引き確定後に具体化する〉` → 週間まわり一式（週間献立の提案・引き直し・保存・週間の買い物リスト）が premium である旨。4条7項 `〈7〉` → 7。
  - `制定日` は「公開日」= **main マージ時に確定**。この時点では確定できないため、**唯一 `制定日` だけは最終記入をマージ直前に行う**旨をコミットメッセージ/PR に明記（他の `〈 〉` は残さない）。
- [ ] **Step 4: 検証** — `grep -rn "〈" docs/legal/{tokushoho,terms,privacy}.md` が**制定日以外0件**。`npx tsc -b && npm test`（ページが整形後 md を描画しても緑）。
- [ ] **Step 5: コミット** — `git commit -m "docs: 法務3文書を公開用に整形し事業者情報・weekly線引きを記入"`（PII を含むためメッセージに値は書かない）。

---

### Task 5（任意）: E2E フッター→法務ページ

**Files:** `frontend/e2e/legal-pages.spec.ts`

- [ ] フッターの「特定商取引法に基づく表記」リンク→`/legal/tokushoho` が開き見出しが出る、を1本。未ログインで到達できることを確認。実スタックで実行（`make up`）。落ちる/スタック不可なら BLOCKED 報告（テストは書いてコミット）。
- [ ] コミット: `test: 法務ページのE2E`。

---

## Self-Review
- 仕様カバレッジ: 3ページ描画(Task1-2)/フッター(Task3)/weekly=premium・猶予7日・事業者情報・Neon・解析・制定日(Task4)/公開ルート(Task2)/E2E(Task5)。設計§2-§6を網羅。✅
- PII 分離: Task 4 のみが法務 md を触り、コントローラ専用。コード側タスクは md 非依存 or 安定部分文字列 assert。✅
- プレースホルダ: コード付き step は実コード提示。vite raw import / fs.allow は実装時確認と明記。
- 依存: 利用規約 weekly 記述は #145 前提（Global Constraints）。制定日はマージ直前記入（Task4-3）。
- 型一貫性: `LegalPage({markdown})`（Task1→2）、ルート `/legal/*`（Task2→3 Footer）一致。✅

## 実行の引き継ぎ
計画 `docs/superpowers/plans/2026-07-25-legal-pages-publishing.md`。base `main`、ブランチ `feat/legal-pages`。**Task 4 はコントローラが直接実施**（PII）。コード側（1–3,5）は subagent 可。`feat/legal-pages`→`main` は本番境界（人間がマージ）で、#145 と揃えて出す。
