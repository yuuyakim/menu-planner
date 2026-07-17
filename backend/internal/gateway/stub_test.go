package gateway_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// Stub が service の要求するインターフェースを満たすことをコンパイル時に保証する。
var _ service.RecipeSearchGateway = (*gateway.Stub)(nil)

func TestStub_3件返る(t *testing.T) {
	t.Parallel()

	got, err := gateway.NewStub().Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	require.Len(t, got, 3)
}

func TestStub_決定的に同じ結果を返す(t *testing.T) {
	t.Parallel()

	// APIキー無しで開発とE2Eを回すための実装なので、同じ入力なら常に同じ結果を返す。
	// ここが揺れるとE2Eが不安定になる。
	s := gateway.NewStub()

	first, err := s.Search(context.Background(), "親子丼", 3)
	require.NoError(t, err)
	second, err := s.Search(context.Background(), "親子丼", 3)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

func TestStub_別インスタンスでも同じ結果を返す(t *testing.T) {
	t.Parallel()

	// 内部に状態を持つと、プロセスをまたいだ再現性が失われる。
	first, err := gateway.NewStub().Search(context.Background(), "麻婆豆腐", 3)
	require.NoError(t, err)
	second, err := gateway.NewStub().Search(context.Background(), "麻婆豆腐", 3)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

func TestStub_献立名がタイトルに含まれる(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"親子丼", "ハンバーグ", "麻婆豆腐"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := gateway.NewStub().Search(context.Background(), name, 3)

			require.NoError(t, err)
			require.NotEmpty(t, got)
			for _, link := range got {
				assert.Contains(t, link.Title, name)
			}
		})
	}
}

func TestStub_献立名が違えば結果も違う(t *testing.T) {
	t.Parallel()

	oyakodon, err := gateway.NewStub().Search(context.Background(), "親子丼", 3)
	require.NoError(t, err)
	hamburg, err := gateway.NewStub().Search(context.Background(), "ハンバーグ", 3)
	require.NoError(t, err)

	assert.NotEqual(t, oyakodon, hamburg)
	assert.NotEqual(t, oyakodon[0].URL, hamburg[0].URL, "献立ごとにURLが分かれること")
}

func TestStub_limitで件数を絞る(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		limit int
		want  int
	}{
		"1件":        {1, 1},
		"2件":        {2, 2},
		"3件":        {3, 3},
		"3件を超えても3件": {10, 3},
		"0件は空":      {0, 0},
		"負数は空":      {-1, 0},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := gateway.NewStub().Search(context.Background(), "親子丼", tt.limit)

			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestStub_返るリンクは全て有効(t *testing.T) {
	t.Parallel()

	// domain.NewRecipeLink を通していれば Domain が URL から導出されている。
	// stub が作る値だけ検証を素通りする、という状態を防ぐ。
	got, err := gateway.NewStub().Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	for _, link := range got {
		assert.NotEmpty(t, link.Title)
		assert.NotEmpty(t, link.Snippet)
		assert.True(t, strings.HasPrefix(link.URL, "https://"), "URL: %s", link.URL)
		assert.NotEmpty(t, link.Domain)
		assert.Contains(t, link.URL, link.Domain, "DomainがURLと対応していること")
	}
}

func TestStub_リンクは重複しない(t *testing.T) {
	t.Parallel()

	got, err := gateway.NewStub().Search(context.Background(), "親子丼", 3)

	require.NoError(t, err)
	seen := map[string]bool{}
	for _, link := range got {
		assert.False(t, seen[link.URL], "URLが重複している: %s", link.URL)
		seen[link.URL] = true
	}
}

func TestStub_献立名が空ならエラー(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"空":    "",
		"空白のみ": "   ",
	}
	for name, menuName := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// 空の検索語で作ったリンクは「「」のレシピ」のような無意味な結果になる。
			// 呼び出し側の不具合を握りつぶさないようエラーにする。
			_, err := gateway.NewStub().Search(context.Background(), menuName, 3)

			assert.ErrorIs(t, err, gateway.ErrEmptyMenuName)
		})
	}
}

func TestStub_contextが切れていてもエラーにしない(t *testing.T) {
	t.Parallel()

	// stub は外部通信をしないため、context の状態に関係なく即座に返す。
	// 実 gateway との違いを明示しておく。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := gateway.NewStub().Search(ctx, "親子丼", 3)

	require.NoError(t, err)
	assert.Len(t, got, 3)
}
