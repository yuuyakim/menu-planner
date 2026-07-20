import { expect, test } from '@playwright/test'

import { signUp, uniqueEmail } from './helpers'

test('お気に入りに追加して一覧で確認し、外せる', async ({ page }) => {
  await signUp(page, uniqueEmail('favorite'))

  await page.getByRole('link', { name: '献立を探す' }).first().click()
  await page.getByRole('button', { name: '献立を探す' }).click()

  const card = page.getByRole('article')
  const menuName = await card.getByRole('heading', { level: 2 }).textContent()

  // 星を押す。押した結果は見た目（塗り）と状態（aria-pressed）に出る。
  const star = card.getByRole('button', { name: 'お気に入りに追加' })
  await expect(star).toHaveAttribute('aria-pressed', 'false')
  await star.click()
  await expect(
    card.getByRole('button', { name: 'お気に入り済み' }),
  ).toHaveAttribute('aria-pressed', 'true')

  // 一覧へ。リロードせずに反映されている必要がある。
  await page.getByRole('link', { name: 'お気に入り' }).first().click()
  const items = page.getByRole('listitem')
  await expect(items).toHaveCount(1)
  await expect(items.first()).toContainText(menuName!)

  await items.first().getByRole('button', { name: '外す' }).click()
  await expect(page.getByText('まだお気に入りがありません')).toBeVisible()
})

test('未ログインで星を押すとログインへ誘導される', async ({ page }) => {
  await page.goto('/search')
  await page.getByRole('button', { name: '献立を探す' }).click()

  const card = page.getByRole('article')
  await expect(card).toBeVisible()

  // 未ログインでも星は出ている（機能の存在に気づけるようにするため）。
  await card.getByRole('button', { name: 'お気に入りに追加' }).click()

  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  await dialog.getByRole('link', { name: /ログイン/ }).click()

  await expect(
    page.getByRole('heading', { level: 1, name: 'ログイン' }),
  ).toBeVisible()
})
