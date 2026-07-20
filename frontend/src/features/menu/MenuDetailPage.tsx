import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router'

import { ErrorMessage } from '../../components/ErrorMessage'
import { fetchMenu } from './api'
import { MenuCard } from './MenuCard'
import { RecipeList } from './RecipeList'

// MenuDetailPage は献立1件とそのレシピを表示する。
// 週間献立・履歴・お気に入りからの遷移先。
export function MenuDetailPage() {
  const { id } = useParams<{ id: string }>()

  const {
    data: menu,
    isPending,
    error,
  } = useQuery({
    queryKey: ['menu', id],
    queryFn: () => fetchMenu(id!),
    // ルート定義上 id は必ずあるが、型の上では undefined を取りうる。
    enabled: Boolean(id),
  })

  return (
    <section className="space-y-6">
      <Link to="/" className="text-sm text-slate-600 hover:text-slate-900">
        ← 献立を探す
      </Link>

      {isPending && (
        <p role="status" className="text-slate-600">
          読み込み中…
        </p>
      )}

      {error && <ErrorMessage error={error} />}

      {menu && (
        <>
          <MenuCard menu={menu} />
          <section className="space-y-3">
            <h2 className="font-bold">レシピ</h2>
            <RecipeList menuId={menu.id} />
          </section>
        </>
      )}
    </section>
  )
}
