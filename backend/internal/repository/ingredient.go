package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// IngredientRepository は献立に紐づく食材へのアクセスを提供する。
type IngredientRepository struct {
	pool *pgxpool.Pool
}

// NewIngredientRepository は IngredientRepository を生成する。
func NewIngredientRepository(pool *pgxpool.Pool) *IngredientRepository {
	return &IngredientRepository{pool: pool}
}

// FindByMenuIDs は指定した献立に使う食材を、献立との対応つきで返す。
//
// 献立ごとに1回ずつ引くと問い合わせが献立の数だけ飛ぶため、`= ANY` でまとめて引く。
// 存在しない献立IDは結合で落ちるだけなのでエラーにしない（件数の判断は service の仕事）。
//
// 並びは食材のカナ順。**カテゴリ順への並べ替えはここではしない。**
// カテゴリの表示順は domain の知識（`AllIngredientCategories`）であり、
// SQLに CASE で書き写すと定義が二重になって食い違うため。
func (r *IngredientRepository) FindByMenuIDs(ctx context.Context, ids []domain.MenuID) ([]service.MenuIngredient, error) {
	if len(ids) == 0 {
		// 空配列を投げても0件が返るだけだが、無駄な往復を省く。
		return []service.MenuIngredient{}, nil
	}

	// 同じIDが重複していても `= ANY` の結合は1行にしかならないため、
	// 呼び出し側で重複を除く必要はない。
	raw := make([]string, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, id.String())
	}

	rows, err := r.pool.Query(ctx,
		`SELECT mi.menu_id, i.id, i.name, i.name_kana, i.category
		   FROM menu_ingredients mi
		   JOIN ingredients i ON i.id = mi.ingredient_id
		  WHERE mi.menu_id = ANY($1::uuid[])
		  ORDER BY i.name_kana, i.name`, raw)
	if err != nil {
		return nil, fmt.Errorf("食材の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	return scanMenuIngredients(rows)
}

// FindAll は食材マスタを全件返す。手持ちの食材を選ぶ画面の選択肢に使う（spec.md 2.9）。
//
// 並びはカナ順。**FindByMenuIDs と同じくカテゴリ順への並べ替えはしない。**
// カテゴリの表示順は domain の知識（`AllIngredientCategories`）で、
// SQLに CASE で書き写すと定義が二重になって食い違う。
//
// 166件で固定的なため、件数の上限も検索条件も設けない。
func (r *IngredientRepository) FindAll(ctx context.Context) ([]domain.Ingredient, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, name_kana, category
		   FROM ingredients
		  ORDER BY name_kana, name`)
	if err != nil {
		return nil, fmt.Errorf("食材マスタの取得に失敗しました: %w", err)
	}
	defer rows.Close()

	return scanIngredients(rows)
}

// scanMenuIngredients は行を service.MenuIngredient に写す。
func scanMenuIngredients(rows pgx.Rows) ([]service.MenuIngredient, error) {
	items := []service.MenuIngredient{}
	for rows.Next() {
		var menuID, ingredientID, name, kana, category string
		if err := rows.Scan(&menuID, &ingredientID, &name, &kana, &category); err != nil {
			return nil, fmt.Errorf("食材の読み取りに失敗しました: %w", err)
		}

		mID, err := domain.ParseMenuID(menuID)
		if err != nil {
			return nil, fmt.Errorf("献立IDの解釈に失敗しました: %w", err)
		}
		iID, err := domain.ParseIngredientID(ingredientID)
		if err != nil {
			return nil, fmt.Errorf("食材IDの解釈に失敗しました: %w", err)
		}
		// DBのCHECK制約と同じ集合だが、ここでも解釈して不正値を弾く。
		// 制約を後から緩めたときに、上位へ黙って不正な値が流れるのを防ぐ。
		c, err := domain.ParseIngredientCategory(category)
		if err != nil {
			return nil, fmt.Errorf("食材カテゴリの解釈に失敗しました: %w", err)
		}

		items = append(items, service.MenuIngredient{
			MenuID: mID,
			Ingredient: domain.Ingredient{
				ID:       iID,
				Name:     name,
				NameKana: kana,
				Category: c,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("食材の読み取りに失敗しました: %w", err)
	}
	return items, nil
}

// FindByIDs は指定したIDの食材を返す。存在しないIDは黙って落ちる
// （件数の判断は service の仕事。買い物リストの献立と同じ扱い）。
func (r *IngredientRepository) FindByIDs(ctx context.Context, ids []domain.IngredientID) ([]domain.Ingredient, error) {
	if len(ids) == 0 {
		return []domain.Ingredient{}, nil
	}

	raw := make([]string, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, id.String())
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, name, name_kana, category
		   FROM ingredients
		  WHERE id = ANY($1::uuid[])
		  ORDER BY name_kana, name`, raw)
	if err != nil {
		return nil, fmt.Errorf("食材の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	return scanIngredients(rows)
}

// FindMenuIDsByIngredientIDs は指定した食材のいずれかを使う献立のIDを返す。
//
// **1つも重ならない献立は返らない**（spec.md 5.6）。手持ちで作れるものを探す機能なので、
// 重なりゼロの献立を候補に含める意味がない。この絞り込みをSQLでやることで、
// 献立380件すべてを読んで捨てる無駄をなくす。
//
// 不足食材の算出に必要な「その献立の全食材」はここでは返さない。
// FindByMenuIDs が既にその役目を持っているため、呼び出し側で組み合わせる。
func (r *IngredientRepository) FindMenuIDsByIngredientIDs(
	ctx context.Context, ids []domain.IngredientID,
) ([]domain.MenuID, error) {
	if len(ids) == 0 {
		return []domain.MenuID{}, nil
	}

	raw := make([]string, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, id.String())
	}

	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT menu_id
		   FROM menu_ingredients
		  WHERE ingredient_id = ANY($1::uuid[])`, raw)
	if err != nil {
		return nil, fmt.Errorf("食材から献立の絞り込みに失敗しました: %w", err)
	}
	defer rows.Close()

	out := []domain.MenuID{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("献立IDの読み取りに失敗しました: %w", err)
		}
		mID, err := domain.ParseMenuID(id)
		if err != nil {
			return nil, fmt.Errorf("献立IDの解釈に失敗しました: %w", err)
		}
		out = append(out, mID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("献立IDの読み取りに失敗しました: %w", err)
	}
	return out, nil
}

// scanIngredients は行を domain.Ingredient に写す。
func scanIngredients(rows pgx.Rows) ([]domain.Ingredient, error) {
	items := []domain.Ingredient{}
	for rows.Next() {
		var id, name, kana, category string
		if err := rows.Scan(&id, &name, &kana, &category); err != nil {
			return nil, fmt.Errorf("食材の読み取りに失敗しました: %w", err)
		}
		iID, err := domain.ParseIngredientID(id)
		if err != nil {
			return nil, fmt.Errorf("食材IDの解釈に失敗しました: %w", err)
		}
		c, err := domain.ParseIngredientCategory(category)
		if err != nil {
			return nil, fmt.Errorf("食材カテゴリの解釈に失敗しました: %w", err)
		}
		items = append(items, domain.Ingredient{
			ID: iID, Name: name, NameKana: kana, Category: c,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("食材の読み取りに失敗しました: %w", err)
	}
	return items, nil
}
