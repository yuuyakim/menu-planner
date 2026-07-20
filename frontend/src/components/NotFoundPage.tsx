import { Link } from 'react-router'

// NotFoundPage はどのルートにも一致しなかったときの画面。
export function NotFoundPage() {
  return (
    <section>
      <h1 className="text-2xl font-bold">ページが見つかりません</h1>
      <p className="mt-2 text-slate-600">
        URLが変わったか、削除された可能性があります。
      </p>
      <Link to="/" className="mt-4 inline-block text-emerald-700 underline">
        ホームへ
      </Link>
    </section>
  )
}
