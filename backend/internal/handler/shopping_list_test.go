package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// service.ShoppingListService が handler の要求を満たすことをコンパイル時に保証する。
var _ handler.ShoppingListUseCase = (*service.ShoppingListService)(nil)

// fakeShoppingList は定型の買い物リストを返す。
type fakeShoppingList struct {
	items   []service.ShoppingItem
	err     error
	lastIDs []domain.MenuID
	calls   int
}

func (f *fakeShoppingList) Build(_ context.Context, ids []domain.MenuID) ([]service.ShoppingItem, error) {
	f.calls++
	f.lastIDs = ids
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func shoppingTestMenu(name string) domain.Menu {
	return domain.Menu{
		ID:          domain.NewMenuID(),
		Name:        name,
		NameKana:    name,
		Genre:       domain.GenreJapanese,
		Difficulty:  domain.DifficultyEasy,
		Description: name + "の説明",
	}
}

func shoppingTestIngredient(name, kana string, c domain.IngredientCategory) domain.Ingredient {
	return domain.Ingredient{
		ID:       domain.NewIngredientID(),
		Name:     name,
		NameKana: kana,
		Category: c,
	}
}

// shoppingApp は買い物リストのハンドラだけを載せた echo を返す。
func shoppingApp(t *testing.T, svc handler.ShoppingListUseCase) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewShoppingListHandler(svc).RegisterRoutes(e)
	return e
}

// postShoppingList は買い物リストAPIを叩く。
func postShoppingList(e *echo.Echo, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shopping-list", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestShoppingListHandler_ReturnsItems(t *testing.T) {
	nikujaga := shoppingTestMenu("肉じゃが")
	oyakodon := shoppingTestMenu("親子丼")
	svc := &fakeShoppingList{items: []service.ShoppingItem{
		{
			Ingredient: shoppingTestIngredient("玉ねぎ", "たまねぎ", domain.CategoryVegetable),
			UsedIn:     []domain.Menu{nikujaga, oyakodon},
		},
		{
			Ingredient: shoppingTestIngredient("鶏もも肉", "とりももにく", domain.CategoryMeat),
			UsedIn:     []domain.Menu{oyakodon},
		},
	}}
	e := shoppingApp(t, svc)

	rec := postShoppingList(e, `{"menuIds":["`+nikujaga.ID.String()+`","`+oyakodon.ID.String()+`"]}`)

	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		Items []struct {
			Ingredient struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				NameKana string `json:"nameKana"`
				Category string `json:"category"`
			} `json:"ingredient"`
			UsedIn []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"usedIn"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	require.Len(t, got.Items, 2)
	assert.Equal(t, "玉ねぎ", got.Items[0].Ingredient.Name)
	assert.Equal(t, "たまねぎ", got.Items[0].Ingredient.NameKana)
	assert.Equal(t, "vegetable", got.Items[0].Ingredient.Category)
	// どの献立で使うかが並ぶ（分量を持たない設計の補償）。
	require.Len(t, got.Items[0].UsedIn, 2)
	assert.Equal(t, "肉じゃが", got.Items[0].UsedIn[0].Name)
	assert.Equal(t, "親子丼", got.Items[0].UsedIn[1].Name)

	// service にはパースされた献立IDがそのまま渡る。
	assert.Len(t, svc.lastIDs, 2)
}

func TestShoppingListHandler_EmptyResultIsArrayNotNull(t *testing.T) {
	// 0件でも null ではなく [] を返す。フロントが length を見るだけで扱えるようにする。
	e := shoppingApp(t, &fakeShoppingList{items: nil})

	rec := postShoppingList(e, `{"menuIds":["`+domain.NewMenuID().String()+`"]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}

func TestShoppingListHandler_InvalidUUIDIs400(t *testing.T) {
	// 不正なIDは service を呼ばずに弾く。
	svc := &fakeShoppingList{}
	e := shoppingApp(t, svc)

	rec := postShoppingList(e, `{"menuIds":["not-a-uuid"]}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, svc.calls, "不正な入力で service を呼ばない")
}

func TestShoppingListHandler_MalformedBodyIs400(t *testing.T) {
	svc := &fakeShoppingList{}
	e := shoppingApp(t, svc)

	rec := postShoppingList(e, `{"menuIds":`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, svc.calls)
}

func TestShoppingListHandler_InvalidCountIs400(t *testing.T) {
	// 0件・上限超過は service が判断し、ハンドラは 400 に変換する。
	svc := &fakeShoppingList{err: service.ErrInvalidMenuIDs}
	e := shoppingApp(t, svc)

	rec := postShoppingList(e, `{"menuIds":["`+domain.NewMenuID().String()+`"]}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)
}

func TestShoppingListHandler_UnknownMenuIs404(t *testing.T) {
	svc := &fakeShoppingList{err: service.ErrMenuNotFoundInList}
	e := shoppingApp(t, svc)

	rec := postShoppingList(e, `{"menuIds":["`+domain.NewMenuID().String()+`"]}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestShoppingListHandler_UnknownErrorIs500(t *testing.T) {
	svc := &fakeShoppingList{err: errors.New("DB障害")}
	e := shoppingApp(t, svc)

	rec := postShoppingList(e, `{"menuIds":["`+domain.NewMenuID().String()+`"]}`)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "DB障害", "内部の詳細を外に出さない")
}

// --- GET /menus/:id/ingredients ---

// fakeMenuIngredients は献立1件分の食材を返す。
type fakeMenuIngredients struct {
	items []domain.Ingredient
	err   error
	calls int
}

func (f *fakeMenuIngredients) MenuIngredients(_ context.Context, _ domain.MenuID) ([]domain.Ingredient, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func ingredientsApp(t *testing.T, svc handler.MenuIngredientsUseCase) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewIngredientHandler(svc).RegisterRoutes(e)
	return e
}

func getIngredients(e *echo.Echo, menuID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/menus/"+menuID+"/ingredients", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestIngredientHandler_ReturnsIngredients(t *testing.T) {
	svc := &fakeMenuIngredients{items: []domain.Ingredient{
		shoppingTestIngredient("じゃがいも", "じゃがいも", domain.CategoryVegetable),
		shoppingTestIngredient("豚こま切れ肉", "ぶたこまぎれにく", domain.CategoryMeat),
	}}
	e := ingredientsApp(t, svc)

	rec := getIngredients(e, domain.NewMenuID().String())

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Ingredients []struct {
			Name     string `json:"name"`
			NameKana string `json:"nameKana"`
			Category string `json:"category"`
		} `json:"ingredients"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Ingredients, 2)
	assert.Equal(t, "じゃがいも", got.Ingredients[0].Name)
	assert.Equal(t, "meat", got.Ingredients[1].Category)
}

func TestIngredientHandler_EmptyIsArrayNotNull(t *testing.T) {
	e := ingredientsApp(t, &fakeMenuIngredients{items: nil})

	rec := getIngredients(e, domain.NewMenuID().String())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ingredients":[]`)
}

func TestIngredientHandler_InvalidUUIDIs400(t *testing.T) {
	svc := &fakeMenuIngredients{}
	e := ingredientsApp(t, svc)

	rec := getIngredients(e, "not-a-uuid")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, svc.calls, "不正なIDで service を呼ばない")
}

func TestIngredientHandler_UnknownMenuIs404(t *testing.T) {
	svc := &fakeMenuIngredients{err: repository.ErrMenuNotFound}
	e := ingredientsApp(t, svc)

	rec := getIngredients(e, domain.NewMenuID().String())

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
