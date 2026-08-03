package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ResolutionRepository は入力語から食材への解決キャッシュへのアクセスを提供する。
//
// このテーブルは利用者に紐づかない（設計 5章）。誰が書いた語でも
// 対応づけの結果は同じなので、未認証も含めて全員で共有する。
type ResolutionRepository struct {
	pool *pgxpool.Pool
}

// NewResolutionRepository は ResolutionRepository を生成する。
func NewResolutionRepository(pool *pgxpool.Pool) *ResolutionRepository {
	return &ResolutionRepository{pool: pool}
}

// FindByWords は正規化済みの語に対する解決済みの対応づけを引く。
//
// 戻り値のキーは正規化済みの語。値が nil は「マスタに無いと確定済み」を表し、
// **キーが存在しないことは「まだ問い合わせていない」を表す。** この2つを
// 区別できないと、マスタに無い語を毎回 LLM に聞き直すことになる。
func (r *ResolutionRepository) FindByWords(
	ctx context.Context, words []string,
) (map[string]*domain.IngredientID, error) {
	out := make(map[string]*domain.IngredientID, len(words))
	if len(words) == 0 {
		// 空配列を投げても0件が返るだけだが、無駄な往復を省く。
		return out, nil
	}

	rows, err := r.pool.Query(ctx,
		`SELECT input_word, ingredient_id
		   FROM ingredient_resolutions
		  WHERE input_word = ANY($1::text[])`, words)
	if err != nil {
		return nil, fmt.Errorf("解決キャッシュの取得に失敗しました: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var word string
		var raw *string
		if err := rows.Scan(&word, &raw); err != nil {
			return nil, fmt.Errorf("解決キャッシュの読み取りに失敗しました: %w", err)
		}
		if raw == nil {
			out[word] = nil
			continue
		}
		id, err := domain.ParseIngredientID(*raw)
		if err != nil {
			// DBの外部キーが効いているのでここに来るのは異常。握りつぶさない。
			return nil, fmt.Errorf("解決キャッシュの食材IDが不正です(%q): %w", word, err)
		}
		out[word] = &id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("解決キャッシュの取得に失敗しました: %w", err)
	}
	return out, nil
}

// Save は1語ぶんの解決結果を保存する。id が nil なら「マスタに無い」として残す。
//
// 同じ語を再度保存したときは上書きする。食材マスタが更新されて
// 解決可能になった語を、後から正しい値に置き換えられるようにするため。
func (r *ResolutionRepository) Save(
	ctx context.Context, word string, id *domain.IngredientID,
) error {
	var raw *string
	if id != nil {
		s := id.String()
		raw = &s
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO ingredient_resolutions (input_word, ingredient_id)
		 VALUES ($1, $2)
		 ON CONFLICT (input_word)
		 DO UPDATE SET ingredient_id = EXCLUDED.ingredient_id, resolved_at = now()`,
		word, raw)
	if err != nil {
		return fmt.Errorf("解決キャッシュの保存に失敗しました(%q): %w", word, err)
	}
	return nil
}

// DeleteUnresolved は「マスタに無い」と保存された行だけを消す。
//
// 食材マスタに新しい食材を足すと、過去に未解決として保存した語が
// 解決可能になる（設計 5章）。TTL を持たない設計なので、
// マスタ更新のたびにこれを流して聞き直させる。
//
// **解決済みの行は消さない。** 食材の別名は変わらないため、
// 消すと LLM への問い合わせが無駄に増えるだけになる。
func (r *ResolutionRepository) DeleteUnresolved(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM ingredient_resolutions WHERE ingredient_id IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("未解決キャッシュの削除に失敗しました: %w", err)
	}
	return tag.RowsAffected(), nil
}
