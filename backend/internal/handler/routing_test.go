package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// unknownAPIPaths は /api/v1 配下でどのルートにも一致しないパス。
var unknownAPIPaths = []string{"/api/v1/nope", "/api/v1/foo/bar", "/api/v1"}

func statusOf(e *echo.Echo, method, path string) int {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code
}

func TestRouting_UnknownHistoryAPIPathIs404(t *testing.T) {
	// 認証ミドルウェアをグループ全体に付けると、/api/v1 配下の未定義パスにも
	// 走ってしまい 404 ではなく 401 が返る（Issue #73）。
	e, _ := historyApp(t, &fakeHistoryLister{})

	for _, path := range unknownAPIPaths {
		assert.Equalf(t, http.StatusNotFound, statusOf(e, http.MethodGet, path),
			"%s は存在しないので 404 であるべき", path)
	}
}

func TestRouting_UnknownFavoriteAPIPathIs404(t *testing.T) {
	e, _ := favoriteApp(t, &fakeFavoriteService{})

	for _, path := range unknownAPIPaths {
		assert.Equalf(t, http.StatusNotFound, statusOf(e, http.MethodGet, path),
			"%s は存在しないので 404 であるべき", path)
	}
}

func TestRouting_ProtectedRoutesStillRequireAuth(t *testing.T) {
	// 404 を直した結果、本来の認証が外れていないことを守る。
	history, _ := historyApp(t, &fakeHistoryLister{})
	assert.Equal(t, http.StatusUnauthorized, statusOf(history, http.MethodGet, "/api/v1/histories"))
	assert.Equal(t, http.StatusUnauthorized, statusOf(history, http.MethodDelete, "/api/v1/histories"))

	favorite, _ := favoriteApp(t, &fakeFavoriteService{})
	assert.Equal(t, http.StatusUnauthorized, statusOf(favorite, http.MethodGet, "/api/v1/favorites"))
}
