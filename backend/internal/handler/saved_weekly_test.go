package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
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

// fakeSavedWeeklyService は SavedWeeklyMenuUseCase を差し替える。
type fakeSavedWeeklyService struct {
	saveCalls    int
	listCalls    int
	deleteCalls  int
	lastUserID   string
	lastDays     []service.SavedDayInput
	lastDeleteID string
	id           domain.SavedWeeklyMenuID
	saved        []domain.SavedWeeklyMenu
	err          error
}

func (s *fakeSavedWeeklyService) List(
	_ context.Context, userID string,
) ([]domain.SavedWeeklyMenu, error) {
	s.listCalls++
	s.lastUserID = userID
	return s.saved, s.err
}

func (s *fakeSavedWeeklyService) Delete(_ context.Context, userID, id string) error {
	s.deleteCalls++
	s.lastUserID = userID
	s.lastDeleteID = id
	return s.err
}

func (s *fakeSavedWeeklyService) Save(
	_ context.Context, userID string, days []service.SavedDayInput,
) (domain.SavedWeeklyMenuID, error) {
	s.saveCalls++
	s.lastUserID = userID
	s.lastDays = days
	if s.err != nil {
		return domain.SavedWeeklyMenuID{}, s.err
	}
	return s.id, nil
}

func savedWeeklyApp(t *testing.T, svc handler.SavedWeeklyMenuUseCase) (*echo.Echo, *auth.JWT) {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	require.NoError(t, err)
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewSavedWeeklyMenuHandler(svc, tokens).RegisterRoutes(e)
	return e, tokens
}

// postWeeklyMenu は認証つきで POST /weekly-menus を叩く。access が空なら未認証。
func postWeeklyMenu(t *testing.T, e *echo.Echo, access, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/weekly-menus", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if access != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// weekBody は7日分のリクエストボディを組み立てる。
func weekBody() string {
	days := make([]string, 0, domain.WeekLength)
	for day := 1; day <= domain.WeekLength; day++ {
		days = append(days,
			fmt.Sprintf(`{"day":%d,"menuId":%q}`, day, domain.NewMenuID().String()))
	}
	return `{"days":[` + strings.Join(days, ",") + `]}`
}

func TestSavedWeeklyMenus_Save_Created(t *testing.T) {
	t.Parallel()

	want := domain.NewSavedWeeklyMenuID()
	svc := &fakeSavedWeeklyService{id: want}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postWeeklyMenu(t, e, access, weekBody())

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, 1, svc.saveCalls)
	assert.Equal(t, "user-abc", svc.lastUserID)
	require.Len(t, svc.lastDays, domain.WeekLength)

	var body struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, want.String(), body.ID)
}

func TestSavedWeeklyMenus_Save_送った指定がそのままserviceに渡る(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{id: domain.NewSavedWeeklyMenuID()}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	menuID := domain.NewMenuID().String()
	body := `{"days":[{"day":3,"menuId":"` + menuID + `"}]}`
	postWeeklyMenu(t, e, access, body)

	// 日数の検証は service の仕事。handler は詰め替えるだけで弾かない。
	require.Len(t, svc.lastDays, 1)
	assert.Equal(t, 3, svc.lastDays[0].Day)
	assert.Equal(t, menuID, svc.lastDays[0].MenuID)
}

func TestSavedWeeklyMenus_Save_Unauthorized(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{}
	e, _ := savedWeeklyApp(t, svc)

	rec := postWeeklyMenu(t, e, "", weekBody())

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.saveCalls, "未認証なら service を呼ばないべき")
}

