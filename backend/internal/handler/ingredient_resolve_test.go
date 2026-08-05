package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/yuuyakim/menu-planner/backend/internal/auth"
	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/handler"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

type stubResolveUseCase struct {
	result service.ResolveResult
	err    error
	calls  int
	// policy は最後に渡されたポリシー。上限の判定が service まで届くことを確かめる。
	policy service.ResolvePolicy
}

func (s *stubResolveUseCase) Resolve(
	_ context.Context, _ string, p service.ResolvePolicy,
) (service.ResolveResult, error) {
	s.calls++
	s.policy = p
	return s.result, s.err
}

// stubQuota は上限判定のスタブ。
type stubQuota struct {
	allow    bool
	reason   service.DegradedReason
	subjects []service.ResolveSubject
}

func (q *stubQuota) Check(
	_ context.Context, s service.ResolveSubject,
) (bool, service.DegradedReason) {
	q.subjects = append(q.subjects, s)
	return q.allow, q.reason
}

const resolveTestHashSecret = "test-secret"

func newResolveServer(uc handler.IngredientResolveUseCase, q handler.ResolveQuotaChecker) *echo.Echo {
	tokens, err := auth.NewJWT([]byte(authTestSecret))
	if err != nil {
		panic("テスト用JWTの生成に失敗しました: " + err.Error())
	}
	e := echo.New()
	e.HTTPErrorHandler = handler.ErrorHandler()
	handler.NewIngredientResolveHandler(uc, q, resolveTestHashSecret, tokens).RegisterRoutes(e)
	return e
}

func postResolve(t *testing.T, uc handler.IngredientResolveUseCase, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postResolveWith(t, uc, &stubQuota{allow: true}, body, "")
}

// postResolveWith は上限のスタブとアクセストークンを指定して叩く。
// access が空文字なら非ログインとして送る。
func postResolveWith(
	t *testing.T,
	uc handler.IngredientResolveUseCase,
	q handler.ResolveQuotaChecker,
	body string,
	access string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingredients/resolve", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if access != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
	}
	newResolveServer(uc, q).ServeHTTP(rec, req)
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

	t.Run("縮退の理由をdegradedReasonで返す", func(t *testing.T) {
		uc := &stubResolveUseCase{result: service.ResolveResult{
			Resolved:   []service.ResolvedWord{},
			Unresolved: []string{"マツタケ"},
			Degraded:   true,
			Reason:     service.ReasonLLMError,
		}}
		rec := postResolve(t, uc, `{"text":"マツタケ"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("200 を期待しましたが %d でした: %s", rec.Code, rec.Body.String())
		}
		var got struct {
			Degraded       bool   `json:"degraded"`
			DegradedReason string `json:"degradedReason"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("レスポンスを解釈できませんでした: %v", err)
		}
		if !got.Degraded || got.DegradedReason != "llm_error" {
			t.Errorf("degraded=true, degradedReason=llm_error を期待しました: %+v", got)
		}
	})

	t.Run("縮退していなければdegradedReasonを出さない", func(t *testing.T) {
		uc := &stubResolveUseCase{result: service.ResolveResult{
			Resolved:   []service.ResolvedWord{},
			Unresolved: []string{},
			Degraded:   false,
		}}
		rec := postResolve(t, uc, `{"text":"玉ねぎ"}`)

		if strings.Contains(rec.Body.String(), "degradedReason") {
			t.Errorf("縮退していないのに degradedReason が出ています: %s", rec.Body.String())
		}
	})
}

func TestResolveHandler_Quota(t *testing.T) {
	t.Run("非ログインはIPをハッシュ化して数える", func(t *testing.T) {
		uc := &stubResolveUseCase{}
		q := &stubQuota{allow: true}
		postResolveWith(t, uc, q, `{"text":"玉ねぎ"}`, "")

		if len(q.subjects) != 1 {
			t.Fatalf("1回判定するはずです: %d回", len(q.subjects))
		}
		got := q.subjects[0]
		if got.Scope != service.ScopeIP {
			t.Errorf("scope が ip ではありません: %q", got.Scope)
		}
		// 生のIPを保存しないことがこの機能の前提（設計 6.2）。
		if strings.Contains(got.Subject, "192.0.2.1") {
			t.Errorf("生のIPがキーに入っています: %q", got.Subject)
		}
		if len(got.Subject) != 64 {
			t.Errorf("HMAC-SHA256 の hex は64文字のはずです: %q", got.Subject)
		}
	})

	t.Run("ログイン中はユーザーIDで数える", func(t *testing.T) {
		tokens, err := auth.NewJWT([]byte(authTestSecret))
		if err != nil {
			t.Fatalf("JWTの生成に失敗しました: %v", err)
		}
		access, err := tokens.Issue("user-1")
		if err != nil {
			t.Fatalf("アクセストークンの発行に失敗しました: %v", err)
		}

		uc := &stubResolveUseCase{}
		q := &stubQuota{allow: true}
		postResolveWith(t, uc, q, `{"text":"玉ねぎ"}`, access)

		if len(q.subjects) != 1 {
			t.Fatalf("1回判定するはずです: %d回", len(q.subjects))
		}
		if q.subjects[0].Scope != service.ScopeUser || q.subjects[0].Subject != "user-1" {
			t.Errorf("ユーザーIDで数えていません: %+v", q.subjects[0])
		}
	})

	t.Run("上限に達していたら理由をserviceに渡す", func(t *testing.T) {
		uc := &stubResolveUseCase{}
		q := &stubQuota{allow: false, reason: service.ReasonAnonDailyLimit}
		postResolveWith(t, uc, q, `{"text":"玉ねぎ"}`, "")

		if uc.policy.AllowLLM {
			t.Error("AllowLLM が false で渡るはずです")
		}
		if uc.policy.DenyReason != service.ReasonAnonDailyLimit {
			t.Errorf("理由が渡っていません: %q", uc.policy.DenyReason)
		}
		if uc.policy.Subject.Scope != service.ScopeIP {
			t.Errorf("キーが渡っていません: %+v", uc.policy.Subject)
		}
	})

	t.Run("上限に達していても400の検証が先に効く", func(t *testing.T) {
		uc := &stubResolveUseCase{}
		q := &stubQuota{allow: false, reason: service.ReasonAnonDailyLimit}
		rec := postResolveWith(t, uc, q, `{"text":"`+strings.Repeat("あ", 201)+`"}`, "")

		if rec.Code != http.StatusBadRequest {
			t.Errorf("400 を期待しましたが %d でした", rec.Code)
		}
		if uc.calls != 0 {
			t.Error("検証で落ちたのに service が呼ばれています")
		}
	})
}
