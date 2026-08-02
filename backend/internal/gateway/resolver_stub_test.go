package gateway_test

import (
	"context"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/gateway"
)

func TestStubResolver(t *testing.T) {
	ctx := context.Background()
	r := gateway.NewStubResolver(map[string]string{"ぶたこま": "豚肉"})

	t.Run("対応づけのある語は食材名を返す", func(t *testing.T) {
		got, err := r.Resolve(ctx, []string{"ぶたこま"}, []string{"豚肉", "玉ねぎ"})
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got) != 1 || got[0].Word != "ぶたこま" || got[0].Name != "豚肉" {
			t.Errorf("想定と違います: %+v", got)
		}
	})

	t.Run("対応づけの無い語は空文字を返す", func(t *testing.T) {
		got, err := r.Resolve(ctx, []string{"まつたけ"}, []string{"豚肉"})
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got) != 1 || got[0].Name != "" {
			t.Errorf("該当なしは空文字であるべきです: %+v", got)
		}
	})

	t.Run("問い合わせた語の数だけ返る", func(t *testing.T) {
		got, err := r.Resolve(ctx, []string{"ぶたこま", "まつたけ"}, []string{"豚肉"})
		if err != nil {
			t.Fatalf("Resolve が失敗しました: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("2件返るべきです: got %d", len(got))
		}
	})
}
