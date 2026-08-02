package repository_test

import (
	"context"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func TestResolutionRepository(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewResolutionRepository(pool)

	// **既存行を借りない。** テスト用DBの ingredients は
	// TestMain がシードするものではなく、他のテストが insertIngredient で
	// 足したものが溜まっているだけ。`SELECT ... LIMIT 1` に頼ると
	// 実行順に依存し、-run で単独実行したときに落ちる。自分で1件作る。
	//
	// insertIngredient は既存のヘルパー（ingredients_schema_test.go）で、
	// 生のUUID文字列を返す。
	rawID := insertIngredient(t, pool, "テスト用豚肉", "てすとようぶたにく", "meat")
	id, err := domain.ParseIngredientID(rawID)
	if err != nil {
		t.Fatalf("食材IDを解釈できませんでした: %v", err)
	}

	t.Run("保存した解決を引ける", func(t *testing.T) {
		if err := repo.Save(ctx, "ぶたこま", &id); err != nil {
			t.Fatalf("Save が失敗しました: %v", err)
		}
		got, err := repo.FindByWords(ctx, []string{"ぶたこま"})
		if err != nil {
			t.Fatalf("FindByWords が失敗しました: %v", err)
		}
		v, ok := got["ぶたこま"]
		if !ok {
			t.Fatal("保存したはずの語が引けませんでした")
		}
		if v == nil || *v != id {
			t.Errorf("食材IDが一致しません: got %v, want %v", v, id)
		}
	})

	t.Run("未解決(NULL)も引ける", func(t *testing.T) {
		if err := repo.Save(ctx, "まつたけ2", nil); err != nil {
			t.Fatalf("Save が失敗しました: %v", err)
		}
		got, err := repo.FindByWords(ctx, []string{"まつたけ2"})
		if err != nil {
			t.Fatalf("FindByWords が失敗しました: %v", err)
		}
		v, ok := got["まつたけ2"]
		if !ok {
			t.Fatal("未解決として保存した語が引けませんでした")
		}
		if v != nil {
			t.Errorf("未解決は nil であるべきです: got %v", v)
		}
	})

	t.Run("未知の語はキーごと入らない", func(t *testing.T) {
		got, err := repo.FindByWords(ctx, []string{"まだきいてない"})
		if err != nil {
			t.Fatalf("FindByWords が失敗しました: %v", err)
		}
		if _, ok := got["まだきいてない"]; ok {
			t.Error("問い合わせたことのない語がキーに入っています")
		}
	})

	t.Run("同じ語の再保存は上書きされる", func(t *testing.T) {
		if err := repo.Save(ctx, "うわがき", nil); err != nil {
			t.Fatalf("1回目の Save が失敗しました: %v", err)
		}
		if err := repo.Save(ctx, "うわがき", &id); err != nil {
			t.Fatalf("2回目の Save が失敗しました: %v", err)
		}
		got, _ := repo.FindByWords(ctx, []string{"うわがき"})
		if got["うわがき"] == nil {
			t.Error("上書きされていません")
		}
	})

	t.Run("0件の問い合わせは空を返す", func(t *testing.T) {
		got, err := repo.FindByWords(ctx, nil)
		if err != nil {
			t.Fatalf("FindByWords が失敗しました: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("空であるべきです: got %v", got)
		}
	})
}
