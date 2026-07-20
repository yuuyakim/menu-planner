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

// favoritesUserMenuConstraint は (user_id, menu_id) の一意制約の名前。
// マイグレーション 000006 で明示的に付けた名前。
const favoritesUserMenuConstraint = "favorites_user_menu_uniq"

// favoritesMenuFKConstraint は menu_id の外部キー制約の名前。
// menu_id カラムに REFERENCES を付けると Postgres がこの名前を自動で採る。
const favoritesMenuFKConstraint = "favorites_menu_id_fkey"

// foreignKeyViolation は外部キー違反を表す SQLSTATE。
const foreignKeyViolation = "23503"

// FavoriteRepository はお気に入りの永続化を提供する。
type FavoriteRepository struct {
	pool *pgxpool.Pool
}

// NewFavoriteRepository は FavoriteRepository を生成する。
func NewFavoriteRepository(pool *pgxpool.Pool) *FavoriteRepository {
	return &FavoriteRepository{pool: pool}
}

// Add はお気に入りを1件追加する。
// 重複は service.ErrFavoriteExists（409）、存在しない献立は ErrMenuNotFound（404）。
//
// どちらも DB の制約に判定させる。事前に SELECT で確かめる方式は、
// 確認と INSERT の間に他のリクエストが割り込むと破綻する。
func (r *FavoriteRepository) Add(ctx context.Context, userID domain.UserID, menuID domain.MenuID) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO favorites (id, user_id, menu_id) VALUES ($1, $2, $3)`,
		uuid.NewString(), userID.String(), menuID.String())
	if err != nil {
		if isFavoriteDuplicate(err) {
			return service.ErrFavoriteExists
		}
		if isUnknownMenu(err) {
			return fmt.Errorf("%w: %s", ErrMenuNotFound, menuID)
		}
		return fmt.Errorf("お気に入りの登録に失敗しました: %w", err)
	}
	return nil
}

// isFavoriteDuplicate は (user_id, menu_id) の重複によるエラーかを判定する。
// 制約名まで見るのは、将来ほかの一意制約が増えたときに取り違えないため。
func isFavoriteDuplicate(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == uniqueViolation &&
		pgErr.ConstraintName == favoritesUserMenuConstraint
}

// isUnknownMenu は存在しない献立を参照したことによるエラーかを判定する。
// user_id 側の外部キー違反（＝トークンが指すユーザーが消えている）と
// 区別する必要があるため、制約名で絞る。
func isUnknownMenu(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == foreignKeyViolation &&
		pgErr.ConstraintName == favoritesMenuFKConstraint
}
