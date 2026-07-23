package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// savedWeeklyMenuFKConstraint は saved_weekly_menu_days.menu_id の外部キー制約の名前。
// menu_id に REFERENCES を付けると Postgres がこの名前を自動で採る。
const savedWeeklyMenuFKConstraint = "saved_weekly_menu_days_menu_id_fkey"

// SavedWeeklyMenuRepository は保存した週間献立の永続化を提供する。
type SavedWeeklyMenuRepository struct {
	pool *pgxpool.Pool
}

// NewSavedWeeklyMenuRepository は SavedWeeklyMenuRepository を生成する。
func NewSavedWeeklyMenuRepository(pool *pgxpool.Pool) *SavedWeeklyMenuRepository {
	return &SavedWeeklyMenuRepository{pool: pool}
}

// Save は1週間分をまとめて保存し、採番したIDを返す。
//
// **親行と7日分は1つのトランザクションで書く。** 途中で失敗したときに
// 「日が3件しか無い週」が残ると、開いた利用者には壊れた週として見える。
// 7日で1つの意味を持つものなので、全部書けるか何も書かないかのどちらかにする。
func (r *SavedWeeklyMenuRepository) Save(
	ctx context.Context, userID domain.UserID, days []domain.DayMenu,
) (domain.SavedWeeklyMenuID, error) {
	id := domain.NewSavedWeeklyMenuID()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.SavedWeeklyMenuID{}, fmt.Errorf("週間献立の保存を開始できませんでした: %w", err)
	}
	// 成功時は下で Commit 済みなので、この Rollback は何もしない。
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO saved_weekly_menus (id, user_id) VALUES ($1, $2)`,
		id.String(), userID.String()); err != nil {
		return domain.SavedWeeklyMenuID{}, fmt.Errorf("週間献立の保存に失敗しました: %w", err)
	}

	for _, d := range days {
		if _, err := tx.Exec(ctx,
			`INSERT INTO saved_weekly_menu_days (saved_weekly_menu_id, day, menu_id)
			 VALUES ($1, $2, $3)`,
			id.String(), d.Day, d.Menu.ID.String()); err != nil {
			if isUnknownSavedMenu(err) {
				return domain.SavedWeeklyMenuID{}, fmt.Errorf("%w: %s", ErrMenuNotFound, d.Menu.ID)
			}
			return domain.SavedWeeklyMenuID{}, fmt.Errorf("週間献立の保存に失敗しました: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.SavedWeeklyMenuID{}, fmt.Errorf("週間献立の保存を確定できませんでした: %w", err)
	}
	return id, nil
}

// isUnknownSavedMenu は存在しない献立を参照したことによるエラーかを判定する。
// 親（saved_weekly_menu_id）側の外部キー違反と区別するため制約名で絞る。
func isUnknownSavedMenu(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == foreignKeyViolation &&
		pgErr.ConstraintName == savedWeeklyMenuFKConstraint
}

// List はユーザーの保存を新しい順に、中身の7日分も含めて返す。
// 該当が無い場合は空スライスを返す（nilではない）。
//
// 親と子を1本のJOINで取らず2回に分けるのは、親が0件のときに子を引かずに済み、
// 親の並び順（created_at DESC）と子の並び順（day ASC）を素直に別々に指定できるため。
// 最大10件×7日と小さいので、往復1回の差は問題にならない。
func (r *SavedWeeklyMenuRepository) List(
	ctx context.Context, userID domain.UserID,
) ([]domain.SavedWeeklyMenu, error) {
	// created_at が同値になった場合に並びが揺れないよう id をタイブレークに使う。
	rows, err := r.pool.Query(ctx,
		`SELECT id, created_at
		   FROM saved_weekly_menus
		  WHERE user_id = $1
		  ORDER BY created_at DESC, id DESC`, userID.String())
	if err != nil {
		return nil, fmt.Errorf("保存した週間献立の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	saved := make([]domain.SavedWeeklyMenu, 0)
	// 子を詰め戻すために、IDから位置を引けるようにしておく。
	at := make(map[string]int)
	ids := make([]string, 0)
	for rows.Next() {
		var (
			id        string
			createdAt time.Time
		)
		if err := rows.Scan(&id, &createdAt); err != nil {
			return nil, fmt.Errorf("保存した週間献立の読み取りに失敗しました: %w", err)
		}
		parsed, err := domain.ParseSavedWeeklyMenuID(id)
		if err != nil {
			return nil, fmt.Errorf("保存した週間献立の読み取りに失敗しました: %w", err)
		}
		at[id] = len(saved)
		ids = append(ids, id)
		saved = append(saved, domain.SavedWeeklyMenu{
			ID:        parsed,
			Days:      make([]domain.DayMenu, 0, domain.WeekLength),
			CreatedAt: createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("保存した週間献立の読み取りに失敗しました: %w", err)
	}
	if len(saved) == 0 {
		return saved, nil
	}

	if err := r.fillDays(ctx, ids, at, saved); err != nil {
		return nil, err
	}
	return saved, nil
}

// fillDays は取得済みの週に中身の7日分を詰める。
func (r *SavedWeeklyMenuRepository) fillDays(
	ctx context.Context, ids []string, at map[string]int, saved []domain.SavedWeeklyMenu,
) error {
	rows, err := r.pool.Query(ctx,
		`SELECT d.saved_weekly_menu_id, d.day,
		        `+joinedMenuColumns+`
		   FROM saved_weekly_menu_days d
		   JOIN menus m ON m.id = d.menu_id
		  WHERE d.saved_weekly_menu_id = ANY($1::uuid[])
		  ORDER BY d.day`, ids)
	if err != nil {
		return fmt.Errorf("保存した週間献立の中身の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		weekID, day, menu, err := scanSavedDay(rows)
		if err != nil {
			return err
		}
		i, ok := at[weekID]
		if !ok {
			// 直前のクエリで得たIDしか渡していないので、ここには来ない。
			continue
		}
		saved[i].Days = append(saved[i].Days, domain.DayMenu{Day: day, Menu: menu})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("保存した週間献立の中身の読み取りに失敗しました: %w", err)
	}
	return nil
}

// scanSavedDay は1行を「どの週の何日目のどの献立か」に読む。
func scanSavedDay(row pgx.Row) (string, int, domain.Menu, error) {
	var (
		weekID                                          string
		day                                             int16
		menuID, name, kana, genre, diff, role, descript string
	)
	if err := row.Scan(&weekID, &day,
		&menuID, &name, &kana, &genre, &diff, &role, &descript); err != nil {
		return "", 0, domain.Menu{},
			fmt.Errorf("保存した週間献立の中身の読み取りに失敗しました: %w", err)
	}
	menu, err := hydrateMenu(menuID, name, kana, genre, diff, role, descript)
	if err != nil {
		return "", 0, domain.Menu{}, err
	}
	return weekID, int(day), menu, nil
}

// Count はユーザーの保存件数を返す。上限判定（spec.md 2.8）に使う。
func (r *SavedWeeklyMenuRepository) Count(
	ctx context.Context, userID domain.UserID,
) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM saved_weekly_menus WHERE user_id = $1`,
		userID.String()).Scan(&n); err != nil {
		return 0, fmt.Errorf("保存した週間献立の件数取得に失敗しました: %w", err)
	}
	return n, nil
}

// Delete は保存を1件削除する。中身は ON DELETE CASCADE で一緒に消える。
//
// user_id でも絞って消すため、他人の行には構造上たどり着けない。
// 消せなかった場合に「無い」と「他人のもの」を区別しないのは、
// 区別すると他人の保存の存在を明かしてしまうため（spec.md 5.3）。
func (r *SavedWeeklyMenuRepository) Delete(
	ctx context.Context, userID domain.UserID, id domain.SavedWeeklyMenuID,
) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM saved_weekly_menus WHERE id = $1 AND user_id = $2`,
		id.String(), userID.String())
	if err != nil {
		return fmt.Errorf("保存した週間献立の削除に失敗しました: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return service.ErrSavedWeeklyMenuNotFound
	}
	return nil
}
