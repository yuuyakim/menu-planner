package gateway

import (
	"context"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// StubResolver は外部APIを呼ばずに、あらかじめ渡された対応表で解決する。
//
// APIキーが無くても全機能を動かせる状態を保つために使う（開発・CI・E2E）。
// Stub（レシピ検索）と同じ役割で、こちらは対応表を差し替えられるようにして
// テストから期待する解決結果を与えられるようにしている。
//
// 状態を変えないため、複数の goroutine から同時に使ってよい。
type StubResolver struct {
	mapping map[string]string
}

// NewStubResolver は対応表を持つスタブを返す。
// mapping のキーは正規化済みの語、値は食材名。
func NewStubResolver(mapping map[string]string) StubResolver {
	return StubResolver{mapping: mapping}
}

// Resolve は対応表を引く。**catalog は参照しない。**
// スタブの目的は決定的な結果を返すことであり、マスタとの整合は
// 呼び出し側（service）が名前→IDの引き当てで確かめる。
func (s StubResolver) Resolve(
	_ context.Context, words []string, _ []string,
) ([]service.GatewayResolution, error) {
	out := make([]service.GatewayResolution, 0, len(words))
	for _, w := range words {
		// 対応表に無ければ空文字＝該当なし。
		out = append(out, service.GatewayResolution{Word: w, Name: s.mapping[w]})
	}
	return out, nil
}
