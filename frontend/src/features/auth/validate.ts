// 入力の検証。サーバ側でも同じ制約を検証している（信頼の境界はサーバ）。
// ここで弾くのは、往復を1回減らして誤りをその場で伝えるため。

/** passwordMinLength はパスワードの最小文字数（spec.md 11・8文字以上）。 */
export const passwordMinLength = 8

// emailPattern は「@ の前後に空白でない文字があり、ドメインにドットがある」形。
// RFC 5322 を完全に実装はしない。厳密な判定はサーバ（mail.ParseAddress）が行う。
const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/**
 * validateCredentials は入力を検証し、問題があれば文言を返す。
 * 問題が無ければ undefined。
 */
export function validateCredentials(
  email: string,
  password: string,
): string | undefined {
  if (!emailPattern.test(email.trim())) {
    return 'メールアドレスの形式が正しくありません。'
  }
  // 文字数は文字単位で数える。絵文字などのサロゲートペアを2文字と数えないため。
  if ([...password].length < passwordMinLength) {
    return `パスワードは${passwordMinLength}文字以上にしてください。`
  }
  return undefined
}
