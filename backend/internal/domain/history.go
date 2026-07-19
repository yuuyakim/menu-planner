package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidHistoryID は履歴IDとして解釈できない値を表す。
var ErrInvalidHistoryID = errors.New("不正な履歴IDです")

// HistoryID は検索履歴の識別子。
type HistoryID struct {
	value uuid.UUID
}

// ParseHistoryID は文字列を HistoryID に変換する。
// ゼロ値のUUIDは未設定と区別できないため受け付けない。
func ParseHistoryID(s string) (HistoryID, error) {
	u, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return HistoryID{}, fmt.Errorf("%w: %q", ErrInvalidHistoryID, s)
	}
	if u == uuid.Nil {
		return HistoryID{}, fmt.Errorf("%w: ゼロ値のUUIDは使用できません", ErrInvalidHistoryID)
	}
	return HistoryID{value: u}, nil
}

// String はUUIDの文字列表現を返す。
func (id HistoryID) String() string {
	return id.value.String()
}

// HistoryEntry は履歴1件の読み取りモデル。献立の情報を含む。
type HistoryEntry struct {
	// ID は履歴の識別子（個別削除に使う）。
	ID HistoryID
	// Menu は履歴に残っている献立。
	Menu Menu
	// Mode は検索の種類（single / weekly）。
	Mode SearchMode
	// SearchedAt は検索した時刻。一覧の並び順に使う。
	SearchedAt time.Time
}
