// Package gateway は外部APIの呼び出しを担う。
// service が定義したインターフェース（service.RecipeSearchGateway）を実装する。
package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrEmptyMenuName は検索語となる献立名が空であることを表す。
var ErrEmptyMenuName = errors.New("献立名が空です")

// stubSites はスタブが返すレシピサイトの定型。
// ドメインは RFC 2606 で予約されている example.* を使う。実在サイトを騙ると
// 開発中に本物のレシピが返ってきたと誤解させるため。
var stubSites = []struct {
	host        string
	pathPrefix  string
	titleFormat string
	snippet     string
}{
	{"recipe.example.com", "/recipes/", "%sの基本レシピ", "定番の作り方を材料の分量から丁寧に解説します。"},
	{"cooking.example.net", "/dish/", "簡単！%sの作り方", "少ない材料と手順で作れる時短レシピです。"},
	{"kitchen.example.org", "/menu/", "プロが教える%s", "料理人が家庭向けに調整したひと工夫のあるレシピです。"},
}

// Stub は外部APIを呼ばずに定型のレシピリンクを返す。
// APIキーが無くても全機能を動かせる状態を保つために使う（開発・CI・E2E）。
//
// 状態を持たないため、複数の goroutine から同時に使ってよい。
type Stub struct{}

// NewStub はスタブのゲートウェイを返す。
func NewStub() Stub {
	return Stub{}
}

// Search は献立名から定型のレシピリンクを組み立てて返す。
// 外部通信をしないため ctx は参照せず、常に同じ入力に同じ結果を返す。
func (Stub) Search(_ context.Context, menuName string, limit int) ([]domain.RecipeLink, error) {
	menuName = strings.TrimSpace(menuName)
	if menuName == "" {
		// 空の検索語からは「「」の基本レシピ」のような無意味なリンクしか作れない。
		// 呼び出し側の不具合を握りつぶさずここで気付けるようにする。
		return nil, ErrEmptyMenuName
	}

	if limit > len(stubSites) {
		limit = len(stubSites)
	}
	if limit <= 0 {
		return []domain.RecipeLink{}, nil
	}

	links := make([]domain.RecipeLink, 0, limit)
	for _, site := range stubSites[:limit] {
		// 献立名をパスに入れるため、日本語やスラッシュをエスケープする。
		rawURL := "https://" + site.host + site.pathPrefix + url.PathEscape(menuName)

		link, err := domain.NewRecipeLink(fmt.Sprintf(site.titleFormat, menuName), rawURL, site.snippet)
		if err != nil {
			// 定型から組み立てているので、ここに来るのはスタブ自身の不具合。
			return nil, fmt.Errorf("スタブのレシピリンクを組み立てられませんでした(%q): %w", menuName, err)
		}
		links = append(links, link)
	}
	return links, nil
}
