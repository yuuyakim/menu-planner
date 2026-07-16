package service

import (
	"context"
	"fmt"

	"github.com/yuuyakim/menu-planner/backend/internal/domain"
)

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
	if err != nil {
		return nil, err
	}

	return &menu, nil
}
