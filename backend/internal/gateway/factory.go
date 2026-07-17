package gateway

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// ErrUnknownProvider は検索プロバイダの指定が解釈できないことを表す。
var ErrUnknownProvider = errors.New("検索APIのプロバイダが不正です")

// 対応する検索プロバイダ。SEARCH_API_PROVIDER に指定する値と一致する。
const (
	// ProviderStub は外部APIを呼ばず定型のレシピを返す（開発・CI・E2E用）。
	ProviderStub = "stub"
	// ProviderBrave は Brave Web Search API を使う（spec.md 13.1）。
	ProviderBrave = "brave"
)

// Config は検索ゲートウェイの設定。
// 環境変数は読まず、値として受け取る。読み出しは結線側（main.go）の責務であり、
// こうしておくとテストで環境変数を触らずに済む。
type Config struct {
	// Provider は使用する検索プロバイダ（ProviderStub | ProviderBrave）。
	Provider string
	// APIKey は Provider が brave の場合に必須。
	APIKey string
	// Options は Brave に渡す任意設定。
	Options []BraveOption
}

// New は設定に対応する検索ゲートウェイを組み立てる。
//
// Provider が空でも stub には既定しない。設定を忘れたまま本番が動くと、
// 利用者にダミーのレシピを配り続けることになり、誰も気付けないため。
func New(cfg Config) (service.RecipeSearchGateway, error) {
	// .env の値に前後の空白や大文字が混じることは設定ミスではない。
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case ProviderStub:
		return NewStub(), nil
	case ProviderBrave:
		return NewBrave(cfg.APIKey, cfg.Options...)
	default:
		return nil, fmt.Errorf("%w: %q（%s または %s）", ErrUnknownProvider, cfg.Provider, ProviderStub, ProviderBrave)
	}
}
