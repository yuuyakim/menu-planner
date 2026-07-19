package service

import (
	"context"
	"fmt"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// AuthService は認証にまつわるユースケースを担う。
type AuthService struct {
	users  UserRepository
	hasher PasswordHasher
}

// NewAuthService は認証サービスを組み立てる。
func NewAuthService(users UserRepository, hasher PasswordHasher) *AuthService {
	return &AuthService{users: users, hasher: hasher}
}

// SignUp はメールとパスワードで新規ユーザーを登録する。
// 表示名は受け取らない（spec.md 5.2）。メールのローカル部から導出する。
//
// 検証の順序は「安いものから」。メール形式 → パスワード（ハッシュ化）→ DB書き込み。
// 不正な入力はDBに触れる前に弾き、無駄な問い合わせとハッシュ計算を避ける。
func (s *AuthService) SignUp(ctx context.Context, email, password string) (domain.User, error) {
	addr, err := domain.NewEmail(email)
	if err != nil {
		return domain.User{}, err
	}

	// パスワードの検証（長さなど）はハッシュ化に含まれる。ここで弾けば
	// メール重複のためのDB問い合わせより先に不正を返せる。
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return domain.User{}, err
	}

	user, err := domain.NewUser(addr)
	if err != nil {
		return domain.User{}, err
	}

	if err := s.users.CreateWithPassword(ctx, user, hash); err != nil {
		// ErrEmailTaken はそのまま通し、呼び出し側で 409 に変換させる。
		return domain.User{}, fmt.Errorf("ユーザーの作成に失敗しました: %w", err)
	}

	return user, nil
}
