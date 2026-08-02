import { expect, test } from '@playwright/test'

import { signUp, uniqueEmail } from './helpers'

test('週間献立を作ると、買い物リストに食材がまとまる', async ({ page }) => {
  const email = uniqueEmail('shopping-list')
  await signUp(page, email)

  // 週間献立の作成はログインすれば誰でも使える。
  await page.goto('/weekly')

  // 週を作る前は導線を出さない。買うものがまだ決まっていないため。
  await expect(
    page.getByRole('link', { name: '買い物リストを見る' }),
  ).toHaveCount(0)

  await page.getByRole('button', { name: '1週間分を作る' }).click()
  await expect(page.getByRole('listitem')).toHaveCount(7)

  await page.getByRole('link', { name: '買い物リストを見る' }).click()

  await expect(
    page.getByRole('heading', { level: 1, name: '買い物リスト' }),
  ).toBeVisible()

  // 7日分の食材が並ぶ。何が出るかは献立次第なので件数だけを見る。
  const items = page.getByRole('listitem')
  await expect
    .poll(async () => items.count(), { message: '食材が1件以上出る' })
    .toBeGreaterThan(0)

  // 売り場が分かるようカテゴリの見出しが付く。
  // 7日分あれば野菜はまず含まれる。
  await expect(page.getByText('野菜', { exact: true })).toBeVisible()

  // 代表的な食材の例であって正確な材料表ではないことを必ず伝える（spec.md 14.1）。
  await expect(page.getByText(/実際の材料はレシピ元/)).toBeVisible()
})

test('週間献立が無ければ、先に作るよう案内する', async ({ page }) => {
  // 週を作らずに直接開く。
  await page.goto('/shopping-list')

  await expect(
    page.getByRole('heading', { level: 1, name: '買い物リスト' }),
  ).toBeVisible()
  await expect(
    page.getByRole('link', { name: '1週間の献立を作る' }),
  ).toBeVisible()
})

test('検索した献立にそのまま必要な食材が出る', async ({ page }) => {
  // 検索はアプリの中核。「何を買えばいいか」は結果を見た直後に知りたいので、
  // 献立詳細まで行かせない（11-H の E2E でこの導線の抜けに気づいた）。
  await page.goto('/search')
  await page.getByRole('button', { name: '献立を探す' }).click()

  await expect(
    page.getByRole('heading', { level: 2, name: '必要な食材' }),
  ).toBeVisible()
  await expect(page.getByText(/調味料は含みません/)).toBeVisible()
})
