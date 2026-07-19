package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidUserID はユーザーIDとして解釈できない値を表す。
var ErrInvalidUserID = errors.New("不正なユーザーIDです")

// displayNameMaxLen は表示名の最大文字数（バイト数ではなく文字数）。
// メールのローカル部から導出するため、極端に長いものは切り詰める。
const displayNameMaxLen = 50

// UserID はユーザーの識別子。
type UserID struct {
	value uuid.UUID
}

// NewUserID は新しいユーザーIDを採番する。
func NewUserID() UserID {
	return UserID{value: uuid.New()}
}

// ParseUserID は文字列を UserID に変換する。
// ゼロ値のUUIDは未設定と区別できないため受け付けない。
func ParseUserID(s string) (UserID, error) {
	u, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return UserID{}, fmt.Errorf("%w: %q", ErrInvalidUserID, s)
	}
	if u == uuid.Nil {
		return UserID{}, fmt.Errorf("%w: ゼロ値のUUIDは使用できません", ErrInvalidUserID)
	}
	return UserID{value: u}, nil
}

// IsZero は未設定かどうかを返す。
func (id UserID) IsZero() bool {
	return id.value == uuid.Nil
}

// String はUUIDの文字列表現を返す。
func (id UserID) String() string {
	return id.value.String()
}

// User はアプリの利用者。
type User struct {
	ID          UserID
	Email       Email
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewUser は新規ユーザーを組み立てる。IDを採番し、表示名をメールの
// ローカル部から導出する。サインアップが表示名を受け取らないため（spec.md 5.2）。
func NewUser(email Email) (User, error) {
	name := email.localPart()
	// ローカル部が上限を超える場合は切り詰める。DBや画面表示のため。
	if r := []rune(name); len(r) > displayNameMaxLen {
		name = string(r[:displayNameMaxLen])
	}
	if name == "" {
		// NewEmail を通っていればローカル部は空にならないが、保険として弾く。
		return User{}, fmt.Errorf("%w: 表示名を導出できません", ErrInvalidEmail)
	}

	return User{
		ID:          NewUserID(),
		Email:       email,
		DisplayName: name,
	}, nil
}
