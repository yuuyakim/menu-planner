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

// fakeSavedShoppingList は SavedShoppingListUseCase の fake。
type fakeSavedShoppingList struct {
	items      []service.SavedShoppingItem
	err        error
	lastUserID string
	lastWeekID string

	replaced   []service.OverrideInput
	replaceErr error
}

func (f *fakeSavedShoppingList) For(
	_ context.Context, userID, savedWeeklyMenuID string,
) ([]service.SavedShoppingItem, error) {
	f.lastUserID = userID
	f.lastWeekID = savedWeeklyMenuID
	return f.items, f.err
}

func (f *fakeSavedShoppingList) ReplaceOverrides(
	_ context.Context, userID, savedWeeklyMenuID string, inputs []service.OverrideInput,
) error {
	f.lastUserID = userID
	f.lastWeekID = savedWeeklyMenuID
	f.replaced = inputs
	return f.replaceErr
}

// savedShoppingListApp は SavedShoppingListHandler のみを積んだ echo アプリを組み立てる。
// saved_weekly_test.go の savedWeeklyApp に倣う。GET/PUT は premium 限定になった（Task 5）
// ため、ent を明示的に渡す。既存の呼び出しは premiumEnt を渡し、従来の 200/204 の期待を保つ。
func savedShoppingListApp(
	t *testing.T, svc handler.SavedShoppingListUseCase, ent fakeEntitlements,
) (*echo.Echo, *auth.JWT) {
	t.Helper()
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	require.NoError(t, err)
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewSavedShoppingListHandler(svc, tokens, ent).RegisterRoutes(e)
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

// doAuthedPut は getShoppingList に倣い、認証済みトークンを発行してから
// PUT /weekly-menus/:id/shopping-list を叩く共通ヘルパ。
func doAuthedPut(
	t *testing.T, svc handler.SavedShoppingListUseCase, ent fakeEntitlements, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	e, tokens := savedShoppingListApp(t, svc, ent)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
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
		Hidden   bool   `json:"hidden"`
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
		{
			Name: "たまねぎ", Category: domain.CategoryVegetable, Origin: domain.OriginDerived,
			Hidden: true,
		},
	}}
	e, tokens := savedShoppingListApp(t, svc, premiumEnt)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	weekID := domain.NewSavedWeeklyMenuID().String()
	rec := getShoppingList(t, e, access, weekID)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-abc", svc.lastUserID)
	assert.Equal(t, weekID, svc.lastWeekID)

	var body savedShoppingListResponseBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Items, 3)

	carrot := body.Items[0]
	assert.Equal(t, "にんじん", carrot.Name)
	assert.Equal(t, "vegetable", carrot.Category)
	assert.Equal(t, "derived", carrot.Origin)
	assert.True(t, carrot.Checked)
	assert.False(t, carrot.Hidden)
	require.Len(t, carrot.UsedIn, 1)
	assert.Equal(t, menu.ID.String(), carrot.UsedIn[0].ID)
	assert.Equal(t, "肉じゃが", carrot.UsedIn[0].Name)

	milk := body.Items[1]
	assert.Equal(t, "牛乳", milk.Name)
	assert.Equal(t, "dairy_egg", milk.Category)
	assert.Equal(t, "manual", milk.Origin)
	assert.False(t, milk.Checked)
	assert.False(t, milk.Hidden)
	assert.Empty(t, milk.UsedIn, "手動品目の usedIn は空")

	onion := body.Items[2]
	assert.Equal(t, "たまねぎ", onion.Name)
	assert.True(t, onion.Hidden, "hidden な導出品目も GET には含める（フロントが overlay を再構築するため）")
}

func TestSavedShoppingListHandler_Get_0件でもnullではなく空配列(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{items: nil}
	e, tokens := savedShoppingListApp(t, svc, premiumEnt)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := getShoppingList(t, e, access, domain.NewSavedWeeklyMenuID().String())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}

func TestSavedShoppingListHandler_Get_他人の週は404(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{err: service.ErrSavedWeeklyMenuNotFound}
	e, tokens := savedShoppingListApp(t, svc, premiumEnt)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := getShoppingList(t, e, access, domain.NewSavedWeeklyMenuID().String())

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "saved-weekly-menu-not-found")
}

func TestSavedShoppingListHandler_Get_未認証は401(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{}
	e, _ := savedShoppingListApp(t, svc, premiumEnt)

	rec := getShoppingList(t, e, "", domain.NewSavedWeeklyMenuID().String())

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, svc.lastUserID, "未認証なら service を呼ばないべき")
}

// GET/PUT の premium 判定自体は RequirePremium（Task 2）の責務なので、ここでは
// 「ハンドラのルーティングに RequireAuth → RequirePremium が実際に掛かっているか」
// だけを固定する。判定ロジック自体は middleware_test.go で見る。
func TestSavedShoppingListHandler_Get_freeは403(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{}
	e, tokens := savedShoppingListApp(t, svc, freeEnt)
	access, err := tokens.Issue("user-abc")
	require.NoError(t, err)

	rec := getShoppingList(t, e, access, domain.NewSavedWeeklyMenuID().String())

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, svc.lastUserID, "freeなら service を呼ばないべき")
}

func TestSavedShoppingListHandler_Put_204(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{}
	body := `{"items":[{"name":"にんじん","category":"vegetable","origin":"derived","checked":true,"hidden":false}]}`
	rec := doAuthedPut(t, svc, premiumEnt,
		"/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list", body)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, svc.replaced, 1)
	require.Equal(t, "にんじん", svc.replaced[0].Name)
}

func TestSavedShoppingListHandler_Put_freeは403(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{replaceErr: service.ErrPremiumRequired}
	rec := doAuthedPut(t, svc, premiumEnt,
		"/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list", `{"items":[]}`)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSavedShoppingListHandler_Put_上限で409(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{replaceErr: service.ErrShoppingListItemLimitReached}
	rec := doAuthedPut(t, svc, premiumEnt,
		"/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list", `{"items":[]}`)
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestSavedShoppingListHandler_Put_壊れたボディは400(t *testing.T) {
	t.Parallel()

	svc := &fakeSavedShoppingList{}
	rec := doAuthedPut(t, svc, premiumEnt,
		"/api/v1/weekly-menus/"+domain.NewSavedWeeklyMenuID().String()+"/shopping-list", `{`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
