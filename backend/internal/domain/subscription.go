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
	SubscriptionPastDue  SubscriptionStatus = "past_due"
	SubscriptionCanceled SubscriptionStatus = "canceled"
)

// ProviderManual は運用者が手で付与した加入を表す provider の値。
// 決済を導入したら "stripe" などが増える。
const ProviderManual = "manual"

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
	case SubscriptionActive, SubscriptionPastDue, SubscriptionCanceled:
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
}

// IsActiveAt は指定時刻に加入が有効かを返す。
//
// 期限切れをバッチでDBに書き戻すことはせず、参照のたびにここで判定する。
// バッチが停止すると、課金していない利用者がプレミアムのまま残るため。
func (s Subscription) IsActiveAt(t time.Time) bool {
	return s.Status == SubscriptionActive && s.CurrentPeriodEnd.After(t)
}
