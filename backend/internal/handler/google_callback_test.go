package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
)

// service.AuthService が handler の要求する認証ユースケースを満たすことを保証する。
var _ handler.AuthUseCase = (*fakeAuthService)(nil)

// fakeGoogle は GoogleAuthenticator を差し替え、Google に接続せず結果を返す。
type fakeGoogle struct {
	configured  bool
	identity    auth.GoogleIdentity
	exchangeErr error

	lastCode      string
	lastVerifier  string
	exchangeCalls int
}

func (g *fakeGoogle) Configured() bool { return g.configured }
func (g *fakeGoogle) AuthCodeURL(_, _ string) string {
	return "https://accounts.google.com/o/oauth2/auth"
}
func (g *fakeGoogle) Exchange(_ context.Context, code, verifier string) (auth.GoogleIdentity, error) {
	g.exchangeCalls++
	g.lastCode = code
	g.lastVerifier = verifier
	if g.exchangeErr != nil {
		return auth.GoogleIdentity{}, g.exchangeErr
	}
	return g.identity, nil
}

// callbackApp は fakeGoogle を差し込んだ echo アプリを返す。
func callbackApp(t *testing.T, svc handler.AuthUseCase, g *fakeGoogle) *echo.Echo {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewAuthHandler(svc, tokens, g, testFrontendURL).RegisterRoutes(e)
	return e
}

// callbackReq は state / verifier Cookie を載せてコールバックを叩く。
func callbackReq(query string, stateCookie, verifierCookie string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback"+query, nil)
	if stateCookie != "" {
		req.AddCookie(&http.Cookie{Name: "oauth_state", Value: stateCookie})
	}
	if verifierCookie != "" {
		req.AddCookie(&http.Cookie{Name: "oauth_verifier", Value: verifierCookie})
	}
	return req
}

func verifiedIdentity() auth.GoogleIdentity {
	return auth.GoogleIdentity{Sub: "google-sub-1", Email: "taro@gmail.com", EmailVerified: true, Name: "太郎"}
}

func TestGoogleCallback_Success(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{googleUser: newTestUser(t, "taro@gmail.com")}
	g := &fakeGoogle{configured: true, identity: verifiedIdentity()}
	e := callbackApp(t, svc, g)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, callbackReq("?code=the-code&state=st4te", "st4te", "the-verifier"))

	// フロントへリダイレクトし、認証 Cookie を発行する。
	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, testFrontendURL, rec.Header().Get("Location"))
	require.NotNil(t, findCookie(rec, "access_token"))
	require.NotNil(t, findCookie(rec, "refresh_token"))

	// 途中状態の Cookie は失効する。
	assert.Negative(t, findCookie(rec, "oauth_state").MaxAge)

	// code と verifier が Exchange に渡る。upsert に sub が渡る。
	assert.Equal(t, "the-code", g.lastCode)
	assert.Equal(t, "the-verifier", g.lastVerifier)
	assert.Equal(t, "google-sub-1", svc.lastGoogleSub)
}

func TestGoogleCallback_StateMismatch(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	g := &fakeGoogle{configured: true, identity: verifiedIdentity()}
	e := callbackApp(t, svc, g)

	rec := httptest.NewRecorder()
	// URL の state と Cookie の state が食い違う。
	e.ServeHTTP(rec, callbackReq("?code=c&state=attacker", "real-state", "v"))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, g.exchangeCalls, "state 不一致なら Exchange しない")
}

func TestGoogleCallback_NoStateCookie(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	g := &fakeGoogle{configured: true, identity: verifiedIdentity()}
	e := callbackApp(t, svc, g)

	rec := httptest.NewRecorder()
	// state Cookie が無い。
	e.ServeHTTP(rec, callbackReq("?code=c&state=s", "", "v"))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, g.exchangeCalls)
}

func TestGoogleCallback_ExchangeFails(t *testing.T) {
	t.Parallel()

	// verifier 不一致などで Exchange が失敗すると 401。
	svc := &fakeAuthService{}
	g := &fakeGoogle{configured: true, exchangeErr: auth.ErrGoogleAuthFailed}
	e := callbackApp(t, svc, g)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, callbackReq("?code=c&state=s", "s", "wrong-verifier"))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.googleCalls, "交換に失敗したら upsert しない")
}

func TestGoogleCallback_UnverifiedEmailRejected(t *testing.T) {
	t.Parallel()

	// メール未確認は紐付け乗っ取りに使えるため拒否する。
	svc := &fakeAuthService{}
	g := &fakeGoogle{configured: true, identity: auth.GoogleIdentity{
		Sub: "s", Email: "taro@gmail.com", EmailVerified: false, Name: "太郎",
	}}
	e := callbackApp(t, svc, g)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, callbackReq("?code=c&state=s", "s", "v"))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.googleCalls)
}

func TestGoogleCallback_NotConfigured(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	g := &fakeGoogle{configured: false}
	e := callbackApp(t, svc, g)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, callbackReq("?code=c&state=s", "s", "v"))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
