package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// List はユーザーのお気に入りを新しい順に返す。献立の情報を JOIN して読み取りモデルにする。
// 該当が無い場合は空スライスを返す（nilではない）。
//
// 履歴と違い件数の上限が無いため切り詰めはしない（spec.md 2.6）。
// created_at が同値になった場合に並びが揺れないよう menu_id をタイブレークに使う。
func (r *FavoriteRepository) List(ctx context.Context, userID domain.UserID) ([]domain.Favorite, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT f.created_at,
		        m.id, m.name, m.name_kana, m.genre, m.difficulty, m.description
		   FROM favorites f
		   JOIN menus m ON m.id = f.menu_id
		  WHERE f.user_id = $1
		  ORDER BY f.created_at DESC, f.menu_id DESC`, userID.String())
	if err != nil {
		return nil, fmt.Errorf("お気に入りの取得に失敗しました: %w", err)
	}
	defer rows.Close()

	favorites := make([]domain.Favorite, 0)
	for rows.Next() {
		fav, err := scanFavorite(rows)
		if err != nil {
			return nil, err
		}
		favorites = append(favorites, fav)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("お気に入りの読み取りに失敗しました: %w", err)
	}
	return favorites, nil
}

// Delete はお気に入りを1件削除する。
// 該当が無ければ service.ErrFavoriteNotFound を返す。
//
// (user_id, menu_id) で絞って消すため、他人の行には構造上たどり着けない。
// 履歴（グローバルな履歴IDで指定するため所有者確認が要る）と違い、
// 所有者の事前確認は不要で、削除件数だけで判定できる。
func (r *FavoriteRepository) Delete(ctx context.Context, userID domain.UserID, menuID domain.MenuID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM favorites WHERE user_id = $1 AND menu_id = $2`,
		userID.String(), menuID.String())
	if err != nil {
		return fmt.Errorf("お気に入りの削除に失敗しました: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return service.ErrFavoriteNotFound
	}
	return nil
}

// scanFavorite は1行を Favorite に読む。
func scanFavorite(row pgx.Row) (domain.Favorite, error) {
	var (
		createdAt                                 time.Time
		menuID, name, kana, genre, diff, descript string
	)
	if err := row.Scan(&createdAt,
		&menuID, &name, &kana, &genre, &diff, &descript); err != nil {
		return domain.Favorite{}, fmt.Errorf("お気に入りの読み取りに失敗しました: %w", err)
	}

	menu, err := hydrateMenu(menuID, name, kana, genre, diff, descript)
	if err != nil {
		return domain.Favorite{}, err
	}
	return domain.Favorite{Menu: menu, CreatedAt: createdAt}, nil
}
