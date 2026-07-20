import { expect, test } from '@playwright/test'

import { signUp, uniqueEmail } from './helpers'

test('サインアップして検索すると履歴に残る', async ({ page }) => {
  await signUp(page, uniqueEmail('history'))

  // 検索する。ログイン中なのでサーバが履歴に記録する。
  await page.getByRole('link', { name: '献立を探す' }).first().click()
  await page.getByRole('button', { name: '献立を探す' }).click()

  const menuName = await page
    .getByRole('article')
    .getByRole('heading', { level: 2 })
    .textContent()
  expect(menuName).toBeTruthy()

  // 履歴へ。リロードせずに反映されている必要がある。
  await page.getByRole('link', { name: '履歴' }).click()

  const items = page.getByRole('listitem')
  await expect(items).toHaveCount(1)
  await expect(items.first()).toContainText(menuName!)
  await expect(items.first()).toContainText('1食分')
})

test('履歴を削除できる', async ({ page }) => {
  await signUp(page, uniqueEmail('history-delete'))

  await page.getByRole('link', { name: '献立を探す' }).first().click()
  await page.getByRole('button', { name: '献立を探す' }).click()
  await expect(page.getByRole('article')).toBeVisible()

  await page.getByRole('link', { name: '履歴' }).click()
  await expect(page.getByRole('listitem')).toHaveCount(1)

  // 「すべて削除」にも一致してしまうため完全一致で指定する。
  await page.getByRole('button', { name: '削除', exact: true }).click()

  await expect(page.getByText('まだ履歴がありません')).toBeVisible()
})

test('未ログインでは履歴がログイン画面へ誘導される', async ({ page }) => {
  await page.goto('/histories')

  await expect(
    page.getByRole('heading', { level: 1, name: 'ログイン' }),
  ).toBeVisible()
})
