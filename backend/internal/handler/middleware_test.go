package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// premiumRoute は RequireAuth → RequirePremium の順にミドルウェアを重ねたエコーアプリを組む。
// userIDContextKey は handler パッケージの非公開定数なので、外部（handler_test）
// からは RequireAuth 自身に有効な Cookie を検証させてコンテキストへ載せてもらう。
func premiumRoute(t *testing.T, ent service.Entitlements) (*echo.Echo, *auth.JWT, *bool) {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	require.NoError(t, err)

	called := false
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	e.GET("/premium-only", func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	}, handler.RequireAuth(tokens), handler.RequirePremium(ent))

	return e, tokens, &called
}

func TestRequirePremium_プレミアムは通す(t *testing.T) {
	t.Parallel()

	e, tokens, called := premiumRoute(t, fakeEntitlements{plan: domain.PlanPremium})
	access, err := tokens.Issue("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/premium-only", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, *called)
}

func TestRequirePremium_freeは403(t *testing.T) {
	t.Parallel()

	e, tokens, called := premiumRoute(t, fakeEntitlements{plan: domain.PlanFree})
	access, err := tokens.Issue("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/premium-only", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, *called)
}

// userID がコンテキストに無い状態（配線ミス・RequireAuth を通していない）を
// 直接検証する。RequirePremium 単体を、userID を載せていない素の echo.Context に
// 対して呼ぶだけでよい（載せなければ UserIDFromContext は自然に ok=false になる）。
func TestRequirePremium_userID無しは401(t *testing.T) {
	t.Parallel()

	mw := handler.RequirePremium(fakeEntitlements{plan: domain.PlanPremium})
	called := false
	next := func(c echo.Context) error { called = true; return nil }

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/premium-only", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := mw(next)(c)

	require.ErrorIs(t, err, auth.ErrTokenInvalid)
	require.False(t, called)
}

// エンタイトルメントの引き当て自体が失敗したら、その err をそのまま返す（500系）。
func TestRequirePremium_エンタイトルメント引き当て失敗はそのままエラーを返す(t *testing.T) {
	t.Parallel()

	wantErr := auth.ErrTokenInvalid // 何らかの sentinel error を再利用して伝播だけ確認する
	e, tokens, called := premiumRoute(t, fakeEntitlements{err: wantErr})
	access, err := tokens.Issue("11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/premium-only", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// ErrTokenInvalid は 401 にマップされる問題詳細タイプなので、経路の疎通だけ確認する。
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, *called)
}
