package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidSavedWeeklyMenuID は保存した週間献立のIDとして解釈できない値。
var ErrInvalidSavedWeeklyMenuID = fmt.Errorf("保存した週間献立のIDが不正です")

// SavedWeeklyMenuID は保存した週間献立の識別子。
type SavedWeeklyMenuID struct {
	value uuid.UUID
}

// NewSavedWeeklyMenuID は新しいIDを採番する。
func NewSavedWeeklyMenuID() SavedWeeklyMenuID {
	return SavedWeeklyMenuID{value: uuid.New()}
}

// ParseSavedWeeklyMenuID は文字列を SavedWeeklyMenuID に変換する。
// ゼロ値のUUIDは未設定と区別できないため受け付けない。
func ParseSavedWeeklyMenuID(s string) (SavedWeeklyMenuID, error) {
	u, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return SavedWeeklyMenuID{}, fmt.Errorf("%w: %q", ErrInvalidSavedWeeklyMenuID, s)
	}
	if u == uuid.Nil {
		return SavedWeeklyMenuID{}, fmt.Errorf("%w: ゼロ値のUUIDは使用できません", ErrInvalidSavedWeeklyMenuID)
	}
	return SavedWeeklyMenuID{value: u}, nil
}

// IsZero は未設定かを返す。
func (id SavedWeeklyMenuID) IsZero() bool {
	return id.value == uuid.Nil
}

// String はUUIDの文字列表現を返す。
func (id SavedWeeklyMenuID) String() string {
	return id.value.String()
}

// SavedWeeklyMenu は保存された1週間分の献立（spec.md 2.8）。
//
// Days は週間献立と同じ DayMenu を使うが、**緩和フラグ（RelaxedDuplicate ほか）は
// 保存しない**ため、読み出したときは常に false になる。
// あれは「組み立てた瞬間に規則を緩めたか」を示す一時的な情報であり、
// 後から開いた週に対して意味を持たない。
type SavedWeeklyMenu struct {
	ID        SavedWeeklyMenuID
	Days      []DayMenu
	CreatedAt time.Time
}
