import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router'

import { difficultyLabels, genreLabels } from '../../api/types'
import type { MenuMatch } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotEmpty } from '../../components/MascotEmpty'
import { MascotStatus } from '../../components/MascotStatus'
import { fetchAllIngredients, ingredientsQueryKey, searchByIngredients } from './api'
import { IngredientPicker } from './IngredientPicker'

// SearchByIngredientsPage は冷蔵庫にあるもので作れる献立を探す画面（spec.md 2.9）。
//
// **自由記述での読み取りを暫定的に外している（2026-08-05）。** Anthropic の
// APIキーを取得できておらず、INGREDIENT_RESOLVER_PROVIDER=stub では何を入れても
// 「登録がありませんでした」と返る。出したままだと、利用者に「自分の食材が
// マスタに無い」と誤って伝えることになるため、入力欄ごと隠す。
// IngredientTextInput / ResolveResultPanel / resolveIngredients は消していない。
// キーを取得したらこのコミットを revert すれば、テストごと元に戻る。
export function SearchByIngredientsPage() {
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const {
    data: ingredients,
    isPending,
    error: loadError,
  } = useQuery({
    queryKey: ingredientsQueryKey,
    queryFn: fetchAllIngredients,
    // 食材マスタは固定的で、画面を開くたびに取り直す意味がない。
    staleTime: 60 * 60 * 1000,
  })

  const search = useMutation({
    mutationFn: () => searchByIngredients([...selected]),
  })

  function toggle(id: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
    // 選び直したら前の結果は消す。残したままだと、いま選んでいる食材の
    // 結果だと誤解させる。
    search.reset()
  }

  function clear() {
    setSelected(new Set())
    search.reset()
  }

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold text-kon-ink">冷蔵庫から探す</h1>

      {isPending && <MascotStatus>読み込み中…</MascotStatus>}
      {loadError && <ErrorMessage error={loadError} />}

      {ingredients && (
        <>
          <p className="text-sm text-kon-ink/60">使える食材を選ぶ</p>

          <IngredientPicker
            ingredients={ingredients}
            selected={selected}
            onToggle={toggle}
            onClear={clear}
          />

          <button
            type="button"
            onClick={() => search.mutate()}
            // 1つも選んでいなければ探せない。押しても 400 になるだけなので押させない。
            disabled={selected.size === 0 || search.isPending}
            className="rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95 disabled:cursor-not-allowed disabled:bg-kon-leaf-soft disabled:text-kon-ink/70"
          >
            {search.isPending ? '探しています…' : 'この食材で探す'}
          </button>
        </>
      )}

      {search.error && <ErrorMessage error={search.error} />}

      {search.data && <Matches matches={search.data} />}
    </section>
  )
}

// Matches は候補の一覧。
function Matches({ matches }: { matches: MenuMatch[] }) {
  if (matches.length === 0) {
    return (
      <MascotEmpty image="/mascot/face-thinking.png">
        その食材で作れる献立が見つかりませんでした。食材を増やすと見つかりやすくなります。
      </MascotEmpty>
    )
  }

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-bold text-kon-ink">
        作れそうな献立（{matches.length}件）
      </h2>

      {/* 「不足0」でも調味料などは要る。食材リストは代表例であって
          正確な材料表ではない（spec.md 14.1）。ここで断らないと
          「これだけ買えば作れる」と受け取られる。 */}
      <p className="rounded-2xl bg-kon-cream px-5 py-3 text-sm text-kon-ink/75">
        食材は代表的なものの例です。調味料は含みません。実際の材料はレシピ元で確認してください。
      </p>

      <ul className="space-y-3">
        {matches.map((m) => (
          <li
            key={m.menu.id}
            aria-label={m.menu.name}
            className="rounded-2xl border border-kon-leaf-soft bg-white p-4"
          >
            <p className="font-medium text-kon-ink">{m.menu.name}</p>
            <p className="mt-1 flex flex-wrap gap-2 text-sm text-kon-ink/60">
              <span className="rounded-full bg-kon-cream px-2 py-0.5">
                {genreLabels[m.menu.genre]}
              </span>
              <span className="rounded-full bg-kon-cream px-2 py-0.5">
                {difficultyLabels[m.menu.difficulty]}
              </span>
            </p>

            <p className="mt-3 text-sm text-kon-ink/75">
              使える食材: {m.matched.map((i) => i.name).join('・')}
            </p>
            {/* 不足を出すのがこの機能の要。「あと何を買えばよいか」が
                買い物の判断に直接効く。 */}
            <p className="mt-1 text-sm font-medium text-kon-ink">
              {m.missing.length === 0
                ? '足りない食材はありません'
                : `あと${m.missing.length}品: ${m.missing.map((i) => i.name).join('・')}`}
            </p>

            <Link
              to={`/menus/${m.menu.id}`}
              className="mt-3 inline-block rounded-full border border-kon-leaf-soft bg-white px-4 py-1.5 text-sm font-medium text-kon-ink transition-colors hover:border-kon-leaf hover:bg-kon-cream"
            >
              レシピを見る
            </Link>
          </li>
        ))}
      </ul>
    </div>
  )
}
