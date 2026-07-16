// Package random は service.Randomizer の実装を提供する。
// 乱数生成をこの層に閉じ込めることで、service のテストを決定的に保てる。
package random

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
)

// ErrInvalidRange は Intn に 1 未満の上限が渡されたことを表す。
var ErrInvalidRange = errors.New("乱数の範囲が不正です")

// Crypto は crypto/rand を用いた Randomizer 実装。
// 献立の提案は同じ利用者に繰り返し見えるため、シード値から次の提案が
// 予測できる math/rand ではなく暗号論的乱数を使う。
type Crypto struct{}

// NewCrypto は crypto/rand ベースの乱数源を返す。
// 状態を持たないため、複数の goroutine から同時に使ってよい。
func NewCrypto() Crypto {
	return Crypto{}
}

// Intn は [0, n) の一様乱数を返す。n が 1 未満の場合は ErrInvalidRange を返す。
func (Crypto) Intn(n int) (int, error) {
	if n < 1 {
		return 0, fmt.Errorf("%w: %d (1以上が必要)", ErrInvalidRange, n)
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, fmt.Errorf("乱数の生成に失敗しました: %w", err)
	}
	return int(v.Int64()), nil
}
