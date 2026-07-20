import { useQuery } from '@tanstack/react-query'
import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { server } from '../test/server'
import { renderWithProviders } from '../test/render'

// TanStack Query が本番と同じプロバイダ構成で動くことの確認。
// 個々の画面ではなく「基盤が繋がっているか」を見る。
function HealthProbe() {
  const { data, isPending, isError } = useQuery({
    queryKey: ['health'],
    queryFn: async () => {
      const res = await fetch('/api/v1/health')
      if (!res.ok) throw new Error('失敗')
      return (await res.json()) as { status: string }
    },
  })

  if (isPending) return <p>読み込み中</p>
  if (isError) return <p>エラー</p>
  return <p>状態: {data.status}</p>
}

describe('プロバイダ', () => {
  it('useQuery でAPIを取得できる', async () => {
    renderWithProviders(<HealthProbe />)

    expect(screen.getByText('読み込み中')).toBeVisible()
    await waitFor(() => expect(screen.getByText('状態: ok')).toBeVisible())
  })

  it('失敗はリトライせずエラーになる', async () => {
    server.use(
      http.get('/api/v1/health', () => new HttpResponse(null, { status: 500 })),
    )

    renderWithProviders(<HealthProbe />)

    // retry: false なので待たされずにエラー状態へ落ちる。
    await waitFor(() => expect(screen.getByText('エラー')).toBeVisible())
  })
})
