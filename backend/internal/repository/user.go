package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
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
