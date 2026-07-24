import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { useOnceFlag } from './useOnceFlag'

afterEach(() => localStorage.clear())

test('mark すると done になり、次回のマウントでも done', () => {
  const { result, unmount } = renderHook(() => useOnceFlag('premium-shopping'))
  expect(result.current[0]).toBe(false)
  act(() => result.current[1]())
  expect(result.current[0]).toBe(true)
  unmount()
  const again = renderHook(() => useOnceFlag('premium-shopping'))
  expect(again.result.current[0]).toBe(true)
})
