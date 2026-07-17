package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// service.MenuService が handler の要求するインターフェースを満たすことを
// コンパイル時に保証する。実行時ではなくビルド時に気付けるようにする。
var _ handler.MenuUseCase = (*service.MenuService)(nil)

// fakeMenuService は service.MenuService の代わりに定型の結果を返す。
type fakeMenuService struct {
	menu *domain.Menu
	err  error
	// lastFilter は最後に SuggestMenu に渡された条件。
	lastFilter domain.MenuFilter
	calls      int

	// getMenu / getErr は GetMenu の返り値。Suggest 側と分けておき、
	// 片方のテストがもう片方の設定に引きずられないようにする。
	getMenu *domain.Menu
	getErr  error
	// lastID は最後に GetMenu に渡されたID。
	lastID   domain.MenuID
	getCalls int

	// recipes / recipeErr は RecipeLinks の返り値。
	recipes   []domain.RecipeLink
	recipeErr error
	// lastRecipeID は最後に RecipeLinks に渡されたID。
	lastRecipeID domain.MenuID
	recipeCalls  int

	// week / weekErr は SuggestWeekly の返り値。
	week    []domain.DayMenu
	weekErr error
	// lastWeeklyFilter / lastWeeklyRecent は最後に SuggestWeekly に渡された値。
	lastWeeklyFilter domain.MenuFilter
	lastWeeklyRecent []domain.MenuID
	weeklyCalls      int
}

func (s *fakeMenuService) SuggestWeekly(_ context.Context, f domain.MenuFilter, recentIDs []domain.MenuID) ([]domain.DayMenu, error) {
	s.weeklyCalls++
	s.lastWeeklyFilter = f
	s.lastWeeklyRecent = recentIDs
	if s.weekErr != nil {
		return nil, s.weekErr
	}
	return s.week, nil
}

func (s *fakeMenuService) RecipeLinks(_ context.Context, id domain.MenuID) ([]domain.RecipeLink, error) {
	s.recipeCalls++
	s.lastRecipeID = id
	if s.recipeErr != nil {
		return nil, s.recipeErr
	}
	return s.recipes, nil
}

func (s *fakeMenuService) SuggestMenu(_ context.Context, f domain.MenuFilter) (*domain.Menu, error) {
	s.calls++
	s.lastFilter = f
	if s.err != nil {
		return nil, s.err
	}
	return s.menu, nil
}

func (s *fakeMenuService) GetMenu(_ context.Context, id domain.MenuID) (*domain.Menu, error) {
	s.getCalls++
	s.lastID = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getMenu, nil
}

func testMenu() *domain.Menu {
	return &domain.Menu{
		ID:          domain.NewMenuID(),
		Name:        "親子丼",
		NameKana:    "おやこどん",
		Genre:       domain.GenreJapanese,
		Difficulty:  domain.DifficultyEasy,
		Description: "鶏肉と卵を甘辛い出汁でとじた定番の丼もの",
	}
}

// doSuggest は GET /api/v1/menus/suggest を実際にルーティング経由で叩く。
func doSuggest(t *testing.T, s *fakeMenuService, query string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewMenuHandler(s).RegisterRoutes(e)

	target := "/api/v1/menus/suggest"
	if query != "" {
		target += "?" + query
	}

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestSuggest_200とJSON構造(t *testing.T) {
	t.Parallel()

	menu := testMenu()
	rec := doSuggest(t, &fakeMenuService{menu: menu}, "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), echo.MIMEApplicationJSON)

	var body struct {
		Menu map[string]any `json:"menu"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Equal(t, menu.ID.String(), body.Menu["id"])
	assert.Equal(t, "親子丼", body.Menu["name"])
	assert.Equal(t, "japanese", body.Menu["genre"])
	assert.Equal(t, "easy", body.Menu["difficulty"])
	assert.Equal(t, "鶏肉と卵を甘辛い出汁でとじた定番の丼もの", body.Menu["description"])
}

func TestSuggest_レスポンスに余分な項目を含めない(t *testing.T) {
	t.Parallel()

	// name_kana は内部の並び替え用でAPIの契約に無い。一度出すと外せなくなるため
	// 増えていないことを固定する。
	rec := doSuggest(t, &fakeMenuService{menu: testMenu()}, "")

	var body struct {
		Menu map[string]any `json:"menu"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.ElementsMatch(t,
		[]string{"id", "name", "genre", "difficulty", "description"},
		keysOf(body.Menu))
}

func TestSuggest_genreがserviceに渡る(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{menu: testMenu()}
	rec := doSuggest(t, s, "genre=japanese")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, s.lastFilter.Genre)
	assert.Equal(t, domain.GenreJapanese, *s.lastFilter.Genre)
	assert.Nil(t, s.lastFilter.Difficulty)
}

func TestSuggest_difficultyがserviceに渡る(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{menu: testMenu()}
	rec := doSuggest(t, s, "difficulty=easy")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, s.lastFilter.Difficulty)
	assert.Equal(t, domain.DifficultyEasy, *s.lastFilter.Difficulty)
	assert.Nil(t, s.lastFilter.Genre)
}

func TestSuggest_両方指定するとどちらも渡る(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{menu: testMenu()}
	rec := doSuggest(t, s, "genre=chinese&difficulty=elaborate")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, s.lastFilter.Genre)
	require.NotNil(t, s.lastFilter.Difficulty)
	assert.Equal(t, domain.GenreChinese, *s.lastFilter.Genre)
	assert.Equal(t, domain.DifficultyElaborate, *s.lastFilter.Difficulty)
}

