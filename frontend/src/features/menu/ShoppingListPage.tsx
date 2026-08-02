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
  // hidden は利用者が消した導出品目であることを表す。表示からは外すが、
  // overlay を再構築できるよう items 自体には残す（サーバの GET 由来）。
  hidden: boolean
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
          hidden: it.hidden,
        }))
      : (derived.data ?? []).map((it) => ({
          key: it.ingredient.id,
          name: it.ingredient.name,
          category: it.ingredient.category,
          usedIn: it.usedIn,
          checked: false,
          origin: 'derived' as const,
          hidden: false,
        }))

  // 保存済みの週のときだけ、チェックはサーバに残る。
  // 未保存の週はその場限りで、画面を離れると消える。
  //
  // savedId は保存時か保存済みの週を開いたときにだけ入り、ログアウトで
  // clearSessionState() により消える。したがって savedId != null は
  // ログイン済みを含意する。
  const canPersist = savedId != null
  const queryClient = useQueryClient()

  // チェック状態はローカルで持つ。全品目に常時チェックボックスを出す
  // という Task 11 の要件のため、未保存の週（サーバ側に checked が無い）
  // でも状態を持てるようにローカルが正になる。
  const [checked, setChecked] = useState<Set<string>>(new Set())
  // manual は手動追加した品目（保存済みの週のみ）。hidden は非表示にした
  // 導出品目の key。どちらもローカルが正で、PUT のたびに overlay 全体へ含める。
  const [manual, setManual] = useState<
    { name: string; category: IngredientCategory }[]
  >([])
  const [hidden, setHidden] = useState<Set<string>>(new Set())
  useEffect(() => {
    setChecked(new Set(items.filter((it) => it.checked).map((it) => it.key)))
    // manual はサーバが返す origin==='manual' の品目から作り直す。
    setManual(
      items
        .filter((it) => it.origin === 'manual')
        .map((it) => ({ name: it.name, category: it.category })),
    )
    // hidden もサーバが返す hidden===true の品目から作り直す。GET が hidden
    // 行も返すようになったため（設計の穴の修正）、ここで再構築できる。
    // これをしないと次回 PUT で overlay から hidden 行が抜け、消したはずの
    // 導出品目が復活してしまう。
    setHidden(new Set(items.filter((it) => it.hidden).map((it) => it.key)))
    // items ではなく取得結果そのもの（saved.data / derived.data）を依存にする。
    // items は毎レンダー新しい配列を作るため、それを依存にすると
    // チェック操作のたびにローカル state がサーバの値へ巻き戻ってしまう。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saved.data, derived.data])

  // buildOverlay は「今の画面状態」から overlay 全体を組み立てる（一括置換）。
  // PUT は部分更新のAPIを持たないため、状態が変わるたびに全体を送り直す。
  function buildOverlay(
    nextChecked: Set<string>,
    nextHidden: Set<string>,
    nextManual: typeof manual,
  ): ShoppingListOverride[] {
    const derivedOverrides: ShoppingListOverride[] = items
      .filter((it) => it.origin === 'derived')
      .filter((it) => nextChecked.has(it.key) || nextHidden.has(it.key))
      .map((it) => ({
        name: it.name,
        category: it.category,
        origin: 'derived',
        checked: nextChecked.has(it.key),
        hidden: nextHidden.has(it.key),
      }))
    const manualOverrides: ShoppingListOverride[] = nextManual.map((m) => ({
      name: m.name,
      category: m.category,
      origin: 'manual',
      checked: nextChecked.has(m.name),
      hidden: false,
    }))
    return [...derivedOverrides, ...manualOverrides]
  }

  const persist = useMutation({
    mutationFn: (overlay: ShoppingListOverride[]) =>
      saveShoppingListOverrides(savedId as string, overlay),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: savedShoppingListQueryKey(savedId as string),
      })
    },
  })

  // 追加フォームの入力値。保存済みの週のときだけ表示する。
  const [draftName, setDraftName] = useState('')
  const [draftCategory, setDraftCategory] = useState<IngredientCategory>(
    categoryOrder[0],
  )

  function toggle(key: string) {
    // setChecked に渡す updater 内で persist.mutate を呼ぶと、
    // StrictMode の開発時二重実行で PUT が2回飛んでしまう
    // （updater は純粋関数であるべきで、副作用を含めてはいけない）。
    // 現在の checked をクロージャから読み、次の Set を先に計算してから
    // setChecked には値を渡し、副作用は updater の外で実行する。
    const next = new Set(checked)
    if (next.has(key)) {
      next.delete(key)
    } else {
      next.add(key)
    }
    setChecked(next)
    if (canPersist) {
      persist.mutate(buildOverlay(next, hidden, manual))
    }
  }

  // addManual は入力欄の内容を手動品目として overlay に加える。
  // 副作用（persist.mutate）は setManual の外で行う（toggle と同じ理由）。
  function addManual() {
    const name = draftName.trim()
    if (!name) return
    const nextManual = [...manual, { name, category: draftCategory }]
    setManual(nextManual)
    setDraftName('')
    if (canPersist) {
      persist.mutate(buildOverlay(checked, hidden, nextManual))
    }
  }

  // remove は品目を消す。導出品目は非表示（hidden）に、手動品目は一覧から
  // 取り除く。canPersist のときだけ呼ばれる（UI 側でボタンを出し分ける）。
  function remove(it: ViewItem) {
    if (it.origin === 'manual') {
      const nextManual = manual.filter((m) => m.name !== it.name)
      setManual(nextManual)
      persist.mutate(buildOverlay(checked, hidden, nextManual))
    } else {
      const nextHidden = new Set(hidden)
      nextHidden.add(it.key)
      setHidden(nextHidden)
      persist.mutate(buildOverlay(checked, nextHidden, manual))
    }
  }

  // visible は画面に出す一覧。hidden な導出品目は items 自体には残す
  // （overlay 再構築のため）が、表示からは外す。
  const visible: ViewItem[] = items.filter((it) => !it.hidden)

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

      {visible.length === 0 ? (
        <MascotEmpty>この献立には食材が登録されていません。</MascotEmpty>
      ) : (
        <div className="space-y-4">
          {groupByCategory(visible).map((group) => (
            <div key={group.category} className="space-y-2">
              <h2 className="text-sm font-medium text-kon-ink/60">
                {categoryLabels[group.category]}
              </h2>
              <ul className="space-y-1">
                {group.items.map((it) => (
                  <li
                    key={it.key}
                    className="flex items-center justify-between rounded-xl border border-kon-leaf-soft bg-white px-4 py-2"
                  >
                    <label className="flex items-center gap-2">
                      {/*
                        保存済みの週以外はその場限り（画面を離れると消える）。
                        それでもチェックボックス自体は常に出す。買い物中は
                        保存済みかどうかを気にせず使える方が自然なため。
                      */}
                      <input
                        type="checkbox"
                        className="accent-kon-leaf"
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
                    {/* 品目の追加・削除は保存済みの週のときだけ出す。 */}
                    {canPersist && (
                      <button
                        type="button"
                        onClick={() => remove(it)}
                        aria-label={`${it.name}を消す`}
                        className="ml-2 text-kon-ink/50 hover:text-kon-ink"
                      >
                        ×
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}

      {canPersist && (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            addManual()
          }}
          className="flex flex-wrap items-center gap-2"
        >
          <input
            aria-label="品目を追加"
            value={draftName}
            onChange={(e) => setDraftName(e.target.value)}
            className="rounded-lg border border-kon-leaf-soft px-3 py-1"
          />
          <select
            aria-label="カテゴリ"
            value={draftCategory}
            onChange={(e) =>
              setDraftCategory(e.target.value as IngredientCategory)
            }
            className="rounded-lg border border-kon-leaf-soft px-3 py-1"
          >
            {categoryOrder.map((c) => (
              <option key={c} value={c}>
                {categoryLabels[c]}
              </option>
            ))}
          </select>
          <button
            type="submit"
            className="rounded-full bg-kon-leaf px-4 py-1 font-medium text-white hover:bg-kon-leaf/90"
          >
            追加
          </button>
        </form>
      )}

      {persist.error && <ErrorMessage error={persist.error} />}

      <p className="text-xs text-kon-ink/60">
        調味料は含みません。実際の材料はレシピ元でご確認ください。
      </p>
    </section>
  )
}
