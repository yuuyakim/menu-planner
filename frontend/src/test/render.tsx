import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, type RenderOptions } from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
import { MemoryRouter } from 'react-router'

import { queryDefaults } from '../app/queryDefaults'

// createTestQueryClient はテスト用の QueryClient を作る。
//
// 本番の既定（queryDefaults）を土台にする。ここだけ違う設定にすると、
// キャッシュに起因する不具合がテストに映らなくなる。
// 唯一変えるのはリトライで、失敗を確かめるテストが指数バックオフを
// 待たされて遅くなる（かつ時間切れになる）のを防ぐため。
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { ...queryDefaults, retry: false },
      mutations: { retry: false },
    },
  })
}

type Options = Omit<RenderOptions, 'wrapper'> & {
  /** 描画時のパス。既定は '/'。 */
  route?: string
  /**
   * 履歴を積んだ状態で描画する。戻る操作を検証したいときに使う。
   * 指定すると route は無視され、最後の要素が現在地になる。
   */
  history?: string[]
}

// renderWithProviders は本番と同じプロバイダで包んで描画する。
// ルータは MemoryRouter を使い、テストごとに履歴を独立させる。
// QueryClient もテストごとに作り直し、キャッシュを持ち越さない。
export function renderWithProviders(ui: ReactElement, options: Options = {}) {
  const { route = '/', history, ...renderOptions } = options
  const entries = history ?? [route]
  const queryClient = createTestQueryClient()

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={entries} initialIndex={entries.length - 1}>
          {children}
        </MemoryRouter>
      </QueryClientProvider>
    )
  }

  return { queryClient, ...render(ui, { wrapper: Wrapper, ...renderOptions }) }
}