func TestSuggest_クエリ無しで両方nilが渡る(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{menu: testMenu()}
	rec := doSuggest(t, s, "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, s.lastFilter.Genre, "指定なしは「全ジャンル」であって不正ではない")
	assert.Nil(t, s.lastFilter.Difficulty)
}

func TestSuggest_空文字のクエリは指定なしとして扱う(t *testing.T) {
	t.Parallel()

	// フロントが未選択を genre= として送っても 400 にしない。
	s := &fakeMenuService{menu: testMenu()}
	rec := doSuggest(t, s, "genre=&difficulty=")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, s.lastFilter.Genre)
	assert.Nil(t, s.lastFilter.Difficulty)
}

func TestSuggest_不正なgenreで400(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{menu: testMenu()}
	rec := doSuggest(t, s, "genre=french")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)
	assert.Equal(t, 0, s.calls, "条件が不正なら service を呼ばないこと")
}

func TestSuggest_不正なdifficultyで400(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{menu: testMenu()}
	rec := doSuggest(t, s, "difficulty=very-hard")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)
	assert.Equal(t, 0, s.calls)
}

func TestSuggest_候補0件で422(t *testing.T) {
	t.Parallel()

	rec := doSuggest(t, &fakeMenuService{err: service.ErrNoMenuFound}, "genre=other")

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)

	body := decodeProblem(t, rec)
	assert.InDelta(t, float64(http.StatusUnprocessableEntity), body["status"], 0)
	assert.NotEmpty(t, body["title"])
}

func TestSuggest_serviceの未知のエラーは500で詳細を漏らさない(t *testing.T) {
	t.Parallel()

	secret := errors.New("pq: password authentication failed for user \"app\"")
	rec := doSuggest(t, &fakeMenuService{err: secret}, "")

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "password")
}

// doGet は GET /api/v1/menus/:id を実際にルーティング経由で叩く。
func doGet(t *testing.T, s *fakeMenuService, id string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewMenuHandler(s).RegisterRoutes(e)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/menus/"+id, nil))
	return rec
}

func TestGet_200と献立の詳細(t *testing.T) {
	t.Parallel()

	menu := testMenu()
	rec := doGet(t, &fakeMenuService{getMenu: menu}, menu.ID.String())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), echo.MIMEApplicationJSON)

	var body struct {
		Menu map[string]any `json:"menu"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Equal(t, menu.ID.String(), body.Menu["id"])
	assert.Equal(t, "親子丼", body.Menu["name"])
	assert.Equal(t, "japanese", body.Menu["genre"])
	assert.Equal(t, "easy", body.Menu["difficulty"])
	assert.Equal(t, "鶏肉と卵を甘辛い出汁でとじた定番の丼もの", body.Menu["description"])
}

func TestGet_レスポンスの形はsuggestと同じ(t *testing.T) {
	t.Parallel()

	// 同じ献立を2つの経路で返すため、片方だけ項目が増減するとフロントが壊れる。
	menu := testMenu()
	rec := doGet(t, &fakeMenuService{getMenu: menu}, menu.ID.String())

	var body struct {
		Menu map[string]any `json:"menu"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.ElementsMatch(t,
		[]string{"id", "name", "genre", "difficulty", "description"},
		keysOf(body.Menu))
}

