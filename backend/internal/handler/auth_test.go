package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
var _ handler.UserSignUpper = (*service.AuthService)(nil)

// fakeSignUpper は service.AuthService の代わりに定型の結果を返す。
type fakeSignUpper struct {
	user domain.User
	err  error

	lastEmail    string
	lastPassword string
	calls        int
}

func (s *fakeSignUpper) SignUp(_ context.Context, email, password string) (domain.User, error) {
	s.calls++
	s.lastEmail = email
	s.lastPassword = password
	if s.err != nil {
		return domain.User{}, s.err
	}
	return s.user, nil
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

// doSignUp はサインアップのリクエストを1本実行してレスポンスを返す。
func doSignUp(t *testing.T, svc handler.UserSignUpper, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewAuthHandler(svc).RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSignUp_Created(t *testing.T) {
	t.Parallel()

	svc := &fakeSignUpper{user: newTestUser(t, "taro@example.com")}
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

	svc := &fakeSignUpper{err: service.ErrEmailTaken}
	rec := doSignUp(t, svc, `{"email":"dup@example.com","password":"supersecret"}`)

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, handler.ProblemContentType, rec.Header().Get(echo.HeaderContentType))
}

func TestSignUp_InvalidEmailBadRequest(t *testing.T) {
	t.Parallel()

	svc := &fakeSignUpper{err: domain.ErrInvalidEmail}
	rec := doSignUp(t, svc, `{"email":"not-an-email","password":"supersecret"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSignUp_ShortPasswordBadRequest(t *testing.T) {
	t.Parallel()

	svc := &fakeSignUpper{err: auth.ErrPasswordTooShort}
	rec := doSignUp(t, svc, `{"email":"taro@example.com","password":"1234567"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSignUp_MalformedJSON(t *testing.T) {
	t.Parallel()

	svc := &fakeSignUpper{}
	rec := doSignUp(t, svc, `{"email":`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	// 壊れたボディは service を呼ばない。
	assert.Zero(t, svc.calls)
}
