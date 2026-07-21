package handler_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/errors"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// API仕様（api/openapi.yaml）と実装のズレを検出する。
//
// 8-B2 で入れたCIの確認は「yaml と 生成されたTSの型」の一致しか見ていない。
// yaml を直し忘れたまま実装だけ変えても気づけないため、実際の応答を
// 仕様に照らす。仕様が「単一の情報源」であるためには、実装がそれに
// 従っていることを機械的に確かめる必要がある。

// specPath は API 仕様の場所。テストは internal/handler から実行される。
const specPath = "../../../api/openapi.yaml"

// newContractApp は契約検証に使う echo アプリを組み立てる。
// 既存の fake をそのまま使い、応答の形だけを仕様と突き合わせる。
func newContractApp(s *fakeMenuService) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewMenuHandler(s, noopMenuHistory{}, menuTestTokens).RegisterRoutes(e)
	return e
}

// newContractValidator は仕様を読み込んで検証器を作る。
func newContractValidator(t *testing.T) validator.Validator {
	t.Helper()

	raw, err := os.ReadFile(filepath.Clean(specPath))
	require.NoError(t, err, "API仕様を読めません")

	doc, err := libopenapi.NewDocument(raw)
	require.NoError(t, err, "API仕様を解釈できません")

	v, errs := validator.NewValidator(doc)
	require.Empty(t, errs, "API仕様自体が不正です")

	return v
}

// assertMatchesSpec はリクエストと応答の両方が仕様どおりかを確かめる。
//
// libopenapi-validator は経路・ステータス・本文の形をまとめて検証する。
// 仕様に無い経路や、必須項目の欠落、型の違いはここで落ちる。
func assertMatchesSpec(t *testing.T, v validator.Validator, req *http.Request, body string, rec *httptest.ResponseRecorder) {
	t.Helper()
	rewind(req, body)
	ok, errs := v.ValidateHttpRequestResponse(req, rec.Result())
	report(t, ok, errs)
}

// rewind はリクエストの本文を読み直せるように戻す。
// ハンドラが既に読み切っているため、そのまま検証に渡すと
// 「本文が空だ」と判定されてしまう。
func rewind(req *http.Request, body string) {
	if body == "" {
		return
	}
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))
}

// assertResponseMatchesSpec は応答だけを仕様と突き合わせる。
//
// エラー経路のテストでは、リクエスト自体を意図的に仕様違反にする
// （例: 定義に無いジャンル）。それをリクエスト検証にかけると
// 「不正なリクエストだ」と当然のことを言われるだけなので、
// ここで見たい「その入力に対する応答の形」だけを確かめる。
func assertResponseMatchesSpec(t *testing.T, v validator.Validator, req *http.Request, rec *httptest.ResponseRecorder) {
	t.Helper()
	ok, errs := v.ValidateHttpResponse(req, rec.Result())
	report(t, ok, errs)
}

// report は検証結果を読める形で出す。
func report(t *testing.T, ok bool, errs []*errors.ValidationError) {
	t.Helper()
	if ok {
		return
	}
	for _, e := range errs {
		t.Errorf("仕様と一致しません: %s\n  理由: %s", e.Message, e.Reason)
		for _, se := range e.SchemaValidationErrors {
			t.Errorf("  該当箇所: %s", se.Error())
		}
	}
	t.FailNow()
}

// contractRequest は仕様検証用のリクエストを作る。
// libopenapi-validator は経路の突き合わせに Host を使うため、
// httptest の既定（example.com）のままにする。
func contractRequest(method, target string, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	return req
}

func TestContract_MenuSuggest(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	e := newContractApp(&fakeMenuService{menu: testMenu()})
	req := contractRequest(http.MethodGet, "/api/v1/menus/suggest?genre=japanese&difficulty=easy", "")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertMatchesSpec(t, v, req, "", rec)
}

func TestContract_MenuSuggest_Problem(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	// 不正なジャンルは 400 の problem+json。エラー応答も仕様の一部。
	e := newContractApp(&fakeMenuService{menu: testMenu()})
	req := contractRequest(http.MethodGet, "/api/v1/menus/suggest?genre=nosuch", "")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assertResponseMatchesSpec(t, v, req, rec)
}

func TestContract_MenuGet(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	menu := testMenu()
	e := newContractApp(&fakeMenuService{getMenu: menu})
	req := contractRequest(http.MethodGet, "/api/v1/menus/"+menu.ID.String(), "")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertMatchesSpec(t, v, req, "", rec)
}

