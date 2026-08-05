package domain

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// ingredientWordSeparators は手持ちの食材テキストの区切り文字（設計 3.4）。
//
// 「・」を入れるのは、買い物メモの書き方として一般的なため。既存の画面も
// 「使える食材: A・B・C」と中黒で並べている。区切り文字を含む食材名は
// マスタに存在しないため、誤分割の恐れはない。
const ingredientWordSeparators = "、,，\n 　・"

// SplitIngredientWords は入力テキストを語に分割する。
//
// 連続した区切りは1つとして扱い、空になった語は捨てる。
// **正規化はここではしない**（呼び出し側が NormalizeIngredientWord を通す）。
// 分割と正規化を分けておくと、利用者が書いた元の語を保持したまま
// 正規化後の値で突き合わせられる（設計 4.1 の `word` は元の語を返す）。
func SplitIngredientWords(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return strings.ContainsRune(ingredientWordSeparators, r)
	})

	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if trimmed := strings.TrimSpace(f); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// NormalizeIngredientWord は1語を突き合わせ用の形に正規化する（設計 3.4）。
//
//  1. 前後の空白を除去
//  2. NFKC 正規化（全角英数→半角、半角カナ→全角カナ）
//  3. カタカナ → ひらがな
//
// 3を入れるのは ingredients.name_kana に当たるようにするため。
// 「タマネギ」→「たまねぎ」で name_kana と一致し、LLM を呼ばずに解決できる。
//
// **小文字化はしない。** 日本語主体のため効果が薄く、食材名に含まれる
// 英字（例: Ｌサイズ）の見た目を変える副作用の方が大きい。
//
// **この関数は ①完全一致 / ②キャッシュ / ③LLM の結果引き当ての
// すべてが通る。** 別々に正規化すると、①で引けなかった語が②でも
// 引けないという穴が開く。
func NormalizeIngredientWord(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = norm.NFKC.String(s)
	return katakanaToHiragana(s)
}

// katakanaToHiragana は全角カタカナをひらがなに変換する。
//
// 長音「ー」や中黒はカタカナの範囲外なのでそのまま残る。
// ヴ（U+30F4）などの濁音付きも符号位置の差が同じなので同じ式で変換できる。
func katakanaToHiragana(s string) string {
	return strings.Map(func(r rune) rune {
		// 'ァ'(U+30A1) 〜 'ヶ'(U+30F6) を 'ぁ'(U+3041) 〜 に寄せる。
		if r >= 'ァ' && r <= 'ヶ' {
			return r - 0x60
		}
		return r
	}, s)
}
