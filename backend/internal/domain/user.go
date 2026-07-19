package domain

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ユーザーにまつわる検証エラー。
var (
	// ErrInvalidUserID はユーザーIDとして解釈できない値を表す。
	ErrInvalidUserID = errors.New("不正なユーザーIDです")
	// ErrInvalidEmail はメールアドレスとして成立しない値を表す。
	ErrInvalidEmail = errors.New("不正なメールアドレスです")
)

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

// Email は検証済みのメールアドレス。
//
// 値は NewEmail でのみ組み立てる。生の文字列を直接使わせないことで、
// 未検証のアドレスがDBやトークンに紛れ込むことを防ぐ。保存・比較に使うため
// 小文字に正規化して持つ。
type Email struct {
	value string
}

// NewEmail はメールアドレスを検証して組み立てる。
// 前後の空白を除き、小文字に揃える（大小違いでの二重登録を防ぐ）。
func NewEmail(raw string) (Email, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Email{}, fmt.Errorf("%w: 空です", ErrInvalidEmail)
	}

	// ParseAddress は "名前 <addr>" の形も通すため、素のアドレスだけを受け付ける。
	// Name が付く・角括弧が付くといった形は trimmed と一致しなくなり弾かれる。
	addr, err := mail.ParseAddress(trimmed)
	if err != nil || addr.Name != "" || addr.Address != trimmed {
		return Email{}, fmt.Errorf("%w: %q", ErrInvalidEmail, raw)
	}

	// ドメイン部にドットが無いもの（user@example）は受け付けない。
	// ParseAddress はこれを通すが、到達可能なアドレスとしては不十分。
	at := strings.LastIndex(addr.Address, "@")
	if at < 0 || !strings.Contains(addr.Address[at+1:], ".") {
		return Email{}, fmt.Errorf("%w: %q", ErrInvalidEmail, raw)
	}

	return Email{value: strings.ToLower(addr.Address)}, nil
}

// String は正規化済みのメールアドレスを返す。
func (e Email) String() string {
	return e.value
}

// localPart はメールの @ より前の部分を返す。表示名の導出に使う。
func (e Email) localPart() string {
	if at := strings.LastIndex(e.value, "@"); at >= 0 {
		return e.value[:at]
	}
	return e.value
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
