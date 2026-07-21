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
      // **キャッシュは丸ごと初期化する。** ['me'] だけ消すと、お気に入りや履歴は
      // 前のユーザーのまま残り、別アカウントでログインし直したときに
      // そのデータが表示されてしまう（Issue #78）。
      // 消すキーを列挙する方式は採らない。ユーザー固有のクエリを増やしたときに
      // 消し忘れて静かに漏れるため。ユーザー非依存のデータも一緒に消えるが、
      // 取り直されるだけで実害はない。
      //
      // clear() ではなく resetQueries() を使う。clear() は表示中のクエリを
      // 取り直さないため、守られた画面でログアウトすると「読み込み中…」の
      // まま固まってしまう。resetQueries は初期化したうえで再取得する。
      void queryClient.resetQueries()
    },
  })
}
