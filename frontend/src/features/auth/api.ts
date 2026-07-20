import { apiGet, apiPost } from '../../api/client'
import type { User } from '../../api/types'

/** Googleログインの開始URL。fetch ではなくブラウザ自身に辿らせる。 */
export const googleLoginPath = '/api/v1/auth/google'

type Credentials = {
  email: string
  password: string
}

/** login はメールとパスワードでログインする。認証Cookieはサーバが設定する。 */
export async function login(credentials: Credentials): Promise<User> {
  const res = await apiPost<{ user: User }>('/auth/login', credentials)
  return res.user
}

/** signUp はメールとパスワードで登録する。登録と同時にログイン状態になる。 */
export async function signUp(credentials: Credentials): Promise<User> {
  const res = await apiPost<{ user: User }>('/auth/signup', credentials)
  return res.user
}

/** fetchMe は現在のユーザーを返す。未ログインなら 401（ApiError）になる。 */
export async function fetchMe(): Promise<User> {
  const res = await apiGet<{ user: User }>('/auth/me')
  return res.user
}

/** logout は認証Cookieを破棄する。 */
export function logout(): Promise<void> {
  return apiPost<void>('/auth/logout')
}
