// Package randomtest はテスト用の決定的な Randomizer を提供する。
// 本番コードから使ってはならない。
package randomtest

import (
	"fmt"

	"github.com/yuuyakim/menu-planner/backend/internal/random"
)

// Fixed はあらかじめ与えた値を順に返す決定的な Randomizer。
// 値を使い切ったら先頭に戻るため、呼び出し回数を気にせず使える。
//
// 上限 n に収まらない値も設定できる。呼び出し側の防御的なチェックを
// テストするためで、この型は値の範囲を検証しない。
//
// 並行に使うことは想定していない。
type Fixed struct {
	values []int
	calls  int
}

// NewFixed は values を順に返す乱数源を返す。
func NewFixed(values ...int) *Fixed {
	return &Fixed{values: values}
}

// Intn は次の値を返す。n が 1 未満、または値が設定されていない場合はエラーを返す。
func (f *Fixed) Intn(n int) (int, error) {
	if n < 1 {
		return 0, fmt.Errorf("%w: %d (1以上が必要)", random.ErrInvalidRange, n)
	}
	if len(f.values) == 0 {
		return 0, fmt.Errorf("返す値が設定されていません")
	}
	v := f.values[f.calls%len(f.values)]
	f.calls++
	return v, nil
}

// Calls は Intn が呼ばれた回数を返す。エラーになった呼び出しは数えない。
func (f *Fixed) Calls() int {
	return f.calls
}
