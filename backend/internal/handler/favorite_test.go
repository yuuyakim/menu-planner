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
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeFavoriteService は FavoriteUseCase を差し替える。
type fakeFavoriteService struct {
	lastUserID string
	lastMenuID string
	addCalls   int
	err        error
}

func (s *fakeFavoriteService) Add(_ context.Context, userID, menuID string) error {
	s.addCalls++
	s.lastUserID = userID
	s.lastMenuID = menuID
	return s.err
}

func favoriteApp(t *testing.T, svc handler.FavoriteUseCase) (*echo.Echo, *auth.JWT) {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	require.NoError(t, err)
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewFavoriteHandler(svc, tokens).RegisterRoutes(e)
	return e, tokens
}

// postFavorite は認証つきで POST /favorites を叩く。access が空なら未認証。
func postFavorite(t *testing.T, e *echo.Echo, access, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/favorites", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if access != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestFavorites_Add_Created(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	menuID := domain.NewMenuID().String()
	rec := postFavorite(t, e, access, `{"menuId":"`+menuID+`"}`)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, 1, svc.addCalls)
	assert.Equal(t, "user-abc", svc.lastUserID)
	assert.Equal(t, menuID, svc.lastMenuID)

	var body struct {
		MenuID string `json:"menuId"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, menuID, body.MenuID)
}

func TestFavorites_Add_Duplicate409(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{err: service.ErrFavoriteExists}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postFavorite(t, e, access, `{"menuId":"`+domain.NewMenuID().String()+`"}`)

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, handler.ProblemContentType, rec.Header().Get(echo.HeaderContentType))
}

func TestFavorites_Add_UnknownMenu404(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{err: repository.ErrMenuNotFound}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postFavorite(t, e, access, `{"menuId":"`+domain.NewMenuID().String()+`"}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFavorites_Add_Unauthenticated401(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{}
	e, _ := favoriteApp(t, svc)

	rec := postFavorite(t, e, "", `{"menuId":"`+domain.NewMenuID().String()+`"}`)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.addCalls, "未認証なら service を呼ばない")
}

func TestFavorites_Add_InvalidMenuID400(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{err: domain.ErrInvalidMenuID}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postFavorite(t, e, access, `{"menuId":"not-a-uuid"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFavorites_Add_MalformedBody400(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postFavorite(t, e, access, `{`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, svc.addCalls)
}
