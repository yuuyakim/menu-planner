import { Link } from 'react-router'

import { useCurrentUser } from '../auth/useCurrentUser'

type Entry = {
  to: string
  title: string
  description: string
  /** 入口に添えるこんたてんの絵。装飾なので alt は空にする。 */
  image: string
  /** 認証が要る機能かどうか。未ログインならその旨を添える。 */
  requiresAuth: boolean
}

// 絵は機能の気分に合わせる。考え中＝ひらめき、週の献立＝出来上がり、
// 履歴＝ひと息、お気に入り＝うれしい。
const entries: Entry[] = [
  {
    to: '/search',
    title: '献立を探す',
    description: '今日の夕食を1食分だけ提案します。',
    image: '/mascot/pose-idea.png',
    requiresAuth: false,
  },
  {
    to: '/weekly',
    title: '1週間の献立',
    description: '今日から7日分をまとめて組み立てます。',
    image: '/mascot/pose-dish.png',
    requiresAuth: false,
  },
  {
    to: '/histories',
    title: '検索履歴',
    description: '直近に提案された献立を振り返れます。',
    image: '/mascot/face-relax.png',
    requiresAuth: true,
  },
  {
    to: '/favorites',
    title: 'お気に入り',
    description: '気に入った献立をいつでも呼び出せます。',
    image: '/mascot/face-happy.png',
    requiresAuth: true,
  },
]

// HomePage はアプリの入口。
//
// 検索画面を入口にしていたときは「今どういう状態か」を示す場所が無く、
// ログイン・ログアウトしても画面が変わらなかった。ここを起点にすることで、
// 誰として使っているか・どの機能が使えるかが一目で分かる。
//
// マスコット（こんたてん）は全て装飾として置き、alt を空にする。
// 意味は必ず隣のテキストが持つので、読み上げに絵の説明が割り込まない。
export function HomePage() {
  const { user, isLoading } = useCurrentUser()

  return (
    <section className="space-y-8">
      <div className="rounded-3xl bg-kon-cream px-5 py-6 sm:px-8">
        <div className="flex items-center gap-4 sm:gap-6">
          <img
            src="/mascot/hero.png"
            alt=""
            width={350}
            height={440}
            className="w-24 shrink-0 sm:w-32"
          />
          <div className="min-w-0 flex-1 space-y-3">
            {/* 吹き出しの文言は画像に焼き込まず本文で持つ。
                翻訳・読み上げ・文字の拡大が素直に効く。 */}
            {/* 和文は文字単位で折り返せてしまい、狭い画面で「一／緒」のように
                語の途中で割れる。文節を inline-block で括ると、その中では
                改行されず、区切りでだけ折り返る。 */}
            <p className="relative w-fit rounded-2xl border border-kon-leaf-soft bg-white px-4 py-2 text-sm font-medium text-kon-ink sm:text-base">
              <span className="inline-block">今日のごはん、</span>
              <span className="inline-block">一緒に考えよっ！</span>
              <span
                aria-hidden="true"
                className="absolute -left-[7px] bottom-3 size-3 rotate-45 border-b border-l border-kon-leaf-soft bg-white"
              />
            </p>
            <div className="space-y-1">
              <h1 className="text-2xl font-bold text-kon-ink">献立くん</h1>
              {/* 判定が付くまでは、ログイン状態に依る文言を出さない。
                  決めつけると認証済みの利用者に一瞬ログインを勧めてしまう。 */}
              {!isLoading &&
                (user ? (
                  <p className="text-sm text-kon-ink/75">
                    {user.displayName} として使っています。
                  </p>
                ) : (
                  <p className="text-sm text-kon-ink/75">
                    献立探しはそのまま使えます。履歴とお気に入りを使うには{' '}
                    <Link
                      to="/login"
                      className="font-medium text-kon-ink underline decoration-kon-leaf decoration-2 underline-offset-2"
                    >
                      ログイン
                    </Link>
                    してください。
                  </p>
                ))}
            </div>
          </div>
        </div>
      </div>

      <ul className="grid gap-4 sm:grid-cols-2">
        {entries.map((entry) => (
          <li key={entry.to}>
            <Link
              to={entry.to}
              className="flex h-full items-center gap-3 rounded-2xl border border-kon-leaf-soft bg-white p-4 transition-colors hover:border-kon-leaf hover:bg-kon-cream"
            >
              <img
                src={entry.image}
                alt=""
                className="size-16 shrink-0 object-contain"
              />
              <span className="min-w-0">
                <span className="block font-bold text-kon-ink">
                  {entry.title}
                </span>
                <span className="mt-0.5 block text-sm text-kon-ink/75">
                  {entry.description}
                </span>
                {/* 押してからログイン画面に飛ばされるより、先に断っておく方が親切。 */}
                {entry.requiresAuth && !isLoading && !user && (
                  <span className="mt-1.5 block w-fit rounded-full bg-kon-gold/35 px-2 py-0.5 text-xs text-kon-ink">
                    ログインが必要です
                  </span>
                )}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}
