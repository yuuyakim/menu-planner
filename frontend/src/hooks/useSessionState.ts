import { useCallback, useEffect, useRef, useState } from 'react'

// keyPrefix はこのアプリが sessionStorage に置く値の目印。
// 同じオリジンに他の用途の値があっても巻き込まないよう、
// 消すときはこの接頭辞の付いたものだけを対象にする。
const keyPrefix = 'menu-planner:'

// clearedEvent は「保存した状態を捨てた」ことをマウント中のフックに知らせる合図。
// sessionStorage を消すだけでは、すでに描画されている値は消えない。
const clearedEvent = 'menu-planner:session-cleared'

// useSessionState は state を sessionStorage にも書き、
// 画面遷移でコンポーネントが作り直されても値を保てるようにする。
//
// コンポーネントの useState だけだとルート遷移で消える。週間献立のように
// 「利用者が手間をかけて組み立てたもの」は、別の画面を見て戻っただけで
// 失われてはいけない。
//
// localStorage ではなく sessionStorage を使うのは、タブを閉じれば消える
// 一時的なものだから。別タブで別の週を作れる方が自然でもある。
export function useSessionState<T>(key: string, initial: T) {
  const storageKey = keyPrefix + key
  const [value, setValue] = useState<T>(() => read(storageKey) ?? initial)

  // 初期値は呼び出し側でリテラル（{} など）を渡すことが多く、描画のたびに
  // 別物になる。購読を張り直さないよう ref に固定する。
  const initialRef = useRef(initial)

  // 利用者が変わったら（ログアウト・ログイン）保存した状態は捨てる。
  // 表示中の値も初期値へ戻し、前のユーザーのものが見えたままにしない。
  useEffect(() => {
    const onCleared = () => setValue(initialRef.current)
    window.addEventListener(clearedEvent, onCleared)
    return () => window.removeEventListener(clearedEvent, onCleared)
  }, [])

  const update = useCallback(
    (next: T | ((prev: T) => T)) => {
      setValue((prev) => {
        const resolved =
          typeof next === 'function' ? (next as (p: T) => T)(prev) : next
        write(storageKey, resolved)
        return resolved
      })
    },
    [storageKey],
  )

  return [value, update] as const
}

/**
 * clearSessionState はこのアプリが保存した一時状態をすべて捨てる。
 *
 * 利用者の同一性が変わる瞬間（ログアウト・ログイン）に呼ぶ。これが無いと
 * 週間献立が別アカウントにも見えたままになる（Issue #83）。
 * クエリキャッシュの初期化（Issue #78）と対になる処理。
 */
export function clearSessionState(): void {
  try {
    // 削除しながら走査すると添字がずれるため、先に対象を集める。
    const targets: string[] = []
    for (let i = 0; i < sessionStorage.length; i++) {
      const key = sessionStorage.key(i)
      if (key?.startsWith(keyPrefix)) targets.push(key)
    }
    for (const key of targets) sessionStorage.removeItem(key)
  } catch {
    // 保存領域が使えない環境でも、表示中の値を戻す通知は行う。
  }
  window.dispatchEvent(new Event(clearedEvent))
}

// read は保存された値を読む。壊れていれば無いものとして扱う
// （保存形式を変えたときに、画面が開けなくなるのを防ぐ）。
function read<T>(key: string): T | undefined {
  try {
    const raw = sessionStorage.getItem(key)
    return raw === null ? undefined : (JSON.parse(raw) as T)
  } catch {
    return undefined
  }
}

// write は値を保存する。保存できなくても画面の動作は続ける
// （プライベートモードなどで sessionStorage が使えない場合がある）。
function write<T>(key: string, value: T): void {
  try {
    sessionStorage.setItem(key, JSON.stringify(value))
  } catch {
    // 保存できないだけで、この画面にいる間の操作は成立する。
  }
}
