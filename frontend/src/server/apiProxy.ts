/**
 * apiProxy は `/api/*` へのリクエストを backend にそのまま中継する。
 *
 * これを Cloudflare Pages の Function から呼ぶことで、ブラウザから見て
 * **フロントと API が同一オリジン**になる。開発時に Vite の proxy が
 * 担っている役割を、本番では Pages が担う形。
 *
 * 同一オリジンにする狙いは認証。Cookie は `SameSite=Lax` なので、
 * フロントと backend が別ドメインだとAPI通信に Cookie が付かずログインが壊れる。
 * 同一オリジンなら Lax のまま機能し、CORS も CSRF 対策の追加も要らない。
 */
export async function proxyApiRequest(
  request: Request,
  backendOrigin: string,
  clientIp: string | null,
  proxySecret?: string,
): Promise<Response> {
  const url = new URL(request.url)
  // 末尾スラッシュの有無で `//api/...` にならないようにする。
  const origin = backendOrigin.replace(/\/+$/, '')
  const target = `${origin}${url.pathname}${url.search}`

  const headers = new Headers(request.headers)

  // Host は転送しない。Cloud Run は Host でサービスを判別するため、
  // フロント側の Host を送ると宛先を見失って 404 になる。
  // 削除しておけば fetch が転送先URLから正しい Host を組み立てる。
  headers.delete('host')

  // 実クライアントIPを1件だけ載せる。backend のレート制限はIP単位で、
  // これが無いと全リクエストがプロキシの単一IPに集約されてしまう。
  // 追記ではなく上書きにして、偽装された値を持ち込ませない。
  if (clientIp) {
    headers.set('X-Forwarded-For', clientIp)
    headers.set('X-Real-IP', clientIp)
  }

  // 共有シークレット。backend はこれが一致したときだけ上の転送IPを信頼する。
  // backend のURLは公開されているため、これが無いと誰でもIPを詐称して
  // レート制限を回避できてしまう。
  //
  // クライアントが自分で付けてきた値は必ず捨ててから設定する（詐称防止）。
  headers.delete('X-Proxy-Secret')
  if (proxySecret) {
    headers.set('X-Proxy-Secret', proxySecret)
  }

  const init: RequestInit = {
    method: request.method,
    headers,
    // リダイレクトはプロキシで追わずブラウザに返す。
    // Google SSO の 302 を横取りするとログインが完了しない。
    redirect: 'manual',
  }

  // GET / HEAD に本文は付けられない。
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    init.body = await request.arrayBuffer()
  }

  // backend の応答をそのまま返す。Set-Cookie もこの経路で
  // フロントのオリジンに対する Cookie としてブラウザに渡る。
  return fetch(target, init)
}
