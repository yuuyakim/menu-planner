package handler_test

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
)

// doGoogleStart は GET /auth/google を1本実行してレスポンスを返す。
func doGoogleStart(t *testing.T, google *auth.GoogleOAuth) *httptest.ResponseRecorder {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewAuthHandler(&fakeAuthService{}, tokens, google, testFrontendURL,
		fakeEntitlements{plan: domain.PlanFree}).RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestGoogleStart_RedirectsWithPKCEAndState(t *testing.T) {
	t.Parallel()

	rec := doGoogleStart(t, testGoogleOAuth())
	require.Equal(t, http.StatusFound, rec.Code)

	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "accounts.google.com", loc.Host)

	q := loc.Query()
	// PKCE の code_challenge と S256 が付く。
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.NotEmpty(t, q.Get("code_challenge"))

	// state が生成され、Cookie に保存され、URL の state と一致する。
	stateCookie := findCookie(rec, "oauth_state")
	require.NotNil(t, stateCookie, "state が Cookie に保存されるべき")
	assert.Equal(t, q.Get("state"), stateCookie.Value, "URL の state と Cookie が一致するべき")
	assert.NotEmpty(t, stateCookie.Value)

	// verifier も Cookie に保存され、URL の code_challenge はその S256。
	verifierCookie := findCookie(rec, "oauth_verifier")
	require.NotNil(t, verifierCookie, "verifier が Cookie に保存されるべき")
	sum := sha256.Sum256([]byte(verifierCookie.Value))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	assert.Equal(t, wantChallenge, q.Get("code_challenge"))
}

func TestGoogleStart_CookieAttributes(t *testing.T) {
	t.Parallel()

	rec := doGoogleStart(t, testGoogleOAuth())

	for _, name := range []string{"oauth_state", "oauth_verifier"} {
		ck := findCookie(rec, name)
		require.NotNil(t, ck, name)
		assert.True(t, ck.HttpOnly, "%s は HttpOnly であるべき", name)
		assert.True(t, ck.Secure, "%s は Secure であるべき", name)
		// コールバックはトップレベル遷移なので Lax（Strict だと Cookie が送られない）。
		assert.Equal(t, http.SameSiteLaxMode, ck.SameSite, "%s は SameSite=Lax であるべき", name)
		assert.Equal(t, "/api/v1/auth", ck.Path, "%s は /auth 配下に絞るべき", name)
		assert.Positive(t, ck.MaxAge)
	}
}

func TestGoogleStart_StateDiffersEachTime(t *testing.T) {
	t.Parallel()

	rec1 := doGoogleStart(t, testGoogleOAuth())
	rec2 := doGoogleStart(t, testGoogleOAuth())

	s1 := findCookie(rec1, "oauth_state")
	s2 := findCookie(rec2, "oauth_state")
	require.NotNil(t, s1)
	require.NotNil(t, s2)
	assert.NotEqual(t, s1.Value, s2.Value, "state は毎回異なるべき")
}

func TestGoogleStart_NotConfigured(t *testing.T) {
	t.Parallel()

	// client_id 未設定なら Google ログインは使えない。503 を返す。
	notConfigured := auth.NewGoogleOAuth("", "", "")
	rec := doGoogleStart(t, notConfigured)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
