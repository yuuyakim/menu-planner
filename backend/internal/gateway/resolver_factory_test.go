package gateway_test

import (
	"errors"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
)

func TestNewResolver(t *testing.T) {
	t.Run("stub を組み立てられる", func(t *testing.T) {
		got, err := gateway.NewResolver(gateway.ResolverConfig{Provider: "stub"})
		if err != nil {
			t.Fatalf("NewResolver が失敗しました: %v", err)
		}
		if got == nil {
			t.Fatal("nil が返りました")
		}
	})

	t.Run("大文字や空白が混じっても受ける", func(t *testing.T) {
		if _, err := gateway.NewResolver(gateway.ResolverConfig{Provider: " STUB "}); err != nil {
			t.Errorf("設定ミスではないので受けるべきです: %v", err)
		}
	})

	t.Run("claude はAPIキーが要る", func(t *testing.T) {
		_, err := gateway.NewResolver(gateway.ResolverConfig{Provider: "claude"})
		if !errors.Is(err, gateway.ErrMissingResolverAPIKey) {
			t.Errorf("ErrMissingResolverAPIKey を返すべきです: %v", err)
		}
	})

	t.Run("claude はAPIキーがあれば組み立てられる", func(t *testing.T) {
		got, err := gateway.NewResolver(gateway.ResolverConfig{
			Provider: "claude", APIKey: "sk-ant-dummy",
		})
		if err != nil {
			t.Fatalf("NewResolver が失敗しました: %v", err)
		}
		if got == nil {
			t.Fatal("nil が返りました")
		}
	})

	t.Run("deepseek はAPIキーが要る", func(t *testing.T) {
		_, err := gateway.NewResolver(gateway.ResolverConfig{Provider: "deepseek"})
		if !errors.Is(err, gateway.ErrMissingResolverAPIKey) {
			t.Errorf("ErrMissingResolverAPIKey を返すべきです: %v", err)
		}
	})

	t.Run("deepseek を組み立てられる", func(t *testing.T) {
		got, err := gateway.NewResolver(gateway.ResolverConfig{
			Provider: "deepseek", APIKey: "sk-dummy",
		})
		if err != nil {
			t.Fatalf("NewResolver が失敗しました: %v", err)
		}
		if got == nil {
			t.Fatal("nil が返りました")
		}
	})

	t.Run("未知のプロバイダはエラー", func(t *testing.T) {
		_, err := gateway.NewResolver(gateway.ResolverConfig{Provider: "gpt"})
		if !errors.Is(err, gateway.ErrUnknownResolverProvider) {
			t.Errorf("ErrUnknownResolverProvider を返すべきです: %v", err)
		}
	})

	t.Run("空でも stub に既定しない", func(t *testing.T) {
		_, err := gateway.NewResolver(gateway.ResolverConfig{Provider: ""})
		if err == nil {
			t.Error("設定を忘れたまま本番が動くのを防ぐため、エラーにするべきです")
		}
	})
}
