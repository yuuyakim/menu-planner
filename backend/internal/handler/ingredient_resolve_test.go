package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

type stubResolveUseCase struct {
	result service.ResolveResult
	err    error
	calls  int
}

func (s *stubResolveUseCase) Resolve(context.Context, string) (service.ResolveResult, error) {
	s.calls++
	return s.result, s.err
}

func newResolveServer(uc handler.IngredientResolveUseCase) *echo.Echo {
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewIngredientResolveHandler(uc).RegisterRoutes(e)
	return e
}

func postResolve(t *testing.T, uc handler.IngredientResolveUseCase, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingredients/resolve", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	newResolveServer(uc).ServeHTTP(rec, req)
	return rec
}

func TestResolveHandler(t *testing.T) {
	ing := domain.Ingredient{
		ID: domain.NewIngredientID(), Name: "豚肉", NameKana: "ぶたにく",
		Category: domain.CategoryMeat,
	}

	t.Run("200で解決結果を返す", func(t *testing.T) {
		uc := &stubResolveUseCase{result: service.ResolveResult{
			Resolved:   []service.ResolvedWord{{Word: "豚こま", Ingredient: ing}},
			Unresolved: []string{"マツタケ"},
			Degraded:   false,
		}}
		rec := postResolve(t, uc, `{"text":"豚こま、マツタケ"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("200 を期待しましたが %d でした: %s", rec.Code, rec.Body.String())
		}
		var got struct {
			Resolved []struct {
				Word       string `json:"word"`
				Ingredient struct {
					Name string `json:"name"`
				} `json:"ingredient"`
			} `json:"resolved"`
			Unresolved []string `json:"unresolved"`
			Degraded   bool     `json:"degraded"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("レスポンスを解釈できませんでした: %v", err)
		}
		if len(got.Resolved) != 1 || got.Resolved[0].Word != "豚こま" {
			t.Errorf("resolved が想定と違います: %+v", got.Resolved)
		}
		if got.Resolved[0].Ingredient.Name != "豚肉" {
			t.Errorf("食材名が違います: %q", got.Resolved[0].Ingredient.Name)
		}
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "マツタケ" {
			t.Errorf("unresolved が想定と違います: %+v", got.Unresolved)
		}
	})

	t.Run("解決0件でもnullにしない", func(t *testing.T) {
		uc := &stubResolveUseCase{result: service.ResolveResult{}}
		rec := postResolve(t, uc, `{"text":"卵"}`)

		if body := rec.Body.String(); !strings.Contains(body, `"resolved":[]`) ||
			!strings.Contains(body, `"unresolved":[]`) {
			t.Errorf("0件は空配列で返すべきです: %s", body)
		}
	})

	t.Run("空テキストは400", func(t *testing.T) {
		uc := &stubResolveUseCase{err: service.ErrEmptyResolveText}
		rec := postResolve(t, uc, `{"text":""}`)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("400 を期待しましたが %d でした: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("長すぎるテキストは400", func(t *testing.T) {
		uc := &stubResolveUseCase{}
		rec := postResolve(t, uc, `{"text":"`+strings.Repeat("あ", 201)+`"}`)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("400 を期待しましたが %d でした", rec.Code)
		}
		if uc.calls != 0 {
			t.Error("LLM に渡す前に落とすべきです")
		}
	})

	t.Run("語数が多すぎるテキストは400", func(t *testing.T) {
		words := make([]string, 21)
		for i := range words {
			words[i] = "卵"
		}
		uc := &stubResolveUseCase{}
		rec := postResolve(t, uc, `{"text":"`+strings.Join(words, "、")+`"}`)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("400 を期待しましたが %d でした", rec.Code)
		}
		if uc.calls != 0 {
			t.Error("LLM に渡す前に落とすべきです")
		}
	})

	t.Run("上限ちょうどは通る", func(t *testing.T) {
		words := make([]string, 20)
		for i := range words {
			words[i] = "卵"
		}
		uc := &stubResolveUseCase{}
		rec := postResolve(t, uc, `{"text":"`+strings.Join(words, "、")+`"}`)

		if rec.Code != http.StatusOK {
			t.Errorf("20件は通すべきです: %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("縮退していてもJSONにdegradedが出る", func(t *testing.T) {
		uc := &stubResolveUseCase{result: service.ResolveResult{
			Resolved: []service.ResolvedWord{}, Unresolved: []string{"豚こま"}, Degraded: true,
		}}
		rec := postResolve(t, uc, `{"text":"豚こま"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("縮退でも200であるべきです: %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"degraded":true`) {
			t.Errorf("degraded が出ていません: %s", rec.Body.String())
		}
	})
}
