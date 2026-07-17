package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

func TestNewRecipeLink_有効なリンクを組み立てる(t *testing.T) {
	t.Parallel()

	got, err := domain.NewRecipeLink(
		"基本の親子丼レシピ",
		"https://example.com/recipes/oyakodon",
		"鶏もも肉と卵で作る失敗しない親子丼の作り方",
	)

	require.NoError(t, err)
	assert.Equal(t, "基本の親子丼レシピ", got.Title)
	assert.Equal(t, "https://example.com/recipes/oyakodon", got.URL)
	assert.Equal(t, "example.com", got.Domain)
	assert.Equal(t, "鶏もも肉と卵で作る失敗しない親子丼の作り方", got.Snippet)
}

func TestNewRecipeLink_ドメインを抽出する(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		url  string
		want string
	}{
		"サブドメイン":     {"https://a.example.com/x", "a.example.com"},
		"多段のサブドメイン":  {"https://a.b.example.co.jp/x", "a.b.example.co.jp"},
		"http":       {"http://example.com/x", "example.com"},
		"ポート番号は含めない": {"https://example.com:8443/x", "example.com"},
		"パスなし":       {"https://example.com", "example.com"},
		"クエリ付き":      {"https://example.com/s?q=1&r=2", "example.com"},
		// ホスト名は大小を区別しないため、表示とドメイン比較が揺れないよう小文字に寄せる。
		"大文字は小文字に正規化": {"https://EXAMPLE.com/x", "example.com"},
		"末尾ドットは除く":    {"https://example.com./x", "example.com"},
		"日本語ドメイン":     {"https://例え.jp/x", "例え.jp"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := domain.NewRecipeLink("タイトル", tt.url, "説明")

			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Domain)
			assert.Equal(t, tt.url, got.URL, "URLは受け取ったまま保持すること")
		})
	}
}

func TestNewRecipeLink_不正なURLを拒否する(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"空":            "",
		"空白のみ":         "   ",
		"スキームがない":      "example.com/x",
		"相対パス":         "/recipes/oyakodon",
		"ホストがない":       "https://",
		"ホストがない(パスのみ)": "https:///x",
		// http/https 以外は新しいタブで開く前提に合わない。javascript: と data: は
		// リンクとして描画するとスクリプト実行に繋がるため特に拒否する。
		"javascriptスキーム": "javascript:alert(1)",
		"dataスキーム":       "data:text/html,<script>alert(1)</script>",
		"fileスキーム":       "file:///etc/passwd",
		"ftpスキーム":        "ftp://example.com/x",
		"制御文字を含む":        "https://exa\nmple.com/x",
	}
	for name, url := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := domain.NewRecipeLink("タイトル", url, "説明")

			assert.ErrorIs(t, err, domain.ErrInvalidRecipeLink)
		})
	}
}

func TestNewRecipeLink_タイトルが空なら拒否する(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"空":    "",
		"空白のみ": "   ",
		"改行のみ": "\n\t",
	}
	for name, title := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := domain.NewRecipeLink(title, "https://example.com/x", "説明")

			assert.ErrorIs(t, err, domain.ErrInvalidRecipeLink)
		})
	}
}

func TestNewRecipeLink_スニペットは空でもよい(t *testing.T) {
	t.Parallel()

	// 検索APIがスニペットを返さないことがある。それだけでリンクを捨てると
	// 提示できる件数が減るため、空を許容する。
	got, err := domain.NewRecipeLink("タイトル", "https://example.com/x", "")

	require.NoError(t, err)
	assert.Empty(t, got.Snippet)
}

func TestNewRecipeLink_前後の空白を取り除く(t *testing.T) {
	t.Parallel()

	got, err := domain.NewRecipeLink("  タイトル  ", "  https://example.com/x  ", "  説明  ")

	require.NoError(t, err)
	assert.Equal(t, "タイトル", got.Title)
	assert.Equal(t, "https://example.com/x", got.URL)
	assert.Equal(t, "説明", got.Snippet)
}
