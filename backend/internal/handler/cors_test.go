package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/yuuyakim/menu-planner/backend/internal/handler"
)

const corsFrontend = "https://menu.example.com"

// corsEcho は CORS ミドルウェアだけを載せた echo を返す。
func corsEcho() *echo.Echo {
	e := echo.New()
	e.Use(handler.CORS(corsFrontend))
	e.GET("/x", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	return e
}

// corsResponse は Origin を付けたリクエストを1回投げて応答を返す。
func corsResponse(e *echo.Echo, method, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/x", nil)
	req.Header.Set(echo.HeaderOrigin, origin)
	if method == http.MethodOptions {
		// プリフライトの体裁を最低限整える。
		req.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodGet)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestCORS_AllowsFrontendOrigin(t *testing.T) {
	// FRONTEND_ORIGIN からのリクエストには許可ヘッダを返す。
	rec := corsResponse(corsEcho(), http.MethodGet, corsFrontend)

	assert.Equal(t, corsFrontend, rec.Header().Get(echo.HeaderAccessControlAllowOrigin))
	// Cookie 認証のため資格情報を許可する。
	assert.Equal(t, "true", rec.Header().Get(echo.HeaderAccessControlAllowCredentials))
}

func TestCORS_RejectsOtherOrigin(t *testing.T) {
	// 別オリジンには Allow-Origin を返さない（ブラウザがブロックする）。
	rec := corsResponse(corsEcho(), http.MethodGet, "https://evil.example.com")

	assert.Empty(t, rec.Header().Get(echo.HeaderAccessControlAllowOrigin),
		"許可していないオリジンに Allow-Origin を返してはいけない")
}

func TestCORS_PreflightOnlyForFrontendOrigin(t *testing.T) {
	// プリフライト（OPTIONS）でも同じ判定になる。
	e := corsEcho()

	allowed := corsResponse(e, http.MethodOptions, corsFrontend)
	assert.Equal(t, corsFrontend, allowed.Header().Get(echo.HeaderAccessControlAllowOrigin))

	rejected := corsResponse(e, http.MethodOptions, "https://evil.example.com")
	assert.Empty(t, rejected.Header().Get(echo.HeaderAccessControlAllowOrigin),
		"許可していないオリジンのプリフライトを通してはいけない")
}
