import { Link } from 'react-router'

import { useCurrentUser } from '../auth/useCurrentUser'

type Entry = {
  to: string
  title: string
  description: string
  /** 認証が要る機能かどうか。未ログインならその旨を添える。 */
  requiresAuth: boolean
}

const entries: Entry[] = [
  {
    to: '/search',
    title: '献立を探す',
    description: '今日の夕食を1食分だけ提案します。',
    requiresAuth: false,
  },
  {
    to: '/weekly',
    title: '1週間の献立',
    description: '今日から7日分をまとめて組み立てます。',
    requiresAuth: false,
  },
  {
    to: '/histories',
    title: '検索履歴',
    description: '直近に提案された献立を振り返れます。',
    requiresAuth: true,
  },
  {
    to: '/favorites',
    title: 'お気に入り',
    description: '気に入った献立をいつでも呼び出せます。',
    requiresAuth: true,
  },
]

// HomePage はアプリの入口。
//
// 検索画面を入口にしていたときは「今どういう状態か」を示す場所が無く、
// ログイン・ログアウトしても画面が変わらなかった。ここを起点にすることで、
// 誰として使っているか・どの機能が使えるかが一目で分かる。
export function HomePage() {
  const { user, isLoading } = useCurrentUser()

  return (
    <section className="space-y-8">
      <div className="space-y-2">
        <h1 className="text-2xl font-bold">献立プランナー</h1>
        {/* 判定が付くまでは、ログイン状態に依る文言を出さない。
            決めつけると認証済みの利用者に一瞬ログインを勧めてしまう。 */}
        {!isLoading &&
          (user ? (
            <p className="text-slate-600">
              {user.displayName} として使っています。
            </p>
          ) : (
            <p className="text-slate-600">
              献立探しはそのまま使えます。履歴とお気に入りを使うには{' '}
              <Link to="/login" className="text-emerald-700 underline">
                ログイン
              </Link>
              してください。
            </p>
          ))}
      </div>

      <ul className="grid gap-4 sm:grid-cols-2">
        {entries.map((entry) => (
          <li key={entry.to}>
            <Link
              to={entry.to}
              className="block h-full rounded-xl border border-slate-200 bg-white p-5 hover:border-emerald-400"
            >
              <span className="block font-bold">{entry.title}</span>
              <span className="mt-1 block text-sm text-slate-600">
                {entry.description}
              </span>
              {/* 押してからログイン画面に飛ばされるより、先に断っておく方が親切。 */}
              {entry.requiresAuth && !isLoading && !user && (
                <span className="mt-2 block text-xs text-slate-500">
                  ログインが必要です
                </span>
              )}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}
