package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalidRecipeLink はレシピリンクとして成立しない値を表す。
var ErrInvalidRecipeLink = errors.New("不正なレシピリンクです")

// RecipeLink は献立のレシピを掲載している外部ページへのリンク。
//
// 値は NewRecipeLink でのみ組み立てる。Domain を引数で受け取らず URL から導出するのは、
// 表示するドメインとリンク先が食い違う状態を作れないようにするため。
type RecipeLink struct {
	// Title はページのタイトル。
	Title string
	// URL はページのURL。http または https のみ。
	URL string
	// Domain は URL のホスト名（ポートを含まない）。どのサイトかを利用者に示すために使う。
	Domain string
	// Snippet は検索APIが返す抜粋。空のことがある。
	Snippet string
}

// NewRecipeLink はレシピリンクを組み立てる。
// URL は新しいタブで開くため、http/https 以外のスキームは受け付けない。
func NewRecipeLink(title, rawURL, snippet string) (RecipeLink, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return RecipeLink{}, fmt.Errorf("%w: タイトルが空です", ErrInvalidRecipeLink)
	}

	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil {
		return RecipeLink{}, fmt.Errorf("%w: URLを解析できません(%q): %w", ErrInvalidRecipeLink, rawURL, err)
	}

	// javascript: や data: をリンクとして描画するとスクリプト実行に繋がる。
	// ftp: や file: は新しいタブで開く前提に合わない。許可するものだけを列挙する。
	switch u.Scheme {
	case "http", "https":
	default:
		return RecipeLink{}, fmt.Errorf("%w: httpまたはhttpsのみ受け付けます(%q)", ErrInvalidRecipeLink, rawURL)
	}

	host := canonicalHost(u)
	if host == "" {
		return RecipeLink{}, fmt.Errorf("%w: ホストがありません(%q)", ErrInvalidRecipeLink, rawURL)
	}

	return RecipeLink{
		Title:   title,
		URL:     rawURL,
		Domain:  host,
		Snippet: strings.TrimSpace(snippet),
	}, nil
}

// canonicalHost は比較・表示に使うホスト名を返す。
// ホスト名は大小を区別せず、末尾のドット（DNSの絶対表記）も同じホストを指すため、
// 表記の揺れで別サイトに見えないよう正規化する。
func canonicalHost(u *url.URL) string {
	host := strings.TrimSuffix(u.Hostname(), ".")
	return strings.ToLower(host)
}
