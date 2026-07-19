package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// usersEmailUniqueConstraint はメールの一意制約の名前。
// email カラムに UNIQUE を付けると Postgres がこの名前を自動で採る。
const usersEmailUniqueConstraint = "users_email_key"

// uniqueViolation は一意制約違反を表す SQLSTATE。
const uniqueViolation = "23505"

// UserRepository はユーザーと認証情報を Postgres に保存する。
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository は UserRepository を生成する。
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// CreateWithPassword は user とパスワード認証の identity を1トランザクションで作る。
// メールが既に使われている場合は service.ErrEmailTaken を返す。
func (r *UserRepository) CreateWithPassword(ctx context.Context, u domain.User, passwordHash string) error {
	// users と auth_identities は必ず対で存在すべき。片方だけ作られると
	// ログインできないユーザーや持ち主のいない認証情報が生まれるため、
	// トランザクションで束ねる。
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	// Commit 済みなら Rollback は無害な no-op。失敗経路で確実に戻すため defer で呼ぶ。
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, email, display_name) VALUES ($1, $2, $3)`,
		u.ID.String(), u.Email.String(), u.DisplayName)
	if err != nil {
		if isEmailTaken(err) {
			return service.ErrEmailTaken
		}
		return fmt.Errorf("ユーザーの保存に失敗しました: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO auth_identities (id, user_id, provider, password_hash)
		 VALUES ($1, $2, 'password', $3)`,
		uuid.NewString(), u.ID.String(), passwordHash)
	if err != nil {
		return fmt.Errorf("認証情報の保存に失敗しました: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("トランザクションのコミットに失敗しました: %w", err)
	}
	return nil
}

// FindOrCreateGoogleUser は Google 認証のユーザーを取得または作成する。
//  1. (provider=google, provider_uid=sub) が既にあればそのユーザー（2回目以降）。
//  2. 無ければ email で既存ユーザーを探し、あれば google の identity を足す
//     （パスワードユーザーと同一メールなら同じユーザーに紐付ける。spec.md 1.4）。
//  3. どちらも無ければ user と google identity を新規作成する（初回）。
//
// 一連の判断と書き込みは、途中に別のログインが割り込んで二重作成しないよう
// トランザクションで束ねる。
func (r *UserRepository) FindOrCreateGoogleUser(ctx context.Context, sub string, email domain.Email, displayName string) (domain.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. 既に Google で結び付いているユーザー。
	if user, found, err := findUserByGoogleSub(ctx, tx, sub); err != nil {
		return domain.User{}, err
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return domain.User{}, fmt.Errorf("トランザクションのコミットに失敗しました: %w", err)
		}
		return user, nil
	}

	// 2. 同じメールの既存ユーザーがいれば、その人に Google を足す。
	user, found, err := findUserByEmail(ctx, tx, email)
	if err != nil {
		return domain.User{}, err
	}
	if found {
		if err := insertGoogleIdentity(ctx, tx, user.ID, sub); err != nil {
			return domain.User{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.User{}, fmt.Errorf("トランザクションのコミットに失敗しました: %w", err)
		}
		return user, nil
	}

	// 3. 新規ユーザーを作る。
	newUser := domain.User{ID: domain.NewUserID(), Email: email, DisplayName: displayName}
	if _, err := tx.Exec(ctx,
		`INSERT INTO users (id, email, display_name) VALUES ($1, $2, $3)`,
		newUser.ID.String(), newUser.Email.String(), newUser.DisplayName); err != nil {
		return domain.User{}, fmt.Errorf("ユーザーの保存に失敗しました: %w", err)
	}
	if err := insertGoogleIdentity(ctx, tx, newUser.ID, sub); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, fmt.Errorf("トランザクションのコミットに失敗しました: %w", err)
	}
	return newUser, nil
}

