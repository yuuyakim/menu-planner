package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
	"github.com/yuuyakim/menu-planner/backend/internal/service"
)

// RecipeLinkCache はレシピリンクのキャッシュを Postgres に保存する。
type RecipeLinkCache struct {
	pool *pgxpool.Pool
}

// NewRecipeLinkCache は RecipeLinkCache を生成する。
func NewRecipeLinkCache(pool *pgxpool.Pool) *RecipeLinkCache {
	return &RecipeLinkCache{pool: pool}
}

// recipeLinkRow は links カラムに入れる1件分の表現。
//
// domain.RecipeLink をそのまま保存しないのは、ドメインの構造体の変更が
// 保存済みデータの解釈を壊さないようにするため。永続化の形はこの層で決める。
type recipeLinkRow struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Find は献立IDに対応するキャッシュを返す。
// 該当が無い場合は service.ErrRecipeCacheMiss を返す。
func (c *RecipeLinkCache) Find(ctx context.Context, id domain.MenuID) (service.CachedRecipeLinks, error) {
	row := c.pool.QueryRow(ctx,
		`SELECT links, fetched_at FROM recipe_link_caches WHERE menu_id = $1`, id.String())

	var raw []byte
	var fetchedAt time.Time
	if err := row.Scan(&raw, &fetchedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.CachedRecipeLinks{}, service.ErrRecipeCacheMiss
		}
		return service.CachedRecipeLinks{}, fmt.Errorf("レシピのキャッシュの取得に失敗しました: %w", err)
	}

	var rows []recipeLinkRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return service.CachedRecipeLinks{}, fmt.Errorf("レシピのキャッシュを解釈できませんでした: %w", err)
	}

	links := make([]domain.RecipeLink, 0, len(rows))
	for _, r := range rows {
		// 保存時と同じ検証を通す。DBの中身が壊れていても不正なリンク
		// （javascript: など）がそのまま画面に出ることを防ぐ。
		// ドメインを再構築するので domain も URL から導出し直される。
		link, err := domain.NewRecipeLink(r.Title, r.URL, r.Snippet)
		if err != nil {
			return service.CachedRecipeLinks{}, fmt.Errorf("レシピのキャッシュが壊れています(menu_id=%s): %w", id, err)
		}
		links = append(links, link)
	}

	return service.CachedRecipeLinks{Links: links, FetchedAt: fetchedAt}, nil
}

// Save はキャッシュを保存する。既存があれば上書きする。
func (c *RecipeLinkCache) Save(ctx context.Context, id domain.MenuID, links []domain.RecipeLink, fetchedAt time.Time) error {
	rows := make([]recipeLinkRow, 0, len(links))
	for _, l := range links {
		rows = append(rows, recipeLinkRow{Title: l.Title, URL: l.URL, Snippet: l.Snippet})
	}

	raw, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("レシピのキャッシュを組み立てられませんでした: %w", err)
	}

	// 取り直した結果で上書きする。同じ献立に複数の行を作らない。
	_, err = c.pool.Exec(ctx,
		`INSERT INTO recipe_link_caches (menu_id, links, fetched_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (menu_id) DO UPDATE
		 SET links = EXCLUDED.links, fetched_at = EXCLUDED.fetched_at`,
		id.String(), raw, fetchedAt)
	if err != nil {
		return fmt.Errorf("レシピのキャッシュの保存に失敗しました: %w", err)
	}
	return nil
}
