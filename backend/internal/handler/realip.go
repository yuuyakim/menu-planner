package handler

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// ProxySecretHeader は「自分たちのプロキシを通ったリクエストである」ことを示すヘッダ。
// Cloudflare Pages の Function がこの値を付け、backend が突き合わせる。
const ProxySecretHeader = "X-Proxy-Secret"

// TrustedProxyIPExtractor は実クライアントIPを取り出す echo.IPExtractor を返す。
//
// **共有シークレットが一致したリクエストの転送ヘッダだけを信頼する。**
// backend の URL は公開されており誰でも直接叩けるため、X-Forwarded-For を
// 無条件に信じると、毎回違うIPを詐称するだけでレート制限を回避できてしまう。
// 前段（Pages Function）だけが知るシークレットで「自分のプロキシ経由」を確かめる。
//
// secret が空（ローカル開発など）なら常に接続元を使う。設定を忘れたときに
// 「全部信用する」側へ倒れないよう、安全側を既定にしている。
func TrustedProxyIPExtractor(secret string) echo.IPExtractor {
	return func(req *http.Request) string {
		if trustedProxy(req, secret) {
			// 前段が載せた実クライアントIP。Cloud Run は受け取った XFF に
			// 自分から見た接続元を足すため、先頭が実クライアントになる。
			if xff := req.Header.Get(echo.HeaderXForwardedFor); xff != "" {
				if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
					return ip
				}
			}
			if ip := strings.TrimSpace(req.Header.Get(echo.HeaderXRealIP)); ip != "" {
				return ip
			}
		}
		return remoteAddrIP(req)
	}
}

// trustedProxy はリクエストが自分たちのプロキシ経由かを判定する。
func trustedProxy(req *http.Request, secret string) bool {
	if secret == "" {
		return false
	}
	got := req.Header.Get(ProxySecretHeader)
	// 比較時間から値を推測されないよう定数時間で比較する。
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}

// remoteAddrIP は実際の接続元のIPを返す。RemoteAddr は "host:port" 形式。
func remoteAddrIP(req *http.Request) string {
	if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		return host
	}
	return req.RemoteAddr
}
