type Props = {
  value: string
  onChange: (value: string) => void
  onSubmit: () => void
  isPending: boolean
}

// IngredientTextInput は冷蔵庫の中身を自由記述で受け取る（設計 6.1）。
//
// **チェックボックスを置き換えない。** 読み取り結果は IngredientPicker の
// チェック状態として反映され、利用者が目で確認・修正してから検索する。
// これにより確認の場が既存UIで賄え、LLM が落ちても手動チェックで機能が残る。
export function IngredientTextInput({
  value,
  onChange,
  onSubmit,
  isPending,
}: Props) {
  return (
    <div className="space-y-3">
      <label
        htmlFor="ingredient-text"
        className="block font-medium text-kon-ink"
      >
        冷蔵庫にあるものを書く
      </label>
      <textarea
        id="ingredient-text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={3}
        maxLength={200}
        placeholder="豚こま、玉ねぎ、にんじん、卵"
        className="w-full rounded-2xl border border-kon-leaf-soft bg-white p-4 text-kon-ink placeholder:text-kon-ink/40 focus:border-kon-leaf focus:outline-none"
      />
      <button
        type="button"
        onClick={onSubmit}
        // 空のまま押しても 400 になるだけなので押させない。
        disabled={value.trim() === '' || isPending}
        className="rounded-full bg-kon-leaf px-6 py-2.5 font-medium text-white transition-colors hover:brightness-95 disabled:cursor-not-allowed disabled:bg-kon-leaf-soft disabled:text-kon-ink/70"
      >
        {isPending ? '読み取っています…' : '読み取る'}
      </button>
    </div>
  )
}
