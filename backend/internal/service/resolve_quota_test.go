package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// fakeUsageCounter は日次カウンタの最小のインメモリ実装。
type fakeUsageCounter struct {
	// total は日付ごとの全体の回数。
	total map[string]int
	// own は "日付|scope|subject" ごとの回数。
	own map[string]int
	err error
	// days は Counts / Increment に渡された日付。JST の境界を検証する。
	days []string
}

func newFakeUsageCounter() *fakeUsageCounter {
	return &fakeUsageCounter{total: map[string]int{}, own: map[string]int{}}
}

func (c *fakeUsageCounter) key(day, scope, subject string) string {
	return day + "|" + scope + "|" + subject
}

func (c *fakeUsageCounter) Counts(
	_ context.Context, day, scope, subject string,
) (int, int, error) {
	c.days = append(c.days, day)
	if c.err != nil {
		return 0, 0, c.err
	}
	return c.total[day], c.own[c.key(day, scope, subject)], nil
}

func (c *fakeUsageCounter) Increment(
	_ context.Context, day, scope, subject string,
) error {
	c.days = append(c.days, day)
	if c.err != nil {
		return c.err
	}
	c.total[day]++
	c.own[c.key(day, scope, subject)]++
	return nil
}

// fixedQuotaNow は時刻を固定する。日付境界を確かめるために使う。
//
// パッケージ内には entitlement_test.go の `fixedNow`（時刻そのものの変数）が
// 既にあるため、同名は使えない。こちらは関数を返す点も違う。
func fixedQuotaNow(s string) func() time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("テストの時刻が不正です: " + err.Error())
	}
	return func() time.Time { return t }
}

var testLimits = service.ResolveQuotaLimits{Anon: 10, User: 30, Total: 300}

func anonSubject() service.ResolveSubject {
	return service.ResolveSubject{Scope: service.ScopeIP, Subject: "hash-a"}
}

func userSubject() service.ResolveSubject {
	return service.ResolveSubject{Scope: service.ScopeUser, Subject: "user-1"}
}

func TestResolveQuota_AllowsWithinLimit(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	// 9回まで使っている状態。10回目は通る。
	c.own[c.key("2026-08-05", "ip", "hash-a")] = 9
	q := service.NewResolveQuota(c, testLimits, fixedQuotaNow("2026-08-05T01:00:00Z"))

	allow, reason := q.Check(ctx, anonSubject())
	if !allow {
		t.Errorf("上限内なので通るはずです: reason=%q", reason)
	}
}

func TestResolveQuota_BlocksAtAnonLimit(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.own[c.key("2026-08-05", "ip", "hash-a")] = 10
	q := service.NewResolveQuota(c, testLimits, fixedQuotaNow("2026-08-05T01:00:00Z"))

	allow, reason := q.Check(ctx, anonSubject())
	if allow {
		t.Fatal("上限に達しているので止まるはずです")
	}
	if reason != service.ReasonAnonDailyLimit {
		t.Errorf("理由が anon_daily_limit ではありません: %q", reason)
	}
}

func TestResolveQuota_BlocksAtUserLimit(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.own[c.key("2026-08-05", "user", "user-1")] = 30
	q := service.NewResolveQuota(c, testLimits, fixedQuotaNow("2026-08-05T01:00:00Z"))

	allow, reason := q.Check(ctx, userSubject())
	if allow {
		t.Fatal("上限に達しているので止まるはずです")
	}
	if reason != service.ReasonUserDailyLimit {
		t.Errorf("理由が user_daily_limit ではありません: %q", reason)
	}
}

func TestResolveQuota_TotalWinsOverSubject(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	// 全体も本人も同時に上限。
	c.total["2026-08-05"] = 300
	c.own[c.key("2026-08-05", "ip", "hash-a")] = 10
	q := service.NewResolveQuota(c, testLimits, fixedQuotaNow("2026-08-05T01:00:00Z"))

	_, reason := q.Check(ctx, anonSubject())
	// 全体が詰まっているときに「ログインすると増えます」と出すと誤導になる（設計 5.1）。
	if reason != service.ReasonServiceDailyLimit {
		t.Errorf("理由が service_daily_limit ではありません: %q", reason)
	}
}

func TestResolveQuota_ZeroMeansUnlimited(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.total["2026-08-05"] = 100000
	c.own[c.key("2026-08-05", "ip", "hash-a")] = 100000
	q := service.NewResolveQuota(c,
		service.ResolveQuotaLimits{Anon: 0, User: 0, Total: 0},
		fixedQuotaNow("2026-08-05T01:00:00Z"))

	allow, _ := q.Check(ctx, anonSubject())
	if !allow {
		t.Fatal("無制限なので通るはずです")
	}
	// 無制限なら数を読む必要が無い。開発・E2Eで無駄な往復を出さない。
	if len(c.days) != 0 {
		t.Errorf("カウンタを読んでいます: %v", c.days)
	}
}

func TestResolveQuota_FailsClosedWhenCounterUnavailable(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.err = errors.New("DBが落ちました")
	q := service.NewResolveQuota(c, testLimits, fixedQuotaNow("2026-08-05T01:00:00Z"))

	allow, reason := q.Check(ctx, anonSubject())
	// カウンタが読めない状況ではキャッシュも読めておらず、全語がLLMに行く
	// ＝最も高い状態。素通しはコスト保護そのものを裏切る（設計 9.1）。
	if allow {
		t.Fatal("カウンタが読めないときは止まるはずです")
	}
	if reason != service.ReasonCounterUnavailable {
		t.Errorf("理由が counter_unavailable ではありません: %q", reason)
	}
}

func TestResolveQuota_UsesJSTDate(t *testing.T) {
	ctx := context.Background()

	t.Run("UTC14:59はまだJSTの当日", func(t *testing.T) {
		c := newFakeUsageCounter()
		q := service.NewResolveQuota(c, testLimits, fixedQuotaNow("2026-08-05T14:59:59Z"))
		q.Check(ctx, anonSubject())
		if c.days[0] != "2026-08-05" {
			t.Errorf("2026-08-05 を期待しましたが %q でした", c.days[0])
		}
	})

	t.Run("UTC15:00でJSTの翌日になる", func(t *testing.T) {
		c := newFakeUsageCounter()
		q := service.NewResolveQuota(c, testLimits, fixedQuotaNow("2026-08-05T15:00:00Z"))
		q.Check(ctx, anonSubject())
		if c.days[0] != "2026-08-06" {
			t.Errorf("2026-08-06 を期待しましたが %q でした", c.days[0])
		}
	})
}

func TestResolveQuota_RecordIncrements(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	q := service.NewResolveQuota(c, testLimits, fixedQuotaNow("2026-08-05T01:00:00Z"))

	if err := q.Record(ctx, anonSubject()); err != nil {
		t.Fatalf("Record が失敗しました: %v", err)
	}
	if c.total["2026-08-05"] != 1 {
		t.Errorf("全体が加算されていません: %d", c.total["2026-08-05"])
	}
	if c.own[c.key("2026-08-05", "ip", "hash-a")] != 1 {
		t.Error("利用者の枠が加算されていません")
	}
}

func TestResolveQuota_RecordReturnsError(t *testing.T) {
	ctx := context.Background()
	c := newFakeUsageCounter()
	c.err = errors.New("DBが落ちました")
	q := service.NewResolveQuota(c, testLimits, fixedQuotaNow("2026-08-05T01:00:00Z"))

	if err := q.Record(ctx, anonSubject()); err == nil {
		t.Fatal("加算に失敗したらエラーを返すはずです")
	}
}
