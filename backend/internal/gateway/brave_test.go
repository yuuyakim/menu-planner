package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

var _ service.RecipeSearchGateway = (*gateway.Brave)(nil)

const testAPIKey = "test-token"

// braveResult は Brave のレスポンスに含まれる1件。テスト用に必要な項目だけ持つ。
type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// braveServer は Brave の web search を模したサーバを起動する。
// 受け取ったリクエストは capture に記録し、呼び出し側が検証できるようにする。
func braveServer(t *testing.T, results []braveResult, capture *http.Request) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			*capture = *r.Clone(context.Background())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "search",
			"web": map[string]any{
				"type":    "search",
				"results": results,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestBrave(t *testing.T, srv *httptest.Server) *gateway.Brave {
	t.Helper()

	g, err := gateway.NewBrave(testAPIKey, gateway.WithEndpoint(srv.URL))
	require.NoError(t, err)
	return g
}

func sampleResults(n int) []braveResult {
	all := []braveResult{
		{"親子丼の作り方", "https://recipe.example.com/1", "鶏肉と卵の定番"},
		{"簡単 親子丼", "https://cooking.example.net/2", "15分で完成"},
		{"本格 親子丼", "https://kitchen.example.org/3", "出汁からとる"},
		{"親子丼 アレンジ", "https://food.example.com/4", "きのこを加える"},
		{"親子丼 弁当", "https://bento.example.net/5", "冷めても美味しい"},
	}
	return all[:n]
}

func TestBrave_正常系で3件返る(t *testing.T) {
	t.Parallel()

	srv := braveServer(t, sampleResults(3), nil)

	got, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	require.Len(t, got, 3)

	assert.Equal(t, "親子丼の作り方", got[0].Title)
	assert.Equal(t, "https://recipe.example.com/1", got[0].URL)
	assert.Equal(t, "recipe.example.com", got[0].Domain, "ドメインがURLから導出されること")
	assert.Equal(t, "鶏肉と卵の定番", got[0].Snippet)
}

func TestBrave_リクエストの内容(t *testing.T) {
	t.Parallel()

	var got http.Request
	srv := braveServer(t, sampleResults(3), &got)

	_, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 3)
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, testAPIKey, got.Header.Get("X-Subscription-Token"), "APIキーがヘッダで渡ること")
	assert.Equal(t, "application/json", got.Header.Get("Accept"))

	q := got.URL.Query()
	// spec.md 2.3 の「{献立名} レシピ」で検索する。
	assert.Equal(t, "親子丼 レシピ", q.Get("q"))
	assert.Equal(t, "3", q.Get("count"), "必要な件数だけ要求すること")
}

func TestBrave_APIキーはURLに載せない(t *testing.T) {
	t.Parallel()

	// クエリ文字列はアクセスログやRefererに残る。キーはヘッダだけで渡す。
	var got http.Request
	srv := braveServer(t, sampleResults(1), &got)

	_, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 1)
	require.NoError(t, err)

	assert.NotContains(t, got.URL.RawQuery, testAPIKey)
}

func TestBrave_3件未満ならその件数を返す(t *testing.T) {
	t.Parallel()

	tests := map[string]int{"1件": 1, "2件": 2}
	for name, n := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := braveServer(t, sampleResults(n), nil)

			got, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 3)

			require.NoError(t, err)
			assert.Len(t, got, n)
		})
	}
}

func TestBrave_4件以上返っても3件に切り詰める(t *testing.T) {
	t.Parallel()

	// count で3件を要求しても、APIが多く返す可能性はある。契約は呼び出し側の limit。
	srv := braveServer(t, sampleResults(5), nil)

	got, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "https://kitchen.example.org/3", got[2].URL, "先頭から3件が採られること")
}

func TestBrave_0件でも成功扱い(t *testing.T) {
	t.Parallel()

	// 該当なしは障害ではない。呼び出し側が 502 ではなく空表示にできるようにする。
	srv := braveServer(t, []braveResult{}, nil)

	got, err := newTestBrave(t, srv).Search(context.Background(), "存在しない料理", 3)

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NotNil(t, got, "nilではなく空スライスを返すこと")
}

func TestBrave_webフィールドが無くても成功扱い(t *testing.T) {
	t.Parallel()

	// web の結果が1件も無い場合、web ごと省かれることがある。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"search","query":{"original":"x"}}`))
	}))
	t.Cleanup(srv.Close)

	got, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestBrave_スニペットのHTMLを除去する(t *testing.T) {
	t.Parallel()

	// Brave は一致語を <strong> で囲んで返す。そのまま画面に出すと
	// タグが見えるか、HTMLとして描画するならXSSの経路になる。テキストに落とす。
	srv := braveServer(t, []braveResult{{
		Title:       "<strong>親子丼</strong>の作り方",
		URL:         "https://recipe.example.com/1",
		Description: "<strong>親子丼</strong>は<strong>鶏肉</strong>と卵で作る",
	}}, nil)

	got, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "親子丼の作り方", got[0].Title)
	assert.Equal(t, "親子丼は鶏肉と卵で作る", got[0].Snippet)
}

