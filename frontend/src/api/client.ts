import type { Problem } from './types'

// APIのベースパス。フロントと同一オリジンで配信し、
// dev server / 本番の nginx が /api を backend に転送する。
// 同一オリジンにするのは、認証Cookieがクロスオリジン扱いにならないようにするため。
const basePath = '/api/v1'

// ApiError はサーバから応答は返ったが、失敗のステータスだったことを表す。
// status を持つので、呼び出し側は 401 でログインへ、409 で重複の案内、と分岐できる。
export class ApiError extends Error {
  readonly status: number
  readonly type?: string
  readonly title?: string
  readonly detail?: string

  constructor(status: number, problem?: Partial<Problem>) {
    // 画面に出せる文言を必ず持たせる。problem+json でない応答（プロキシが返す
    // HTMLなど）もあるため、解釈できなければステータスから組み立てる。
    super(problem?.title ?? `通信に失敗しました (${status})`)
    this.name = 'ApiError'
    this.status = status
    this.type = problem?.type
    this.title = problem?.title
    this.detail = problem?.detail
  }

  /** 認証が必要／認証に失敗した状態。ログイン画面へ誘導する判断に使う。 */
  get isUnauthorized(): boolean {
    return this.status === 401
  }
}

// NetworkError は応答そのものを利用できなかったことを表す。
// 通信が届かない場合と、届いたが本文を解釈できない場合の両方を含む。
// どちらも「ステータスによる分岐ができない」点で呼び出し側の扱いは同じなので、
// ApiError とは別の型にして取り違えを防ぐ。
export class NetworkError extends Error {
  constructor(message = '通信に失敗しました。接続を確認してください。') {
    super(message)
    this.name = 'NetworkError'
  }
}

// parseProblem は失敗応答の本文を RFC 7807 として読む。
// 解釈できなければ undefined を返す（エラーの握り潰しではなく、
// ステータスだけで ApiError を作るため）。
async function parseProblem(res: Response): Promise<Partial<Problem> | undefined> {
  try {
    const body: unknown = await res.json()
    if (typeof body === 'object' && body !== null) {
      return body as Partial<Problem>
    }
  } catch {
    // problem+json 以外（HTMLや空）は解釈できない。
  }
  return undefined
}

// セッションを再発行しないパス。
//
// これらで 401 が返るのは資格情報そのものが通らなかったということなので、
// 再発行しても結果は変わらない。とくに /auth/refresh を除いておかないと、
// 失敗した再発行がまた再発行を呼んで往復が止まらなくなる。
const noRefreshPaths = ['/auth/login', '/auth/signup', '/auth/refresh']

// 進行中の再発行。同時に 401 になった複数のリクエストが
// それぞれ再発行を投げないよう、1本の Promise を共有する。
let refreshing: Promise<boolean> | null = null

// refreshSession はリフレッシュCookieでアクセストークンの再発行を試み、
// 成否を返す。失敗は呼び出し側が元の 401 を返すための情報でしかないので、
// 例外にはしない。
function refreshSession(): Promise<boolean> {
  refreshing ??= (async () => {
    try {
      const res = await fetch(`${basePath}/auth/refresh`, {
        method: 'POST',
        credentials: 'include',
      })
      return res.ok
    } catch {
      return false
    } finally {
      // 次に 401 が来たときは改めて試せるようにする。
      refreshing = null
    }
  })()
  return refreshing
}

// request は全リクエストの共通処理。
// path は '/menus/suggest' のようにベースパスを除いたもの。
//
// retry は 401 でセッションを再発行してやり直す余地が残っているかを表す。
// やり直しは一度だけで、そこでも 401 なら諦めて呼び出し側に返す。
async function request<T>(
  path: string,
  init: RequestInit = {},
  retry = true,
): Promise<T> {
  let res: Response
  try {
    res = await fetch(`${basePath}${path}`, {
      ...init,
      // 認証は HttpOnly Cookie なので、これが無いと一切認証が通らない。
      credentials: 'include',
    })
  } catch {
    // fetch が reject するのは通信自体が成立しなかったとき。
    throw new NetworkError()
  }

  // アクセストークンは15分で切れるが、リフレッシュトークンは30日有効。
  // 期限切れは利用者にとってログアウトではないので、黙って再発行してやり直す。
  if (res.status === 401 && retry && !noRefreshPaths.includes(path)) {
    if (await refreshSession()) {
      return request<T>(path, init, false)
    }
  }

  if (!res.ok) {
    throw new ApiError(res.status, await parseProblem(res))
  }

  // 204 と本文なしはそのまま解決する。DELETE や refresh がこれ。
  if (res.status === 204 || res.headers.get('Content-Length') === '0') {
    return undefined as T
  }

  try {
    return (await res.json()) as T
  } catch {
    // 成功ステータスなのに本文が読めない＝応答を利用できない。
    throw new NetworkError('応答を読み取れませんでした。')
  }
}

/** GET でJSONを取得する。 */
export function apiGet<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'GET' })
}

/** POST でJSONを送る。body 省略時は本文なしで送る（refresh / logout）。 */
export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    ...(body === undefined
      ? {}
      : {
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        }),
  })
}

/** PUT でJSONを送る。overlay の一括置換など、全体を送り直す更新に使う。 */
export function apiPut<T = void>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'PUT',
    ...(body === undefined
      ? {}
      : {
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        }),
  })
}

/** DELETE する。応答は基本 204。 */
export function apiDelete<T = void>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}
