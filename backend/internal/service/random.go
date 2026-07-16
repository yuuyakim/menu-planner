package service

import (
	"errors"
	"fmt"
)

// ErrNoCandidates は選ぶ対象が1件も無いことを表す。
var ErrNoCandidates = errors.New("候補がありません")

// Pick は候補から1件を無作為に選ぶ。候補が空の場合は ErrNoCandidates を返す。
func Pick[T any](r Randomizer, candidates []T) (T, error) {
	var zero T

	if len(candidates) == 0 {
		return zero, ErrNoCandidates
	}

	i, err := r.Intn(len(candidates))
	if err != nil {
		return zero, fmt.Errorf("候補の選択に失敗しました: %w", err)
	}
	// 乱数源の実装ミスを範囲外アクセスによるパニックにせず、エラーとして扱う。
	if i < 0 || i >= len(candidates) {
		return zero, fmt.Errorf("乱数源が範囲外の値を返しました: %d (候補%d件)", i, len(candidates))
	}

	return candidates[i], nil
}
