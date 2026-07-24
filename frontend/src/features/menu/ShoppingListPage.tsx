import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Link } from 'react-router'

import type {
  DayMenu,
  IngredientCategory,
  Origin,
  ShoppingListOverride,
} from '../../api/types'
import { categoryLabels, categoryOrder } from '../../api/types'
import { ErrorMessage } from '../../components/ErrorMessage'
import { MascotEmpty } from '../../components/MascotEmpty'
import { MascotStatus } from '../../components/MascotStatus'
import { useSessionState } from '../../hooks/useSessionState'
import { useCurrentUser } from '../auth/useCurrentUser'
import {
  fetchSavedShoppingList,
  fetchShoppingList,
  saveShoppingListOverrides,
  savedShoppingListQueryKey,
} from './api'

// weekKey/savedIdKey は WeeklyPage / SavedWeeklyPage が同じ場所に置いている値と
// 同じキー。買い物リストは「いま画面に出ている週」に対して作る。
const weekKey = 'weekly.week'
const savedIdKey = 'weekly.savedId'

// ViewItem は POST（未保存の週・ステートレス）と GET（保存済みの週・差分適用後）
// のどちらから来た項目でも同じ形で描画するための正規化後の形。
// サーバから来る形が違っても、表示側はこれだけを見ればよい。
type ViewItem = {
  key: string
  name: string
  category: IngredientCategory
  usedIn: { id: string; name: string }[]
  checked: boolean
  origin: Origin
}

// groupByCategory は買い物リストをカテゴリごとにまとめ、売り場を回る順に並べる。
// サーバも同じ順で返すが、並び順をサーバの実装に依存させないためここでも持つ。
function groupByCategory(
  items: ViewItem[],
): { category: IngredientCategory; items: ViewItem[] }[] {
  return categoryOrder
    .map((category) => ({
      category,
      items: items.filter((i) => i.category === category),
    }))
    .filter((group) => group.items.length > 0)
}

