import type { Page } from '@playwright/test'

/**
 * uniqueEmail はテストごとに使い捨てのメールアドレスを作る。
 * メールは一意制約があるため、使い回すと2回目以降が 409 で落ちる。
 */
export function uniqueEmail(prefix: string): string {
  return `e2e-${prefix}-${Date.now()}-${Math.floor(Math.random() * 10000)}@example.com`
}

/** testPassword は登録・ログインに使うパスワード（8文字以上）。 */
export const testPassword = 'password123'

/**
 * signUp は新規ユーザーを登録してログイン状態にする。
 * 画面から操作するのは、実際の利用者と同じ経路を通すため。
 */
export async function signUp(page: Page, email: string): Promise<void> {
  await page.goto('/login')
  await page.getByRole('button', { name: '新規登録はこちら' }).click()
  await page.getByLabel('メールアドレス').fill(email)
  await page.getByLabel('パスワード').fill(testPassword)
  await page.getByRole('button', { name: '登録する' }).click()

  // 登録できるとホームへ移り、ヘッダにログアウトが出る。
  await page.getByRole('button', { name: 'ログアウト' }).waitFor()
}

/**
 * choose は検索フォームの選択肢を選ぶ。
 *
 * ラジオ本体は sr-only（見た目を label 側で作っている）ため、
 * 実際の利用者と同じように label を押す。input を直接クリックしようとすると
 * label に覆われていて押せない。
 */
export async function choose(
  page: Page,
  group: string,
  option: string,
): Promise<void> {
  await page
    .getByRole('group', { name: group })
    .locator('label')
    .filter({ hasText: new RegExp(`^${option}$`) })
    .click()
}
