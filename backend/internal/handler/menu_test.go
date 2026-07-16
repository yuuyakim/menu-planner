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

// service.MenuService が handler の要求するインターフェースを満たすことを
// コンパイル時に保証する。実行時ではなくビルド時に気付けるようにする。
var _ handler.MenuSuggester = (*service.MenuService)(nil)

// fakeSuggester は service.MenuService の代わりに定型の結果を返す。
type fakeSuggester struct {
	menu *domain.Menu
	err  error
	// lastFilter は最後に SuggestMenu に渡された条件。
	lastFilter domain.MenuFilter
	calls      int
}

func (s *fakeSuggester) SuggestMenu(_ context.Context, f domain.MenuFilter) (*domain.Menu, error) {
	s.calls++
	s.lastFilter = f
	if s.err != nil {
		return nil, s.err
	}
	return s.menu, nil
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
func doSuggest(t *testing.T, s *fakeSuggester, query string) *httptest.ResponseRecorder {
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
	rec := doSuggest(t, &fakeSuggester{menu: menu}, "")

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
	rec := doSuggest(t, &fakeSuggester{menu: testMenu()}, "")

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

	s := &fakeSuggester{menu: testMenu()}
	rec := doSuggest(t, s, "genre=japanese")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, s.lastFilter.Genre)
	assert.Equal(t, domain.GenreJapanese, *s.lastFilter.Genre)
	assert.Nil(t, s.lastFilter.Difficulty)
}

func TestSuggest_difficultyがserviceに渡る(t *testing.T) {
	t.Parallel()

	s := &fakeSuggester{menu: testMenu()}
	rec := doSuggest(t, s, "difficulty=easy")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, s.lastFilter.Difficulty)
	assert.Equal(t, domain.DifficultyEasy, *s.lastFilter.Difficulty)
	assert.Nil(t, s.lastFilter.Genre)
}

func TestSuggest_両方指定するとどちらも渡る(t *testing.T) {
	t.Parallel()

	s := &fakeSuggester{menu: testMenu()}
	rec := doSuggest(t, s, "genre=chinese&difficulty=elaborate")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, s.lastFilter.Genre)
	require.NotNil(t, s.lastFilter.Difficulty)
	assert.Equal(t, domain.GenreChinese, *s.lastFilter.Genre)
	assert.Equal(t, domain.DifficultyElaborate, *s.lastFilter.Difficulty)
}

func TestSuggest_クエリ無しで両方nilが渡る(t *testing.T) {
	t.Parallel()

	s := &fakeSuggester{menu: testMenu()}
	rec := doSuggest(t, s, "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, s.lastFilter.Genre, "指定なしは「全ジャンル」であって不正ではない")
	assert.Nil(t, s.lastFilter.Difficulty)
}

func TestSuggest_空文字のクエリは指定なしとして扱う(t *testing.T) {
	t.Parallel()

	// フロントが未選択を genre= として送っても 400 にしない。
	s := &fakeSuggester{menu: testMenu()}
	rec := doSuggest(t, s, "genre=&difficulty=")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, s.lastFilter.Genre)
	assert.Nil(t, s.lastFilter.Difficulty)
}

func TestSuggest_不正なgenreで400(t *testing.T) {
	t.Parallel()

	s := &fakeSuggester{menu: testMenu()}
	rec := doSuggest(t, s, "genre=french")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)
	assert.Equal(t, 0, s.calls, "条件が不正なら service を呼ばないこと")
}

func TestSuggest_不正なdifficultyで400(t *testing.T) {
	t.Parallel()

	s := &fakeSuggester{menu: testMenu()}
	rec := doSuggest(t, s, "difficulty=very-hard")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)
	assert.Equal(t, 0, s.calls)
}

func TestSuggest_候補0件で422(t *testing.T) {
	t.Parallel()

	rec := doSuggest(t, &fakeSuggester{err: service.ErrNoMenuFound}, "genre=other")

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), handler.ProblemContentType)

	body := decodeProblem(t, rec)
	assert.InDelta(t, float64(http.StatusUnprocessableEntity), body["status"], 0)
	assert.NotEmpty(t, body["title"])
}

func TestSuggest_serviceの未知のエラーは500で詳細を漏らさない(t *testing.T) {
	t.Parallel()

	secret := errors.New("pq: password authentication failed for user \"app\"")
	rec := doSuggest(t, &fakeSuggester{err: secret}, "")

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
