package domain

import "errors"

// ErrInvalidDifficulty は文字列が既知の難易度に一致しないことを表す。
var ErrInvalidDifficulty = errors.New("不正な難易度です")

// Difficulty は調理の手間を表す。
type Difficulty string

// 定義済みの難易度。DBの difficulty カラムに格納される値と一致する。
const (
	// DifficultyEasy は手順が簡単なもの。
	DifficultyEasy Difficulty = "easy"
	// DifficultyNormal は普通のもの。
	DifficultyNormal Difficulty = "normal"
	// DifficultyElaborate は手が込んだもの。
	DifficultyElaborate Difficulty = "elaborate"
)

// AllDifficulties は全難易度を易しい順で返す。
func AllDifficulties() []Difficulty {
	return []Difficulty{DifficultyEasy, DifficultyNormal, DifficultyElaborate}
}

// ParseDifficulty は文字列を Difficulty に変換する。
func ParseDifficulty(s string) (Difficulty, error) {
	d := Difficulty(s)
	if !d.Valid() {
		return "", ErrInvalidDifficulty
	}
	return d, nil
}

// Valid は定義済みの難易度かどうかを返す。
func (d Difficulty) Valid() bool {
	switch d {
	case DifficultyEasy, DifficultyNormal, DifficultyElaborate:
		return true
	default:
		return false
	}
}

// String は DB およびAPIで用いる文字列表現を返す。
func (d Difficulty) String() string {
	return string(d)
}
