import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { ErrorMessage } from '../../components/ErrorMessage'
import { historiesQueryKey } from '../history/api'
import { suggestMenu } from './api'
import { MenuCard } from './MenuCard'
import { RecipeList } from './RecipeList'
import { SearchForm, type MenuFilter } from './SearchForm'

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
      <h1 className="text-2xl font-bold">献立を探す</h1>

      <SearchForm onSubmit={run} isPending={isPending} />

      {isPending && (
        // role=status で、結果を待っていることを支援技術にも伝える。
        <p role="status" className="text-slate-600">
          検索中…
        </p>
      )}

      {error && <ErrorMessage error={error} />}

      {/* 失敗したときに古い結果を残すと、それが今の結果に見える。 */}
      {menu && !isPending && !error && (
        <div className="space-y-4">
          <MenuCard menu={menu} />
          <button
            type="button"
            onClick={() => filter && run(filter)}
            className="rounded-lg border border-slate-300 bg-white px-5 py-2 font-medium text-slate-700 hover:bg-slate-50"
          >
            別の献立を見る
          </button>

          <section className="space-y-3 pt-2">
            <h2 className="font-bold">レシピ</h2>
            {/* レシピの失敗はこの中で閉じる。献立の表示は道連れにしない。 */}
            <RecipeList menuId={menu.id} />
          </section>
        </div>
      )}
    </section>
  )
}
