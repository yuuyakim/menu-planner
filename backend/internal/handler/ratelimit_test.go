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

// rateLimitEcho は指定したレート制限だけを適用した echo を組み立てる。
// ハンドラは常に 200 を返すので、200 か 429 かでミドルウェアの判定だけを見られる。
func rateLimitEcho(limit int, window time.Duration) *echo.Echo {
	e := echo.New()
	// エラーを RFC 7807 に載せる本番と同じ経路を通し、超過が 429 になることを確かめる。
	e.HTTPErrorHandler = handler.ErrorHandler()
	e.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "pong")
	}, handler.RateLimiter(limit, window))
	return e
}

// doFrom は指定したIPからのリクエストを1回投げてステータスを返す。
// echo の RealIP は RemoteAddr を見るため、IPごとの独立性はこれで検証できる。
func doFrom(e *echo.Echo, ip string) int {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = ip + ":12345"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code
}

func TestRateLimiter_AuthAllowsTenPerMinute(t *testing.T) {
	// 認証エンドポイントは 10req/min/IP（spec.md 11章）。
	e := rateLimitEcho(10, time.Minute)

	for i := 0; i < 10; i++ {
		code := doFrom(e, "10.0.0.1")
		require.Equalf(t, http.StatusOK, code, "%d 回目までは通るはず", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, doFrom(e, "10.0.0.1"), "11回目は制限を超える")
}

func TestRateLimiter_SearchAllowsSixtyPerMinute(t *testing.T) {
	// 検索は 60req/min/IP（spec.md 11章）。認証より緩い上限が効くことを確かめる。
	e := rateLimitEcho(60, time.Minute)

	for i := 0; i < 60; i++ {
		code := doFrom(e, "10.0.0.2")
		require.Equalf(t, http.StatusOK, code, "%d 回目までは通るはず", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, doFrom(e, "10.0.0.2"), "61回目は制限を超える")
}

func TestRateLimiter_ExceedReturns429Problem(t *testing.T) {
	// 超過時は 429 かつ RFC 7807 の Content-Type で返す。
	e := rateLimitEcho(1, time.Minute)

	require.Equal(t, http.StatusOK, doFrom(e, "10.0.0.3"))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "10.0.0.3:12345"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, handler.ProblemContentType, rec.Header().Get(echo.HeaderContentType))
	// 超過時は Retry-After を返し、いつ再試行できるかを示す。
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestRateLimiter_IsolatedPerIP(t *testing.T) {
	// 1つのIPが上限に達しても、別のIPは影響を受けない。
	e := rateLimitEcho(2, time.Minute)

	require.Equal(t, http.StatusOK, doFrom(e, "10.0.0.4"))
	require.Equal(t, http.StatusOK, doFrom(e, "10.0.0.4"))
	require.Equal(t, http.StatusTooManyRequests, doFrom(e, "10.0.0.4"), "10.0.0.4 は上限")

	// 別IPは自分のカウントを持つので通る。
	assert.Equal(t, http.StatusOK, doFrom(e, "10.0.0.5"), "別IPは独立して通る")
	assert.Equal(t, http.StatusOK, doFrom(e, "10.0.0.5"))
	assert.Equal(t, http.StatusTooManyRequests, doFrom(e, "10.0.0.5"), "別IPも自分の上限で 429")
}

func TestRateLimiter_NonPositiveLimitDisables(t *testing.T) {
	// 上限 0 以下は「無制限」。プロキシ配下で全リクエストが1つのIPに
	// 集約される開発・E2E環境では、制限をこの値で切って詰まらせない。
	for _, limit := range []int{0, -1} {
		e := rateLimitEcho(limit, time.Minute)
		for i := 0; i < 100; i++ {
			require.Equalf(t, http.StatusOK, doFrom(e, "10.0.0.6"),
				"limit=%d では何度でも通る（%d回目）", limit, i+1)
		}
	}
}
