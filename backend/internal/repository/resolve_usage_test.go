package repository_test

import (
	"context"
	"testing"

	"github.com/yuuyakim/menu-planner/backend/internal/repository"
)

func TestResolveUsageRepository(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := repository.NewResolveUsageRepository(pool)

	// 日付はこのテスト関数専用のものを使う。このパッケージは1つのDBを共有し、
	// 後始末の TRUNCATE はこの表を対象にしていない。
	const day = "2026-02-01"
	const other = "2026-02-02"

	t.Run("加算すると全体と利用者の両方が増える", func(t *testing.T) {
		if err := repo.Increment(ctx, day, "ip", "hash-a"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		total, own, err := repo.Counts(ctx, day, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if total != 1 || own != 1 {
			t.Errorf("total=1, own=1 を期待しましたが total=%d, own=%d でした", total, own)
		}
	})

	t.Run("同じ利用者を重ねると加算される", func(t *testing.T) {
		if err := repo.Increment(ctx, day, "ip", "hash-a"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		total, own, err := repo.Counts(ctx, day, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if total != 2 || own != 2 {
			t.Errorf("total=2, own=2 を期待しましたが total=%d, own=%d でした", total, own)
		}
	})

	t.Run("別の利用者は自分の枠を消さないが全体は増やす", func(t *testing.T) {
		if err := repo.Increment(ctx, day, "ip", "hash-b"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		total, own, err := repo.Counts(ctx, day, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if total != 3 {
			t.Errorf("全体は 3 のはずです: %d", total)
		}
		if own != 2 {
			t.Errorf("hash-a の枠は 2 のままのはずです: %d", own)
		}
	})

	t.Run("scope が違えば別に数える", func(t *testing.T) {
		if err := repo.Increment(ctx, day, "user", "hash-a"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		_, own, err := repo.Counts(ctx, day, "user", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		// subject の文字列が同じでも ip と user は混ざらない。
		if own != 1 {
			t.Errorf("user スコープは 1 のはずです: %d", own)
		}
	})

	t.Run("日付が違えば0から数える", func(t *testing.T) {
		total, own, err := repo.Counts(ctx, other, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if total != 0 || own != 0 {
			t.Errorf("翌日は 0 から始まるはずです: total=%d, own=%d", total, own)
		}
	})

	t.Run("問い合わせたことのない利用者は0", func(t *testing.T) {
		_, own, err := repo.Counts(ctx, day, "ip", "hash-unknown")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if own != 0 {
			t.Errorf("0 のはずです: %d", own)
		}
	})

	t.Run("指定日より前の行を消せる", func(t *testing.T) {
		if err := repo.Increment(ctx, "2026-01-20", "ip", "hash-old"); err != nil {
			t.Fatalf("Increment が失敗しました: %v", err)
		}
		n, err := repo.DeleteOlderThan(ctx, "2026-02-01")
		if err != nil {
			t.Fatalf("DeleteOlderThan が失敗しました: %v", err)
		}
		if n < 1 {
			t.Errorf("1件以上消えるはずです: %d", n)
		}
		_, own, err := repo.Counts(ctx, "2026-01-20", "ip", "hash-old")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if own != 0 {
			t.Errorf("古い行が残っています: %d", own)
		}
		// 当日ぶんは残る。
		_, todayOwn, err := repo.Counts(ctx, day, "ip", "hash-a")
		if err != nil {
			t.Fatalf("Counts が失敗しました: %v", err)
		}
		if todayOwn != 2 {
			t.Errorf("当日の行まで消えています: %d", todayOwn)
		}
	})
}
