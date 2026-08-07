package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeIngredientCatalog は IngredientCatalogUseCase を差し替える。
type fakeIngredientCatalog struct {
	items       []domain.Ingredient
	matches     []service.MenuMatch
	nearMisses  []service.MenuMatch
	lastInput   service.SearchByIngredientsInput
	lastIDs     []domain.IngredientID
	err         error
	calls       int
	searchCalls int
}

func (s *fakeIngredientCatalog) All(_ context.Context) ([]domain.Ingredient, error) {
	s.calls++
	return s.items, s.err
}

func (s *fakeIngredientCatalog) SearchByIngredients(
	_ context.Context, in service.SearchByIngredientsInput,
) (service.SearchByIngredientsResult, error) {
	s.searchCalls++
	s.lastInput = in
	s.lastIDs = in.IngredientIDs
	return service.SearchByIngredientsResult{
		Matches: s.matches, NearMisses: s.nearMisses,
	}, s.err
}

func catalogApp(t *testing.T, catalog handler.IngredientCatalogUseCase) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewIngredientHandler(&fakeMenuIngredients{}, catalog).RegisterRoutes(e)
	return e
}

func getIngredientCatalog(t *testing.T, e *echo.Echo) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingredients", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestIngredients_All_全件返す(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{items: []domain.Ingredient{
		{ID: domain.NewIngredientID(), Name: "玉ねぎ", NameKana: "たまねぎ", Category: domain.CategoryVegetable},
		{ID: domain.NewIngredientID(), Name: "鶏もも肉", NameKana: "とりももにく", Category: domain.CategoryMeat},
	}}
	e := catalogApp(t, catalog)

	rec := getIngredientCatalog(t, e)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Ingredients []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			NameKana string `json:"nameKana"`
			Category string `json:"category"`
		} `json:"ingredients"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Ingredients, 2)
	// service が決めた並びをそのまま出す（handler で並べ替えない）。
	assert.Equal(t, "玉ねぎ", body.Ingredients[0].Name)
	assert.Equal(t, "vegetable", body.Ingredients[0].Category)
	assert.Equal(t, "とりももにく", body.Ingredients[1].NameKana)
}

func TestIngredients_All_未認証でも取得できる(t *testing.T) {
	t.Parallel()

	// 検索と同じ扱い（spec.md 2.9）。Cookie を付けずに 200 が返る。
	catalog := &fakeIngredientCatalog{items: []domain.Ingredient{}}
	e := catalogApp(t, catalog)

	rec := getIngredientCatalog(t, e)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, catalog.calls)
}

func TestIngredients_All_0件でもnullではなく空配列(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{items: nil}
	e := catalogApp(t, catalog)

	rec := getIngredientCatalog(t, e)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ingredients":[]`)
}

func TestIngredients_All_取得に失敗したら500(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{err: errors.New("DB障害")}
	e := catalogApp(t, catalog)

	rec := getIngredientCatalog(t, e)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
