package service

import "context"

// GatewayResolution は1語ぶんの対応づけ結果。
type GatewayResolution struct {
	// Word は問い合わせた語（正規化済み）。
	Word string
	// Name は対応づいた食材名。**空文字は「該当なし」を表す。**
	Name string
}

// IngredientResolveGateway は未解決語を食材名に対応づける外部サービス。
//
// RecipeSearchGateway（spec.md 3.3）と同じく、実装差し替えのみで
// プロバイダを切り替えられる状態にする。
//
// **食材IDではなく食材名をやり取りする**（設計 3.5）。UUID は36文字あり
// 166件で約3000トークンに達するうえ、1文字の取り違えが解決失敗になる。
// 名前で受けてアプリ側でIDに引き直せば、マスタに無い名前が返っても
// 「該当なし」に落ちるだけで済む。
type IngredientResolveGateway interface {
	// Resolve は words を catalog のいずれかの食材名に対応づける。
	// 戻り値は words と同じ件数・同じ順序であることを期待する。
	Resolve(ctx context.Context, words []string, catalog []string) ([]GatewayResolution, error)
}
