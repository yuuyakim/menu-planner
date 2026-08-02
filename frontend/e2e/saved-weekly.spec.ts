import { expect, test } from '@playwright/test'

import { signUp, uniqueEmail } from './helpers'

test('作った週を保存し、別画面を経て開き直して買い物リストまで進める', async ({
  page,
}) => {
  const email = uniqueEmail('saved-weekly')
  await signUp(page, email)

  // 週間献立の作成・保存・保存一覧の閲覧はログインすれば誰でも使える。
  await page.goto('/weekly')
  await page.getByRole('button', { name: '1週間分を作る' }).click()
  await expect(page.getByRole('listitem')).toHaveCount(7)

  // 保存する前に、この週の献立名を控えておく。
  // 開き直したときに「同じ週が戻った」ことを名前で確かめるため。
  const before = await page
    .getByRole('listitem')
    .locator('h2')
    .allTextContents()
  expect(before).toHaveLength(7)

  await page.getByRole('button', { name: 'この週を保存する' }).click()
  await expect(page.getByRole('status')).toContainText('保存しました')

  // ここが要件の肝。タブを閉じても消えないことを、sessionStorage を
  // 消してから確かめる。消さずに遷移するだけだと、サーバに保存できて
  // いなくても「残っている」ように見えてしまう。
  await page.evaluate(() => sessionStorage.clear())

  await page.goto('/saved-weekly')
  await expect(
    page.getByRole('heading', { level: 1, name: '保存した週間献立' }),
  ).toBeVisible()

  const saved = page.getByRole('listitem').first()
  await saved.getByRole('button', { name: '開く' }).click()

  // 保存した週がそのまま戻る。
  await expect(
    page.getByRole('heading', { level: 1, name: '1週間の献立' }),
  ).toBeVisible()
  await expect(page.getByRole('listitem')).toHaveCount(7)
  const after = await page
    .getByRole('listitem')
    .locator('h2')
    .allTextContents()
  expect(after).toEqual(before)

  // 開いた週から買い物リストまで続けられる（12-A の見込みの最終確認）。
  await page.getByRole('link', { name: '買い物リストを見る' }).click()
  await expect(
    page.getByRole('heading', { level: 1, name: '買い物リスト' }),
  ).toBeVisible()
  await expect
    .poll(async () => page.getByRole('listitem').count(), {
      message: '開き直した週からも食材が出る',
    })
    .toBeGreaterThan(0)
})

test('未ログインで保存一覧を開くとログイン画面へ送られる', async ({ page }) => {
  await page.goto('/saved-weekly')

  await expect(page.getByRole('heading', { name: 'ログイン' })).toBeVisible()
})

test('保存した週を削除できる', async ({ page }) => {
  const email = uniqueEmail('saved-weekly-del')
  await signUp(page, email)

  // 週間献立の作成・保存・保存一覧の閲覧はログインすれば誰でも使える。
  await page.goto('/weekly')
  await page.getByRole('button', { name: '1週間分を作る' }).click()
  await expect(page.getByRole('listitem')).toHaveCount(7)
  await page.getByRole('button', { name: 'この週を保存する' }).click()
  await expect(page.getByRole('status')).toContainText('保存しました')

  await page.goto('/saved-weekly')
  await expect(page.getByRole('listitem')).toHaveCount(1)

  await page.getByRole('button', { name: '削除' }).click()

  // 消えたら、作って保存するよう促す表示に戻る。
  await expect(page.getByText(/まだ保存した週間献立がありません/)).toBeVisible()
  await expect(page.getByRole('listitem')).toHaveCount(0)
})

test('保存は本人のものだけが見える', async ({ page }) => {
  // 1人目が保存する。
  const emailA = uniqueEmail('saved-weekly-a')
  await signUp(page, emailA)

  // 週間献立の作成・保存はログインすれば誰でも使える。
  await page.goto('/weekly')
  await page.getByRole('button', { name: '1週間分を作る' }).click()
  await expect(page.getByRole('listitem')).toHaveCount(7)
  await page.getByRole('button', { name: 'この週を保存する' }).click()
  await expect(page.getByRole('status')).toContainText('保存しました')

  // 2人目に切り替える。
  await page.getByRole('button', { name: 'ログアウト' }).click()
  const emailB = uniqueEmail('saved-weekly-b')
  await signUp(page, emailB)

  await page.goto('/saved-weekly')
  await expect(page.getByText(/まだ保存した週間献立がありません/)).toBeVisible()
})
