import { expect, test } from '@playwright/test'

import { choose } from './helpers'

/**
 * pickIngredient は食材を1つ選ぶ。
 *
 * チェックボックス本体は sr-only（見た目を label 側で作っている）ため、
 * 実際の利用者と同じように label を押す。input を直接クリックしようとすると
 * label に覆われていて押せない（helpers.ts の choose と同じ理由）。
 */
async function pickIngredient(page: import('@playwright/test').Page, name: string) {
  await page
    .locator('label')
    .filter({ hasText: new RegExp(`^${name}$`) })
    .first()
    .click()
}

test('冷蔵庫にある食材から献立を探し、レシピへ進める', async ({ page }) => {
  await page.goto('/from-fridge')
  await expect(
    page.getByRole('heading', { level: 1, name: '冷蔵庫から探す' }),
  ).toBeVisible()

  // 何も選んでいなければ探せない。押しても400になるだけなので押させない。
  await expect(page.getByRole('button', { name: 'この食材で探す' })).toBeDisabled()

  // 定番の3品。実マスタに必ずある食材を選ぶ。
  await pickIngredient(page, '玉ねぎ')
  await pickIngredient(page, 'じゃがいも')
  await pickIngredient(page, 'にんじん')
  await expect(page.getByRole('status').first()).toHaveText('3個を選択中')

  await page.getByRole('button', { name: 'この食材で探す' }).click()

  await expect(
    page.getByRole('heading', { level: 2, name: /作れそうな献立/ }),
  ).toBeVisible()

  // 食材リストは代表例であって正確な材料表ではない（spec.md 14.1）。必ず断る。
  await expect(page.getByText(/調味料は含みません/)).toBeVisible()

  // 候補には「使える食材」と不足が出る。何が出るかはマスタ次第なので、
  // 件数と表示の形だけを見る。
  const items = page.getByRole('listitem')
  await expect
    .poll(async () => items.count(), { message: '候補が1件以上出る' })
    .toBeGreaterThan(0)
  await expect(page.getByText(/使える食材:/).first()).toBeVisible()

  // 候補からレシピへ進める。
  await page.getByRole('link', { name: 'レシピを見る' }).first().click()
  await expect(page.getByRole('heading', { level: 2, name: 'レシピ' })).toBeVisible()
})

test('選び直すと前の結果が消える', async ({ page }) => {
  await page.goto('/from-fridge')

  await pickIngredient(page, '玉ねぎ')
  await page.getByRole('button', { name: 'この食材で探す' }).click()
  await expect(
    page.getByRole('heading', { level: 2, name: /作れそうな献立/ }),
  ).toBeVisible()

  // 残したままだと、いま選んでいる食材の結果だと誤解させる。
  await pickIngredient(page, 'じゃがいも')
  await expect(
    page.getByRole('heading', { level: 2, name: /作れそうな献立/ }),
  ).toHaveCount(0)
})

test('選択をまとめて外せる', async ({ page }) => {
  await page.goto('/from-fridge')

  await pickIngredient(page, '玉ねぎ')
  await pickIngredient(page, 'じゃがいも')
  await expect(page.getByRole('status').first()).toHaveText('2個を選択中')

  await page.getByRole('button', { name: '選択をすべて外す' }).click()

  await expect(page.getByRole('status').first()).toHaveText(
    '冷蔵庫にあるものを選んでください',
  )
  await expect(page.getByRole('button', { name: 'この食材で探す' })).toBeDisabled()
})

test('未ログインでも使える', async ({ page }) => {
  // 検索と同じ扱い（spec.md 2.9）。ログイン画面に飛ばされない。
  await page.goto('/from-fridge')

  await pickIngredient(page, '玉ねぎ')
  await page.getByRole('button', { name: 'この食材で探す' }).click()

  await expect(
    page.getByRole('heading', { level: 2, name: /作れそうな献立/ }),
  ).toBeVisible()
  await expect(page.getByRole('heading', { level: 1, name: 'ログイン' })).toHaveCount(0)
})

test('作れるものだけに切り替えると候補が減る', async ({ page }) => {
  await page.goto('/from-fridge')

  // 頻出の食材を選ぶ。不足を許せば候補は必ず出る。
  await pickIngredient(page, '玉ねぎ')
  await pickIngredient(page, 'にんじん')
  await pickIngredient(page, 'じゃがいも')
  await pickIngredient(page, '米')

  await page.getByRole('button', { name: 'この食材で探す' }).click()
  await expect
    .poll(async () => page.getByRole('listitem').count(), {
      message: '不足を許した候補が1件以上出る',
    })
    .toBeGreaterThan(0)

  // ラジオ本体は sr-only ではないが、label を押す（choose と同じ経路）。
  await choose(page, '探し方', 'この中だけで作れるもの')
  await page.getByRole('button', { name: 'この食材で探す' }).click()

  // この4種では不足0の献立は無いが、不足ちょうど1の献立は5件ある
  // （seeds/menu_ingredients.sql で確認済み。玉ねぎ・にんじん・じゃがいもの
  // 3種だけだと不足の最小値が2で「あと1品」が1件も出ない）。
  // 0件だと明言した上で「あと1品」が別枠で出る。
  await expect(
    page.getByText(/この中だけで作れる献立はありませんでした/),
  ).toBeVisible()
  await expect(
    page.getByRole('heading', { level: 2, name: /あと1品買えば作れます/ }),
  ).toBeVisible()
  await expect
    .poll(async () => page.getByRole('listitem').count(), {
      message: '「あと1品」候補が1件以上出る',
    })
    .toBeGreaterThan(0)

  // 「この中だけで作れるもの」の間は並び順のつまみ自体が意味を持たず隠れる。
  await expect(page.getByRole('group', { name: '並び順' })).toHaveCount(0)
})
