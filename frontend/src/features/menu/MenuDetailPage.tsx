import { useQuery } from '@tanstack/react-query'
import { Link, useLocation, useNavigate, useParams } from 'react-router'

import { ErrorMessage } from '../../components/ErrorMessage'
import { fetchMenu } from './api'
import { MenuCard } from './MenuCard'
import { RecipeList } from './RecipeList'

// MenuDetailPage は献立1件とそのレシピを表示する。
// 週間献立・履歴・お気に入りからの遷移先。
export function MenuDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const location = useLocation()

  // この画面に至る履歴があるかどうか。URLを直接開いた場合は 'default' になる。
  // 履歴が無いのに戻ると、アプリの外へ出てしまう。
  const canGoBack = location.key !== 'default'

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
      {/* 戻り先を '/' に固定すると、週間献立から来ても検索画面に着いてしまう。 */}
      {canGoBack ? (
        <button
          type="button"
          onClick={() => void navigate(-1)}
          className="text-sm text-slate-600 hover:text-slate-900"
        >
          ← 戻る
        </button>
      ) : (
        <Link to="/" className="text-sm text-slate-600 hover:text-slate-900">
          ← 献立を探す
        </Link>
      )}

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
