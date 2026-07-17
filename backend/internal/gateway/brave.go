package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// ErrMissingAPIKey はAPIキーが設定されていないことを表す。
var ErrMissingAPIKey = errors.New("検索APIのキーが設定されていません")

// ErrSearchFailed は外部の検索APIから結果を得られなかったことを表す。
// 呼び出し側はこれを 502 に変換する。設定漏れ（ErrMissingAPIKey）や
// 呼び出し方の誤り（ErrEmptyMenuName）とは区別する。
//
// 実体はインターフェースの持ち主が定義する service.ErrRecipeSearchFailed。
// service 側も自身が課した締め切り超過をこの失敗として返す必要があり、
// 判定を1つの識別子に揃えるため同じ値を指す。
var ErrSearchFailed = service.ErrRecipeSearchFailed

// braveEndpoint は Brave Web Search API のエンドポイント。
const braveEndpoint = "https://api.search.brave.com/res/v1/web/search"

// braveMaxCount は count パラメータの上限。これを超える値はAPIに拒否される。
const braveMaxCount = 20

// recipeQuerySuffix は検索語に付ける接尾辞。
// 献立名だけで検索すると料理の解説や通販が混ざるため（spec.md 2.3）。
const recipeQuerySuffix = " レシピ"

// 日本語のレシピを返させるための指定。
//
// 指定しないと英語圏のページが混ざる（実キーでの確認では「親子丼 レシピ」の
// 3件中2件が kurashiru.com/us/ と cookpad.com/eng/ だった）。
//
// braveSearchLang は "ja" ではなく "jp"。"ja" は 422 で拒否される。
// APIリファレンスが値を列挙していないため実測で確かめた。
const (
	braveCountry    = "JP"
	braveSearchLang = "jp"
)

// 既定値。
const (
	// defaultTimeout は1回の試行に許す時間。
	defaultTimeout = 3 * time.Second
	// defaultMaxRetries は初回失敗後に再試行する回数。
	defaultMaxRetries = 2
	// defaultBackoff は最初の再試行までの待ち時間。試行ごとに倍にする。
	defaultBackoff = 200 * time.Millisecond
)

// Brave は Brave Web Search API を使ってレシピ掲載ページを検索する。
type Brave struct {
	apiKey     string
	endpoint   string
	client     *http.Client
	timeout    time.Duration
	maxRetries int
	backoff    time.Duration
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

// WithTimeout は1回の試行に許す時間を変える。
func WithTimeout(d time.Duration) BraveOption {
	return func(b *Brave) { b.timeout = d }
}

// WithBackoff は最初の再試行までの待ち時間を変える。
func WithBackoff(d time.Duration) BraveOption {
	return func(b *Brave) { b.backoff = d }
}

// WithMaxRetries は再試行の回数を変える。
func WithMaxRetries(n int) BraveOption {
	return func(b *Brave) { b.maxRetries = n }
}

// NewBrave は Brave のゲートウェイを生成する。
// APIキーが空の場合はエラーを返す。キー無しで組み立てられると、
// 実行時に401が返って初めて設定漏れに気付くことになるため。
func NewBrave(apiKey string, opts ...BraveOption) (*Brave, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrMissingAPIKey
	}

	b := &Brave{
		apiKey:     apiKey,
		endpoint:   braveEndpoint,
		client:     http.DefaultClient,
		timeout:    defaultTimeout,
		maxRetries: defaultMaxRetries,
		backoff:    defaultBackoff,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b, nil
}

// Endpoint は問い合わせ先を返す。
func (b *Brave) Endpoint() string { return b.endpoint }

// Timeout は1回の試行に許す時間を返す。
func (b *Brave) Timeout() time.Duration { return b.timeout }

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

	parsed, err := b.fetch(ctx, menuName, limit)
	if err != nil {
		return nil, err
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

// fetch は検索APIを呼ぶ。一時的な失敗は指数バックオフで再試行する。
//
// 呼び出し側の ctx が切れている場合は1回も呼ばない。利用者が画面を離れた後に
// APIを消費しても意味がないため。
func (b *Brave) fetch(ctx context.Context, menuName string, limit int) (*braveResponse, error) {
	wait := b.backoff
	var lastErr error

	for attempt := 0; attempt <= b.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(err, lastErr)
		}

		parsed, err := b.attempt(ctx, menuName, limit)
		if err == nil {
			return parsed, nil
		}
		lastErr = err

		// 再試行しても結果が変わらないもの（4xx など）はここで打ち切る。
		if !isRetryable(err) || attempt == b.maxRetries {
			break
		}

		slog.Warn("検索APIの呼び出しに失敗しました。リトライします",
			"attempt", attempt+1, "max", b.maxRetries+1, "wait", wait, "error", err)

		select {
		case <-ctx.Done():
			return nil, errors.Join(ctx.Err(), lastErr)
		case <-time.After(wait):
		}
		wait *= 2
	}

	return nil, fmt.Errorf("%w: %w", ErrSearchFailed, lastErr)
}

// retryableError は時間をおけば成功しうる失敗を包む。
type retryableError struct{ err error }

func (e retryableError) Error() string { return e.err.Error() }
func (e retryableError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var r retryableError
	return errors.As(err, &r)
}

// attempt は検索APIを1回だけ呼ぶ。
func (b *Brave) attempt(ctx context.Context, menuName string, limit int) (*braveResponse, error) {
	// 1回の試行ごとに区切る。全体ではなく試行単位にすることで、
	// 遅い1回に引きずられて再試行の機会を失わないようにする。
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	req, err := b.newRequest(ctx, menuName, limit)
	if err != nil {
		return nil, err
	}

	res, err := b.client.Do(req)
	if err != nil {
		// 接続断やタイムアウトは一時的なことが多い。
		return nil, retryableError{fmt.Errorf("検索APIへのリクエストに失敗しました: %w", err)}
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		err := fmt.Errorf("検索APIが %d を返しました", res.StatusCode)
		if isRetryableStatus(res.StatusCode) {
			return nil, retryableError{err}
		}
		return nil, err
	}

	var parsed braveResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		// 応答が途中で切れた場合もここに来る。読み直す価値がある。
		return nil, retryableError{fmt.Errorf("検索APIのレスポンスを解釈できませんでした: %w", err)}
	}
	return &parsed, nil
}

// isRetryableStatus は再試行する価値のあるステータスかを返す。
//
// 4xx は原因が自分の側にあり、同じリクエストを繰り返しても同じ結果になるため
// 再試行しない。ただし 429 はレート制限であり、時間をおけば成功しうる。
func isRetryableStatus(status int) bool {
	return status >= 500 || status == http.StatusTooManyRequests
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
	q.Set("country", braveCountry)
	q.Set("search_lang", braveSearchLang)
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
