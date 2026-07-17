package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrMissingAPIKey はAPIキーが設定されていないことを表す。
var ErrMissingAPIKey = errors.New("検索APIのキーが設定されていません")

// braveEndpoint は Brave Web Search API のエンドポイント。
const braveEndpoint = "https://api.search.brave.com/res/v1/web/search"

// braveMaxCount は count パラメータの上限。これを超える値はAPIに拒否される。
const braveMaxCount = 20

// recipeQuerySuffix は検索語に付ける接尾辞。
// 献立名だけで検索すると料理の解説や通販が混ざるため（spec.md 2.3）。
const recipeQuerySuffix = " レシピ"

// Brave は Brave Web Search API を使ってレシピ掲載ページを検索する。
type Brave struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

// BraveOption は Brave の任意設定。
type BraveOption func(*Brave)

// WithEndpoint はエンドポイントを差し替える。テストで httptest.Server を指すために使う。
func WithEndpoint(endpoint string) BraveOption {
	return func(b *Brave) { b.endpoint = endpoint }
}

// WithHTTPClient は HTTP クライアントを差し替える。
func WithHTTPClient(c *http.Client) BraveOption {
	return func(b *Brave) { b.client = c }
}

// NewBrave は Brave のゲートウェイを生成する。
// APIキーが空の場合はエラーを返す。キー無しで組み立てられると、
// 実行時に401が返って初めて設定漏れに気付くことになるため。
func NewBrave(apiKey string, opts ...BraveOption) (*Brave, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrMissingAPIKey
	}

	b := &Brave{
		apiKey:   apiKey,
		endpoint: braveEndpoint,
		client:   http.DefaultClient,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b, nil
}

// Endpoint は問い合わせ先を返す。
func (b *Brave) Endpoint() string { return b.endpoint }

// braveResponse は Brave のレスポンスのうち本アプリが使う部分。
// APIは多くの項目を返すが、使うものだけを宣言して契約を小さく保つ。
type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// Search は「{献立名} レシピ」で検索し、上位 limit 件を返す。
func (b *Brave) Search(ctx context.Context, menuName string, limit int) ([]domain.RecipeLink, error) {
	menuName = strings.TrimSpace(menuName)
	if menuName == "" {
		// 無駄にAPIを消費しないよう、呼ぶ前に弾く。
		return nil, ErrEmptyMenuName
	}
	if limit <= 0 {
		return []domain.RecipeLink{}, nil
	}

	req, err := b.newRequest(ctx, menuName, limit)
	if err != nil {
		return nil, err
	}

	res, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("検索APIへのリクエストに失敗しました: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	var parsed braveResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("検索APIのレスポンスを解釈できませんでした: %w", err)
	}

	links := make([]domain.RecipeLink, 0, limit)
	for _, r := range parsed.Web.Results {
		if len(links) == limit {
			// count で要求した件数より多く返ることがある。契約は呼び出し側の limit。
			break
		}

		link, err := domain.NewRecipeLink(plainText(r.Title), r.URL, plainText(r.Description))
		if err != nil {
			// 1件が壊れていても他の結果まで捨てる理由はない。飛ばして次を採る。
			continue
		}
		links = append(links, link)
	}
	return links, nil
}

// newRequest は検索リクエストを組み立てる。
func (b *Brave) newRequest(ctx context.Context, menuName string, limit int) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("検索APIのリクエストを組み立てられませんでした: %w", err)
	}

	q := url.Values{}
	q.Set("q", menuName+recipeQuerySuffix)
	q.Set("count", strconv.Itoa(min(limit, braveMaxCount)))
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Accept", "application/json")
	// キーはヘッダでのみ渡す。クエリ文字列はアクセスログやRefererに残るため。
	req.Header.Set("X-Subscription-Token", b.apiKey)

	return req, nil
}

// tagPattern はHTMLタグに一致する。Brave は一致語を <strong> で囲んで返す。
var tagPattern = regexp.MustCompile(`<[^>]*>`)

// plainText はAPIが返すHTML片をプレーンテキストに落とす。
// そのまま画面に出すとタグが見え、HTMLとして描画するならXSSの経路になる。
//
// タグを除いてからエンティティを戻す。順序が逆だと、&lt;script&gt; が
// <script> になった後でタグとして除去され、文字列が消えてしまう。
func plainText(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(s, "")))
}
