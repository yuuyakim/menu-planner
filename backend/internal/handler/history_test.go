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
)

// service.HistoryService が handler の要求を満たすことを保証する。
// （HistoryLister は List(ctx, string) を要求する）

// fakeHistoryLister は HistoryLister を差し替える。
type fakeHistoryLister struct {
	entries    []domain.HistoryEntry
	err        error
	lastUserID string
	calls      int
}

func (s *fakeHistoryLister) List(_ context.Context, userID string) ([]domain.HistoryEntry, error) {
	s.calls++
	s.lastUserID = userID
	return s.entries, s.err
}

func historyApp(t *testing.T, svc handler.HistoryLister) (*echo.Echo, *auth.JWT) {
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