func TestSavedWeeklyMenus_Save_日数不正は400(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{err: service.ErrInvalidWeek}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postWeeklyMenu(t, e, access, `{"days":[]}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid-week")
}

func TestSavedWeeklyMenus_Save_日の指定不正は400(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{err: service.ErrInvalidDay}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postWeeklyMenu(t, e, access, weekBody())

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid-day")
}

func TestSavedWeeklyMenus_Save_壊れた献立IDは400(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{err: domain.ErrInvalidMenuID}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postWeeklyMenu(t, e, access, weekBody())

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid-menu-id")
}

func TestSavedWeeklyMenus_Save_存在しない献立は404(t *testing.T) {
	t.Parallel()

	// 献立の実在はDBの外部キーが判定し、repository がこのエラーに変換する。
	// お気に入り・買い物リストと同じく「参照先が無い」ので 404 に揃える。
	svc := &fakeSavedWeeklyService{err: repository.ErrMenuNotFound}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postWeeklyMenu(t, e, access, weekBody())

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "menu-not-found")
}

func TestSavedWeeklyMenus_Save_上限超過は409(t *testing.T) {
	t.Parallel()

	// 押し出さずに断るため、入力の誤り（400）ではなく状態との競合（409）。
	svc := &fakeSavedWeeklyService{err: service.ErrSavedWeeklyMenuLimitReached}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postWeeklyMenu(t, e, access, weekBody())

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "saved-weekly-menu-limit-reached")
}

func TestSavedWeeklyMenus_Save_壊れたJSONは400(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := postWeeklyMenu(t, e, access, `{"days":`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, svc.saveCalls)
}

// getWeeklyMenus は認証つきで GET /weekly-menus を叩く。
func getWeeklyMenus(t *testing.T, e *echo.Echo, access string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/weekly-menus", nil)
	if access != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// deleteWeeklyMenu は認証つきで DELETE /weekly-menus/:id を叩く。
func deleteWeeklyMenu(t *testing.T, e *echo.Echo, access, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/weekly-menus/"+id, nil)
	if access != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSavedWeeklyMenus_List_中身の7日分を含めて返す(t *testing.T) {
	t.Parallel()

	id := domain.NewSavedWeeklyMenuID()
	days := make([]domain.DayMenu, 0, domain.WeekLength)
	for day := 1; day <= domain.WeekLength; day++ {
		days = append(days, domain.DayMenu{Day: day, Menu: domain.Menu{
			ID:          domain.NewMenuID(),
			Name:        "献立",
			NameKana:    "こんだて",
			Genre:       domain.GenreJapanese,
			Difficulty:  domain.DifficultyEasy,
			Description: "説明",
		}})
	}
	created := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

	svc := &fakeSavedWeeklyService{saved: []domain.SavedWeeklyMenu{
		{ID: id, Days: days, CreatedAt: created},
	}}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := getWeeklyMenus(t, e, access)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		WeeklyMenus []struct {
			ID   string `json:"id"`
			Days []struct {
				Day  int `json:"day"`
				Menu struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"menu"`
			} `json:"days"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"weeklyMenus"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.WeeklyMenus, 1)
	assert.Equal(t, id.String(), body.WeeklyMenus[0].ID)
	// 「開く」に個別取得が要らないよう、一覧に7日分を含める（spec.md 5.3）。
	require.Len(t, body.WeeklyMenus[0].Days, domain.WeekLength)
	assert.Equal(t, 1, body.WeeklyMenus[0].Days[0].Day)
	assert.Equal(t, "献立", body.WeeklyMenus[0].Days[0].Menu.Name)
	assert.True(t, created.Equal(body.WeeklyMenus[0].CreatedAt))
}

func TestSavedWeeklyMenus_List_0件でもnullではなく空配列(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{saved: nil}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := getWeeklyMenus(t, e, access)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"weeklyMenus":[]`)
}

func TestSavedWeeklyMenus_List_Unauthorized(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{}
	e, _ := savedWeeklyApp(t, svc)

	rec := getWeeklyMenus(t, e, "")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.listCalls)
}

func TestSavedWeeklyMenus_Delete_NoContent(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	id := domain.NewSavedWeeklyMenuID().String()
	rec := deleteWeeklyMenu(t, e, access, id)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, 1, svc.deleteCalls)
	assert.Equal(t, "user-abc", svc.lastUserID)
	assert.Equal(t, id, svc.lastDeleteID)
}

func TestSavedWeeklyMenus_Delete_他人のものは404(t *testing.T) {
	t.Parallel()

	// 403 だと他人が何を保存しているかを漏らすため、存在しない扱いにする。
	svc := &fakeSavedWeeklyService{err: service.ErrSavedWeeklyMenuNotFound}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := deleteWeeklyMenu(t, e, access, domain.NewSavedWeeklyMenuID().String())

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "saved-weekly-menu-not-found")
}

func TestSavedWeeklyMenus_Delete_壊れたIDは400(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{err: domain.ErrInvalidSavedWeeklyMenuID}
	e, tokens := savedWeeklyApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := deleteWeeklyMenu(t, e, access, "not-a-uuid")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid-saved-weekly-menu-id")
}

func TestSavedWeeklyMenus_Delete_Unauthorized(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedWeeklyService{}
	e, _ := savedWeeklyApp(t, svc)

	rec := deleteWeeklyMenu(t, e, "", domain.NewSavedWeeklyMenuID().String())

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, svc.deleteCalls)
}
