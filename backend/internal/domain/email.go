package domain

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// ErrInvalidEmail はメールアドレスとして成立しない値を表す。
var ErrInvalidEmail = errors.New("不正なメールアドレスです")

// Email は検証済みのメールアドレス。
//
// 値は NewEmail でのみ組み立てる。生の文字列を直接使わせないことで、
// 未検証のアドレスがDBやトークンに紛れ込むことを防ぐ。保存・比較に使うため
// 小文字に正規化して持つ。User に限らずログインや履歴の突合など
// メールを扱う箇所で再利用する。
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