func TestGet_パスのIDがserviceに渡る(t *testing.T) {
	t.Parallel()

	menu := testMenu()
	s := &fakeMenuService{getMenu: menu}
	rec := doGet(t, s, menu.ID.String())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, s.getCalls)
	assert.Equal(t, menu.ID, s.lastID)
}

func TestGet_存在しないIDで404(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{getErr: fmt.Errorf("献立の取得に失敗しました: %w", repository.ErrMenuNotFound)}
	rec := doGet(t, s, domain.NewMenuID().String())

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)

	body := decodeProblem(t, rec)
	assert.InDelta(t, float64(http.StatusNotFound), body["status"], 0)
	assert.NotEmpty(t, body["title"])
}

func TestGet_不正なUUIDで400(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"UUIDでない文字列": "not-a-uuid",
		"桁が足りない":     "123e4567-e89b-12d3-a456",
		"数値":         "42",
		// ゼロ値のUUIDは「未設定」と区別できないため domain が受け付けない。
		"ゼロ値のUUID": "00000000-0000-0000-0000-000000000000",
	}
	for name, id := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := &fakeMenuService{getMenu: testMenu()}
			rec := doGet(t, s, id)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)
			assert.Equal(t, 0, s.getCalls, "IDが不正なら service を呼ばないこと")
		})
	}
}

func TestGet_suggestはIDパラメータにマッチしない(t *testing.T) {
	t.Parallel()

	// /menus/suggest と /menus/:id は同じ階層にある。:id が suggest を
	// 飲み込むと検索APIが 400 に化けるため、経路が分かれることを固定する。
	s := &fakeMenuService{menu: testMenu(), getMenu: testMenu()}
	rec := doGet(t, s, "suggest")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, s.calls, "Suggest が呼ばれること")
	assert.Equal(t, 0, s.getCalls, "Get は呼ばれないこと")
}

func TestGet_serviceの未知のエラーは500で詳細を漏らさない(t *testing.T) {
	t.Parallel()

	secret := errors.New("pq: password authentication failed for user \"app\"")
	rec := doGet(t, &fakeMenuService{getErr: secret}, domain.NewMenuID().String())

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "password")
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// doGetRecipes は GET /api/v1/menus/:id/recipes をルーティング経由で叩く。
func doGetRecipes(t *testing.T, s *fakeMenuService, id string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewMenuHandler(s).RegisterRoutes(e)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/menus/"+id+"/recipes", nil))
	return rec
}

func testRecipeLinks(n int) []domain.RecipeLink {
	all := []struct{ title, url string }{
		{"親子丼の作り方", "https://recipe.example.com/1"},
		{"簡単 親子丼", "https://cooking.example.net/2"},
		{"本格 親子丼", "https://kitchen.example.org/3"},
	}
	links := make([]domain.RecipeLink, 0, n)
	for _, r := range all[:n] {
		link, err := domain.NewRecipeLink(r.title, r.url, r.title+"の説明")
		if err != nil {
			panic(err)
		}
		links = append(links, link)
	}
	return links
}

func TestRecipes_200と3件(t *testing.T) {
	t.Parallel()

	id := domain.NewMenuID()
	s := &fakeMenuService{recipes: testRecipeLinks(3)}
	rec := doGetRecipes(t, s, id.String())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), echo.MIMEApplicationJSON)

	var body struct {
		Recipes []map[string]any `json:"recipes"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Recipes, 3)

	assert.Equal(t, "親子丼の作り方", body.Recipes[0]["title"])
	assert.Equal(t, "https://recipe.example.com/1", body.Recipes[0]["url"])
	assert.Equal(t, "recipe.example.com", body.Recipes[0]["domain"])
	assert.Equal(t, "親子丼の作り方の説明", body.Recipes[0]["snippet"])
	assert.Equal(t, id, s.lastRecipeID, "パスのIDが service に渡ること")
}

func TestRecipes_レスポンスの項目はspecの通り(t *testing.T) {
	t.Parallel()

	// spec.md 5.1 の例: title / url / domain / snippet
	rec := doGetRecipes(t, &fakeMenuService{recipes: testRecipeLinks(1)}, domain.NewMenuID().String())

	var body struct {
		Recipes []map[string]any `json:"recipes"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Recipes, 1)

	assert.ElementsMatch(t, []string{"title", "url", "domain", "snippet"}, keysOf(body.Recipes[0]))
}

