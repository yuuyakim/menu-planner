import { expect, test } from '@playwright/test'

// 法務2ページ（利用規約・プライバシーポリシー）は表示義務のあるページのため、
// 未ログインでもフッターから辿れて内容が表示される必要がある
// （spec.md、docs/legal/*.md）。
//
// 特定商取引法に基づく表記は、有料の役務提供が無くなり表示義務も無くなったため、
// ルートとフッター導線を落としている（frontend/src/features/legal/TokushohoPage.tsx
// と content/tokushoho.md はファイルとして残るが、いまは辿れない）。
//
// ここでは docs/legal/*.md を `?raw` で読み込んで描画しているため（vite.config.ts
// の server.fs.allow）、単体テスト（vitest + jsdom、モックの上で動く）では
// raw import や実サーバのファイルアクセス制限を通した検証ができない。
// 実スタック（`make up`）に対して実行し、実際に配信されたHTMLに
// md本文が入っていることまで確かめる。
test('未ログインでフッターから法務2ページへ辿れ、md本文が表示される', async ({
  page,
}) => {
  await page.goto('/')

  // 未ログインであることの前提確認。ログインしていると導線や表示が
  // 変わる可能性があるため、先に確かめておく。
  await expect(page.getByRole('button', { name: 'ログアウト' })).toBeHidden()

  // フッターの「利用規約」リンクをクリックする。
  await page
    .getByRole('contentinfo')
    .getByRole('link', { name: '利用規約' })
    .click()

  await expect(page).toHaveURL(/\/legal\/terms$/)

  // 見出しだけでなく本文の小見出しまで出ていることを確認する。
  // これにより「md本文がそのまま描画されている」ことまで確かめられる
  // （見出しだけなら決め打ちの静的文言でも通ってしまう）。
  await expect(
    page.getByRole('heading', { name: /利用規約/ }),
  ).toBeVisible()
  await expect(
    page.getByRole('heading', { name: '第1条（適用）' }),
  ).toBeVisible()

  // 同じフッターからプライバシーポリシーにも未ログインで辿れることを
  // 確かめる（1本のテストで安く確認する）。
  await page
    .getByRole('contentinfo')
    .getByRole('link', { name: 'プライバシーポリシー' })
    .click()
  await expect(page).toHaveURL(/\/legal\/privacy$/)
  await expect(
    page.getByRole('heading', { name: /プライバシーポリシー/ }),
  ).toBeVisible()
})
