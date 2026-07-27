import { execSync } from 'node:child_process'

import { expect, test } from '@playwright/test'

import { signUp, testPassword, uniqueEmail } from './helpers'

test('プレミアムを付与するとバッジが出る', async ({ page }) => {
  const email = uniqueEmail('premium')
  await signUp(page, email)

  // 付与前は出ていない。ここを確かめておかないと、
  // 「元から出ていた」のか「付与で出た」のかを区別できない。
  await expect(page.getByLabel('プレミアム会員')).toBeHidden()

  // 決済が無いので付与はCLIで行う。既存E2Eが make up / make seed を
  // 前提にしているのと同じ流儀で、起動中のコンテナに対して実行する。
  execSync(`docker compose run --rm backend go run ./cmd/grant -email=${email} -months=1`, {
    cwd: '..',
    stdio: 'inherit',
  })

  // useCurrentUser は staleTime 5分でキャッシュするため、取り直しが要る。
  await page.reload()

  await expect(page.getByLabel('プレミアム会員')).toBeVisible()
})

test('無料の利用者にはバッジが出ない', async ({ page }) => {
  await signUp(page, uniqueEmail('free'))

  // signUp がログアウトボタンの出現まで待つので、ヘッダの描画は完了している。
  // それを確かめたうえで「無いこと」を検査する。
  await expect(page.getByRole('button', { name: 'ログアウト' })).toBeVisible()
  await expect(page.getByLabel('プレミアム会員')).toBeHidden()
})

// free が週間画面でロックされたその場から加入画面へ進めること。
// この繋ぎ目が切れていたのが今回の不具合だったため、通しで押さえる。
test('free は週間献立でロックされ、その場から加入画面へ進める', async ({ page }) => {
  await signUp(page, uniqueEmail('lock'))

  await page.goto('/weekly')

  await page
    .getByRole('link', { name: 'プレミアムにアップグレード' })
    .click()

  await expect(page).toHaveURL(/\/checkout$/)
  await expect(
    page.getByRole('heading', { name: 'お申込み内容の確認' }),
  ).toBeVisible()
})

// 未ログインが料金ページから加入へ向かうと、ログインを挟んで加入画面に戻る。
// 戻り先は RequireAuth が state.from に残し、LoginPage がそこへ navigate する。
test('未ログインは料金ページからログインを経て加入画面に着く', async ({ page }) => {
  await page.goto('/pricing')

  await page.getByRole('link', { name: 'プレミアムを試す' }).click()

  await expect(page).toHaveURL(/\/login$/)

  await page.getByRole('button', { name: '新規登録はこちら' }).click()
  await page.getByLabel('メールアドレス').fill(uniqueEmail('pricing'))
  await page.getByLabel('パスワード').fill(testPassword)
  await page.getByRole('button', { name: '登録する' }).click()

  await expect(page).toHaveURL(/\/checkout$/)
  await expect(
    page.getByRole('heading', { name: 'お申込み内容の確認' }),
  ).toBeVisible()
})
