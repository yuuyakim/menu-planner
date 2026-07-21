import { useQuery } from '@tanstack/react-query'

import type { Ingredient, IngredientCategory } from '../../api/types'
import { categoryLabels, categoryOrder } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotStatus } from '../../components/MascotStatus'
import { fetchIngredients } from './api'

// groupByCategory は食材をカテゴリごとにまとめ、売り場を回る順に並べる。
//
// サーバも同じ順で返すが、**並び順をサーバの実装に依存させない**ために
// ここでも順序を持つ。該当の無いカテゴリは見出しごと出さない。
function groupByCategory(
  items: Ingredient[],
): { category: IngredientCategory; items: Ingredient[] }[] {
  return categoryOrder
    .map((category) => ({
      category,
      items: items.filter((i) => i.category === category),
    }))
    .filter((group) => group.items.length > 0)
}

// IngredientList は献立に必要な食材を表示する。
export function IngredientList({ menuId }: { menuId: string }) {
  const {
    data: ingredients,
    isPending,
    error,
  } = useQuery({
    queryKey: ['ingredients', menuId],
    queryFn: () => fetchIngredients(menuId),
  })

  if (isPending) return <MascotStatus>食材を確かめています…</MascotStatus>
  if (error) return <ErrorMessage error={error} />

  return (
    <section className="space-y-3">
      <h2 className="text-lg font-bold text-kon-ink">必要な食材</h2>

      {ingredients.length === 0 ? (
        <p className="text-kon-ink/70">この献立の食材はまだ登録されていません。</p>
      ) : (
        <div className="space-y-3">
          {groupByCategory(ingredients).map((group) => (
            <div key={group.category}>
              <h3 className="text-sm font-medium text-kon-ink/60">
                {categoryLabels[group.category]}
              </h3>
              <ul className="mt-1 flex flex-wrap gap-2">
                {group.items.map((i) => (
                  <li
                    key={i.id}
                    className="rounded-full bg-kon-cream px-3 py-1 text-sm text-kon-ink"
                  >
                    {i.name}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}

      {/*
        代表的な食材の例であって、特定レシピの正確な材料表ではない（spec.md 14.1）。
        アレルギー対応と誤解されると実際に危害が及びうるため、必ず添える。
      */}
      <p className="text-xs text-kon-ink/60">
        調味料は含みません。実際の材料はレシピ元でご確認ください。
      </p>
    </section>
  )
}
