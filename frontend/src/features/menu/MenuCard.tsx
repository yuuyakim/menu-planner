import { difficultyLabels, genreLabels, type Menu } from '../../api/types'
import { FavoriteButton } from '../favorite/FavoriteButton'

// MenuCard は献立1件の表示。検索結果・週間献立・詳細で共通に使う。
export function MenuCard({ menu }: { menu: Menu }) {
  return (
    // お気に入りは献立の右端に置く。下に積むと、献立の情報が増えていないのに
    // カードの縦幅だけが伸びる。週間献立では7枚分まとめて効いてくる。
    <article className="flex gap-4 rounded-xl border border-slate-200 bg-white p-6">
      <div className="min-w-0 flex-1">
        <h2 className="text-xl font-bold">{menu.name}</h2>
        <div className="mt-2 flex gap-2 text-sm">
          <span className="rounded-full bg-slate-100 px-3 py-0.5 text-slate-700">
            {genreLabels[menu.genre]}
          </span>
          <span className="rounded-full bg-slate-100 px-3 py-0.5 text-slate-700">
            {difficultyLabels[menu.difficulty]}
          </span>
        </div>
        <p className="mt-3 text-slate-600">{menu.description}</p>
      </div>

      {/* 献立が出る場所ならどこでもお気に入りにできる。
          未ログインのときは何も描画されない。 */}
      <FavoriteButton menu={menu} />
    </article>
  )
}
