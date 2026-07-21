import { proxyApiRequest } from '../../src/server/apiProxy'

/**
 * Cloudflare Pages Function: `/api/*` をすべて backend に中継する。
 *
 * `[[path]]` は catch-all のファイル名規約で、`/api` 配下の全パス・全メソッドが
 * ここに来る。中身のロジックは `src/server/apiProxy.ts`（テスト対象）に置き、
 * ここは環境変数と Cloudflare 固有のヘッダを渡すだけの薄い層に留める。
 *
 * 必要な環境変数（Pages の設定で与える）:
 *   BACKEND_ORIGIN … 例 https://menu-planner-backend-xxxx.asia-northeast1.run.app
 *
 * CF-Connecting-IP は Cloudflare が付ける実クライアントIP。
 * backend のレート制限をIP単位で正しく効かせるために前送りする。
 */
export function onRequest(context: {
  request: Request
  env: { BACKEND_ORIGIN?: string }
}): Promise<Response> {
  const backendOrigin = context.env.BACKEND_ORIGIN
  if (!backendOrigin) {
    // 設定漏れを黙って通すと「APIが全部404」の原因が分からなくなる。
    return Promise.resolve(
      new Response(
        JSON.stringify({
          title: 'BACKEND_ORIGIN が設定されていません',
          status: 500,
        }),
        { status: 500, headers: { 'content-type': 'application/json' } },
      ),
    )
  }

  return proxyApiRequest(
    context.request,
    backendOrigin,
    context.request.headers.get('CF-Connecting-IP'),
  )
}
