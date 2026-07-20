import { setupServer } from 'msw/node'

import { handlers } from './handlers'

// テスト用のモックサーバ。実際のネットワークには出ず、
// リクエストを横取りして定義した応答を返す。
export const server = setupServer(...handlers)
