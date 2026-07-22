import { useMemo } from 'react'

import type { Ingredient, IngredientCategory } from '../../api/types'
import { categoryLabels, categoryOrder } from '../../api/types'

type Props = {
  /** 選択肢。サーバがカテゴリ順→カナ順で返す。 */
  ingredients: Ingredient[]
  /** 選択中の食材ID。 */
  selected: Set<string>
  onToggle: (id: string) => void
  onClear: () => void
}

// groupByCategory は食材をカテゴリごとにまとめる。
// 買い物リストと同じ並び（売り場を回る順）に揃える。
//
// サーバも同じ順で返すが、並び順をサーバの実装に依存させないためここでも持つ
// （ShoppingListPage と同じ方針）。
function groupByCategory(
  ingredients: Ingredient[],
): { category: IngredientCategory; items: Ingredient[] }[] {
  return categoryOrder
    .map((category) => ({
      category,
      items: ingredients.filter((i) => i.category === category),
    }))
    .filter((group) => group.items.length > 0)
}

// IngredientPicker は手持ちの食材をカテゴリ別に選ぶ（spec.md 2.9）。
//
// 自由入力にしないのは、食材が自前マスタで表記揺れ（にんじん／人参）を
// こちらで吸収する必要が出るため。一覧から選ぶ形なら、選べるものが
// そのまま探せるものになる。
export function IngredientPicker({
  ingredients,
  selected,
  onToggle,
  onClear,
}: Props) {
  const groups = useMemo(() => groupByCategory(ingredients), [ingredients])

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-kon-ink/70" role="status">
          {selected.size === 0
            ? '冷蔵庫にあるものを選んでください'
            : `${selected.size}個を選択中`}
        </p>
        {selected.size > 0 && (
          <button
            type="button"
            onClick={onClear}
            className="shrink-0 text-sm text-kon-ink/60 underline decoration-kon-leaf decoration-2 underline-offset-2 hover:text-kon-ink"
          >
            選択をすべて外す
          </button>
        )}
      </div>

      {groups.map((group) => (
        // fieldset/legend にするのは、どのカテゴリの選択肢かを
        // 支援技術（とテスト）が区別できるようにするため。
        <fieldset key={group.category}>
          <legend className="mb-2 font-medium text-kon-ink">
            {categoryLabels[group.category]}
          </legend>
          <div className="flex flex-wrap gap-2">
            {group.items.map((item) => (
              <label
                key={item.id}
                className="cursor-pointer rounded-full border border-kon-leaf-soft bg-white px-4 py-1.5 text-sm text-kon-ink/80 transition-colors hover:bg-kon-cream has-checked:border-kon-leaf has-checked:bg-kon-leaf/20 has-checked:font-medium has-checked:text-kon-ink"
              >
                <input
                  type="checkbox"
                  checked={selected.has(item.id)}
                  onChange={() => onToggle(item.id)}
                  // 見た目は label 側の has-checked で作るが、
                  // キーボード操作と読み上げのために input 自体は残す
                  // （SearchForm のラジオと同じ作り）。
                  className="sr-only"
                />
                {item.name}
              </label>
            ))}
          </div>
        </fieldset>
      ))}
    </div>
  )
}
