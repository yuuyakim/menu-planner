package handler_test

import (
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
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

// doErrorHandler はカスタムエラーハンドラにエラーを渡した結果を返す。
func doErrorHandler(t *testing.T, err error) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	e.HTTPErrorHandler(err, c)
	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func TestProblem_JSONに各フィールドが出る(t *testing.T) {
	t.Parallel()

	p := handler.Problem{
		Type:   "https://example.com/probs/menu-not-found",
		Title:  "Menu not found",
		Status: http.StatusNotFound,
		Detail: "献立 018f... は存在しません",
	}

	raw, err := json.Marshal(p)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, "https://example.com/probs/menu-not-found", got["type"])
	assert.Equal(t, "Menu not found", got["title"])
	assert.InDelta(t, float64(404), got["status"], 0)
	assert.Equal(t, "献立 018f... は存在しません", got["detail"])
}

func TestErrorHandler_ContentTypeがproblem_json(t *testing.T) {
	t.Parallel()

	rec := doErrorHandler(t, domain.ErrInvalidGenre)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), "application/problem+json")
}

func TestErrorHandler_ドメインのエラーがHTTPステータスに変換される(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err        error
		wantStatus int
	}{
		"不正なジャンルは400":   {domain.ErrInvalidGenre, http.StatusBadRequest},
		"不正な難易度は400":    {domain.ErrInvalidDifficulty, http.StatusBadRequest},
		"不正な献立IDは400":   {domain.ErrInvalidMenuID, http.StatusBadRequest},
		"不正な献立は400":     {domain.ErrInvalidMenu, http.StatusBadRequest},
		"献立が見つからないは404": {repository.ErrMenuNotFound, http.StatusNotFound},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rec := doErrorHandler(t, tt.err)
			assert.Equal(t, tt.wantStatus, rec.Code)

			body := decodeProblem(t, rec)
			assert.InDelta(t, float64(tt.wantStatus), body["status"], 0)
			assert.NotEmpty(t, body["title"])
			assert.NotEmpty(t, body["type"])
		})
	}
}

func TestErrorHandler_ラップされたエラーも変換される(t *testing.T) {
	t.Parallel()

	// fmt.Errorf("%w", ...) でラップされていても errors.Is で辿れること
	wrapped := errors.Join(errors.New("外側"), domain.ErrInvalidGenre)

	rec := doErrorHandler(t, wrapped)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestErrorHandler_未知のエラーは500で詳細を漏らさない(t *testing.T) {
	t.Parallel()

	secret := errors.New("pq: password authentication failed for user \"app\"")

	rec := doErrorHandler(t, secret)
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	// DBの認証情報などが外部に漏れてはいけない
	assert.NotContains(t, rec.Body.String(), "password")
	assert.NotContains(t, rec.Body.String(), "app")

	body := decodeProblem(t, rec)
	assert.NotEmpty(t, body["title"])
}

func TestErrorHandler_echoのHTTPErrorはそのステータスを使う(t *testing.T) {
	t.Parallel()

	rec := doErrorHandler(t, echo.NewHTTPError(http.StatusMethodNotAllowed, "method not allowed"))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestErrorHandler_404はecho既定の404を返す(t *testing.T) {
	t.Parallel()

	rec := doErrorHandler(t, echo.ErrNotFound)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Header().Get(echo.HeaderContentType), "application/problem+json")
}
