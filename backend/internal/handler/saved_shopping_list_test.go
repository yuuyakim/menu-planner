package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeSavedShoppingList は SavedShoppingListUseCase の fake。
type fakeSavedShoppingList struct {
	items      []service.SavedShoppingItem
	err        error
	lastUserID string
	lastWeekID string
}

func (f *fakeSavedShoppingList) For(
	_ context.Context, userID, savedWeeklyMenuID string,
) ([]service.SavedShoppingItem, error) {
	f.lastUserID = userID
	f.lastWeekID = savedWeeklyMenuID
	return f.items, f.err
}

// savedShoppingListApp は SavedShoppingListHandler のみを積んだ echo アプリを組み立てる。
// saved_weekly_test.go の savedWeeklyApp に倣う。
func savedShoppingListApp(t *testing.T, svc handler.SavedShoppingListUseCase) (*echo.Echo, *auth.JWT) {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	require.NoError(t, err)
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewSavedShoppingListHandler(svc, tokens).RegisterRoutes(e)
	return e, tokens
}

// getShoppingList は認証つきで GET /weekly-menus/:id/shopping-list を叩く。access が空なら未認証。
func getShoppingList(t *testing.T, e *echo.Echo, access, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/weekly-menus/"+id+"/shopping-list", nil)
	if access != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// savedShoppingListResponseBody はテストで検証するレスポンスの形。
type savedShoppingListResponseBody struct {
	Items []struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Origin   string `json:"origin"`
		Checked  bool   `json:"checked"`
		UsedIn   []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"usedIn"`
	} `json:"items"`
}

func TestSavedShoppingListHandler_Get_200(t *testing.T) {
	t.Parallel()

	menu := domain.Menu{ID: domain.NewMenuID(), Name: "肉じゃが"}
	svc := &fakeSavedShoppingList{items: []service.SavedShoppingItem{
		{
			Name: "にんじん", Category: domain.CategoryVegetable, Origin: domain.OriginDerived,
			Checked: true, UsedIn: []domain.Menu{menu},
		},
		{
			Name: "牛乳", Category: domain.CategoryDairyEgg, Origin: domain.OriginManual,
		},
	}}
	e, tokens := savedShoppingListApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	weekID := domain.NewSavedWeeklyMenuID().String()
	rec := getShoppingList(t, e, access, weekID)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-abc", svc.lastUserID)
	assert.Equal(t, weekID, svc.lastWeekID)

	var body savedShoppingListResponseBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Items, 2)

	carrot := body.Items[0]
	assert.Equal(t, "にんじん", carrot.Name)
	assert.Equal(t, "vegetable", carrot.Category)
	assert.Equal(t, "derived", carrot.Origin)
	assert.True(t, carrot.Checked)
	require.Len(t, carrot.UsedIn, 1)
	assert.Equal(t, menu.ID.String(), carrot.UsedIn[0].ID)
	assert.Equal(t, "肉じゃが", carrot.UsedIn[0].Name)

	milk := body.Items[1]
	assert.Equal(t, "牛乳", milk.Name)
	assert.Equal(t, "dairy_egg", milk.Category)
	assert.Equal(t, "manual", milk.Origin)
	assert.False(t, milk.Checked)
	assert.Empty(t, milk.UsedIn, "手動品目の usedIn は空")
}

func TestSavedShoppingListHandler_Get_0件でもnullではなく空配列(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{items: nil}
	e, tokens := savedShoppingListApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := getShoppingList(t, e, access, domain.NewSavedWeeklyMenuID().String())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}

func TestSavedShoppingListHandler_Get_他人の週は404(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{err: service.ErrSavedWeeklyMenuNotFound}
	e, tokens := savedShoppingListApp(t, svc)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := getShoppingList(t, e, access, domain.NewSavedWeeklyMenuID().String())

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "saved-weekly-menu-not-found")
}

func TestSavedShoppingListHandler_Get_未認証は401(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{}
	e, _ := savedShoppingListApp(t, svc)

	rec := getShoppingList(t, e, "", domain.NewSavedWeeklyMenuID().String())

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, svc.lastUserID, "未認証なら service を呼ばないべき")
}
