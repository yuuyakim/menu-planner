package gateway

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// ErrUnknownResolverProvider は食材解決のプロバイダ指定が解釈できないことを表す。
var ErrUnknownResolverProvider = errors.New("食材解決のプロバイダが不正です")

// 対応する食材解決プロバイダ。INGREDIENT_RESOLVER_PROVIDER に指定する値と一致する。
const (
	// ResolverProviderStub は外部APIを呼ばない（開発・CI・E2E用）。
	ResolverProviderStub = "stub"
	// ResolverProviderClaude は Claude Haiku 4.5 を使う。
	ResolverProviderClaude = "claude"
	// ResolverProviderDeepSeek は DeepSeek を使う。
	ResolverProviderDeepSeek = "deepseek"
)

// ResolverConfig は食材解決ゲートウェイの設定。
// 環境変数は読まず値として受け取る（Config と同じ方針）。
type ResolverConfig struct {
	Provider string
	APIKey   string
}

// NewResolver は設定に対応する食材解決ゲートウェイを組み立てる。
//
// **Provider が空でも stub に既定しない。** 設定を忘れたまま本番が動くと、
// 表記揺れが一切吸収されない状態を誰も気付けないまま配り続けることになる
// （Config.New と同じ判断）。
func NewResolver(cfg ResolverConfig) (service.IngredientResolveGateway, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case ResolverProviderStub:
		// 対応表を持たないスタブ。すべて「該当なし」を返す。
		return NewStubResolver(nil), nil
	case ResolverProviderClaude:
		return NewClaudeResolver(cfg.APIKey)
	case ResolverProviderDeepSeek:
		return NewDeepSeekResolver(cfg.APIKey)
	default:
		return nil, fmt.Errorf("%w: %q（%s / %s / %s）",
			ErrUnknownResolverProvider, cfg.Provider,
			ResolverProviderStub, ResolverProviderClaude, ResolverProviderDeepSeek)
	}
}
