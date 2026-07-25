import { execSync } from 'node:child_process'

import { expect, test } from '@playwright/test'

import { signUp, uniqueEmail } from './helpers'

test('free は週間がロックされ、premium 付与後に使える', async ({ page }) => {
  const email = uniqueEmail('weekly-premium')
  await signUp(page, email)

  // free: /weekly はロック（生成ボタンは出ない）。
  await page.goto('/weekly')
  await expect(page.getByText(/1週間まとめて|プレミアム/).first()).toBeVisible()
  await expect(page.getByRole('button', { name: '1週間分を作る' })).toHaveCount(0)

  // 決済が無いので付与はCLIで行う（premium.spec.ts と同じ流儀で、
  // 起動中のコンテナに対して実行する）。
  execSync(
    `docker compose run --rm backend go run ./cmd/grant -email=${email} -months=1`,
    { cwd: '..', stdio: 'inherit' },
  )

  // useCurrentUser は staleTime 5分でキャッシュするため、取り直しが要る。
  await page.reload()

  // premium: 生成UIが出て作れる。
  await page.getByRole('button', { name: '1週間分を作る' }).click()
  await expect(page.getByRole('listitem')).toHaveCount(7)
})
