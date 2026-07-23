package domain

import "errors"

// ErrInvalidPlan は文字列が既知のプランに一致しないことを表す。
var ErrInvalidPlan = errors.New("不正なプランです")

// Plan は利用者の契約プラン。
type Plan string

// 定義済みのプラン。DBの subscriptions.plan に格納される値と一致する。
const (
	PlanFree    Plan = "free"
	PlanPremium Plan = "premium"
)

// ParsePlan は文字列を Plan に変換する。
// 表記ゆれを許容するとDBの値と乖離するため、完全一致のみを受け付ける。
func ParsePlan(s string) (Plan, error) {
	p := Plan(s)
	if !p.Valid() {
		return "", ErrInvalidPlan
	}
	return p, nil
}

// Valid は定義済みのプランかどうかを返す。
func (p Plan) Valid() bool {
	switch p {
	case PlanFree, PlanPremium:
		return true
	default:
		return false
	}
}

// String は DB およびAPIで用いる文字列表現を返す。
func (p Plan) String() string {
	return string(p)
}
