package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// fakeHistoryLister は HistoryUseCase を差し替える。
type fakeHistoryLister struct {
	entries       []domain.HistoryEntry
	err           error
	deleteErr     error
	lastUserID    string
	lastHistoryID string
	calls         int
	deleteCalls   int
	deleteAllCall int
}

func (s *fakeHistoryLister) List(_ context.Context, userID string) ([]domain.HistoryEntry, error) {
	s.calls++
	s.lastUserID = userID
	return s.entries, s.err
}

func (s *fakeHistoryLister) Delete(_ context.Context, userID, historyID string) error {
	s.deleteCalls++
	s.lastUserID = userID
	s.lastHistoryID = historyID
	return s.deleteErr
}

func (s *fakeHistoryLister) DeleteAll(_ context.Context, userID string) error {
	s.deleteAllCall++
	s.lastUserID = userID
	return s.deleteErr
}

func historyApp(t *testing.T, svc handler.HistoryUseCase) (*echo.Echo, *auth.JWT) {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	require.NoError(t, err)
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewHistoryHandler(svc, tokens).RegisterRoutes(e)
	return e, tokens
}

func historyEntry(t *testing.T, name string, mode domain.SearchMode, at time.Time) domain.HistoryEntry {
	t.Helper()
	menu := domain.Menu{
		ID:          domain.NewMenuID(),
		Name:        name,
		NameKana:    name + "かな",
		Genre:       domain.GenreJapanese,
		Difficulty:  domain.DifficultyEasy,
		Description: name + "の説明",
	}
	id, err := domain.ParseHistoryID("018f0000-0000-7000-8000-000000000009")
	require.NoError(t, err)
	return domain.HistoryEntry{ID: id, Menu: menu, Mode: mode, SearchedAt: at}
}

func TestHistories_List_OK(t *testing.T) {
	t.Parallel()

	entries := []domain.HistoryEntry{
		historyEntry(t, "親子丼", domain.SearchModeSingle, time.Now()),
		historyEntry(t, "カレー", domain.SearchModeWeekly, time.Now()),
	}
	svc := &fakeHistoryLister{entries: entries}
	e, tokens := historyApp(t, svc)

	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/histories", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// 認証済み userID が service に渡る。
	assert.Equal(t, "user-abc", svc.lastUserID)

	var body struct {
		Histories []struct {
			ID         string                `json:"id"`
			Menu       struct{ Name string } `json:"menu"`
			SearchMode string                `json:"searchMode"`
		} `json:"histories"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Histories, 2)
	assert.Equal(t, "親子丼", body.Histories[0].Menu.Name)
	assert.Equal(t, "single", body.Histories[0].SearchMode)
	assert.NotEmpty(t, body.Histories[0].ID)
}

func TestHistories_List_Unauthenticated(t *testing.T) {
	t.Parallel()

	svc := &fakeHistoryLister{}
	e, _ := historyApp(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/histories", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.calls, "未認証なら service を呼ばない")
}

func TestHistories_List_EmptyIsArray(t *testing.T) {
	t.Parallel()

	svc := &fakeHistoryLister{entries: nil}
	e, tokens := historyApp(t, svc)

	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/histories", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// null ではなく [] を返す。
	assert.Contains(t, rec.Body.String(), `"histories":[]`)
}

// doHistoryDelete は認証つきで DELETE を1本実行する。
func doHistoryDelete(t *testing.T, svc handler.HistoryUseCase, path string) *httptest.ResponseRecorder {
	t.Helper()
	e, tokens := historyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestHistories_Delete_OK(t *testing.T) {
	t.Parallel()

	svc := &fakeHistoryLister{}
	rec := doHistoryDelete(t, svc, "/api/v1/histories/018f0000-0000-7000-8000-000000000001")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, 1, svc.deleteCalls)
	assert.Equal(t, "user-abc", svc.lastUserID)
	assert.Equal(t, "018f0000-0000-7000-8000-000000000001", svc.lastHistoryID)
}

func TestHistories_Delete_NotFound(t *testing.T) {
	t.Parallel()

	svc := &fakeHistoryLister{deleteErr: service.ErrHistoryNotFound}
	rec := doHistoryDelete(t, svc, "/api/v1/histories/018f0000-0000-7000-8000-000000000001")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHistories_Delete_Forbidden(t *testing.T) {
	t.Parallel()

	svc := &fakeHistoryLister{deleteErr: service.ErrHistoryForbidden}
	rec := doHistoryDelete(t, svc, "/api/v1/histories/018f0000-0000-7000-8000-000000000001")
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHistories_Delete_Unauthenticated(t *testing.T) {
	t.Parallel()

	svc := &fakeHistoryLister{}
	e, _ := historyApp(t, svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/histories/018f0000-0000-7000-8000-000000000001", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.deleteCalls)
}

func TestHistories_DeleteAll_OK(t *testing.T) {
	t.Parallel()

	svc := &fakeHistoryLister{}
	rec := doHistoryDelete(t, svc, "/api/v1/histories")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, 1, svc.deleteAllCall)
	// 静的な /histories が :id に飲み込まれていない。
	assert.Zero(t, svc.deleteCalls)
}
