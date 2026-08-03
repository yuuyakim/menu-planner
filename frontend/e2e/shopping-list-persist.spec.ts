import { expect, test } from '@playwright/test'

import { signUp, uniqueEmail } from './helpers'

test('保存済み週の買い物リストのチェックはリロード後も残る', async ({
  page,
}) => {
  const email = uniqueEmail('slo-persist')
  await signUp(page, email)

  // 週を作って保存し、そこから買い物リストへ進む（saved-weekly.spec.ts と同じ導線）。
  await page.goto('/weekly')
  await page.getByRole('button', { name: '1週間分を作る' }).click()
  await expect(page.getByRole('listitem')).toHaveCount(7)
  await page.getByRole('button', { name: 'この週を保存する' }).click()
  await expect(page.getByRole('status')).toContainText('保存しました')

  await page.getByRole('link', { name: '買い物リストを見る' }).click()
  await expect(
    page.getByRole('heading', { level: 1, name: '買い物リスト' }),
  ).toBeVisible()

  // ランダム献立のため品目名は固定できない。最初のチェックボックスに絞る。
  const first = page.getByRole('checkbox').first()
  // チェック操作は PUT（非keepalive）を発火するだけで完了を待たない。
  // reload がそれより先に走ると保存前に中断されるため、PUT応答を待ってから進める。
  await Promise.all([
    page.waitForResponse(
      (res) =>
        res.url().includes('/shopping-list') &&
        res.request().method() === 'PUT',
    ),
    first.check(),
  ])
  await expect(first).toBeChecked()

  // リロードしても残る（＝サーバに永続化された）。
  await page.reload()
  await expect(page.getByRole('checkbox').first()).toBeChecked()
})
