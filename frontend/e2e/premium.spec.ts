import { execSync } from 'node:child_process'

import { expect, test } from '@playwright/test'

import { signUp, uniqueEmail } from './helpers'

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
