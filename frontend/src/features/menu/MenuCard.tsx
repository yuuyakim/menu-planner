import {
  difficultyLabels,
  genreLabels,
  type Menu,
} from '../../api/types'

// MenuCard は献立1件の表示。検索結果・週間献立・履歴・お気に入りで共通に使う。
export function MenuCard({ menu }: { menu: Menu }) {
  return (
    <article className="rounded-xl border border-slate-200 bg-white p-6">
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
    </article>
  )
}
