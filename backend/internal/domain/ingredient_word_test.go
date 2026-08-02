package domain_test

import (
	"reflect"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestSplitIngredientWords(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"読点", "豚こま、玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"半角カンマ", "豚こま,玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"全角カンマ", "豚こま，玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"改行", "豚こま\n玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"半角空白", "豚こま 玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"全角空白", "豚こま　玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"中黒", "豚こま・玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"区切りの連続は1つ", "豚こま、、 玉ねぎ", []string{"豚こま", "玉ねぎ"}},
		{"前後の区切りは捨てる", "、豚こま、", []string{"豚こま"}},
		{"空文字", "", []string{}},
		{"区切りだけ", "、、、", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.SplitIngredientWords(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitIngredientWords(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeIngredientWord(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"前後の空白を除く", "  玉ねぎ  ", "玉ねぎ"},
		{"漢字かなはそのまま", "玉ねぎ", "玉ねぎ"},
		{"カタカナはひらがなに", "タマネギ", "たまねぎ"},
		{"ひらがなはそのまま", "たまねぎ", "たまねぎ"},
		{"半角カナは全角経由でひらがなに", "ﾀﾏﾈｷﾞ", "たまねぎ"},
		// 小文字化はしない方針なので、NFKC 後の大文字がそのまま残る（設計 3.4）。
		{"全角英数は半角に", "Ａ１", "A1"},
		{"長音は保つ", "ベーコン", "べーこん"},
		{"混在", "　ブタこま　", "ぶたこま"},
		{"空文字", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.NormalizeIngredientWord(tt.in); got != tt.want {
				t.Errorf("NormalizeIngredientWord(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
