package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

func postSearchByIngredients(t *testing.T, e *echo.Echo, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/menus/search-by-ingredients", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSearchByIngredients_候補と不足を返す(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{matches: []service.MenuMatch{{
		Menu: domain.Menu{
			ID: domain.NewMenuID(), Name: "肉じゃが", NameKana: "にくじゃが",
			Genre: domain.GenreJapanese, Difficulty: domain.DifficultyEasy, Role: domain.RoleMain,
			Description: "説明",
		},
		Matched: []domain.Ingredient{{
			ID: domain.NewIngredientID(), Name: "玉ねぎ", NameKana: "たまねぎ",
			Category: domain.CategoryVegetable,
		}},
		Missing: []domain.Ingredient{{
			ID: domain.NewIngredientID(), Name: "牛肉", NameKana: "ぎゅうにく",
			Category: domain.CategoryMeat,
		}},
	}}}
	e := catalogApp(t, catalog)

	id := domain.NewIngredientID().String()
	rec := postSearchByIngredients(t, e, `{"ingredientIds":["`+id+`"]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"肉じゃが"`)
	assert.Contains(t, rec.Body.String(), `"matched"`)
	assert.Contains(t, rec.Body.String(), `"牛肉"`)
	// 指定したIDがそのまま service に渡る。
	require.Len(t, catalog.lastIDs, 1)
	assert.Equal(t, id, catalog.lastIDs[0].String())
}

func TestSearchByIngredients_0件でもnullではなく空配列(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{matches: nil}
	e := catalogApp(t, catalog)

	rec := postSearchByIngredients(t, e,
		`{"ingredientIds":["`+domain.NewIngredientID().String()+`"]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"matches":[]`)
}

func TestSearchByIngredients_未認証でも使える(t *testing.T) {
	t.Parallel()

	// 検索と同じ扱い（spec.md 2.9）。Cookie 無しで 200。
	catalog := &fakeIngredientCatalog{matches: []service.MenuMatch{}}
	e := catalogApp(t, catalog)

	rec := postSearchByIngredients(t, e,
		`{"ingredientIds":["`+domain.NewIngredientID().String()+`"]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, catalog.searchCalls)
}

func TestSearchByIngredients_壊れた食材IDは400(t *testing.T) {
	t.Parallel()

	// 13-C で初めて外に出る経路。写像が無いと 500 になる。
	catalog := &fakeIngredientCatalog{}
	e := catalogApp(t, catalog)

	rec := postSearchByIngredients(t, e, `{"ingredientIds":["not-a-uuid"]}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid-ingredient-id")
	assert.Zero(t, catalog.searchCalls, "解釈できない時点で service を呼ばない")
}

func TestSearchByIngredients_0件指定は400(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{err: service.ErrInvalidIngredientIDs}
	e := catalogApp(t, catalog)

	rec := postSearchByIngredients(t, e, `{"ingredientIds":[]}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid-ingredient-ids")
}

func TestSearchByIngredients_存在しない食材は404(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{err: service.ErrIngredientNotFound}
	e := catalogApp(t, catalog)

	rec := postSearchByIngredients(t, e,
		`{"ingredientIds":["`+domain.NewIngredientID().String()+`"]}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "ingredient-not-found")
}

func TestSearchByIngredients_壊れたJSONは400(t *testing.T) {
	t.Parallel()

	catalog := &fakeIngredientCatalog{}
	e := catalogApp(t, catalog)

	rec := postSearchByIngredients(t, e, `{"ingredientIds":`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, catalog.searchCalls)
}
