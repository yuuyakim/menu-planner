import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'

import type { DayMenu, IngredientCategory, ShoppingItem } from '../../api/types'
import { categoryLabels, categoryOrder } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotEmpty } from '../../components/MascotEmpty'
import { MascotStatus } from '../../components/MascotStatus'
import { useSessionState } from '../../hooks/useSessionState'
import { fetchShoppingList } from './api'

// weekKey は WeeklyPage が週間献立を置いている場所と同じキー。
// 買い物リストは「いま画面に出ている週」に対して作るため、そこを読む。
const weekKey = 'weekly.week'

// groupByCategory は買い物リストをカテゴリごとにまとめ、売り場を回る順に並べる。
// サーバも同じ順で返すが、並び順をサーバの実装に依存させないためここでも持つ。
function groupByCategory(
  items: ShoppingItem[],
): { category: IngredientCategory; items: ShoppingItem[] }[] {
  return categoryOrder
    .map((category) => ({
      category,
      items: items.filter((i) => i.ingredient.category === category),
    }))
    .filter((group) => group.items.length > 0)
}

// ShoppingListPage は週間献立から買い物リストを作る画面。
export function ShoppingListPage() {
  // 週間献立は WeeklyPage が sessionStorage に持っている。
  // サーバに保存していないため、ここから読んで献立IDを送る。
  const [week] = useSessionState<DayMenu[] | null>(weekKey, null)
  const menuIds = week?.map((d) => d.menu.id) ?? []

  const {
    data: items,
    isPending,
    error,
  } = useQuery({
    queryKey: ['shopping-list', menuIds],
    queryFn: () => fetchShoppingList(menuIds),
    // 週間献立が無ければ問い合わせない（0件はサーバが400にする）。
    enabled: menuIds.length > 0,
  })

  if (menuIds.length === 0) {
    return (
      <section className="space-y-4">
        <h1 className="text-2xl font-bold text-kon-ink">買い物リスト</h1>
        <MascotEmpty>
          まだ献立が決まっていません。先に「1週間の献立」を作ると、
          必要な食材をまとめて出せます。
        </MascotEmpty>
        <Link
          to="/weekly"
          className="inline-block rounded-full bg-kon-leaf px-5 py-2 font-medium text-white hover:bg-kon-leaf/90"
        >
          1週間の献立を作る
        </Link>
      </section>
    )
  }

  if (isPending) return <MascotStatus>買うものを数えています…</MascotStatus>
  if (error) return <ErrorMessage error={error} />

  return (
    <section className="space-y-5">
      <div>
        <h1 className="text-2xl font-bold text-kon-ink">買い物リスト</h1>
        <p className="mt-1 text-sm text-kon-ink/70">
          1週間の献立 {menuIds.length}日分に必要な食材です。
        </p>
      </div>

      {items.length === 0 ? (
        <MascotEmpty>この献立には食材が登録されていません。</MascotEmpty>
      ) : (
        <div className="space-y-4">
          {groupByCategory(items).map((group) => (
            <div key={group.category} className="space-y-2">
              <h2 className="text-sm font-medium text-kon-ink/60">
                {categoryLabels[group.category]}
              </h2>
              <ul className="space-y-1">
                {group.items.map((it) => (
                  <li
                    key={it.ingredient.id}
                    className="rounded-xl border border-kon-leaf-soft bg-white px-4 py-2"
                  >
                    <span className="font-medium text-kon-ink">
                      {it.ingredient.name}
                    </span>
                    {/*
                      分量を持たない設計（spec.md 14.2）の補償。
                      どの献立で使うかが分かれば、必要量は利用者が判断できる。
                    */}
                    <span className="ml-2 text-sm text-kon-ink/60">
                      {it.usedIn.map((m) => m.name).join('、')}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}

      <p className="text-xs text-kon-ink/60">
        調味料は含みません。実際の材料はレシピ元でご確認ください。
      </p>
    </section>
  )
}
