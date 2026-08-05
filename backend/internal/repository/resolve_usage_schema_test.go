package repository_test

import (
	"context"
	"testing"
)

// TestResolveUsageCountersSchema は日次カウンタのテーブル定義を確かめる。
//
// 他のテスト関数と日付を分けているのは、このパッケージが1つのDBを共有しており、
// 後始末の TRUNCATE がこの表を対象にしていないため。
func TestResolveUsageCountersSchema(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	t.Run("total は subject を空にして入る", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'total', '', 1)`)
		if err != nil {
			t.Fatalf("total 行を保存できませんでした: %v", err)
		}
	})

	t.Run("未知の scope は入らない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'session', 'abc', 1)`)
		if err == nil {
			t.Fatal("CHECK 違反になるはずが成功しました")
		}
	})

	t.Run("total 以外は subject が必須", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'ip', '', 1)`)
		if err == nil {
			t.Fatal("CHECK 違反になるはずが成功しました")
		}
	})

	t.Run("total に subject は付けられない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'total', 'abc', 1)`)
		if err == nil {
			t.Fatal("CHECK 違反になるはずが成功しました")
		}
	})

	t.Run("同じ日付・scope・subject は二重に入らない", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO resolve_usage_counters (usage_date, scope, subject, count)
			 VALUES ('2026-01-10', 'total', '', 1)`)
		if err == nil {
			t.Fatal("主キー違反になるはずが成功しました")
		}
	})
}
