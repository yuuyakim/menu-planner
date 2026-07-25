package domain

import (
	"errors"
	"time"
)

// ErrInvalidSubscriptionStatus は文字列が既知の加入状態に一致しないことを表す。
var ErrInvalidSubscriptionStatus = errors.New("不正な加入状態です")

// SubscriptionStatus は加入の状態。
type SubscriptionStatus string

// 定義済みの加入状態。DBの subscriptions.status に格納される値と一致する。
const (
	SubscriptionActive   SubscriptionStatus = "active"
	SubscriptionTrialing SubscriptionStatus = "trialing"
	SubscriptionPastDue  SubscriptionStatus = "past_due"
	SubscriptionCanceled SubscriptionStatus = "canceled"
)

// ProviderManual は運用者が手で付与した加入を表す provider の値。
// 決済を導入したら "stripe" などが増える。
const ProviderManual = "manual"

// ProviderStripe は Stripe 決済で作られた加入を表す provider の値。
const ProviderStripe = "stripe"

// PaymentGracePeriod は支払い失敗（past_due）後もプレミアムを維持する猶予。
// 利用規約4条7項の「7日間の猶予期間」に対応する。実際の打ち切りは Stripe の
// 督促設定が行い、この猶予は安全弁として二重に効かせる。
const PaymentGracePeriod = 7 * 24 * time.Hour

// ParseSubscriptionStatus は文字列を SubscriptionStatus に変換する。
func ParseSubscriptionStatus(s string) (SubscriptionStatus, error) {
	st := SubscriptionStatus(s)
	if !st.Valid() {
		return "", ErrInvalidSubscriptionStatus
	}
	return st, nil
}

// Valid は定義済みの状態かどうかを返す。
func (s SubscriptionStatus) Valid() bool {
	switch s {
	case SubscriptionActive, SubscriptionTrialing, SubscriptionPastDue, SubscriptionCanceled:
		return true
	default:
		return false
	}
}

// String は DB で用いる文字列表現を返す。
func (s SubscriptionStatus) String() string {
	return string(s)
}

// Subscription は1利用者の加入。1利用者につき高々1件（DBでは user_id が主キー）。
type Subscription struct {
	UserID           UserID
	Plan             Plan
	Status           SubscriptionStatus
	CurrentPeriodEnd time.Time
	// CancelAtPeriodEnd は解約予約中かどうか。利用者都合の解約は即時失効させず、
	// 期末まで使えるようにする（即時失効は返金の争いを招く）。
	// 書き込む経路は決済フェーズで作る。
	CancelAtPeriodEnd bool
	// Provider は加入を作った経路。手動付与は ProviderManual。
	Provider string
	// ProviderSubscriptionID は決済事業者側の加入ID。手動付与では空。
	ProviderSubscriptionID string
	// ProviderCustomerID は決済事業者側の顧客ID。手動付与では空。
	// 顧客の再利用と将来の解約/ポータル画面で使う。
	ProviderCustomerID string
}

// GivesPremiumAt は now 時点でこの加入がプレミアム権限を与えるかを返す。
//
// active / trialing は期間内なら premium。past_due は支払い失敗後の状態だが、
// 利用規約の猶予（PaymentGracePeriod）内はプレミアムを維持する。それ以外
// （canceled や未知の状態）は権限を与えない（安全側）。
func (s Subscription) GivesPremiumAt(now time.Time) bool {
	switch s.Status {
	case SubscriptionActive, SubscriptionTrialing:
		return s.CurrentPeriodEnd.After(now)
	case SubscriptionPastDue:
		return now.Before(s.CurrentPeriodEnd.Add(PaymentGracePeriod))
	default:
		return false
	}
}
