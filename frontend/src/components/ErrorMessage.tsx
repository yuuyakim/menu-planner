import { ApiError } from '../api/client'

// toMessage は例外を利用者に見せる文言にする。
// ApiError / NetworkError は message に表示可能な文言を持つ（8-C）。
// 想定外の例外だけは中身を出さず、汎用の文言に置き換える
// （内部の情報が画面に漏れるのを防ぐ）。
function toMessage(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message
  }
  return '予期しないエラーが発生しました。'
}

// ErrorMessage は失敗を利用者に伝える。
// role=alert にして、画面を見ていなくても伝わるようにする。
//
// ここにマスコットは出さない。こんたてんは待ち時間・空・404 のような
// 「何も壊れていない場面」に出す約束にしている。失敗に絵を添えると、
// うまくいかなかったことを軽く扱っているように見える。
export function ErrorMessage({ error }: { error: unknown }) {
  const isUnauthorized = error instanceof ApiError && error.isUnauthorized

  return (
    <p
      role="alert"
      className="rounded-2xl border border-kon-peach bg-kon-peach/25 px-4 py-3 text-kon-ink"
    >
      {toMessage(error)}
      {isUnauthorized && '（ログインし直してください）'}
    </p>
  )
}
