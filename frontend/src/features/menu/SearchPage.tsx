import { useState } from 'react'

import { SearchForm, type MenuFilter } from './SearchForm'

// SearchPage は献立検索の画面。
// 検索の実行と結果の表示は 8-E で入れる。今は条件を保持するところまで。
export function SearchPage() {
  const [, setFilter] = useState<MenuFilter>({})

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold">献立を探す</h1>
      <SearchForm onSubmit={setFilter} />
    </section>
  )
}
