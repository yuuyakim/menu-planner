package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/handler"
)

const proxySecret = "s3cret-from-our-proxy"

// extractFrom は指定のヘッダ・接続元でリクエストを組み立て、抽出されたIPを返す。
func extractFrom(secret string, headers map[string]string, remoteAddr string) string {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return handler.TrustedProxyIPExtractor(secret)(req)
}

func TestTrustedProxyIPExtractor_TrustsForwardedIPWithSecret(t *testing.T) {
	// 自分のプロキシ（共有シークレット付き）が前送りした実クライアントIPを使う。
	got := extractFrom(proxySecret, map[string]string{
		handler.ProxySecretHeader: proxySecret,
		echo.HeaderXForwardedFor:  "203.0.113.7",
	}, "10.1.2.3:5000")

	assert.Equal(t, "203.0.113.7", got)
}

func TestTrustedProxyIPExtractor_IgnoresSpoofedHeaderWithoutSecret(t *testing.T) {
	// シークレット無しの直リクエストでは、詐称された X-Forwarded-For を信用せず
	// 実際の接続元を使う。これが無いとレート制限をヘッダ偽装で回避できてしまう。
	got := extractFrom(proxySecret, map[string]string{
		echo.HeaderXForwardedFor: "1.1.1.1",
	}, "198.51.100.9:5000")

	assert.Equal(t, "198.51.100.9", got, "偽装ヘッダを信用してはいけない")
}

func TestTrustedProxyIPExtractor_IgnoresWrongSecret(t *testing.T) {
	// シークレットが違えば信用しない。
	got := extractFrom(proxySecret, map[string]string{
		handler.ProxySecretHeader: "wrong-secret",
		echo.HeaderXForwardedFor:  "1.1.1.1",
	}, "198.51.100.9:5000")

	assert.Equal(t, "198.51.100.9", got)
}

func TestTrustedProxyIPExtractor_EmptySecretNeverTrusts(t *testing.T) {
	// シークレット未設定（ローカル開発など）では常に接続元を使う。
	// 「設定を忘れたら全部信用する」という危険な既定にしない。
	got := extractFrom("", map[string]string{
		handler.ProxySecretHeader: "",
		echo.HeaderXForwardedFor:  "1.1.1.1",
	}, "198.51.100.9:5000")

	assert.Equal(t, "198.51.100.9", got)
}

func TestTrustedProxyIPExtractor_UsesLeftmostForwardedEntry(t *testing.T) {
	// Cloud Run は受け取った XFF に自分が見た接続元を足す。
	// 先頭が我々のプロキシが載せた実クライアント。
	got := extractFrom(proxySecret, map[string]string{
		handler.ProxySecretHeader: proxySecret,
		echo.HeaderXForwardedFor:  "203.0.113.7, 172.16.0.1",
	}, "10.1.2.3:5000")

	assert.Equal(t, "203.0.113.7", got)
}

func TestTrustedProxyIPExtractor_FallsBackToRealIPHeader(t *testing.T) {
	// XFF が無ければ X-Real-IP を見る。
	got := extractFrom(proxySecret, map[string]string{
		handler.ProxySecretHeader: proxySecret,
		echo.HeaderXRealIP:        "203.0.113.8",
	}, "10.1.2.3:5000")

	assert.Equal(t, "203.0.113.8", got)
}

func TestTrustedProxyIPExtractor_RateLimitIsPerRealClient(t *testing.T) {
	// 抽出器とレート制限を組み合わせ、同じプロキシ経由でも
	// 実クライアントごとに別々に数えられることを確かめる。
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	e.IPExtractor = handler.TrustedProxyIPExtractor(proxySecret)
	e.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "pong")
	}, handler.RateLimiter(2, time.Minute))

	call := func(clientIP string) int {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		// 前段プロキシは同一（同じ接続元）だが、実クライアントは別。
		req.RemoteAddr = "10.1.2.3:5000"
		req.Header.Set(handler.ProxySecretHeader, proxySecret)
		req.Header.Set(echo.HeaderXForwardedFor, clientIP)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusOK, call("203.0.113.1"))
	require.Equal(t, http.StatusOK, call("203.0.113.1"))
	assert.Equal(t, http.StatusTooManyRequests, call("203.0.113.1"), "同一クライアントは上限に達する")

	// 別クライアントはプロキシが同じでも影響を受けない。
	assert.Equal(t, http.StatusOK, call("203.0.113.2"), "別クライアントは独立して通る")
}
