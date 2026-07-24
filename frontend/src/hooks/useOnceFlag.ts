import { useCallback, useState } from 'react'

const prefix = 'menu-planner:once:'

// useOnceFlag は「一度きり」をブラウザに恒久的に記録する。
// 一時状態の sessionStorage（タブで消える）と違い、案内は端末で一度出れば十分なので
// localStorage を使う。private モードで失敗しても画面は動く（消えても「もう一度出る」だけ）。
export function useOnceFlag(key: string): [boolean, () => void] {
  const storageKey = prefix + key
  const [done, setDone] = useState(() => {
    try {
      return localStorage.getItem(storageKey) === '1'
    } catch {
      return false
    }
  })
  const mark = useCallback(() => {
    setDone(true)
    try {
      localStorage.setItem(storageKey, '1')
    } catch {
      /* private モード: 記録できなくても実害は「次も出る」だけ */
    }
  }, [storageKey])
  return [done, mark]
}
