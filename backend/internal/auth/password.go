// Package auth はパスワードやトークンなど認証にまつわる処理を提供する。
package auth

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	// bcryptCost はハッシュ化のコスト。spec.md で 12 に定める。
	// 大きいほど総当たりに強いが、その分ログインが重くなる。
	bcryptCost = 12

	// minPasswordChars は最小の文字数。spec.md 11章のセキュリティ要件で
	// 8文字以上。バイト数ではなく文字数で数えるため、全角8文字も満たす。
	minPasswordChars = 8

	// maxPasswordBytes は bcrypt が扱えるバイト数の上限。これを超える入力は
	// bcrypt が拒否する（古い版は黙って切り詰めていた）。切り詰めは別々の
	// パスワードが同じハッシュになりうるので、自分でも明示的に弾く。
	maxPasswordBytes = 72
)

var (
	// ErrPasswordTooShort はパスワードが短すぎることを表す。
	ErrPasswordTooShort = errors.New("パスワードが短すぎます")
	// ErrPasswordTooLong はパスワードが bcrypt の上限を超えることを表す。
	ErrPasswordTooLong = errors.New("パスワードが長すぎます")
	// ErrPasswordMismatch はパスワードがハッシュと一致しないことを表す。
	// 壊れたハッシュなど他の失敗と区別できるよう独立させる。
	ErrPasswordMismatch = errors.New("パスワードが一致しません")
)

// HashPassword は平文パスワードを bcrypt でハッシュ化する。
// 長さの検証もここで行い、DBに渡る前に不正な入力を弾く。
func HashPassword(plain string) (string, error) {
	if utf8.RuneCountInString(plain) < minPasswordChars {
		return "", fmt.Errorf("%w: %d文字以上にしてください", ErrPasswordTooShort, minPasswordChars)
	}
	// bcrypt はバイト数で上限を判定するため、こちらもバイト数で見る。
	if len(plain) > maxPasswordBytes {
		return "", fmt.Errorf("%w: %dバイト以下にしてください", ErrPasswordTooLong, maxPasswordBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("パスワードのハッシュ化に失敗しました: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword は平文パスワードがハッシュと一致するか検証する。
// 一致しない場合は ErrPasswordMismatch を、ハッシュ自体が壊れている場合は
// それ以外のエラーを返す。呼び出し側が両者を区別できるようにするため。
func VerifyPassword(hash, plain string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if err == nil {
		return nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrPasswordMismatch
	}
	return fmt.Errorf("パスワードの検証に失敗しました: %w", err)
}
