import '@testing-library/jest-dom/vitest'

import { cleanup } from '@testing-library/react'
import { afterAll, afterEach, beforeAll } from 'vitest'

import { server } from './server'

// 未定義のリクエストは黙って素通しせずエラーにする。
// モックし忘れたAPI呼び出しが「なぜか通った」状態になるのを防ぐ。
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))

afterEach(() => {
  // テストごとに上書きしたハンドラとDOMを戻し、テスト間の依存を断つ。
  server.resetHandlers()
  cleanup()
  // 画面が sessionStorage に保存した状態も消す。
  // 残すと、あるテストが作った週が次のテストに現れる。
  sessionStorage.clear()
})

afterAll(() => server.close())
