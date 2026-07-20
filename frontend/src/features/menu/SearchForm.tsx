import { useId, useState } from 'react'

import {
  difficultyLabels,
  genreLabels,
  type Difficulty,
  type Genre,
} from '../../api/types'

/** MenuFilter は検索の絞り込み条件。undefined は「絞り込まない」。 */
export type MenuFilter = {
  genre?: Genre
  difficulty?: Difficulty
}

// allValue は「すべて」を表すラジオの value。
// ラジオの value は文字列しか持てないため、undefined の代わりに使う。
const allValue = ''

type RadioGroupProps<T extends string> = {
  legend: string
  /** value → 表示ラベル。ここに無い値は選択肢に出ない。 */
  labels: Record<T, string>
  selected: T | undefined
  onChange: (value: T | undefined) => void
}

// RadioGroup は「すべて + 選択肢」のラジオ1組。
// fieldset/legend にするのは、同じ「すべて」が2組あるため、
// どちらの「すべて」かを支援技術（とテスト）が区別できるようにするため。
function RadioGroup<T extends string>({
  legend,
  labels,
  selected,
  onChange,
}: RadioGroupProps<T>) {
  // 同じ画面に複数のラジオ組が並ぶため、name を一意にする。
  const name = useId()
  const options = Object.entries(labels) as [T, string][]

  return (
    <fieldset>
      <legend className="mb-2 font-medium">{legend}</legend>
      <div className="flex flex-wrap gap-2">
        {[[allValue, 'すべて'] as const, ...options].map(([value, label]) => (
          <label
            key={value}
            className="cursor-pointer rounded-full border border-slate-300 bg-white px-4 py-1.5 text-sm has-checked:border-emerald-600 has-checked:bg-emerald-50 has-checked:text-emerald-800"
          >
            <input
              type="radio"
              name={name}
              value={value}
              checked={(selected ?? allValue) === value}
              onChange={() =>
                onChange(value === allValue ? undefined : (value as T))
              }
              // 見た目は label 側の has-checked で作るが、
              // キーボード操作とスクリーンリーダーのために input 自体は残す。
              className="sr-only"
            />
            {label}
          </label>
        ))}
      </div>
    </fieldset>
  )
}

type Props = {
  onSubmit: (filter: MenuFilter) => void
  /** 検索中はボタンを無効にして二重送信を防ぐ。 */
  isPending?: boolean
}

// SearchForm は検索条件を選んで送るフォーム。
// 検索の実行そのものは持たず、条件を親に渡すだけにする
// （結果の表示とAPI呼び出しは 8-E の SearchPage が持つ）。
export function SearchForm({ onSubmit, isPending = false }: Props) {
  const [genre, setGenre] = useState<Genre | undefined>(undefined)
  const [difficulty, setDifficulty] = useState<Difficulty | undefined>(undefined)

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit({ genre, difficulty })
      }}
    >
      <RadioGroup
        legend="ジャンル"
        labels={genreLabels}
        selected={genre}
        onChange={setGenre}
      />
      <RadioGroup
        legend="難易度"
        labels={difficultyLabels}
        selected={difficulty}
        onChange={setDifficulty}
      />

      <button
        type="submit"
        disabled={isPending}
        className="rounded-lg bg-emerald-600 px-6 py-2.5 font-medium text-white hover:bg-emerald-700 disabled:cursor-not-allowed disabled:bg-slate-300"
      >
        {isPending ? '検索中…' : '献立を探す'}
      </button>
    </form>
  )
}
