import { execSync } from 'node:child_process'

import { expect, test } from '@playwright/test'

import { choose, signUp, uniqueEmail } from './helpers'

// 献立の役割（spec.md 2.10）。
//
// 利用者の動機は引き直しコスト。晩ごはんを決めようとしてカプレーゼや
// コーンスープが単品で出ると、それだけでは夕食にならず必ず引き直しになる。
// **既定で副菜・汁物が出ないこと**が、この機能が入った意味そのもの。

test('何も選ばずに探すと主菜が出る', async ({ page }) => {
  await page.goto('/search')

  // 「種類」に触れずそのまま探す。ジャンル・難易度の未指定は「すべて」だが、
  // 役割の未指定だけは「主菜」に倒れる。
  await page.getByRole('button', { name: '献立を探す' }).click()

  const card = page.getByRole('article')
  await expect(card).toBeVisible()
  await expect(card.getByText('主菜', { exact: true })).toBeVisible()
})

test('種類の既定は主菜が選ばれている', async ({ page }) => {
  await page.goto('/search')

  const kind = page.getByRole('group', { name: '種類' })
  await expect(kind.getByRole('radio', { name: '主菜' })).toBeChecked()
  await expect(kind.getByRole('radio', { name: 'すべて' })).not.toBeChecked()
})

test('副菜に切り替えると副菜が出る', async ({ page }) => {
  await page.goto('/search')

  await choose(page, '種類', '副菜')
  await page.getByRole('button', { name: '献立を探す' }).click()

  const card = page.getByRole('article')
  await expect(card).toBeVisible()
  await expect(card.getByText('副菜', { exact: true })).toBeVisible()
  // 主菜のバッジは出ない。取り違えるとこの機能の意味が無くなる。
  await expect(card.getByText('主菜', { exact: true })).toHaveCount(0)
})

test('汁物に切り替えると汁物が出る', async ({ page }) => {
  await page.goto('/search')

  await choose(page, '種類', '汁物')
  await page.getByRole('button', { name: '献立を探す' }).click()

  const card = page.getByRole('article')
  await expect(card).toBeVisible()
  await expect(card.getByText('汁物', { exact: true })).toBeVisible()
})

test('すべてを選んでも探せる', async ({ page }) => {
  await page.goto('/search')

  await choose(page, '種類', 'すべて')
  await page.getByRole('button', { name: '献立を探す' }).click()

  // **何の役割が出たかは検証しない。** 何が出るかは献立マスタ次第で、
  // 主菜が83%を占めるため「副菜が出ること」を待つと確率的なテストになる
  // （13-F の「候補の中身は検証しない」と同じ理由）。
  // ここで見たいのは、`all` を送っても 400 にならず献立が返ること。
  // 絞り込みが外れること自体は repository / handler の単体テストが持つ。
  const card = page.getByRole('article')
  await expect(card).toBeVisible()

  // 役割のバッジはどれか1つだけ出る。0個なら表示が壊れており、
  // 2個以上なら役割が一意でなくなっている。
  const roleBadges = card.getByText(/^(主菜|副菜|汁物)$/)
  await expect(roleBadges).toHaveCount(1)
})

test('引き直しても選んだ種類のまま', async ({ page }) => {
  await page.goto('/search')

  await choose(page, '種類', '副菜')
  await page.getByRole('button', { name: '献立を探す' }).click()

  const card = page.getByRole('article')
  await expect(card.getByText('副菜', { exact: true })).toBeVisible()

  // 引き直しは同じ条件を使い回す。ここで条件が抜けると主菜に戻ってしまう。
  for (let i = 0; i < 3; i++) {
    await page.getByRole('button', { name: '別の献立を見る' }).click()
    await expect(card.getByText('副菜', { exact: true })).toBeVisible()
  }
})

test('週間献立の既定は7日とも主菜', async ({ page }) => {
  const email = uniqueEmail('menu-role-weekly')
  await signUp(page, email)

  // 週間献立の作成は premium 限定（決済が無いのでCLIで付与する。
  // premium.spec.ts と同じ流儀）。
  execSync(
    `docker compose run --rm backend go run ./cmd/grant -email=${email} -months=1`,
    { cwd: '..', stdio: 'inherit' },
  )
  // useCurrentUser は staleTime 5分でキャッシュするため、取り直しが要る。
  await page.reload()

  await page.goto('/weekly')

  await page.getByRole('button', { name: '1週間分を作る' }).click()

  const cards = page.getByRole('article')
  await expect(cards).toHaveCount(7)
  // 週に副菜だけの日が混ざらないことが要（spec.md 2.2）。
  await expect(cards.getByText('主菜', { exact: true })).toHaveCount(7)
})

test('未ログインでも種類を選んで探せる', async ({ page }) => {
  await page.goto('/search')

  // ログインの導線が出ている＝未ログインの状態。
  await expect(page.getByRole('link', { name: 'ログイン' })).toBeVisible()

  await choose(page, '種類', '副菜')
  await page.getByRole('button', { name: '献立を探す' }).click()

  await expect(page.getByRole('article').getByText('副菜', { exact: true })).toBeVisible()
})
