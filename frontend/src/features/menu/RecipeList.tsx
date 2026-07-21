import { useQuery } from '@tanstack/react-query'

import { ErrorMessage } from '../../components/ErrorMessage'
import { fetchRecipes } from './api'

// RecipeList は献立のレシピリンクを表示する。
//
// 献立の表示とは別のクエリにしている。レシピは外部の検索APIに依存していて
// 単独で失敗しうるため、ここが失敗しても献立の表示は残る（spec.md 2.4）。
export function RecipeList({ menuId }: { menuId: string }) {
  const {
    data: recipes,
    isPending,
    error,
    refetch,
    isFetching,
  } = useQuery({
    // 献立ごとに結果が決まるのでキーに含める。
    // 献立が変われば自動で取り直される。
    queryKey: ['recipes', menuId],
    queryFn: () => fetchRecipes(menuId),
  })

  if (isPending) {
    return (
      <p role="status" className="text-kon-ink/70">
        レシピを読み込み中…
      </p>
    )
  }

  if (error) {
    return (
      <div className="space-y-3">
        <ErrorMessage error={error} />
        <button
          type="button"
          onClick={() => void refetch()}
          disabled={isFetching}
          className="rounded-full border border-kon-leaf-soft bg-white px-4 py-1.5 text-sm font-medium text-kon-ink hover:bg-kon-cream disabled:cursor-not-allowed disabled:text-kon-ink/40"
        >
          再試行
        </button>
      </div>
    )
  }

  // 0件は障害ではない（spec.md 2.4）。エラー扱いにせず、そう伝える。
  if (recipes.length === 0) {
    return <p className="text-kon-ink/70">レシピが見つかりませんでした。</p>
  }

  return (
    <ul className="space-y-3">
      {recipes.map((r) => (
        <li key={r.url}>
          <a
            href={r.url}
            // 別タブで開き、元の画面（検索結果）を失わせない。
            target="_blank"
            // noopener が無いと開いた先から window.opener でこの画面を操作できる。
            // noreferrer は参照元URLを渡さない。
            rel="noopener noreferrer"
            className="block rounded-2xl border border-kon-leaf-soft bg-white p-4 transition-colors hover:border-kon-leaf hover:bg-kon-cream"
          >
            {/* タイトルが空でもリンクの名前が空にならないようURLで補う。 */}
            <span className="font-medium text-kon-ink underline decoration-kon-leaf decoration-2 underline-offset-2">
              {r.title || r.url}
            </span>
            <span className="mt-1 block text-sm text-kon-ink/55">{r.domain}</span>
            {r.snippet && (
              <span className="mt-1 block text-sm text-kon-ink/75">
                {r.snippet}
              </span>
            )}
          </a>
        </li>
      ))}
    </ul>
  )
}
