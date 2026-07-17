package gateway_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
)

func TestNew_stubでスタブが返る(t *testing.T) {
	t.Parallel()

	got, err := gateway.New(gateway.Config{Provider: "stub"})

	require.NoError(t, err)
	assert.IsType(t, gateway.Stub{}, got)
}

func TestNew_stubはAPIキーが無くてよい(t *testing.T) {
	t.Parallel()

	// キー無しで全機能が動く状態を保つのが stub の目的（spec.md 8章）。
	got, err := gateway.New(gateway.Config{Provider: "stub", APIKey: ""})

	require.NoError(t, err)
	require.NotNil(t, got)

	links, err := got.Search(context.Background(), "親子丼", 3)
	require.NoError(t, err)
	assert.Len(t, links, 3, "返ったゲートウェイが実際に使えること")
}

func TestNew_braveで実装が返る(t *testing.T) {
	t.Parallel()

	got, err := gateway.New(gateway.Config{Provider: "brave", APIKey: "key"})

	require.NoError(t, err)
	assert.IsType(t, &gateway.Brave{}, got)
}

func TestNew_braveでAPIキーが空ならエラー(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"空":    "",
		"空白のみ": "   ",
	}
	for name, key := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// 起動時に落とす。キー無しで起動できると、実際に検索されるまで
			// 設定漏れに気付けない。
			_, err := gateway.New(gateway.Config{Provider: "brave", APIKey: key})

			assert.ErrorIs(t, err, gateway.ErrMissingAPIKey)
		})
	}
}

func TestNew_未知のプロバイダでエラー(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"知らない名前":     "duckduckgo",
		"廃止されたプロバイダ": "google_cse",
		"タイプミス":      "bravo",
	}
	for name, provider := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := gateway.New(gateway.Config{Provider: provider, APIKey: "key"})

			assert.ErrorIs(t, err, gateway.ErrUnknownProvider)
			assert.Contains(t, err.Error(), provider, "どの値が弾かれたか分かること")
		})
	}
}

func TestNew_プロバイダが空ならエラー(t *testing.T) {
	t.Parallel()

	// stub に既定しない。設定を忘れたまま本番が動くと、利用者に
	// ダミーのレシピを配り続けることになり、誰も気付けない。
	tests := map[string]string{
		"空":    "",
		"空白のみ": "  ",
	}
	for name, provider := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := gateway.New(gateway.Config{Provider: provider, APIKey: "key"})

			assert.ErrorIs(t, err, gateway.ErrUnknownProvider)
		})
	}
}

func TestNew_プロバイダ名の表記ゆれを吸収する(t *testing.T) {
	t.Parallel()

	// .env の値に前後の空白や大文字が混じっても、設定ミス扱いにはしない。
	tests := map[string]string{
		"大文字":   "BRAVE",
		"混在":    "Brave",
		"前後の空白": "  brave  ",
	}
	for name, provider := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := gateway.New(gateway.Config{Provider: provider, APIKey: "key"})

			require.NoError(t, err)
			assert.IsType(t, &gateway.Brave{}, got)
		})
	}
}

func TestNew_braveのオプションが渡る(t *testing.T) {
	t.Parallel()

	got, err := gateway.New(gateway.Config{
		Provider: "brave",
		APIKey:   "key",
		Options:  []gateway.BraveOption{gateway.WithEndpoint("https://example.com/search")},
	})

	require.NoError(t, err)
	b, ok := got.(*gateway.Brave)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/search", b.Endpoint())
}

func TestProviders_定数が値と一致する(t *testing.T) {
	t.Parallel()

	// .env.example や spec.md に書いた文字列と実装がずれないようにする。
	assert.Equal(t, "stub", gateway.ProviderStub)
	assert.Equal(t, "brave", gateway.ProviderBrave)
}
