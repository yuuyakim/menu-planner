import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotStatus } from '../../components/MascotStatus'
import { historiesQueryKey } from '../history/api'
import { suggestMenu } from './api'
import { IngredientList } from './IngredientList'
import { MenuCard } from './MenuCard'
import { RecipeList } from './RecipeList'
import { SearchForm } from './SearchForm'
import type { MenuFilter } from './filter'

// SearchPage は献立検索の画面。
//
// 検索は useQuery ではなく useMutation で行う。ボタンを押したときだけ走り、
// 同じ条件でも押すたびに新しい結果が欲しい（＝キャッシュしたくない）ため。
// useQuery はキー が同じなら再取得を避けるので「別の献立を見る」と噛み合わない。
export function SearchPage() {
  // 引き直しに使うため、検索したときの条件を覚えておく。
  const [filter, setFilter] = useState<MenuFilter | null>(null)

  const queryClient = useQueryClient()

  const { mutate, data: menu, isPending, error } = useMutation({
    mutationFn: suggestMenu,
    onSuccess: () => {
      // ログイン中の検索はサーバ側で履歴に記録される（4-F）。
      // 無効化しないと、履歴画面が古いキャッシュのままになる。
      void queryClient.invalidateQueries({ queryKey: historiesQueryKey })
    },
  })

  function run(next: MenuFilter) {
    setFilter(next)
    mutate(next)
  }

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold text-kon-ink">献立を探す</h1>

      <SearchForm onSubmit={run} isPending={isPending} />

      {isPending && <MascotStatus>検索中…</MascotStatus>}

      {error && <ErrorMessage error={error} />}

      {/* 失敗したときに古い結果を残すと、それが今の結果に見える。 */}
      {menu && !isPending && !error && (
        <div className="space-y-4">
          <MenuCard menu={menu} />
          <button
            type="button"
            onClick={() => filter && run(filter)}
            className="rounded-full border border-kon-leaf-soft bg-white px-5 py-2 font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream"
          >
            別の献立を見る
          </button>

          {/* 「何を買えばいいか」は結果を見た直後に知りたい。
              献立詳細まで行かないと分からないのでは遠い。 */}
          <IngredientList menuId={menu.id} />

          <section className="space-y-3 pt-2">
            <h2 className="font-bold text-kon-ink">レシピ</h2>
            {/* レシピの失敗はこの中で閉じる。献立の表示は道連れにしない。 */}
            <RecipeList menuId={menu.id} />
          </section>
        </div>
      )}
    </section>
  )
}
