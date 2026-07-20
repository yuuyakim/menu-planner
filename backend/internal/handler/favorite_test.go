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
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeFavoriteService は FavoriteUseCase を差し替える。
type fakeFavoriteService struct {
	lastUserID  string
	lastMenuID  string
	addCalls    int
	listCalls   int
	deleteCalls int
	favorites   []domain.Favorite
	listErr     error
	deleteErr   error
	err         error
}

func (s *fakeFavoriteService) Add(_ context.Context, userID, menuID string) error {
	s.addCalls++
	s.lastUserID = userID
	s.lastMenuID = menuID
	return s.err
}

func (s *fakeFavoriteService) List(_ context.Context, userID string) ([]domain.Favorite, error) {
	s.listCalls++
	s.lastUserID = userID
	return s.favorites, s.listErr
}

func (s *fakeFavoriteService) Delete(_ context.Context, userID, menuID string) error {
	s.deleteCalls++
	s.lastUserID = userID
	s.lastMenuID = menuID
	return s.deleteErr
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

// favoriteOf はテスト用のお気に入り1件を作る。
func favoriteOf(name string, at time.Time) domain.Favorite {
	return domain.Favorite{
		Menu: domain.Menu{
			ID:          domain.NewMenuID(),
			Name:        name,
			NameKana:    name + "かな",
			Genre:       domain.GenreJapanese,
			Difficulty:  domain.DifficultyEasy,
			Description: name + "の説明",
		},
		CreatedAt: at,
	}
}

func TestFavorites_List_OK(t *testing.T) {
	t.Parallel()

	now := time.Now()
	svc := &fakeFavoriteService{favorites: []domain.Favorite{
		favoriteOf("カレー", now),
		favoriteOf("親子丼", now.Add(-time.Hour)),
	}}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-abc", svc.lastUserID)

	var body struct {
		Favorites []struct {
			Menu      struct{ Name string } `json:"menu"`
			CreatedAt time.Time             `json:"createdAt"`
		} `json:"favorites"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Favorites, 2)
	assert.Equal(t, "カレー", body.Favorites[0].Menu.Name)
	assert.False(t, body.Favorites[0].CreatedAt.IsZero())
}

func TestFavorites_List_EmptyIsArray(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{favorites: nil}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"favorites":[]`, "null ではなく [] を返す")
}

func TestFavorites_List_Unauthenticated401(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{}
	e, _ := favoriteApp(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.listCalls)
}

// deleteFavorite は認証つきで DELETE /favorites/:menuId を叩く。
func deleteFavorite(t *testing.T, e *echo.Echo, access, menuID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/favorites/"+menuID, nil)
	if access != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestFavorites_Delete_NoContent(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	menuID := domain.NewMenuID().String()
	rec := deleteFavorite(t, e, access, menuID)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, 1, svc.deleteCalls)
	assert.Equal(t, "user-abc", svc.lastUserID)
	assert.Equal(t, menuID, svc.lastMenuID)
}

func TestFavorites_Delete_NotFound404(t *testing.T) {
	t.Parallel()

	// 他ユーザーのお気に入りもこの経路に入る（自分の (user, menu) が無い＝404）。
	svc := &fakeFavoriteService{deleteErr: service.ErrFavoriteNotFound}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := deleteFavorite(t, e, access, domain.NewMenuID().String())

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFavorites_Delete_Unauthenticated401(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{}
	e, _ := favoriteApp(t, svc)

	rec := deleteFavorite(t, e, "", domain.NewMenuID().String())

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.deleteCalls)
}

func TestFavorites_Delete_InvalidMenuID400(t *testing.T) {
	t.Parallel()

	svc := &fakeFavoriteService{deleteErr: domain.ErrInvalidMenuID}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := deleteFavorite(t, e, access, "not-a-uuid")

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
