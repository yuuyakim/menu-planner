type Props = {
  unresolved: string[]
  degraded: boolean
}

// ResolveResultPanel は読み取りの結果のうち、チェックに現れないものを伝える。
//
// **ピッカーの上に置く。** IngredientPicker は max-h-[55vh] のスクロール領域を
// 持つため、入ったチェックが画面外になりうる。変化に気付ける位置に出す（設計 6.2）。
export function ResolveResultPanel({ unresolved, degraded }: Props) {
  if (unresolved.length === 0 && !degraded) return null

  return (
    // aria-label を付けるのは、IngredientPicker の選択数表示も role="status" を
    // 持つため。名前が無いと支援技術（とテスト）がどちらの通知か区別できない。
    <div className="space-y-2" role="status" aria-label="読み取りの結果">
      {degraded && (
        <p className="rounded-2xl bg-kon-cream px-5 py-3 text-sm text-kon-ink/80">
          一部だけ読み取れました。残りは下から選んでください。
        </p>
      )}
      {unresolved.length > 0 && (
        <p className="rounded-2xl bg-kon-cream px-5 py-3 text-sm text-kon-ink/80">
          登録がありませんでした: {unresolved.join('・')}
          <span className="mt-1 block text-kon-ink/60">
            この{unresolved.length}件は検索に使われません。
          </span>
        </p>
      )}
    </div>
  )
}