func TestContract_Recipes(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	menu := testMenu()
	e := newContractApp(&fakeMenuService{
		getMenu: menu,
		recipes: []domain.RecipeLink{
			{Title: "親子丼のレシピ", URL: "https://example.com/1", Domain: "example.com", Snippet: "作り方"},
		},
	})
	req := contractRequest(http.MethodGet, "/api/v1/menus/"+menu.ID.String()+"/recipes", "")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertMatchesSpec(t, v, req, "", rec)
}

func TestContract_Histories(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	svc := &fakeHistoryLister{
		entries: []domain.HistoryEntry{
			historyEntry(t, "親子丼", domain.SearchModeSingle, time.Now()),
		},
	}
	e, tokens := historyApp(t, svc)
	access, err := tokens.Issue("018f0000-0000-7000-8000-000000000001")
	require.NoError(t, err)

	req := contractRequest(http.MethodGet, "/api/v1/histories", "")
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertMatchesSpec(t, v, req, "", rec)
}

func TestContract_Histories_Unauthorized(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	e, _ := historyApp(t, &fakeHistoryLister{})
	req := contractRequest(http.MethodGet, "/api/v1/histories", "")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assertResponseMatchesSpec(t, v, req, rec)
}

func TestContract_AddFavorite(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	e, tokens := favoriteApp(t, &fakeFavoriteService{})
	access, err := tokens.Issue("018f0000-0000-7000-8000-000000000001")
	require.NoError(t, err)

	body := `{"menuId":"` + domain.NewMenuID().String() + `"}`
	req := contractRequest(http.MethodPost, "/api/v1/favorites", body)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	assertMatchesSpec(t, v, req, body, rec)
}

func TestContract_AddFavorite_Conflict(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	e, tokens := favoriteApp(t, &fakeFavoriteService{err: service.ErrFavoriteExists})
	access, err := tokens.Issue("018f0000-0000-7000-8000-000000000001")
	require.NoError(t, err)

	body := `{"menuId":"` + domain.NewMenuID().String() + `"}`
	req := contractRequest(http.MethodPost, "/api/v1/favorites", body)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	assertMatchesSpec(t, v, req, body, rec)
}

func TestContract_ListFavorites(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	svc := &fakeFavoriteService{
		favorites: []domain.Favorite{{Menu: *testMenu(), CreatedAt: time.Now()}},
	}
	e, tokens := favoriteApp(t, svc)
	access, err := tokens.Issue("018f0000-0000-7000-8000-000000000001")
	require.NoError(t, err)

	req := contractRequest(http.MethodGet, "/api/v1/favorites", "")
	req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertMatchesSpec(t, v, req, "", rec)
}

// newContractShoppingApp は買い物リスト・食材の契約検証用アプリを組み立てる。
func newContractShoppingApp(shopping handler.ShoppingListUseCase, ing handler.MenuIngredientsUseCase) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewShoppingListHandler(shopping).RegisterRoutes(e)
	handler.NewIngredientHandler(ing).RegisterRoutes(e)
	return e
}

func TestContract_MenuIngredients(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	e := newContractShoppingApp(&fakeShoppingList{}, &fakeMenuIngredients{
		items: []domain.Ingredient{
			shoppingTestIngredient("じゃがいも", "じゃがいも", domain.CategoryVegetable),
			shoppingTestIngredient("豚こま切れ肉", "ぶたこまぎれにく", domain.CategoryMeat),
		},
	})
	req := contractRequest(http.MethodGet,
		"/api/v1/menus/"+domain.NewMenuID().String()+"/ingredients", "")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertMatchesSpec(t, v, req, "", rec)
}

func TestContract_ShoppingList(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	menu := shoppingTestMenu("肉じゃが")
	e := newContractShoppingApp(&fakeShoppingList{items: []service.ShoppingItem{
		{
			Ingredient: shoppingTestIngredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable),
			UsedIn:     []domain.Menu{menu},
		},
	}}, &fakeMenuIngredients{})

	body := `{"menuIds":["` + menu.ID.String() + `"]}`
	req := contractRequest(http.MethodPost, "/api/v1/shopping-list", body)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertMatchesSpec(t, v, req, body, rec)
}

func TestContract_ShoppingList_Problem(t *testing.T) {
	t.Parallel()
	v := newContractValidator(t)

	// 件数が不正なときの 400 も仕様の一部。
	e := newContractShoppingApp(&fakeShoppingList{err: service.ErrInvalidMenuIDs}, &fakeMenuIngredients{})
	body := `{"menuIds":["` + domain.NewMenuID().String() + `"]}`
	req := contractRequest(http.MethodPost, "/api/v1/shopping-list", body)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assertMatchesSpec(t, v, req, body, rec)
}
