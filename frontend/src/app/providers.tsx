import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

// queryClient はアプリ全体で1つ。モジュールスコープに置くのは、
// 再描画のたびに作り直してキャッシュが消えるのを防ぐため。
// テストは createTestQueryClient で別インスタンスを使う。
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // 献立マスタも履歴も秒単位で変わるものではない。
      // 画面を切り替えるたびに取り直さないよう少し寝かせる。
      staleTime: 30_000,
      // 401 は再試行しても通らないため、既定の3回リトライは無駄。
      // 実際の判定は 8-C のAPIクライアントで入れる。
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

// AppProviders はアプリ全体で必要なコンテキストをまとめる。
export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}
