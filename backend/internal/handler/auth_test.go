package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// service.AuthService が handler の要求するインターフェースを満たすことを
// コンパイル時に保証する。
var _ handler.AuthUseCase = (*service.AuthService)(nil)

// fakeAuthService は service.AuthService の代わりに定型の結果を返す。
// SignUp と Login で返り値・記録を分け、片方のテストが他方に引きずられないようにする。
type fakeAuthService struct {
	user domain.User
	err  error

	lastEmail    string
	lastPassword string
	calls        int

	loginUser     domain.User
	loginErr      error
	lastLoginMail string
	lastLoginPass string
	loginCalls    int

	currentUser   domain.User
	currentErr    error
	lastCurrentID string
	currentCalls  int
}

func (s *fakeAuthService) SignUp(_ context.Context, email, password string) (domain.User, error) {
	s.calls++
	s.lastEmail = email
	s.lastPassword = password
	if s.err != nil {
		return domain.User{}, s.err
	}
	return s.user, nil
}

func (s *fakeAuthService) Login(_ context.Context, email, password string) (domain.User, error) {
	s.loginCalls++
	s.lastLoginMail = email
	s.lastLoginPass = password
	if s.loginErr != nil {
		return domain.User{}, s.loginErr
	}
	return s.loginUser, nil
}

func (s *fakeAuthService) CurrentUser(_ context.Context, userID string) (domain.User, error) {
	s.lastCurrentID = userID
	s.currentCalls++
	if s.currentErr != nil {
		return domain.User{}, s.currentErr
	}
	return s.currentUser, nil
}

// newTestUser はレスポンス検証用のユーザーを作る。
func newTestUser(t *testing.T, email string) domain.User {
	t.Helper()
	e, err := domain.NewEmail(email)
	require.NoError(t, err)
	u, err := domain.NewUser(e)
	require.NoError(t, err)
	return u
}

// authTestSecret はハンドラテスト用の JWT 秘密鍵。
const authTestSecret = "handler-test-secret-please-ignore-1234"

// newAuthApp は AuthHandler を登録した echo アプリと、そのトークン発行器を返す。
func newAuthApp(t *testing.T, svc handler.AuthUseCase, opts ...auth.JWTOption) (*echo.Echo, *auth.JWT) {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret), opts...)
	require.NoError(t, err)

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewAuthHandler(svc, tokens).RegisterRoutes(e)
	return e, tokens
}

// doSignUp はサインアップのリクエストを1本実行してレスポンスを返す。
func doSignUp(t *testing.T, svc handler.AuthUseCase, body string) *httptest.ResponseRecorder {
	t.Helper()
	e, _ := newAuthApp(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// findCookie はレスポンスの Set-Cookie から指定名の Cookie を探す。
func findCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == name {
			return ck
		}
	}
	return nil
}

