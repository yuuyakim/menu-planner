import { execSync } from 'node:child_process'

import { expect, test } from '@playwright/test'

import { signUp, uniqueEmail } from './helpers'

test('1週間分の献立を作り、1日だけ引き直せる', async ({ page }) => {
  const email = uniqueEmail('weekly')
  await signUp(page, email)

  // 週間献立の作成・引き直しは premium 限定（決済が無いのでCLIで付与する。
  // premium.spec.ts と同じ流儀）。
  execSync(
    `docker compose run --rm backend go run ./cmd/grant -email=${email} -months=1`,
    { cwd: '..', stdio: 'inherit' },
  )
  // useCurrentUser は staleTime 5分でキャッシュするため、取り直しが要る。
  await page.reload()

  await page.goto('/weekly')

  await page.getByRole('button', { name: '1週間分を作る' }).click()

  // 7日分が並ぶ。
  const days = page.getByRole('listitem')
  await expect(days).toHaveCount(7)

  // 献立は重複しない（spec.md 2.2 のルール1）。
  const names = await days
    .getByRole('heading', { level: 2 })
    .allTextContents()
  expect(new Set(names).size).toBe(7)

  // 3日目だけを引き直す。
  const third = days.nth(2)
  const others = [0, 1, 3, 4, 5, 6].map((i) => names[i])
  await third.getByRole('button', { name: '引き直す' }).click()

  await expect
    .poll(async () => third.getByRole('heading', { level: 2 }).textContent())
    .not.toBe(names[2])

  // 他の日は動かない。
  const after = await days.getByRole('heading', { level: 2 }).allTextContents()
  expect([0, 1, 3, 4, 5, 6].map((i) => after[i])).toEqual(others)
})

test('画面を離れて戻っても週が残る', async ({ page }) => {
  const email = uniqueEmail('weekly-persist')
  await signUp(page, email)

  // 週間献立の作成は premium 限定（決済が無いのでCLIで付与する）。
  execSync(
    `docker compose run --rm backend go run ./cmd/grant -email=${email} -months=1`,
    { cwd: '..', stdio: 'inherit' },
  )
  await page.reload()

  await page.goto('/weekly')
  await page.getByRole('button', { name: '1週間分を作る' }).click()

  const days = page.getByRole('listitem')
  await expect(days).toHaveCount(7)
  const first = await days
    .first()
    .getByRole('heading', { level: 2 })
    .textContent()

  // レシピを見に行って戻る。
  await days.first().getByRole('link', { name: 'レシピを見る' }).click()
  // 遷移の完了は URL と見出しの中身で確かめる。単に h1 の存在を待つと、
  // 遷移前の画面の h1 に一致してしまい、実際には遷移前でも通ってしまう。
  await expect(page).toHaveURL(/\/menus\//)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(first!)
  await page.getByRole('button', { name: '戻る' }).click()

  // 作り直さなくても7日分がそのまま残っている。
  await expect(page.getByRole('listitem')).toHaveCount(7)
  await expect(
    page.getByRole('listitem').first().getByRole('heading', { level: 2 }),
  ).toHaveText(first!)
})
