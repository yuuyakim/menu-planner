import { expect, test } from '@playwright/test'

import { choose } from './helpers'

// PLAN.md の中核シナリオ。このアプリが何のためにあるかを表す1本。
// 「和食×簡単を選ぶと献立が1つ出て、そのレシピを新しいタブで開ける」
test('和食×簡単で献立を探し、レシピを新しいタブで開ける', async ({
  page,
  context,
}) => {
  await page.goto('/search')

  // 条件を選ぶ。ジャンルと難易度で同じ「すべて」があるため、組で絞る。
  // ラジオ本体は sr-only なので、実際の利用者と同じく label を押す。
  await choose(page, 'ジャンル', '和食')
  await choose(page, '難易度', '簡単')
  await page.getByRole('button', { name: '献立を探す' }).click()

  // 献立が1つ出る。選ばれた条件がそのまま反映されている。
  const card = page.getByRole('article')
  await expect(card).toBeVisible()
  await expect(card.getByText('和食')).toBeVisible()
  await expect(card.getByText('簡単')).toBeVisible()
  await expect(card.getByRole('heading', { level: 2 })).not.toBeEmpty()

  // レシピが3件出る。
  const recipeLinks = page.getByRole('list').getByRole('link')
  await expect(recipeLinks).toHaveCount(3)

  // 1件目を開くと新しいタブになる。元の画面（献立）は残る。
  const [popup] = await Promise.all([
    context.waitForEvent('page'),
    recipeLinks.first().click(),
  ])

  expect(popup).not.toBe(page)
  expect(popup.url()).not.toBe(page.url())
  await expect(card).toBeVisible()
})

test('別の献立を見るで引き直せる', async ({ page }) => {
  await page.goto('/search')

  await choose(page, 'ジャンル', '和食')
  await page.getByRole('button', { name: '献立を探す' }).click()

  const heading = page.getByRole('article').getByRole('heading', { level: 2 })
  await expect(heading).toBeVisible()
  const first = await heading.textContent()

  // 献立はランダムに選ばれるため、同じものが続けて出ることがある。
  // 数回試して、いずれかで変わることを確かめる。
  let changed = false
  for (let i = 0; i < 5 && !changed; i++) {
    await page.getByRole('button', { name: '別の献立を見る' }).click()
    await expect(heading).toBeVisible()
    changed = (await heading.textContent()) !== first
  }

  expect(changed).toBe(true)
})