func TestSignUp_Created(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{user: newTestUser(t, "taro@example.com")}
	rec := doSignUp(t, svc, `{"email":"taro@example.com","password":"supersecret"}`)

	require.Equal(t, http.StatusCreated, rec.Code)

	// service に生の入力がそのまま渡る（検証は service の責務）。
	require.Equal(t, 1, svc.calls)
	assert.Equal(t, "taro@example.com", svc.lastEmail)
	assert.Equal(t, "supersecret", svc.lastPassword)

	// レスポンスは user を包み、パスワード類は含めない。
	var got struct {
		User struct {
			ID          string `json:"id"`
			Email       string `json:"email"`
			DisplayName string `json:"displayName"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "taro@example.com", got.User.Email)
	assert.Equal(t, "taro", got.User.DisplayName)
	assert.NotEmpty(t, got.User.ID)
	assert.NotContains(t, rec.Body.String(), "password")
	assert.NotContains(t, rec.Body.String(), "hash")
}

func TestSignUp_DuplicateEmailConflict(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{err: service.ErrEmailTaken}
	rec := doSignUp(t, svc, `{"email":"dup@example.com","password":"supersecret"}`)

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, handler.ProblemContentType, rec.Header().Get(echo.HeaderContentType))
}

func TestSignUp_InvalidEmailBadRequest(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{err: domain.ErrInvalidEmail}
	rec := doSignUp(t, svc, `{"email":"not-an-email","password":"supersecret"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSignUp_ShortPasswordBadRequest(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{err: auth.ErrPasswordTooShort}
	rec := doSignUp(t, svc, `{"email":"taro@example.com","password":"1234567"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSignUp_MalformedJSON(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	rec := doSignUp(t, svc, `{"email":`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	// 壊れたボディは service を呼ばない。
	assert.Zero(t, svc.calls)
}

// doLogin はログインのリクエストを1本実行してレスポンスを返す。
func doLogin(t *testing.T, svc handler.AuthUseCase, body string) *httptest.ResponseRecorder {
	t.Helper()
	e, _ := newAuthApp(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestLogin_OK(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{loginUser: newTestUser(t, "taro@example.com")}
	rec := doLogin(t, svc, `{"email":"taro@example.com","password":"supersecret"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, svc.loginCalls)
	assert.Equal(t, "taro@example.com", svc.lastLoginMail)
	assert.Equal(t, "supersecret", svc.lastLoginPass)

	// レスポンスは user を包み、パスワード類は含めない。
	assert.Contains(t, rec.Body.String(), "taro@example.com")
	assert.NotContains(t, rec.Body.String(), "password")
	assert.NotContains(t, rec.Body.String(), "hash")
}

func TestLogin_InvalidCredentialsUnauthorized(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{loginErr: service.ErrInvalidCredentials}
	rec := doLogin(t, svc, `{"email":"taro@example.com","password":"wrongpass"}`)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, handler.ProblemContentType, rec.Header().Get(echo.HeaderContentType))
}

func TestLogin_InvalidEmailBadRequest(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{loginErr: domain.ErrInvalidEmail}
	rec := doLogin(t, svc, `{"email":"not-an-email","password":"whatever12"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLogin_MalformedJSON(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	rec := doLogin(t, svc, `{"email":`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, svc.loginCalls)
}

func TestLogin_SetsAuthCookies(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{loginUser: newTestUser(t, "taro@example.com")}
	rec := doLogin(t, svc, `{"email":"taro@example.com","password":"supersecret"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	access := findCookie(rec, "access_token")
	require.NotNil(t, access, "アクセス Cookie が発行されるべき")
	assert.NotEmpty(t, access.Value)
	assert.True(t, access.HttpOnly, "HttpOnly であるべき")
	assert.True(t, access.Secure, "Secure であるべき")
	assert.Equal(t, http.SameSiteLaxMode, access.SameSite, "SameSite=Lax であるべき")
	assert.Equal(t, "/", access.Path)
	assert.Positive(t, access.MaxAge, "寿命が設定されるべき")

	refresh := findCookie(rec, "refresh_token")
	require.NotNil(t, refresh, "リフレッシュ Cookie が発行されるべき")
	assert.True(t, refresh.HttpOnly)
	assert.True(t, refresh.Secure)
	assert.Equal(t, http.SameSiteLaxMode, refresh.SameSite)
	// リフレッシュは /auth 配下にだけ送る。
	assert.Equal(t, "/api/v1/auth", refresh.Path)
	// 30日のほうがアクセス(15分)より寿命が長い。
	assert.Greater(t, refresh.MaxAge, access.MaxAge)
}

func TestSignUp_SetsAuthCookies(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{user: newTestUser(t, "taro@example.com")}
	rec := doSignUp(t, svc, `{"email":"taro@example.com","password":"supersecret"}`)
	require.Equal(t, http.StatusCreated, rec.Code)

	// サインアップでも自動ログインとして Cookie を発行する。
	require.NotNil(t, findCookie(rec, "access_token"))
	require.NotNil(t, findCookie(rec, "refresh_token"))
}

func TestRefresh_RenewsAccessCookie(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	e, tokens := newAuthApp(t, svc)

	// 正当なリフレッシュトークンを Cookie に載せて送る。
	refresh, err := tokens.IssueRefresh("user-123")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refresh})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)

	// 新しいアクセス Cookie が返り、その中身は user-123 のアクセストークン。
	access := findCookie(rec, "access_token")
	require.NotNil(t, access)
	claims, err := tokens.Verify(access.Value)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
}

func TestRefresh_MissingCookieUnauthorized(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	e, _ := newAuthApp(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRefresh_ExpiredTokenUnauthorized(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 発行時刻を過去に固定してリフレッシュトークンを作る。
	_, issuer := newAuthApp(t, svc, auth.WithNow(func() time.Time { return base }))
	expired, err := issuer.IssueRefresh("user-123")
	require.NoError(t, err)

	// 検証側は30日より後の「今」で動かす。
	e, _ := newAuthApp(t, svc, auth.WithNow(func() time.Time { return base.Add(31 * 24 * time.Hour) }))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: expired})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRefresh_AccessTokenRejected(t *testing.T) {
	t.Parallel()

	// アクセストークンをリフレッシュ Cookie に載せても再発行できない（種別違い）。
	svc := &fakeAuthService{}
	e, tokens := newAuthApp(t, svc)
	access, err := tokens.Issue("user-123")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestLogout_ClearsCookies(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	e, _ := newAuthApp(t, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)

	// 認証 Cookie が即時失効する（MaxAge<0）。
	access := findCookie(rec, "access_token")
	require.NotNil(t, access)
	assert.Negative(t, access.MaxAge, "アクセス Cookie が失効するべき")
	assert.Empty(t, access.Value)

	refresh := findCookie(rec, "refresh_token")
	require.NotNil(t, refresh)
	assert.Negative(t, refresh.MaxAge, "リフレッシュ Cookie が失効するべき")
	assert.Equal(t, "/api/v1/auth", refresh.Path)
}

func TestMe_ReturnsCurrentUser(t *testing.T) {
	t.Parallel()

	user := newTestUser(t, "taro@example.com")
	svc := &fakeAuthService{currentUser: user}
	e, tokens := newAuthApp(t, svc)

	// 有効なアクセストークンを Cookie に載せる。
	access, err := tokens.Issue(user.ID.String())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// ミドルウェアが検証した userID が service に渡る。
	assert.Equal(t, user.ID.String(), svc.lastCurrentID)
	assert.Contains(t, rec.Body.String(), "taro@example.com")
}

func TestMe_NoCookieUnauthorized(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	e, _ := newAuthApp(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	// ミドルウェアで弾かれ、service は呼ばれない。
	assert.Zero(t, svc.currentCalls)
}

func TestMe_InvalidTokenUnauthorized(t *testing.T) {
	t.Parallel()

	svc := &fakeAuthService{}
	e, _ := newAuthApp(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "garbage.token.value"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.currentCalls)
}

func TestMe_RefreshTokenRejected(t *testing.T) {
	t.Parallel()

	// リフレッシュトークンをアクセス Cookie に載せても認証は通らない（種別違い）。
	svc := &fakeAuthService{}
	e, tokens := newAuthApp(t, svc)
	refresh, err := tokens.IssueRefresh("user-123")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: refresh})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.currentCalls)
}
