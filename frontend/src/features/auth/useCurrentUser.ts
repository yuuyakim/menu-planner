import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { ApiError } from '../../api/client'
import type { User } from '../../api/types'
import { fetchMe, logout } from './api'

/** meQueryKey は現在のユーザーのキャッシュキー。ログイン成功時にも使う。 */
export const meQueryKey = ['me'] as const

/**
 * useCurrentUser は現在のユーザーを返す。未ログインなら user は undefined。
 *
 * 未ログインはサーバが 401 で表す。これは異常ではなく通常の状態なので、
 * エラーとして扱わず「ユーザーが居ない」に読み替える。
 */
export function useCurrentUser() {
  const { data, error, isPending } = useQuery({
    queryKey: meQueryKey,
    queryFn: fetchMe,
    // 401 は再試行しても結果が変わらない。無駄な問い合わせをしない。
    retry: false,
    // ログイン状態は画面をまたいで何度も参照するため、都度取り直さない。
    staleTime: 5 * 60 * 1000,
  })

  const isUnauthenticated = error instanceof ApiError && error.isUnauthorized

  return {
    user: data as User | undefined,
    // 判定が付くまでは「ログインしていない」と決めつけない。
    // 決めつけると、認証済みでも一瞬ログイン画面が見えてしまう。
    isLoading: isPending,
    // 401 以外の失敗（通信断など）は、ログイン状態が不明なだけ。
    isUnauthenticated,
  }
}

/** useLogout はログアウトし、保持しているユーザー情報を捨てる。 */
export function useLogout() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: logout,
    onSettled: () => {
      // 通信が失敗しても、この端末に残った状態は消す。
      // 「押したのにログインしたままに見える」状態を作らない。
      //
      // setQueryData(key, undefined) では消えない。undefined は
      // 「更新しない」の意味に解釈され、キャッシュが残ってしまう。
      // resetQueries なら初期状態に戻したうえで取り直す。
      void queryClient.resetQueries({ queryKey: meQueryKey })
    },
  })
}
