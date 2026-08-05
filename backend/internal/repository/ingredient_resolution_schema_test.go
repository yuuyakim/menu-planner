package repository_test

import (
	"context"
	"testing"
)

// TestIngredientResolutionsSchema は解決キャッシュのテーブル定義を確かめる。
func TestIngredientResolutionsSchema(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	t.Run("未解決はNULLで保存できる", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO ingredient_resolutions (input_word, ingredient_id)
			 VALUES ('まつたけ', NULL)
			 ON CONFLICT (input_word) DO NOTHING`)
		if err != nil {
			t.Fatalf("NULL の解決を保存できませんでした: %v", err)
		}
	})

	t.Run("同じ語は二重に入らない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO ingredient_resolutions (input_word, ingredient_id)
			 VALUES ('まつたけ', NULL)`)
		if err == nil {
			t.Fatal("主キー違反になるはずが成功しました")
		}
	})

	t.Run("存在しない食材IDは入らない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO ingredient_resolutions (input_word, ingredient_id)
			 VALUES ('ありえない', '00000000-0000-0000-0000-000000000001')`)
		if err == nil {
			t.Fatal("外部キー違反になるはずが成功しました")
		}
	})
}