func TestRecipes_3件未満でも200(t *testing.T) {
	t.Parallel()

	// spec.md 2.3: 取得できた件数のみを表示する。
	for _, n := range []int{1, 2} {
		t.Run(fmt.Sprintf("%d件", n), func(t *testing.T) {
			t.Parallel()

			rec := doGetRecipes(t, &fakeMenuService{recipes: testRecipeLinks(n)}, domain.NewMenuID().String())

			require.Equal(t, http.StatusOK, rec.Code)

			var body struct {
				Recipes []map[string]any `json:"recipes"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Len(t, body.Recipes, n)
		})
	}
}

func TestRecipes_0件でも200で空配列(t *testing.T) {
	t.Parallel()

	// null ではなく [] を返す。フロントが length を見るだけで扱えるようにする。
	rec := doGetRecipes(t, &fakeMenuService{recipes: []domain.RecipeLink{}}, domain.NewMenuID().String())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"recipes":[]}`, rec.Body.String())
}

func TestRecipes_存在しない献立で404(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{recipeErr: fmt.Errorf("献立の取得に失敗しました: %w", repository.ErrMenuNotFound)}
	rec := doGetRecipes(t, s, domain.NewMenuID().String())

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)
}

func TestRecipes_gateway障害で502(t *testing.T) {
	t.Parallel()

	// 外部APIの不調は自分の障害ではない。500ではなく502で「上流が悪い」と示す。
	s := &fakeMenuService{recipeErr: fmt.Errorf("レシピの取得に失敗しました: %w", service.ErrRecipeSearchFailed)}
	rec := doGetRecipes(t, s, domain.NewMenuID().String())

	require.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)

	body := decodeProblem(t, rec)
	assert.InDelta(t, float64(http.StatusBadGateway), body["status"], 0)
	assert.NotEmpty(t, body["title"])
}

func TestRecipes_不正なUUIDで400(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{recipes: testRecipeLinks(3)}
	rec := doGetRecipes(t, s, "not-a-uuid")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, s.recipeCalls, "IDが不正なら service を呼ばないこと")
}

func TestRecipes_未知のエラーは500で詳細を漏らさない(t *testing.T) {
	t.Parallel()

	secret := errors.New("pq: password authentication failed for user \"app\"")
	rec := doGetRecipes(t, &fakeMenuService{recipeErr: secret}, domain.NewMenuID().String())

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "password")
}

