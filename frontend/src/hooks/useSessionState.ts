import { useCallback, useState } from 'react'

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
  const [value, setValue] = useState<T>(() => read(key) ?? initial)

  const update = useCallback(
    (next: T | ((prev: T) => T)) => {
      setValue((prev) => {
        const resolved =
          typeof next === 'function' ? (next as (p: T) => T)(prev) : next
        write(key, resolved)
        return resolved
      })
    },
    [key],
  )

  return [value, update] as const
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