// insertGoogleIdentity は user に google の認証手段を1件足す。
func insertGoogleIdentity(ctx context.Context, tx pgx.Tx, userID domain.UserID, sub string) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO auth_identities (id, user_id, provider, provider_uid)
		 VALUES ($1, $2, 'google', $3)`,
		uuid.NewString(), userID.String(), sub); err != nil {
		return fmt.Errorf("認証情報(Google)の保存に失敗しました: %w", err)
	}
	return nil
}

// findUserByGoogleSub は google の provider_uid でユーザーを引く。
func findUserByGoogleSub(ctx context.Context, tx pgx.Tx, sub string) (domain.User, bool, error) {
	row := tx.QueryRow(ctx,
		`SELECT u.id, u.email, u.display_name
		   FROM users u
		   JOIN auth_identities a
		     ON a.user_id = u.id AND a.provider = 'google' AND a.provider_uid = $1`, sub)
	return scanUserRow(row)
}

// findUserByEmail は email でユーザーを引く。
func findUserByEmail(ctx context.Context, tx pgx.Tx, email domain.Email) (domain.User, bool, error) {
	row := tx.QueryRow(ctx,
		`SELECT id, email, display_name FROM users WHERE email = $1`, email.String())
	return scanUserRow(row)
}

// scanUserRow は1行を domain.User に読む。行が無ければ found=false。
func scanUserRow(row pgx.Row) (domain.User, bool, error) {
	var id, mail, displayName string
	if err := row.Scan(&id, &mail, &displayName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, false, nil
		}
		return domain.User{}, false, fmt.Errorf("ユーザーの取得に失敗しました: %w", err)
	}
	userID, err := domain.ParseUserID(id)
	if err != nil {
		return domain.User{}, false, fmt.Errorf("DBのユーザーIDが不正です: %w", err)
	}
	addr, err := domain.NewEmail(mail)
	if err != nil {
		return domain.User{}, false, fmt.Errorf("DBのメールが不正です: %w", err)
	}
	return domain.User{ID: userID, Email: addr, DisplayName: displayName}, true, nil
}

// FindByID はIDでユーザーを取得する。存在しない場合は service.ErrUserNotFound を返す。
func (r *UserRepository) FindByID(ctx context.Context, id domain.UserID) (domain.User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, email, display_name FROM users WHERE id = $1`, id.String())

	var rawID, mail, displayName string
	if err := row.Scan(&rawID, &mail, &displayName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, service.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("ユーザーの取得に失敗しました: %w", err)
	}

	userID, err := domain.ParseUserID(rawID)
	if err != nil {
		return domain.User{}, fmt.Errorf("DBのユーザーIDが不正です: %w", err)
	}
	addr, err := domain.NewEmail(mail)
	if err != nil {
		return domain.User{}, fmt.Errorf("DBのメールが不正です: %w", err)
	}

	return domain.User{ID: userID, Email: addr, DisplayName: displayName}, nil
}

// FindPasswordCredential はメールに対応するパスワード認証を返す。
// ユーザーが居ない、または Google 認証のみでパスワードを持たない場合は
// service.ErrCredentialNotFound を返す（JOIN が1行も返さない）。
func (r *UserRepository) FindPasswordCredential(ctx context.Context, email domain.Email) (service.PasswordCredential, error) {
	// provider='password' の identity を内部結合するため、パスワードを
	// 持たないユーザー（Google のみ）は自然に0行になる。
	row := r.pool.QueryRow(ctx,
		`SELECT u.id, u.email, u.display_name, a.password_hash
		   FROM users u
		   JOIN auth_identities a
		     ON a.user_id = u.id AND a.provider = 'password'
		  WHERE u.email = $1`, email.String())

	var id, mail, displayName, hash string
	if err := row.Scan(&id, &mail, &displayName, &hash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.PasswordCredential{}, service.ErrCredentialNotFound
		}
		return service.PasswordCredential{}, fmt.Errorf("認証情報の取得に失敗しました: %w", err)
	}

	userID, err := domain.ParseUserID(id)
	if err != nil {
		return service.PasswordCredential{}, fmt.Errorf("DBのユーザーIDが不正です: %w", err)
	}
	addr, err := domain.NewEmail(mail)
	if err != nil {
		return service.PasswordCredential{}, fmt.Errorf("DBのメールが不正です: %w", err)
	}

	return service.PasswordCredential{
		User: domain.User{
			ID:          userID,
			Email:       addr,
			DisplayName: displayName,
		},
		PasswordHash: hash,
	}, nil
}

// isEmailTaken はエラーがメールの一意制約違反かどうかを返す。
// 他の一意制約（将来の追加分）を巻き込まないよう制約名まで見る。
func isEmailTaken(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == uniqueViolation &&
		pgErr.ConstraintName == usersEmailUniqueConstraint
}
