import { Link } from 'react-router'

// NotFoundPage はどのルートにも一致しなかったときの画面。
//
// こんたてんを出す場面にしている。通信の失敗や不具合と違い、ここは
// 何も壊れていない（行き先が無いだけ）。迷ったときに迎えてくれる相手が
// 居た方が、行き止まり感が和らぐ。
// 絵は装飾（alt=""）で、伝えるのは見出しと本文の役目。
export function NotFoundPage() {
  return (
    <section className="flex flex-col items-center gap-4 py-8 text-center">
      <img src="/mascot/face-thinking.png" alt="" className="size-28" />
      <h1 className="text-2xl font-bold text-kon-ink">ページが見つかりません</h1>
      <p className="text-kon-ink/75">
        URLが変わったか、削除された可能性があります。
      </p>
      <Link
        to="/"
        className="rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95"
      >
        ホームへ
      </Link>
    </section>
  )
}
