package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrInvalidCredentials はログインの認証情報が正しくないことを表す。
// 呼び出し側はこれを 401 に変換する。「メールが存在しない」と
// 「パスワードが違う」を区別せずこれに丸めることで、エラーの差から
// アカウントの有無を推測されるのを防ぐ（ユーザー列挙対策）。
var ErrInvalidCredentials = errors.New("メールアドレスまたはパスワードが正しくありません")

// dummyPasswordHash は照合対象が見つからないときに、それでも bcrypt 検証を
// 走らせて処理時間を揃えるための固定ハッシュ（cost 12）。存在しないメールだけ
// 即座に返すと、応答時間の差からアカウントの有無を推測されうるため。
// "dummy-password-for-timing" のハッシュで、どんな入力にも一致しない。
// 本物の認証情報ではなくタイミング等化用の固定値のため gosec の誤検知を抑制する。
const dummyPasswordHash = "$2a$12$bqKQ7XPKatsapwEWkXTk4.pPsmBfxJSq6DB6km7GnlU7GbsXnb84e" //nolint:gosec // 認証情報ではなくタイミング等化用のダミーハッシュ

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

// Login はメールとパスワードで認証し、一致したユーザーを返す。
//
// 「メールが存在しない」「パスワードが違う」「Google 認証のみ」を
// すべて ErrInvalidCredentials に丸める。エラーの差でアカウントの有無を
// 推測されないようにするため（spec.md 5.2 / ユーザー列挙対策）。
func (s *AuthService) Login(ctx context.Context, email, password string) (domain.User, error) {
	// メール形式の不正は照合以前の問題で、存在推測には使えないため
	// そのまま返す（呼び出し側で 400）。
	addr, err := domain.NewEmail(email)
	if err != nil {
		return domain.User{}, err
	}

	cred, err := s.users.FindPasswordCredential(ctx, addr)
	if err != nil {
		if errors.Is(err, ErrCredentialNotFound) {
			// 照合対象が無くても bcrypt を1回走らせ、応答時間を
			// 「存在するがパスワード違い」と揃える。返す結果は同じ。
			_ = s.hasher.Verify(dummyPasswordHash, password)
			return domain.User{}, ErrInvalidCredentials
		}
		return domain.User{}, fmt.Errorf("認証情報の取得に失敗しました: %w", err)
	}

	if err := s.hasher.Verify(cred.PasswordHash, password); err != nil {
		// パスワード不一致も、壊れたハッシュなどの内部異常も、外向きには
		// 同じ 401 に丸める。内部異常はログで追える（ハンドラが記録する）。
		return domain.User{}, ErrInvalidCredentials
	}

	return cred.User, nil
}

// CurrentUser はトークンの主体(userID)からユーザーを取得する。
// 認証ミドルウェアが検証済みの userID を渡す前提。
//
// userID が壊れている・指すユーザーが消えている場合は ErrUserNotFound を返す。
// どちらも「有効なセッションが無い」状態なので、呼び出し側は 401 に丸める。
func (s *AuthService) CurrentUser(ctx context.Context, userID string) (domain.User, error) {
	// sub は自分が署名した UUID のはずだが、壊れていてもパニックせず
	// セッション不正として扱う。
	id, err := domain.ParseUserID(userID)
	if err != nil {
		return domain.User{}, ErrUserNotFound
	}
	return s.users.FindByID(ctx, id)
}
