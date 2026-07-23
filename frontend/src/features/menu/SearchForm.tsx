import { useId, useState } from 'react'

import {
  defaultRoleFilter,
  difficultyLabels,
  genreLabels,
  roleFilterLabels,
  type Difficulty,
  type Genre,
  type RoleFilter,
} from '../../api/types'
import type { MenuFilter } from './filter'

// allValue は「すべて」を表すラジオの value。
// ラジオの value は文字列しか持てないため、undefined の代わりに使う。
const allValue = ''

type RadioGroupProps<T extends string> = {
  legend: string
  /** 選択肢を並び順どおりに [value, ラベル] で渡す。 */
  options: readonly (readonly [T, string])[]
  selected: T
  onChange: (value: T) => void
}

// RadioGroup はラジオ1組。**選択肢と並び順は呼び出し側が決める。**
// ジャンル・難易度は「すべて」が先頭で既定だが、役割は「主菜」が既定で
// 「すべて」は末尾に来る（spec.md 2.10）。ここで「すべて」を足すと
// その違いを表せない。
//
// fieldset/legend にするのは、同じ「すべて」が複数の組にあるため、
// どちらの「すべて」かを支援技術（とテスト）が区別できるようにするため。
function RadioGroup<T extends string>({
  legend,
  options,
  selected,
  onChange,
}: RadioGroupProps<T>) {
  // 同じ画面に複数のラジオ組が並ぶため、name を一意にする。
  const name = useId()

  return (
    <fieldset>
      <legend className="mb-2 font-medium text-kon-ink">{legend}</legend>
      <div className="flex flex-wrap gap-2">
        {options.map(([value, label]) => (
          <label
            key={value}
            className="cursor-pointer rounded-full border border-kon-leaf-soft bg-white px-4 py-1.5 text-sm text-kon-ink/80 transition-colors hover:bg-kon-cream has-checked:border-kon-leaf has-checked:bg-kon-leaf/20 has-checked:font-medium has-checked:text-kon-ink"
          >
            <input
              type="radio"
              name={name}
              value={value}
              checked={selected === value}
              onChange={() => onChange(value)}
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

// allFirst は「すべて」を先頭に置いた選択肢を作る。
// ジャンル・難易度は未指定＝すべてなので、既定である「すべて」が先頭に来る。
// 値の型に空文字（＝すべて）が混ざるため、戻り値は T | '' になる。
function allFirst<T extends string>(
  labels: Record<T, string>,
): readonly (readonly [T | typeof allValue, string])[] {
  return [
    [allValue, 'すべて'],
    ...(Object.entries(labels) as [T, string][]),
  ]
}

// asOptions はラベルの定義順をそのまま選択肢の並びにする。
// 役割は「すべて」が末尾で、先頭の「主菜」が既定であることが並びから読み取れる。
function asOptions<T extends string>(
  labels: Record<T, string>,
): readonly (readonly [T, string])[] {
  return Object.entries(labels) as [T, string][]
}

type Props = {
  onSubmit: (filter: MenuFilter) => void
  /** 検索中はボタンを無効にして二重送信を防ぐ。 */
  isPending?: boolean
  /** 送信ボタンの文言。週間献立では「1週間分を作る」になる。 */
  submitLabel?: string
}

// SearchForm は検索条件を選んで送るフォーム。
// 検索の実行そのものは持たず、条件を親に渡すだけにする
// （結果の表示とAPI呼び出しは 8-E の SearchPage が持つ）。
export function SearchForm({
  onSubmit,
  isPending = false,
  submitLabel = '献立を探す',
}: Props) {
  const [genre, setGenre] = useState<Genre | undefined>(undefined)
  const [difficulty, setDifficulty] = useState<Difficulty | undefined>(undefined)
  const [role, setRole] = useState<RoleFilter>(defaultRoleFilter)

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit({ genre, difficulty, role })
      }}
    >
      {/* 「種類」は役割を出すが、legend は利用者の言葉に寄せる。
          主菜/副菜/汁物 のどれを探すかを選ぶ、という意味。 */}
      <RadioGroup
        legend="種類"
        options={asOptions(roleFilterLabels)}
        selected={role}
        onChange={setRole}
      />
      <RadioGroup
        legend="ジャンル"
        options={allFirst(genreLabels)}
        selected={genre ?? allValue}
        onChange={(v) => setGenre(v === allValue ? undefined : v)}
      />
      <RadioGroup
        legend="難易度"
        options={allFirst(difficultyLabels)}
        selected={difficulty ?? allValue}
        onChange={(v) => setDifficulty(v === allValue ? undefined : v)}
      />

      <button
        type="submit"
        disabled={isPending}
        // 無効時に白文字のままだと淡い緑に埋もれて読めない。文字色も落とす。
        className="rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95 disabled:cursor-not-allowed disabled:bg-kon-leaf-soft disabled:text-kon-ink/70"
      >
        {isPending ? '検索中…' : submitLabel}
      </button>
    </form>
  )
}