func TestBrave_HTMLエンティティを戻す(t *testing.T) {
	t.Parallel()

	srv := braveServer(t, []braveResult{{
		Title:       "鶏肉 &amp; 卵のレシピ",
		URL:         "https://recipe.example.com/1",
		Description: "&quot;親子丼&quot; の作り方 &lt;決定版&gt;",
	}}, nil)

	got, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "鶏肉 & 卵のレシピ", got[0].Title)
	assert.Equal(t, `"親子丼" の作り方 <決定版>`, got[0].Snippet)
}

func TestBrave_不正なリンクは飛ばして残りを返す(t *testing.T) {
	t.Parallel()

	// 外部APIの1件が壊れていても、他の結果まで捨てる理由はない。
	srv := braveServer(t, []braveResult{
		{"javascriptのリンク", "javascript:alert(1)", "スキームが不正"},
		{"", "https://recipe.example.com/1", "タイトルが空"},
		{"<strong></strong>", "https://recipe.example.com/2", "タグを除くとタイトルが空になる"},
		{"URLが空", "", "説明"},
		{"正常なリンク", "https://cooking.example.net/3", "説明"},
	}, nil)

	got, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	require.Len(t, got, 1, "壊れた4件は飛ばし、正常な1件だけを返すこと")
	assert.Equal(t, "https://cooking.example.net/3", got[0].URL)
}

func TestBrave_不正なリンクを飛ばした上でlimitまで採る(t *testing.T) {
	t.Parallel()

	srv := braveServer(t, []braveResult{
		{"壊れている", "javascript:alert(1)", "説明"},
		{"1件目", "https://a.example.com/1", "説明"},
		{"2件目", "https://b.example.com/2", "説明"},
		{"3件目", "https://c.example.com/3", "説明"},
		{"4件目", "https://d.example.com/4", "説明"},
	}, nil)

	got, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "https://a.example.com/1", got[0].URL)
	assert.Equal(t, "https://c.example.com/3", got[2].URL)
}

func TestBrave_limitで件数を絞る(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		limit int
		want  int
	}{
		"1件":   {1, 1},
		"0件は空": {0, 0},
		"負数は空": {-1, 0},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := braveServer(t, sampleResults(5), nil)

			got, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", tt.limit)

			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestBrave_limitが上限を超えてもAPIには20までしか要求しない(t *testing.T) {
	t.Parallel()

	// Brave の count は最大20。それを超える値を渡すとAPIがエラーを返す。
	var got http.Request
	srv := braveServer(t, sampleResults(5), &got)

	_, err := newTestBrave(t, srv).Search(context.Background(), "親子丼", 50)

	require.NoError(t, err)
	assert.Equal(t, "20", got.URL.Query().Get("count"))
}

func TestBrave_献立名が空ならAPIを呼ばずエラー(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	_, err := newTestBrave(t, srv).Search(context.Background(), "  ", 3)

	assert.ErrorIs(t, err, gateway.ErrEmptyMenuName)
	assert.False(t, called, "無駄にAPIを消費しないこと")
}

func TestBrave_APIキーが空なら生成に失敗する(t *testing.T) {
	t.Parallel()

	// キー無しで組み立てられると、実行時に401で初めて気付くことになる。
	_, err := gateway.NewBrave("")

	assert.ErrorIs(t, err, gateway.ErrMissingAPIKey)
}

func TestBrave_既定のエンドポイントはBraveの公式URL(t *testing.T) {
	t.Parallel()

	g, err := gateway.NewBrave(testAPIKey)

	require.NoError(t, err)
	assert.Equal(t, "https://api.search.brave.com/res/v1/web/search", g.Endpoint())
}

func TestBrave_クエリはURLエスケープされる(t *testing.T) {
	t.Parallel()

	var got http.Request
	srv := braveServer(t, sampleResults(1), &got)

	_, err := newTestBrave(t, srv).Search(context.Background(), "肉じゃが&煮物", 1)
	require.NoError(t, err)

	// 生のクエリ文字列ではエスケープされ、解釈すると元に戻る。
	assert.NotContains(t, got.URL.RawQuery, "肉じゃが&煮物")
	q, err := url.ParseQuery(got.URL.RawQuery)
	require.NoError(t, err)
	assert.Equal(t, "肉じゃが&煮物 レシピ", q.Get("q"))
}
