package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
)

// errorApp は必ず err を返すルートを1つ持つ echo を返す。
func errorApp(err error) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	e.GET("/boom", func(_ echo.Context) error { return err })
	return e
}

// requestWithAccept は Accept を指定して /boom を叩く。
func requestWithAccept(e *echo.Echo, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	if accept != "" {
		req.Header.Set(echo.HeaderAccept, accept)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// browserAccept はブラウザがトップレベル遷移で送る Accept。
const browserAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

func TestErrorHandler_BrowserNavigationRedirectsInsteadOfJSON(t *testing.T) {
	// ブラウザの画面遷移（Googleログインなど）でエラーになったとき、
	// 生の problem+json を見せずにログイン画面へ戻す。
	rec := requestWithAccept(errorApp(handler.ErrRateLimited), browserAccept)

	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/login?error=rate-limited", rec.Header().Get(echo.HeaderLocation))
	assert.NotContains(t, rec.Body.String(), "probs/", "生のJSONを本文に出さない")
}

func TestErrorHandler_BrowserNavigationUsesErrorKind(t *testing.T) {
	// 種別ごとに違う理由を渡し、フロントで文言を出し分けられるようにする。
	rec := requestWithAccept(errorApp(auth.ErrGoogleAuthFailed), browserAccept)

	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/login?error=google-auth-failed", rec.Header().Get(echo.HeaderLocation))
}

func TestErrorHandler_APIClientStillGetsProblemJSON(t *testing.T) {
	// アプリ内の fetch は Accept: */* で来る。従来どおり problem+json を返す。
	rec := requestWithAccept(errorApp(handler.ErrRateLimited), "*/*")

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, handler.ProblemContentType, rec.Header().Get(echo.HeaderContentType))
	assert.Contains(t, rec.Body.String(), "rate-limited")
	assert.Empty(t, rec.Header().Get(echo.HeaderLocation))
}

func TestErrorHandler_NoAcceptHeaderStillGetsProblemJSON(t *testing.T) {
	// Accept が無い場合もAPI扱い（curl やサーバ間通信）。
	rec := requestWithAccept(errorApp(handler.ErrRateLimited), "")

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, handler.ProblemContentType, rec.Header().Get(echo.HeaderContentType))
}

func TestErrorHandler_JSONAcceptIsNotTreatedAsBrowser(t *testing.T) {
	// application/json を明示するクライアントもAPI扱い。
	rec := requestWithAccept(errorApp(handler.ErrRateLimited), "application/json")

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Empty(t, rec.Header().Get(echo.HeaderLocation))
}