// doSuggestWeekly は POST /api/v1/menus/suggest-weekly をルーティング経由で叩く。
func doSuggestWeekly(t *testing.T, s *fakeMenuService, body string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewMenuHandler(s).RegisterRoutes(e)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/menus/suggest-weekly", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// testWeek は7日分のテストデータを組み立てる。
func testWeek() []domain.DayMenu {
	week := make([]domain.DayMenu, 0, 7)
	for i := range 7 {
		week = append(week, domain.DayMenu{
			Day:  i + 1,
			Menu: *testMenu(),
		})
	}
	return week
}

func TestSuggestWeekly_200とweek7件(t *testing.T) {
	t.Parallel()

	rec := doSuggestWeekly(t, &fakeMenuService{week: testWeek()}, `{}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), echo.MIMEApplicationJSON)

	var body struct {
		Week []struct {
			Day  int            `json:"day"`
			Menu map[string]any `json:"menu"`
		} `json:"week"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Week, 7)

	for i, d := range body.Week {
		assert.Equal(t, i+1, d.Day, "day が 1..7 の連番であること")
		assert.Equal(t, "親子丼", d.Menu["name"])
	}
}

func TestSuggestWeekly_献立の項目はsuggestと同じ(t *testing.T) {
	t.Parallel()

	// 同じ献立を別経路で返すため、片方だけ項目が増減するとフロントが壊れる。
	rec := doSuggestWeekly(t, &fakeMenuService{week: testWeek()}, `{}`)

	var body struct {
		Week []struct {
			Menu map[string]any `json:"menu"`
		} `json:"week"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotEmpty(t, body.Week)

	assert.ElementsMatch(t,
		[]string{"id", "name", "genre", "difficulty", "description"},
		keysOf(body.Week[0].Menu))
}

func TestSuggestWeekly_条件がserviceに渡る(t *testing.T) {
	t.Parallel()

	s := &fakeMenuService{week: testWeek()}
	rec := doSuggestWeekly(t, s, `{"genre":"japanese","difficulty":"easy"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, s.lastWeeklyFilter.Genre)
	require.NotNil(t, s.lastWeeklyFilter.Difficulty)
	assert.Equal(t, domain.GenreJapanese, *s.lastWeeklyFilter.Genre)
	assert.Equal(t, domain.DifficultyEasy, *s.lastWeeklyFilter.Difficulty)
}

func TestSuggestWeekly_未指定は絞り込まない(t *testing.T) {
	t.Parallel()

	// spec.md 5.1 のリクエスト例は difficulty に null を渡している。
	tests := map[string]string{
		"空のオブジェクト": `{}`,
		"null":     `{"genre":null,"difficulty":null}`,
		"空文字":      `{"genre":"","difficulty":""}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := &fakeMenuService{week: testWeek()}
			rec := doSuggestWeekly(t, s, body)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Nil(t, s.lastWeeklyFilter.Genre)
			assert.Nil(t, s.lastWeeklyFilter.Difficulty)
		})
	}
}

func TestSuggestWeekly_不正なリクエストボディで400(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"JSONとして壊れている": `{"genre":`,
		"配列":           `[]`,
		"genreが数値":     `{"genre":123}`,
		"未知のジャンル":      `{"genre":"french"}`,
		"未知の難易度":       `{"difficulty":"very-hard"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := &fakeMenuService{week: testWeek()}
			rec := doSuggestWeekly(t, s, body)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)
			assert.Equal(t, 0, s.weeklyCalls, "不正なら service を呼ばないこと")
		})
	}
}

func TestSuggestWeekly_ボディが空でも200(t *testing.T) {
	t.Parallel()

	// 条件なしの提案として扱う。空ボディを不正にすると、素直な使い方が弾かれる。
	s := &fakeMenuService{week: testWeek()}

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewMenuHandler(s).RegisterRoutes(e)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/menus/suggest-weekly", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, s.lastWeeklyFilter.Genre)
}

func TestSuggestWeekly_候補0件で422(t *testing.T) {
	t.Parallel()

	rec := doSuggestWeekly(t, &fakeMenuService{weekErr: service.ErrNoMenuFound}, `{"genre":"other"}`)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)

	body := decodeProblem(t, rec)
	assert.InDelta(t, float64(http.StatusUnprocessableEntity), body["status"], 0)
}

func TestSuggestWeekly_未知のエラーは500で詳細を漏らさない(t *testing.T) {
	t.Parallel()

	secret := errors.New("pq: password authentication failed for user \"app\"")
	rec := doSuggestWeekly(t, &fakeMenuService{weekErr: secret}, `{}`)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "password")
}

func TestSuggestWeekly_POST以外は受け付けない(t *testing.T) {
	t.Parallel()

	// 提案は状態を変えないが、条件をボディで受けるため POST にしている（spec.md 5.1）。
	//
	// GET だけは 405 ではなく 400 になる。GET /menus/:id が
	// suggest-weekly を献立IDとして拾い、UUIDではないので弾かれるため。
	// 経路として素通りするわけではないので実害は無い。
	tests := map[string]struct {
		method string
		want   int
	}{
		"GET":    {http.MethodGet, http.StatusBadRequest},
		"DELETE": {http.MethodDelete, http.StatusMethodNotAllowed},
		"PUT":    {http.MethodPut, http.StatusMethodNotAllowed},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := &fakeMenuService{week: testWeek()}
			e := echo.New()
			e.HTTPErrorHandler = handler.ErrorHandler()
			handler.NewMenuHandler(s).RegisterRoutes(e)

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(tt.method, "/api/v1/menus/suggest-weekly", nil))

			assert.Equal(t, tt.want, rec.Code)
			assert.Equal(t, 0, s.weeklyCalls, "週間献立の提案が動かないこと")
		})
	}
}
