import { expect, test } from '@playwright/test'

// 法務3ページ（特定商取引法に基づく表記・利用規約・プライバシーポリシー）は
// 表示義務のあるページのため、未ログインでもフッターから辿れて内容が
// 表示される必要がある（spec.md、docs/legal/*.md）。
//
// ここでは docs/legal/*.md を `?raw` で読み込んで描画しているため（vite.config.ts
// の server.fs.allow）、単体テスト（vitest + jsdom、モックの上で動く）では
// raw import や実サーバのファイルアクセス制限を通した検証ができない。
// 実スタック（`make up`）に対して実行し、実際に配信されたHTMLに
// md本文が入っていることまで確かめる。
test('未ログインでフッターから法務3ページへ辿れ、md本文が表示される', async ({
  page,
}) => {
  await page.goto('/')

  // 未ログインであることの前提確認。ログインしていると導線や表示が
  // 変わる可能性があるため、先に確かめておく。
  await expect(page.getByRole('button', { name: 'ログアウト' })).toBeHidden()

  // フッターの「特定商取引法に基づく表記」リンクをクリックする。
  await page
    .getByRole('contentinfo')
    .getByRole('link', { name: '特定商取引法に基づく表記' })
    .click()

  await expect(page).toHaveURL(/\/legal\/tokushoho$/)

  // 見出しだけでなく表のセルまで出ていることを確認する。
  // これにより「md本文がそのまま描画されている」ことまで確かめられる
  // （見出しだけなら決め打ちの静的文言でも通ってしまう）。
  await expect(
    page.getByRole('heading', { name: '特定商取引法に基づく表記' }),
  ).toBeVisible()
  await expect(page.getByRole('cell', { name: '販売事業者名' })).toBeVisible()
  await expect(
    page.getByRole('cell', { name: /月額\s*300円（税込）/ }),
  ).toBeVisible()

  // 同じフッターから利用規約・プライバシーポリシーにも未ログインで辿れることを
  // 確かめる（1本のテストで安く確認する）。
  await page
    .getByRole('contentinfo')
    .getByRole('link', { name: '利用規約' })
    .click()
  await expect(page).toHaveURL(/\/legal\/terms$/)
  await expect(
    page.getByRole('heading', { name: /利用規約/ }),
  ).toBeVisible()

  await page
    .getByRole('contentinfo')
    .getByRole('link', { name: 'プライバシーポリシー' })
    .click()
  await expect(page).toHaveURL(/\/legal\/privacy$/)
  await expect(
    page.getByRole('heading', { name: /プライバシーポリシー/ }),
  ).toBeVisible()
})