// ShoppingListPage は週間献立から買い物リストを作る画面。
//
// 保存済みの週（savedId あり）は GET /weekly-menus/:id/shopping-list を使う。
// サーバ側にチェック済み・手動追加の差分を持てるため（Task 11 以降のチェックUI）、
// 開くたびに差分適用後の状態を取り直す。
// 未保存の週はサーバに状態が無いので、従来どおり献立IDから毎回組み立てる
// （POST /shopping-list、ステートレス）。
export function ShoppingListPage() {
  // 週間献立は WeeklyPage が sessionStorage に持っている。
  // サーバに保存していないため、ここから読んで献立IDを送る。
  const [week] = useSessionState<DayMenu[] | null>(weekKey, null)
  const [savedId] = useSessionState<string | null>(savedIdKey, null)
  const menuIds = week?.map((d) => d.menu.id) ?? []

  const saved = useQuery({
    queryKey: savedShoppingListQueryKey(savedId ?? ''),
    queryFn: () => fetchSavedShoppingList(savedId as string),
    enabled: savedId != null,
  })

  const derived = useQuery({
    queryKey: ['shopping-list', menuIds],
    queryFn: () => fetchShoppingList(menuIds),
    // 週間献立が無ければ問い合わせない（0件はサーバが400にする）。
    // 保存済みの週を開いているときは GET 側だけを使う。
    enabled: savedId == null && menuIds.length > 0,
  })

  const active = savedId != null ? saved : derived

  const items: ViewItem[] =
    savedId != null
      ? (saved.data ?? []).map((it) => ({
          key: it.name,
          name: it.name,
          category: it.category,
          usedIn: it.usedIn,
          checked: it.checked,
          origin: it.origin,
        }))
      : (derived.data ?? []).map((it) => ({
          key: it.ingredient.id,
          name: it.ingredient.name,
          category: it.ingredient.category,
          usedIn: it.usedIn,
          checked: false,
          origin: 'derived' as const,
        }))

  // 保存済みの週かつ premium のときだけ、チェックはサーバに残る。
  // それ以外（未保存の週／free）はその場限りで、画面を離れると消える。
  const { user } = useCurrentUser()
  const canPersist = savedId != null && user?.plan === 'premium'
  const queryClient = useQueryClient()

  // チェック状態はローカルで持つ。全品目に常時チェックボックスを出す
  // という Task 11 の要件のため、未保存の週（サーバ側に checked が無い）
  // でも状態を持てるようにローカルが正になる。
  const [checked, setChecked] = useState<Set<string>>(new Set())
  useEffect(() => {
    setChecked(new Set(items.filter((it) => it.checked).map((it) => it.key)))
    // items ではなく取得結果そのもの（saved.data / derived.data）を依存にする。
    // items は毎レンダー新しい配列を作るため、それを依存にすると
    // チェック操作のたびにローカル state がサーバの値へ巻き戻ってしまう。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saved.data, derived.data])

  const persist = useMutation({
    mutationFn: (next: Set<string>) => {
      // PUT は overlay 全体の一括置換（部分更新のAPIを持たない）。
      // 手動品目・非表示は Task 13 で overlay に加える。
      const overrides: ShoppingListOverride[] = items
        .filter((it) => next.has(it.key))
        .map((it) => ({
          name: it.name,
          category: it.category,
          origin: 'derived',
          checked: true,
          hidden: false,
        }))
      return saveShoppingListOverrides(savedId as string, overrides)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: savedShoppingListQueryKey(savedId as string),
      })
    },
  })

  function toggle(key: string) {
    setChecked((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      if (canPersist) persist.mutate(next)
      return next
    })
  }

  if (menuIds.length === 0) {
    return (
      <section className="space-y-4">
        <h1 className="text-2xl font-bold text-kon-ink">買い物リスト</h1>
        <MascotEmpty>
          まだ献立が決まっていません。先に「1週間の献立」を作ると、
          必要な食材をまとめて出せます。
        </MascotEmpty>
        <Link
          to="/weekly"
          className="inline-block rounded-full bg-kon-leaf px-5 py-2 font-medium text-white hover:bg-kon-leaf/90"
        >
          1週間の献立を作る
        </Link>
      </section>
    )
  }

  if (active.isPending) return <MascotStatus>買うものを数えています…</MascotStatus>
  if (active.error) return <ErrorMessage error={active.error} />

  return (
    <section className="space-y-5">
      <div>
        <h1 className="text-2xl font-bold text-kon-ink">買い物リスト</h1>
        <p className="mt-1 text-sm text-kon-ink/70">
          1週間の献立 {menuIds.length}日分に必要な食材です。
        </p>
      </div>

      {items.length === 0 ? (
        <MascotEmpty>この献立には食材が登録されていません。</MascotEmpty>
      ) : (
        <div className="space-y-4">
          {groupByCategory(items).map((group) => (
            <div key={group.category} className="space-y-2">
              <h2 className="text-sm font-medium text-kon-ink/60">
                {categoryLabels[group.category]}
              </h2>
              <ul className="space-y-1">
                {group.items.map((it) => (
                  <li
                    key={it.key}
                    className="rounded-xl border border-kon-leaf-soft bg-white px-4 py-2"
                  >
                    <label className="flex items-center gap-2">
                      {/*
                        premium×保存済み以外はその場限り（画面を離れると消える）。
                        それでもチェックボックス自体は常に出す。買い物中は
                        premium かどうかを気にせず使える方が自然なため。
                      */}
                      <input
                        type="checkbox"
                        checked={checked.has(it.key)}
                        onChange={() => toggle(it.key)}
                        aria-label={it.name}
                      />
                      <span
                        className={
                          checked.has(it.key)
                            ? 'font-medium text-kon-ink/50 line-through'
                            : 'font-medium text-kon-ink'
                        }
                      >
                        {it.name}
                      </span>
                    </label>
                    {/*
                      分量を持たない設計（spec.md 14.2）の補償。
                      どの献立で使うかが分かれば、必要量は利用者が判断できる。
                    */}
                    <span className="ml-2 text-sm text-kon-ink/60">
                      {it.usedIn.map((m) => m.name).join('、')}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}

      <p className="text-xs text-kon-ink/60">
        調味料は含みません。実際の材料はレシピ元でご確認ください。
      </p>
    </section>
  )
}
