// Package repository はドメインオブジェクトの永続化を担う。
// SQLはこの層に閉じ込め、上位層はドメイン型のみを扱う。
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrMenuNotFound は指定された献立が存在しないことを表す。
var ErrMenuNotFound = errors.New("献立が見つかりません")

// MenuRepository は献立マスタへのアクセスを提供する。
type MenuRepository struct {
	pool *pgxpool.Pool
}

// NewMenuRepository は MenuRepository を生成する。
func NewMenuRepository(pool *pgxpool.Pool) *MenuRepository {
	return &MenuRepository{pool: pool}
}

const menuColumns = `id, name, name_kana, genre, difficulty, role, description`

// joinedMenuColumns は menus を JOIN して献立を一緒に読むときの列。
//
// **列を足したらここも直す。** 履歴・お気に入り・保存した週間献立は
// それぞれ独自のクエリで menus を JOIN しており、以前は同じ列リストを
// 3箇所にコピーしていた。role を足したとき menu.go だけ直して
// 3経路が取り残され、Menu.Role が空のまま返る状態になった。
// 定数にまとめて、次に列が増えたときに1箇所で済むようにしている。
// 読み取りは hydrateMenu が受け持つ（順序はこの並びと一致させること）。
const joinedMenuColumns = `m.id, m.name, m.name_kana, m.genre, m.difficulty, m.role, m.description`

// FindByID はIDで献立を1件取得する。存在しない場合は ErrMenuNotFound を返す。
func (r *MenuRepository) FindByID(ctx context.Context, id domain.MenuID) (*domain.Menu, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+menuColumns+` FROM menus WHERE id = $1`, id.String())

	m, err := scanMenu(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrMenuNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("献立の取得に失敗しました: %w", err)
	}
	return &m, nil
}

// FindByIDs は複数のIDで献立をまとめて取得する。
// 見つからないIDは黙って除くため、返る件数は ids の件数以下になる。
// 結果は name 順で安定させる。
func (r *MenuRepository) FindByIDs(ctx context.Context, ids []domain.MenuID) ([]domain.Menu, error) {
	if len(ids) == 0 {
		// 空配列を投げても0件が返るだけだが、無駄な往復を省く。
		return []domain.Menu{}, nil
	}

	raw := make([]string, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, id.String())
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+menuColumns+`
		   FROM menus
		  WHERE id = ANY($1::uuid[])
		  ORDER BY name`, raw)
	if err != nil {
		return nil, fmt.Errorf("献立の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	return scanMenus(rows)
}

// FindByFilter は条件に合う献立を返す。該当が無い場合は空スライスを返す（nilではない）。
// 結果は name 順で安定させる。順序が不定だと上位のランダム選択で再現性が取れないため。
func (r *MenuRepository) FindByFilter(ctx context.Context, f domain.MenuFilter) ([]domain.Menu, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}

	// pgx は $1 に nil を渡すと NULL になる。
	// "$1 IS NULL OR genre = $1" とすることで、条件の有無を1つのSQLで表現する。
	var genre, difficulty *string
	if f.Genre != nil {
		genre = ptrTo(f.Genre.String())
	}
	if f.Difficulty != nil {
		difficulty = ptrTo(f.Difficulty.String())
	}

	excludeIDs := make([]string, 0, len(f.ExcludeIDs))
	for _, id := range f.ExcludeIDs {
		excludeIDs = append(excludeIDs, id.String())
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+menuColumns+`
		   FROM menus
		  WHERE ($1::text IS NULL OR genre = $1)
		    AND ($2::text IS NULL OR difficulty = $2)
		    AND NOT (id = ANY($3::uuid[]))
		  ORDER BY name`,
		genre, difficulty, excludeIDs)
	if err != nil {
		return nil, fmt.Errorf("献立の検索に失敗しました: %w", err)
	}
	defer rows.Close()

	return scanMenus(rows)
}

// scanMenus は行を読み切って献立の並びを返す。該当が無い場合は
// 空スライスを返す（nilではない）。
func scanMenus(rows pgx.Rows) ([]domain.Menu, error) {
	menus := make([]domain.Menu, 0)
	for rows.Next() {
		m, err := scanMenu(rows)
		if err != nil {
			return nil, fmt.Errorf("献立の読み取りに失敗しました: %w", err)
		}
		menus = append(menus, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("献立の読み取りに失敗しました: %w", err)
	}
	return menus, nil
}

// scanner は pgx.Row と pgx.Rows の共通部分。
type scanner interface {
	Scan(dest ...any) error
}

func scanMenu(s scanner) (domain.Menu, error) {
	var (
		id, name, kana, genre, difficulty, role, description string
	)
	if err := s.Scan(&id, &name, &kana, &genre, &difficulty, &role, &description); err != nil {
		return domain.Menu{}, err
	}

	menuID, err := domain.ParseMenuID(id)
	if err != nil {
		return domain.Menu{}, fmt.Errorf("DBのIDが不正です: %w", err)
	}
	g, err := domain.ParseGenre(genre)
	if err != nil {
		return domain.Menu{}, fmt.Errorf("DBのジャンルが不正です: %w", err)
	}
	d, err := domain.ParseDifficulty(difficulty)
	if err != nil {
		return domain.Menu{}, fmt.Errorf("DBの難易度が不正です: %w", err)
	}
	r, err := domain.ParseRole(role)
	if err != nil {
		return domain.Menu{}, fmt.Errorf("DBの役割が不正です: %w", err)
	}

	return domain.Menu{
		ID:          menuID,
		Name:        name,
		NameKana:    kana,
		Genre:       g,
		Difficulty:  d,
		Role:        r,
		Description: description,
	}, nil
}

func ptrTo[T any](v T) *T { return &v }
