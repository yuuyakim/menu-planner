import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router'

import { difficultyLabels, genreLabels } from '../../api/types'
import type { MenuMatch } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotEmpty } from '../../components/MascotEmpty'
import { MascotStatus } from '../../components/MascotStatus'
import type { MatchSort, SearchByIngredientsResult } from './api'
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

  const [onlyMakeable, setOnlyMakeable] = useState(false)
  const [sort, setSort] = useState<MatchSort>('missing_asc')

  const search = useMutation({
    mutationFn: () => searchByIngredients([...selected], { onlyMakeable, sort }),
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

  // つまみを変えたら前の結果は消す。食材を選び直したときと同じ理由で、
  // 残したままだと、いまの条件の結果だと誤解させる。
  function changeOnlyMakeable(next: boolean) {
    setOnlyMakeable(next)
    search.reset()
  }

  function changeSort(next: MatchSort) {
    setSort(next)
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

          {/* 各組を fieldset/legend で囲む。囲まないと支援技術もテストも
              どちらの組の選択肢か区別できない（8-D と同じ）。 */}
          <fieldset className="space-y-2">
            <legend className="text-sm text-kon-ink/60">探し方</legend>
            <label className="flex items-center gap-2 text-sm text-kon-ink">
              <input
                type="radio"
                name="makeable"
                checked={!onlyMakeable}
                onChange={() => changeOnlyMakeable(false)}
              />
              足りないものがあってもよい
            </label>
            <label className="flex items-center gap-2 text-sm text-kon-ink">
              <input
                type="radio"
                name="makeable"
                checked={onlyMakeable}
                onChange={() => changeOnlyMakeable(true)}
              />
              この中だけで作れるもの
            </label>
          </fieldset>

          {/* 「作れるものだけ」は不足が全件0になるため、並び順は結果を変えない。
              出しても選ばせる意味がないので隠す。サーバ側では特別扱いしない。 */}
          {!onlyMakeable && (
            <fieldset className="space-y-2">
              <legend className="text-sm text-kon-ink/60">並び順</legend>
              <label className="flex items-center gap-2 text-sm text-kon-ink">
                <input
                  type="radio"
                  name="sort"
                  checked={sort === 'missing_asc'}
                  onChange={() => changeSort('missing_asc')}
                />
                買い足しが少ない順
              </label>
              <label className="flex items-center gap-2 text-sm text-kon-ink">
                <input
                  type="radio"
                  name="sort"
                  checked={sort === 'matched_desc'}
                  onChange={() => changeSort('matched_desc')}
                />
                手持ちを多く使う順
              </label>
            </fieldset>
          )}

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

      {search.data && <Results result={search.data} onlyMakeable={onlyMakeable} />}
    </section>
  )
}

// Results は検索結果。0件のときの文言が探し方によって変わる。
function Results({
  result,
  onlyMakeable,
}: {
  result: SearchByIngredientsResult
  onlyMakeable: boolean
}) {
  if (result.matches.length > 0) {
    return <Matches matches={result.matches} />
  }

  return (
    <div className="space-y-6">
      <MascotEmpty image="/mascot/face-thinking.png">
        {onlyMakeable
          ? 'この中だけで作れる献立はありませんでした。食材をもう少し選ぶと見つかりやすくなります。'
          : 'その食材で作れる献立が見つかりませんでした。食材を増やすと見つかりやすくなります。'}
      </MascotEmpty>

      {/* 見出しを分けて、絞り込みの結果ではないことを見た目で区別する。
          地続きに見せると「作れるものだけ」の約束を破ったように見える。 */}
      {result.nearMisses.length > 0 && (
        <div className="space-y-4">
          <h2 className="text-lg font-bold text-kon-ink">
            あと1品買えば作れます（{result.nearMisses.length}件）
          </h2>
          <MatchList matches={result.nearMisses} />
        </div>
      )}
    </div>
  )
}

// Matches は候補の一覧。見出し・調味料の断り・一覧の3つを並べる。
function Matches({ matches }: { matches: MenuMatch[] }) {
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

      <MatchList matches={matches} />
    </div>
  )
}

// MatchList は献立カードの一覧。Matches（作れる献立）と
// Results の「あと1品」の両方から使う。カード自体はここにしか無い。
function MatchList({ matches }: { matches: MenuMatch[] }) {
  return (
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
  )
}
