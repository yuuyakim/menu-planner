package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// jst は日次の境界を決めるタイムゾーン（設計 6.1）。
//
// time.LoadLocation("Asia/Tokyo") はコンテナに tzdata を要求する。
// 日本にサマータイムは無いため、固定オフセットで足りる。
var jst = time.FixedZone("JST", 9*60*60)

// スコープの値。DB の CHECK 制約と揃える。
const (
	// ScopeIP は非ログイン。subject は IP のHMAC。
	ScopeIP = "ip"
	// ScopeUser はログインユーザー。subject はユーザーID。
	ScopeUser = "user"
)

// ResolveUsageCounter は日次カウンタの読み書き。
type ResolveUsageCounter interface {
	// Counts は指定日の「全体」と「その利用者」の回数を返す。
	Counts(ctx context.Context, day, scope, subject string) (total, own int, err error)
	// Increment は「全体」と「その利用者」を1つずつ加算する。
	Increment(ctx context.Context, day, scope, subject string) error
}

// ResolveSubject は誰のぶんとして数えるかを表す。
type ResolveSubject struct {
	// Scope は ScopeIP または ScopeUser。
	Scope string
	// Subject は IP のHMAC またはユーザーID。**生のIPは入れない**（設計 6.2）。
	Subject string
}

// ResolveQuotaLimits は日次の上限。**0以下は無制限**（設計 7章）。
//
// 0 を無制限にするのは既存 RateLimiter と同じ約束。Vite プロキシ配下では
// 全リクエストが単一IPに集約されるため、開発と E2E ではこれで切る。
type ResolveQuotaLimits struct {
	Anon  int
	User  int
	Total int
}

// ResolveQuota は「読み取る」の日次上限を判定し、実績を記録する。
//
// 判定（Check）と記録（Record）が別のメソッドなのは、LLM を呼ぶかどうかが
// ①完全一致・②キャッシュを試すまで決まらないため。handler が Check し、
// service が③の直前で使い、呼んだら Record する（設計 5章）。
type ResolveQuota struct {
	counter ResolveUsageCounter
	limits  ResolveQuotaLimits
	now     func() time.Time
}

// NewResolveQuota は ResolveQuota を生成する。
func NewResolveQuota(
	c ResolveUsageCounter, l ResolveQuotaLimits, now func() time.Time,
) *ResolveQuota {
	return &ResolveQuota{counter: c, limits: l, now: now}
}

// Check は今日の残枠を読む。allow=false のとき、2つ目の戻り値に理由が入る。
//
// **判定は「前回までの実績」を見る。** 同時に走るリクエストは互いを見ないため、
// max-instances=2 のもとで数件ぶん超過しうる。金額にして数円なので受け入れる。
func (q *ResolveQuota) Check(ctx context.Context, s ResolveSubject) (bool, DegradedReason) {
	own := q.subjectLimit(s)
	if q.limits.Total <= 0 && own <= 0 {
		// 全体も利用者も無制限。数を読む意味が無い。
		return true, ""
	}

	day := q.day()
	total, used, err := q.counter.Counts(ctx, day, s.Scope, s.Subject)
	if err != nil {
		// フェイルクローズ（設計 9.1）。カウンタが読めない状況では解決キャッシュも
		// 読めておらず、全語がLLMに行く＝最も高い状態になっている。
		slog.WarnContext(ctx, "読み取りカウンタを読めませんでした。LLMをスキップします",
			"error", err)
		return false, ReasonCounterUnavailable
	}

	// 全体 → 利用者 の順（設計 5.1）。逆にすると、全体が詰まっているときに
	// 非ログインへ「ログインすると増えます」と出てしまい誤導になる。
	if q.limits.Total > 0 && total >= q.limits.Total {
		slog.WarnContext(ctx, "読み取りの全体上限に達しました",
			"day", day, "count", total, "limit", q.limits.Total)
		return false, ReasonServiceDailyLimit
	}
	if own > 0 && used >= own {
		reason := ReasonAnonDailyLimit
		if s.Scope == ScopeUser {
			reason = ReasonUserDailyLimit
		}
		slog.WarnContext(ctx, "読み取りの日次上限に達しました",
			"day", day, "scope", s.Scope, "count", used, "limit", own)
		return false, reason
	}
	return true, ""
}

// Record は LLM を呼んだ実績を1つ数える。
//
// 上限を切っていても数える。使用量を後から振り返れるようにするため。
func (q *ResolveQuota) Record(ctx context.Context, s ResolveSubject) error {
	if err := q.counter.Increment(ctx, q.day(), s.Scope, s.Subject); err != nil {
		return fmt.Errorf("読み取りカウンタの加算に失敗しました: %w", err)
	}
	return nil
}

// subjectLimit はその利用者に当てる上限を返す。
func (q *ResolveQuota) subjectLimit(s ResolveSubject) int {
	if s.Scope == ScopeUser {
		return q.limits.User
	}
	return q.limits.Anon
}

// day は JST での今日を 2006-01-02 で返す。
func (q *ResolveQuota) day() string {
	return q.now().In(jst).Format("2006-01-02")
}
