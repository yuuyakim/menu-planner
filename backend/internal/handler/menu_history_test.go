package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
)

// fakeMenuHistory は履歴連携を差し替え、呼び出しを記録する。
type fakeMenuHistory struct {
	recentIDs []domain.MenuID
	recentErr error
	recordErr error

	recentCalls     int
	recordCalls     int
	recordManyCalls int
	lastUserID      string
	lastRecordedID  domain.MenuID
	lastMode        domain.SearchMode
	lastManyIDs     []domain.MenuID
}

func (h *fakeMenuHistory) RecentMenuIDs(_ context.Context, userID string) ([]domain.MenuID, error) {
	h.recentCalls++
	h.lastUserID = userID
	return h.recentIDs, h.recentErr
}

func (h *fakeMenuHistory) Record(_ context.Context, userID string, menuID domain.MenuID, mode domain.SearchMode) error {
	h.recordCalls++
	h.lastUserID = userID
	h.lastRecordedID = menuID
	h.lastMode = mode
	return h.recordErr
}

func (h *fakeMenuHistory) RecordMany(_ context.Context, userID string, menuIDs []domain.MenuID, mode domain.SearchMode) error {
	h.recordManyCalls++
	h.lastUserID = userID
	h.lastManyIDs = menuIDs
	h.lastMode = mode
	return h.recordErr
}

// suggestAuthed は認証 Cookie 付きで GET /menus/suggest を叩く。
func suggestAuthed(t *testing.T, svc *fakeMenuService, hist handler.MenuHistory) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewMenuHandler(svc, hist, menuTestTokens, fakeEntitlements{plan: domain.PlanPremium}).RegisterRoutes(e)

	access, err := menuTestTokens.Issue("user-777")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/menus/suggest", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSuggest_Authed_ExcludesRecentAndRecords(t *testing.T) {
	t.Parallel()

	menu := testMenu()
	recent := []domain.MenuID{domain.NewMenuID(), domain.NewMenuID()}
	svc := &fakeMenuService{menu: menu}
	hist := &fakeMenuHistory{recentIDs: recent}

	rec := suggestAuthed(t, svc, hist)
	require.Equal(t, http.StatusOK, rec.Code)

	// 直近履歴が除外候補として service に渡る。
	require.Equal(t, 1, hist.recentCalls)
	assert.Equal(t, "user-777", hist.lastUserID)
	require.Len(t, svc.lastFilter.ExcludeIDs, 2)

	// 提案した献立が履歴に記録される（single）。
	require.Equal(t, 1, hist.recordCalls)
	assert.Equal(t, menu.ID.String(), hist.lastRecordedID.String())
	assert.Equal(t, domain.SearchModeSingle, hist.lastMode)
}

func TestSuggest_Unauthed_DoesNotTouchHistory(t *testing.T) {
	t.Parallel()

	svc := &fakeMenuService{menu: testMenu()}
	hist := &fakeMenuHistory{}

	// Cookie なし（未認証）で叩く。
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewMenuHandler(svc, hist, menuTestTokens, fakeEntitlements{plan: domain.PlanPremium}).RegisterRoutes(e)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/menus/suggest", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, hist.recentCalls, "未認証なら履歴を読まない")
	assert.Zero(t, hist.recordCalls, "未認証なら履歴に記録しない")
}

func TestSuggest_RecordFailureDoesNotFail(t *testing.T) {
	t.Parallel()

	svc := &fakeMenuService{menu: testMenu()}
	hist := &fakeMenuHistory{recordErr: errors.New("DB爆発")}

	rec := suggestAuthed(t, svc, hist)
	// 記録が失敗しても提案は 200 で返す。
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, hist.recordCalls)
}

func TestSuggest_RecentFetchFailureStillSuggests(t *testing.T) {
	t.Parallel()

	svc := &fakeMenuService{menu: testMenu()}
	hist := &fakeMenuHistory{recentErr: errors.New("読めない")}

	rec := suggestAuthed(t, svc, hist)
	require.Equal(t, http.StatusOK, rec.Code)
	// 履歴が読めなくても除外なしで提案する。
	assert.Empty(t, svc.lastFilter.ExcludeIDs)
}

func TestSuggest_HistoryExhaustionFallsBack(t *testing.T) {
	t.Parallel()

	// 履歴除外で候補が枯渇したら、除外を外して再試行する。
	menu := testMenu()
	svc := &fakeMenuService{menu: menu, noMenuWhenExcluded: true}
	hist := &fakeMenuHistory{recentIDs: []domain.MenuID{domain.NewMenuID()}}

	rec := suggestAuthed(t, svc, hist)
	require.Equal(t, http.StatusOK, rec.Code)
	// 1回目（除外あり）で枯渇、2回目（除外なし）で成功。
	require.Equal(t, 2, svc.calls)
	// 最後の呼び出しは除外なし。
	assert.Empty(t, svc.lastFilter.ExcludeIDs)
}
