import { difficultyLabels, genreLabels, type Menu } from '../../api/types'
import { FavoriteButton } from '../favorite/FavoriteButton'

type Props = {
  menu: Menu
  /**
   * 献立名の見出しレベル。既定は 2（その画面の主役ではない場合）。
   * 詳細画面ではこの献立が画面そのものなので 1 を渡す。
   * 画面に h1 が無いと、何のページかが見出しから読み取れなくなる。
   */
  headingLevel?: 1 | 2
}

// MenuCard は献立1件の表示。検索結果・週間献立・詳細で共通に使う。
export function MenuCard({ menu, headingLevel = 2 }: Props) {
  const Heading = headingLevel === 1 ? 'h1' : 'h2'

  return (
    // お気に入りは献立の右端に置く。下に積むと、献立の情報が増えていないのに
    // カードの縦幅だけが伸びる。週間献立では7枚分まとめて効いてくる。
    <article className="flex gap-4 rounded-2xl border border-kon-leaf-soft bg-white p-6">
      <div className="min-w-0 flex-1">
        <Heading
          className={
            headingLevel === 1
              ? 'text-2xl font-bold text-kon-ink'
              : 'text-xl font-bold text-kon-ink'
          }
        >
          {menu.name}
        </Heading>
        <div className="mt-2 flex gap-2 text-sm">
          <span className="rounded-full bg-kon-cream px-3 py-0.5 text-kon-ink/80">
            {genreLabels[menu.genre]}
          </span>
          <span className="rounded-full bg-kon-cream px-3 py-0.5 text-kon-ink/80">
            {difficultyLabels[menu.difficulty]}
          </span>
        </div>
        <p className="mt-3 text-kon-ink/75">{menu.description}</p>
      </div>

      {/* 献立が出る場所ならどこでもお気に入りにできる。
          未ログインのときは何も描画されない。 */}
      <FavoriteButton menu={menu} />
    </article>
  )
}
