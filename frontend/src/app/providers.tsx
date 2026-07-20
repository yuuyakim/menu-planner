import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

import { queryDefaults } from './queryDefaults'

// queryClient はアプリ全体で1つ。モジュールスコープに置くのは、
// 再描画のたびに作り直してキャッシュが消えるのを防ぐため。
// テストは createTestQueryClient で別インスタンスを使う。
const queryClient = new QueryClient({
  defaultOptions: { queries: queryDefaults },
})

// AppProviders はアプリ全体で必要なコンテキストをまとめる。
export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}
