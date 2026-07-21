import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { clearSessionState, useSessionState } from './useSessionState'

afterEach(() => {
  sessionStorage.clear()
})

describe('useSessionState', () => {
  it('保存した値が作り直しても復元される', () => {
    const first = renderHook(() => useSessionState('week', null as string | null))
    act(() => first.result.current[1]('月曜はカレー'))
    first.unmount()

    // 別画面を見て戻った状況。新しくマウントしても値が残っている。
    const second = renderHook(() => useSessionState('week', null as string | null))
    expect(second.result.current[0]).toBe('月曜はカレー')
  })

  it('clearSessionState で保存が消え、作り直すと初期値に戻る', () => {
    const first = renderHook(() => useSessionState('week', null as string | null))
    act(() => first.result.current[1]('月曜はカレー'))
    first.unmount()

    clearSessionState()

    const second = renderHook(() => useSessionState('week', null as string | null))
    expect(second.result.current[0]).toBeNull()
  })

  it('clearSessionState でマウント中の値もその場で初期値に戻る', () => {
    // 画面を開いたままログアウトした状況。別画面へ移るまで
    // 前のユーザーの週間献立が見えたままにならないこと。
    const { result } = renderHook(() =>
      useSessionState('week', null as string | null),
    )
    act(() => result.current[1]('月曜はカレー'))
    expect(result.current[0]).toBe('月曜はカレー')

    act(() => clearSessionState())

    expect(result.current[0]).toBeNull()
  })

  it('アプリ以外のキーは消さない', () => {
    sessionStorage.setItem('unrelated', 'keep me')
    const { result } = renderHook(() =>
      useSessionState('week', null as string | null),
    )
    act(() => result.current[1]('月曜はカレー'))

    act(() => clearSessionState())

    expect(sessionStorage.getItem('unrelated')).toBe('keep me')
  })
})
