package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

// ErrNoMenuFound は条件に合う献立が1件も無いことを表す。
// リクエスト自体は正しいので、呼び出し側はこれを 4xx として扱う。
var ErrNoMenuFound = errors.New("条件に合う献立が見つかりません")

// MenuService は献立の選定を担う。
type MenuService struct {
	repo MenuRepository
	rand Randomizer
}

// NewMenuService は献立サービスを組み立てる。
func NewMenuService(repo MenuRepository, rand Randomizer) *MenuService {
	return &MenuService{repo: repo, rand: rand}
}

// SuggestMenu は条件に合う献立から1件を無作為に選んで返す。
// 条件が不正な場合は domain.ErrInvalidGenre / domain.ErrInvalidDifficulty を返す。
func (s *MenuService) SuggestMenu(ctx context.Context, f domain.MenuFilter) (*domain.Menu, error) {
	// 不正な条件をDBに投げても0件が返るだけで理由が分からないため、先に弾く。
	if err := f.Validate(); err != nil {
		return nil, err
	}

	candidates, err := s.repo.FindByFilter(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("献立の検索に失敗しました: %w", err)
	}

	menu, err := Pick(s.rand, candidates)
	switch {
	// 候補が無いのは障害ではなく「条件に合う献立が無い」という結果。
	// Pick の内部事情(ErrNoCandidates)は外に出さず、ドメインのエラーに変換する。
	case errors.Is(err, ErrNoCandidates):
		return nil, ErrNoMenuFound
	case err != nil:
		return nil, fmt.Errorf("献立の選択に失敗しました: %w", err)
	}

	return &menu, nil
}

// GetMenu はIDで献立を1件返す。
// 献立が存在しない場合とDB障害は repository が別のエラーで表現しており、
// 呼び出し側が 404 と 500 を出し分けられるよう、原因を包んだまま返す。
func (s *MenuService) GetMenu(ctx context.Context, id domain.MenuID) (*domain.Menu, error) {
	menu, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("献立の取得に失敗しました(id=%s): %w", id, err)
	}
	return menu, nil
}
